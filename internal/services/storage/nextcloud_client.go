package storage

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// NextcloudClient is a lightweight WebDAV client for pushing files to Nextcloud.
type NextcloudClient struct {
	baseURL       *url.URL
	username      string
	password      string
	basePath      string
	httpClient    *http.Client
	collectionsMu sync.Mutex
	collections   map[string]time.Time
}

// RemoteEntry represents a file or directory in Nextcloud.
type RemoteEntry struct {
	Path        string
	IsDir       bool
	ContentType string
	Size        int64
	ModTime     time.Time
	ETag        string
}

func NewNextcloudClient(rawURL, username, password, basePath string) (*NextcloudClient, error) {
	parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse nextcloud url: %w", err)
	}
	basePath = strings.Trim(basePath, "\"")
	if basePath == "" {
		basePath = "rentalcore-filepool"
	}
	return &NextcloudClient{
		baseURL:     parsed,
		username:    username,
		password:    password,
		basePath:    strings.Trim(basePath, "/"),
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		collections: make(map[string]time.Time),
	}, nil
}

// normalizeRel trims the synthetic nextcloud: prefix and leading slashes.
func normalizeRel(rel string) string {
	rel = strings.TrimPrefix(rel, "nextcloud:")
	return strings.TrimLeft(rel, "/")
}

// joinPath builds a full WebDAV URL for a given relative path.
func (c *NextcloudClient) joinPath(rel string) string {
	clean := normalizeRel(rel)
	full := path.Join(c.basePath, clean)
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, full)
	return u.String()
}

// EnsureCollections makes sure each segment exists via MKCOL.
func (c *NextcloudClient) EnsureCollections(rel string) error {
	c.collectionsMu.Lock()
	defer c.collectionsMu.Unlock()
	rel = normalizeRel(rel)
	segments := strings.Split(rel, "/")
	current := ""
	for _, seg := range segments[:len(segments)-1] {
		if seg == "" {
			continue
		}
		current = path.Join(current, seg)
		if time.Since(c.collections[current]) < 5*time.Minute {
			continue
		}
		req, err := http.NewRequest("MKCOL", c.joinPath(current), nil)
		if err != nil {
			return fmt.Errorf("mkcol request: %w", err)
		}
		req.SetBasicAuth(c.username, c.password)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("mkcol: %w", err)
		}
		// 201 Created or 405 Method Not Allowed if already exists
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusMethodNotAllowed {
			resp.Body.Close()
			return fmt.Errorf("mkcol %s failed with status %s", current, resp.Status)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		c.collections[current] = time.Now()
	}
	return nil
}

// Upload streams a file to Nextcloud at the given relative path.
func (c *NextcloudClient) Upload(rel string, body io.Reader, contentType string) error {
	rel = normalizeRel(rel)
	if err := c.EnsureCollections(rel); err != nil {
		return err
	}
	// Retry once if a previously cached collection was removed externally.
	seeker, rewindable := body.(io.ReadSeeker)
	start := int64(0)
	if rewindable {
		var err error
		start, err = seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodPut, c.joinPath(rel), body)
		if err != nil {
			return fmt.Errorf("put request: %w", err)
		}
		req.SetBasicAuth(c.username, c.password)
		req.Header.Set("Content-Type", contentType)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("upload: %w", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 300 {
			return nil
		}
		if attempt == 0 && rewindable && (resp.StatusCode == 404 || resp.StatusCode == 409) {
			if _, err := seeker.Seek(start, io.SeekStart); err != nil {
				return err
			}
			c.collectionsMu.Lock()
			clear(c.collections)
			c.collectionsMu.Unlock()
			if err := c.EnsureCollections(rel); err != nil {
				return err
			}
			continue
		}
		return fmt.Errorf("upload failed: %s", resp.Status)
	}
	return fmt.Errorf("upload failed")
}

// Download fetches a file from Nextcloud.
func (c *NextcloudClient) Download(rel string) (io.ReadCloser, string, error) {
	rel = normalizeRel(rel)
	req, err := http.NewRequest(http.MethodGet, c.joinPath(rel), nil)
	if err != nil {
		return nil, "", fmt.Errorf("get request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download: %w", err)
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("download failed: %s", resp.Status)
	}
	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// Delete removes a file from Nextcloud.
func (c *NextcloudClient) Delete(rel string) error {
	rel = normalizeRel(rel)
	req, err := http.NewRequest(http.MethodDelete, c.joinPath(rel), nil)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete failed: %s", resp.Status)
	}
	return nil
}

// Move moves/renames a file in Nextcloud using WebDAV MOVE method.
func (c *NextcloudClient) Move(srcRel, dstRel string) error {
	srcRel = normalizeRel(srcRel)
	dstRel = normalizeRel(dstRel)

	// Ensure destination directories exist
	if err := c.EnsureCollections(dstRel); err != nil {
		return fmt.Errorf("ensure dest collections: %w", err)
	}

	req, err := http.NewRequest("MOVE", c.joinPath(srcRel), nil)
	if err != nil {
		return fmt.Errorf("move request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Destination", c.joinPath(dstRel))
	req.Header.Set("Overwrite", "T")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}
	defer resp.Body.Close()

	// 201 Created or 204 No Content are success codes for MOVE
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("move failed: %s", resp.Status)
	}
	return nil
}

// List returns all files under the given relative folder (recursive).
func (c *NextcloudClient) List(rel string) ([]RemoteEntry, error) {
	results := []RemoteEntry{}
	if err := c.walk(rel, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *NextcloudClient) walk(rel string, results *[]RemoteEntry) error {
	rel = strings.TrimLeft(rel, "/")
	current := rel
	reqBody := `<?xml version="1.0"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:displayname/>
    <d:resourcetype/>
    <d:getcontenttype/>
    <d:getcontentlength/>
    <d:getlastmodified/>
    <d:getetag/>
  </d:prop>
</d:propfind>`

	req, err := http.NewRequest("PROPFIND", c.joinPath(rel), bytes.NewBufferString(reqBody))
	if err != nil {
		return fmt.Errorf("propfind request: %w", err)
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("propfind: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("propfind failed: %s", resp.Status)
	}

	var multistatus multiStatus
	if err := xml.NewDecoder(resp.Body).Decode(&multistatus); err != nil {
		return fmt.Errorf("decode propfind: %w", err)
	}

	for _, r := range multistatus.Responses {
		prop := r.FirstProp()
		relPath := c.normalizeHref(r.Href)
		if relPath == "" {
			continue // self
		}

		isDir := prop.ResourceType.Collection
		var modTime time.Time
		if prop.LastModified != "" {
			modTime, _ = time.Parse(time.RFC1123Z, prop.LastModified)
			if modTime.IsZero() {
				modTime, _ = time.Parse(time.RFC1123, prop.LastModified)
			}
		}

		size := int64(0)
		if prop.ContentLength != "" {
			fmt.Sscan(prop.ContentLength, &size)
		}

		entry := RemoteEntry{
			Path:        relPath,
			IsDir:       isDir,
			ContentType: prop.ContentType,
			Size:        size,
			ModTime:     modTime,
			ETag:        prop.ETag,
		}

		if isDir {
			if relPath == current {
				continue // avoid infinite recursion on self entries
			}
			if err := c.walk(relPath, results); err != nil {
				return err
			}
			continue
		}

		*results = append(*results, entry)
	}

	return nil
}

// normalizeHref makes a relative path (trim base + basePath and trailing slash).
func (c *NextcloudClient) normalizeHref(href string) string {
	decoded, err := url.PathUnescape(href)
	if err != nil {
		return ""
	}
	clean := path.Clean(decoded)
	prefix := path.Join("/", strings.Trim(c.baseURL.Path, "/"), c.basePath)
	rel := strings.TrimPrefix(clean, prefix)
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimSuffix(rel, "/")
	return rel
}

// WebDAV multistatus parsing helpers
type multiStatus struct {
	Responses []response `xml:"response"`
}

type response struct {
	Href     string     `xml:"href"`
	PropStat []propStat `xml:"propstat"`
}

func (r response) FirstProp() prop {
	for _, ps := range r.PropStat {
		if ps.Prop.DisplayName != "" || ps.Prop.ContentType != "" || ps.Prop.ContentLength != "" || ps.Prop.LastModified != "" {
			return ps.Prop
		}
	}
	if len(r.PropStat) > 0 {
		return r.PropStat[0].Prop
	}
	return prop{}
}

type propStat struct {
	Prop prop `xml:"prop"`
}

type prop struct {
	DisplayName   string  `xml:"displayname"`
	ResourceType  resType `xml:"resourcetype"`
	ContentType   string  `xml:"getcontenttype"`
	ContentLength string  `xml:"getcontentlength"`
	LastModified  string  `xml:"getlastmodified"`
	ETag          string  `xml:"getetag"`
}

type resType struct {
	Collection bool `xml:"collection"`
}

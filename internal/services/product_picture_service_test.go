package services

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"warehousecore/internal/services/storage"
)

type pictureDAV struct {
	mu       sync.Mutex
	files    map[string][]byte
	requests map[string]int
	fail     string
	latency  time.Duration
}

func (d *pictureDAV) serve(w http.ResponseWriter, r *http.Request) {
	time.Sleep(d.latency)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requests[r.Method]++
	switch r.Method {
	case "MKCOL":
		w.WriteHeader(201)
	case "PUT":
		if strings.Contains(r.URL.Path, d.fail) && d.fail != "" {
			w.WriteHeader(503)
			return
		}
		data, _ := io.ReadAll(r.Body)
		d.files[r.URL.Path] = data
		w.WriteHeader(201)
	case "GET":
		data, ok := d.files[r.URL.Path]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(data)
	case "DELETE":
		delete(d.files, r.URL.Path)
		w.WriteHeader(204)
	case "PROPFIND":
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:">`)
		for path, data := range d.files {
			if strings.HasPrefix(path, r.URL.Path) {
				u := url.URL{Path: path}
				fmt.Fprintf(w, `<d:response><d:href>%s</d:href><d:propstat><d:prop><d:displayname>picture</d:displayname><d:getcontenttype>image/jpeg</d:getcontenttype><d:getcontentlength>%d</d:getcontentlength></d:prop></d:propstat></d:response>`, u.EscapedPath(), len(data))
			}
		}
		fmt.Fprint(w, `</d:multistatus>`)
	default:
		w.WriteHeader(405)
	}
}
func pictureTestService(t *testing.T) (*ProductPictureService, *pictureDAV) {
	t.Helper()
	dav := &pictureDAV{files: map[string][]byte{}, requests: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(dav.serve))
	t.Cleanup(server.Close)
	t.Setenv("NEXTCLOUD_WEBDAV_URL", server.URL+"/dav")
	t.Setenv("NEXTCLOUD_WEBDAV_USER", "test")
	t.Setenv("NEXTCLOUD_WEBDAV_PASSWORD", "test")
	t.Setenv("NEXTCLOUD_WEBDAV_BASE_PATH", "root")
	t.Setenv("PICTURE_CACHE_DIR", t.TempDir())
	return NewProductPictureServiceFromEnv(), dav
}
func pictureHeaders(t *testing.T, names ...string) []*multipart.FileHeader {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	img.Set(10, 10, color.NRGBA{R: 255, A: 255})
	var data bytes.Buffer
	jpeg.Encode(&data, img, nil)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, name := range names {
		file, _ := writer.CreateFormFile("files", name)
		file.Write(data.Bytes())
	}
	writer.Close()
	request := httptest.NewRequest("POST", "/pictures", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { request.MultipartForm.RemoveAll() })
	// Browsers supply image types; CreateFormFile uses application/octet-stream.
	for _, header := range request.MultipartForm.File["files"] {
		header.Header.Set("Content-Type", "image/jpeg")
	}
	return request.MultipartForm.File["files"]
}
func TestPictureUploadReusesOriginalAndBoundsRemoteCalls(t *testing.T) {
	service, dav := pictureTestService(t)
	dav.latency = 20 * time.Millisecond
	start := time.Now()
	pictures, err := service.UploadPictures("Demo Unit", pictureHeaders(t, "photo.jpg", "photo.jpg", "photo.jpg", "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	uploadTime := time.Since(start)
	names := map[string]bool{}
	for _, picture := range pictures {
		if names[picture.FileName] {
			t.Fatal("parallel uploads collided")
		}
		names[picture.FileName] = true
		for _, variant := range []string{"thumb", "preview"} {
			f, ct, err := service.DownloadPictureWithVariant("Demo Unit", picture.FileName, variant, "webp")
			if err != nil {
				t.Fatal(err)
			}
			if ct != "image/webp" {
				t.Fatal(ct)
			}
			f.Close()
		}
	}
	if _, err := service.ListPictures("Demo Unit"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListPictures("Demo Unit"); err != nil {
		t.Fatal(err)
	}
	dav.mu.Lock()
	counts := map[string]int{}
	for k, v := range dav.requests {
		counts[k] = v
	}
	dav.mu.Unlock()
	if counts["MKCOL"] != 3 || counts["PUT"] != 4 || counts["GET"] != 0 || counts["PROPFIND"] != 1 {
		t.Fatalf("redundant remote requests: %v", counts)
	}
	t.Logf("4 uploads with 20ms remote latency: %s; requests=%v (previous path: 12 MKCOL, 4 PUT, 16 variant GET plus listing)", uploadTime, counts)
	for _, picture := range pictures {
		if err := service.DeletePicture("Demo Unit", picture.FileName); err != nil {
			t.Fatal(err)
		}
		if f, _, err := service.DownloadPictureWithVariant("Demo Unit", picture.FileName, "thumbnail", "webp"); err == nil {
			f.Close()
			t.Fatal("deleted picture remained cached")
		}
	}
}
func TestPictureListingReturnsCopiesAndUploadInvalidatesEmpty(t *testing.T) {
	service, _ := pictureTestService(t)
	before, err := service.ListPictures("Demo Unit")
	if err != nil || len(before) != 0 {
		t.Fatal(before, err)
	}
	pictures, err := service.UploadPictures("Demo Unit", pictureHeaders(t, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := service.ListPictures("Demo Unit")
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	list[0].FileName = "mutated"
	again, err := service.ListPictures("Demo Unit")
	if err != nil || again[0].FileName != pictures[0].FileName {
		t.Fatal("caller mutated cache", again, err)
	}
}
func TestFailedPictureBatchRemovesItsUploads(t *testing.T) {
	service, dav := pictureTestService(t)
	dav.fail = "bad_"
	_, err := service.UploadPictures("Demo", pictureHeaders(t, "good.jpg", "bad.jpg"))
	if err == nil {
		t.Fatal("failed remote upload accepted")
	}
	dav.mu.Lock()
	defer dav.mu.Unlock()
	if len(dav.files) != 0 {
		keys := []string{}
		for k := range dav.files {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatal("partial batch retained files", keys)
	}
}

func TestPictureDeleteCannotResurrectDelayedOriginal(t *testing.T) {
	service, dav := pictureTestService(t)
	headers := pictureHeaders(t, "race.jpg")
	file, _ := headers[0].Open()
	name, err := service.UploadPicture("Race", file, headers[0])
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	service.ClearCachedVariants("Race", name)
	started := make(chan struct{})
	release := make(chan struct{})
	delayed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			dav.mu.Lock()
			data := append([]byte(nil), dav.files[r.URL.Path]...)
			dav.mu.Unlock()
			if len(data) > 0 {
				close(started)
				<-release
				w.Write(data)
				return
			}
		}
		dav.serve(w, r)
	}))
	defer delayed.Close()
	service.client, _ = storage.NewNextcloudClient(delayed.URL+"/dav", "test", "test", "root")
	downloaded := make(chan error, 1)
	go func() {
		body, _, err := service.DownloadPicture("Race", name)
		if body != nil {
			body.Close()
		}
		downloaded <- err
	}()
	<-started
	deleted := make(chan error, 1)
	go func() { deleted <- service.DeletePicture("Race", name) }()
	select {
	case err := <-deleted:
		t.Fatalf("delete passed an in-flight download: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err = <-downloaded; err != nil {
		t.Fatal(err)
	}
	if err = <-deleted; err != nil {
		t.Fatal(err)
	}
	body, _, err := service.DownloadPicture("Race", name)
	if body != nil {
		body.Close()
	}
	if err == nil {
		t.Fatal("deleted image survived in original cache")
	}
}
func TestPictureContentCannotBeSpoofedByExtensionOrHeader(t *testing.T) {
	service, dav := pictureTestService(t)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, _ := writer.CreateFormFile("files", "fake.jpg")
	file.Write([]byte(`<html><script>alert(1)</script></html>`))
	writer.Close()
	request := httptest.NewRequest("POST", "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1024); err != nil {
		t.Fatal(err)
	}
	defer request.MultipartForm.RemoveAll()
	header := request.MultipartForm.File["files"][0]
	header.Header.Set("Content-Type", "image/jpeg")
	if _, err := service.UploadPictures("Spoof", []*multipart.FileHeader{header}); err == nil {
		t.Fatal("accepted active HTML as image")
	}
	dav.mu.Lock()
	defer dav.mu.Unlock()
	if dav.requests["PUT"] != 0 {
		t.Fatal("unsafe bytes reached remote storage")
	}
}

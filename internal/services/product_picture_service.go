package services

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"warehousecore/internal/services/storage"
)

// ProductPictureService handles uploads and retrieval of product pictures in Nextcloud.
type ProductPictureService struct {
	client       *storage.NextcloudClient
	enabled      bool
	maxFileSize  int64
	allowedTypes map[string]bool
	rootFolder   string
	cacheDir     string
	listMu       sync.Mutex
	listings     map[string]pictureListing
	listingLocks sync.Map
	listVersions map[string]uint64
	uploading    chan struct{}
	variantLocks sync.Map
	processing   chan struct{}
	warming      chan struct{}
}

type ProductPictureInfo struct {
	FileName    string    `json:"file_name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	ModifiedAt  time.Time `json:"modified_at"`
}

// NewProductPictureServiceFromEnv builds the service if Nextcloud credentials are present.
func NewProductPictureServiceFromEnv() *ProductPictureService {
	ncURL := trimAndUnquote(os.Getenv("NEXTCLOUD_WEBDAV_URL"))
	ncUser := trimAndUnquote(os.Getenv("NEXTCLOUD_WEBDAV_USER"))
	ncPass := trimAndUnquote(os.Getenv("NEXTCLOUD_WEBDAV_PASSWORD"))
	ncBase := trimAndUnquote(os.Getenv("NEXTCLOUD_WEBDAV_BASE_PATH"))
	cacheDir := trimAndUnquote(os.Getenv("PICTURE_CACHE_DIR"))
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "warehousecore", "pictures_cache")
	}

	service := &ProductPictureService{
		enabled:     false,
		maxFileSize: 10 * 1024 * 1024, // 10 MB
		allowedTypes: map[string]bool{
			"image/jpeg": true,
			"image/jpg":  true,
			"image/png":  true,
			"image/gif":  true,
			"image/webp": true,
			"image/heic": true,
		},
		rootFolder:   path.Join("warehousecore", "pictures"),
		cacheDir:     cacheDir,
		listings:     make(map[string]pictureListing),
		listVersions: make(map[string]uint64),
		uploading:    make(chan struct{}, 3),
		processing:   make(chan struct{}, 2),
		warming:      make(chan struct{}, 8),
	}

	if ncURL == "" || ncUser == "" || ncPass == "" {
		log.Println("[PICTURES] Nextcloud credentials missing - product pictures disabled")
		return service
	}

	client, err := storage.NewNextcloudClient(ncURL, ncUser, ncPass, ncBase)
	if err != nil {
		log.Printf("[PICTURES] Failed to configure Nextcloud client: %v\n", err)
		return service
	}

	service.client = client
	service.enabled = true
	log.Printf("[PICTURES] Nextcloud product pictures enabled at base '%s'\n", service.rootFolder)
	return service
}

// Enabled reports whether uploads are available.
func (s *ProductPictureService) Enabled() bool {
	return s != nil && s.enabled && s.client != nil
}

// MaxFileSize returns the allowed upload size in bytes.
func (s *ProductPictureService) MaxFileSize() int64 {
	if s == nil {
		return 0
	}
	return s.maxFileSize
}

// FolderForProduct returns the sanitized folder path (relative to the Nextcloud base).
func (s *ProductPictureService) FolderForProduct(productName string) string {
	return s.productFolder(productName)
}

// UploadPicture saves a new image for the given product name and returns the stored filename.
func (s *ProductPictureService) UploadPicture(productName string, file multipart.File, header *multipart.FileHeader) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("product pictures not configured")
	}

	s.uploading <- struct{}{}
	defer func() { <-s.uploading }()
	defer file.Seek(0, io.SeekStart) // ensure caller can reuse/close safely

	buf := bytes.Buffer{}
	limitedReader := io.LimitReader(file, s.maxFileSize+1)
	if _, err := io.Copy(&buf, limitedReader); err != nil {
		return "", fmt.Errorf("read upload: %w", err)
	}
	if int64(buf.Len()) > s.maxFileSize {
		return "", fmt.Errorf("file exceeds limit of %d bytes", s.maxFileSize)
	}

	contentType := pictureContentType(buf.Bytes())
	if !s.allowedTypes[contentType] {
		return "", fmt.Errorf("unsupported image content")
	}
	header.Header.Set("Content-Type", contentType)

	filename := sanitizeFileName(header.Filename)
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	ext := strings.ToLower(filepath.Ext(filename))
	if base == "" {
		base = "bild"
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	storedName := fmt.Sprintf("%s_%s%s", base, hex.EncodeToString(suffix[:]), ext)

	relPath := s.productFilePath(productName, storedName)
	if err := s.client.Upload(relPath, bytes.NewReader(buf.Bytes()), contentType); err != nil {
		return "", fmt.Errorf("upload to nextcloud: %w", err)
	}

	_ = writePictureCache(s.cachedPath(productName, storedName, "original", ""), buf.Bytes())
	_ = writePictureCache(s.cachedPath(productName, storedName, "original", "")+".mime", []byte(contentType))
	s.rememberPicture(productName, ProductPictureInfo{FileName: storedName, Size: int64(buf.Len()), ContentType: contentType, ModifiedAt: time.Now()})
	return storedName, nil
}

// ListPictures returns all files for a product.
func (s *ProductPictureService) ListPictures(productName string) ([]ProductPictureInfo, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("product pictures not configured")
	}
	value, _ := s.listingLocks.LoadOrStore(productName, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	for attempt := 0; attempt < 3; attempt++ {
		s.listMu.Lock()
		if listing, ok := s.listings[productName]; ok && time.Now().Before(listing.expires) {
			copy := append([]ProductPictureInfo(nil), listing.items...)
			s.listMu.Unlock()
			return copy, nil
		}
		version := s.listVersions[productName]
		s.listMu.Unlock()
		entries, err := s.client.List(s.productFolder(productName))
		if err != nil {
			if strings.Contains(err.Error(), "404") {
				return []ProductPictureInfo{}, nil
			}
			return nil, fmt.Errorf("list nextcloud folder: %w", err)
		}
		pictures := make([]ProductPictureInfo, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir && isImageFile(entry) {
				pictures = append(pictures, ProductPictureInfo{FileName: path.Base(entry.Path), Size: entry.Size, ContentType: entry.ContentType, ModifiedAt: entry.ModTime})
			}
		}
		s.listMu.Lock()
		current := s.listVersions[productName] == version
		if current {
			s.listings[productName] = pictureListing{items: pictures, expires: time.Now().Add(30 * time.Second)}
		}
		s.listMu.Unlock()
		if current || attempt == 2 {
			return append([]ProductPictureInfo(nil), pictures...), nil
		}
	}
	return nil, fmt.Errorf("picture list changed during refresh")
}

// DownloadPicture streams a picture for a product by filename.
func (s *ProductPictureService) DownloadPicture(productName, fileName string) (io.ReadCloser, string, error) {
	value, _ := s.variantLocks.LoadOrStore(productName+"/"+fileName, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return s.downloadPictureLocked(productName, fileName)
}
func (s *ProductPictureService) downloadPictureLocked(productName, fileName string) (io.ReadCloser, string, error) {
	if !s.Enabled() {
		return nil, "", fmt.Errorf("product pictures not configured")
	}
	if fileName == "" {
		return nil, "", fmt.Errorf("filename required")
	}
	safeName := sanitizeFileName(fileName)
	if body, ct, err := s.originalFromCache(productName, safeName); err == nil {
		return body, ct, nil
	}
	body, _, err := s.client.Download(s.productFilePath(productName, safeName))
	if err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(io.LimitReader(body, s.maxFileSize+1))
	body.Close()
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > s.maxFileSize {
		return nil, "", fmt.Errorf("picture exceeds download limit")
	}
	ct := pictureContentType(data)
	if !s.allowedTypes[ct] {
		return nil, "", fmt.Errorf("unsupported image content")
	}
	_ = writePictureCache(s.cachedPath(productName, safeName, "original", ""), data)
	_ = writePictureCache(s.cachedPath(productName, safeName, "original", "")+".mime", []byte(ct))
	return io.NopCloser(bytes.NewReader(data)), ct, nil
}

// DownloadPictureWithVariant returns the requested variant (thumb/preview) or the original image.
// Variants are cached as compressed WebP or JPEG on disk for faster subsequent loads.
// format can be "webp", "jpeg", or "" (defaults to webp for better compression).
func (s *ProductPictureService) DownloadPictureWithVariant(productName, fileName, variant, format string) (io.ReadCloser, string, error) {
	variant = normalizePictureVariant(variant)
	if variant == "" {
		return s.DownloadPicture(productName, fileName)
	}
	format = strings.ToLower(format)
	if format == "jpeg" || format == "jpg" {
		format = "jpg"
	} else {
		format = "webp"
	}
	ct := "image/webp"
	if format == "jpg" {
		ct = "image/jpeg"
	}
	target := s.cachedPath(productName, fileName, variant, format)
	if f, err := os.Open(target); err == nil {
		return f, ct, nil
	}
	if err := s.generateVariants(productName, fileName, format); err != nil {
		// Formats unsupported by the decoder (e.g. HEIC) retain their original.
		return s.DownloadPicture(productName, fileName)
	}
	f, err := os.Open(target)
	return f, ct, err
}

func (s *ProductPictureService) WarmPictureVariants(productName, fileName string) {
	if !s.Enabled() {
		return
	}
	select {
	case s.warming <- struct{}{}:
	default:
		return
	}
	go func() { defer func() { <-s.warming }(); _ = s.generateVariants(productName, fileName, "webp") }()
}

func (s *ProductPictureService) ClearCachedVariants(productName, fileName string) {
	if s == nil || s.cacheDir == "" {
		return
	}
	lockValue, _ := s.variantLocks.LoadOrStore(productName+"/"+fileName, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	for _, variant := range []string{"thumb", "preview"} {
		for _, format := range []string{"jpg", "webp"} {
			_ = os.Remove(s.cachedPath(productName, fileName, variant, format))
		}
	}
	_ = os.Remove(s.cachedPath(productName, fileName, "original", ""))
	_ = os.Remove(s.cachedPath(productName, fileName, "original", "") + ".mime")
	s.invalidateListing(productName)
}

// DeletePicture removes a specific picture from Nextcloud.
func (s *ProductPictureService) DeletePicture(productName, fileName string) error {
	if !s.Enabled() {
		return fmt.Errorf("product pictures not configured")
	}
	if fileName == "" {
		return fmt.Errorf("filename required")
	}
	if err := s.client.Delete(s.productFilePath(productName, sanitizeFileName(fileName))); err != nil {
		return err
	}
	s.ClearCachedVariants(productName, fileName)
	return nil
}

func (s *ProductPictureService) productFolder(productName string) string {
	return path.Join(s.rootFolder, sanitizeFolderName(productName))
}

func (s *ProductPictureService) productFilePath(productName, fileName string) string {
	return path.Join(s.productFolder(productName), sanitizeFileName(fileName))
}

func sanitizeFolderName(name string) string {
	clean := strings.TrimSpace(name)
	clean = strings.ReplaceAll(clean, "..", "")
	clean = strings.ReplaceAll(clean, "/", "-")
	clean = strings.ReplaceAll(clean, "\\", "-")
	clean = strings.ReplaceAll(clean, ":", "-")
	if clean == "" {
		return "unbenanntes-produkt"
	}
	return clean
}

func sanitizeFileName(name string) string {
	name = path.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "..", "")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	if name == "" {
		return "bild"
	}
	return name
}

func trimAndUnquote(val string) string {
	val = strings.TrimSpace(val)
	val = strings.Trim(val, "\"")
	return val
}

func isImageFile(entry storage.RemoteEntry) bool {
	if strings.HasPrefix(strings.ToLower(entry.ContentType), "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(entry.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".heif":
		return true
	default:
		return false
	}
}

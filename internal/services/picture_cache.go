package services

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
)

type pictureListing struct {
	items   []ProductPictureInfo
	expires time.Time
}

// Atomic replacement prevents browsers from observing partially encoded variants.
func writePictureCache(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".picture-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}

func (s *ProductPictureService) cachedPath(productName, fileName, variant, format string) string {
	extension := ""
	if format != "" {
		extension = "." + format
	}
	return filepath.Join(s.cacheDir, variant, sanitizeFolderName(productName), sanitizeFileName(fileName)+extension)
}

func (s *ProductPictureService) invalidateListing(productName string) {
	s.listMu.Lock()
	delete(s.listings, productName)
	s.listVersions[productName]++
	s.listMu.Unlock()
}

func (s *ProductPictureService) rememberPicture(productName string, info ProductPictureInfo) {
	s.listMu.Lock()
	defer s.listMu.Unlock()
	s.listVersions[productName]++
	if listing, ok := s.listings[productName]; ok {
		listing.items = append(append([]ProductPictureInfo(nil), listing.items...), info)
		listing.expires = time.Now().Add(30 * time.Second)
		s.listings[productName] = listing
	}
}

// UploadPictures bounds parallel remote uploads and preserves input order.
// A failed batch cleans up its own successfully uploaded files before returning.
func (s *ProductPictureService) UploadPictures(productName string, files []*multipart.FileHeader) ([]ProductPictureInfo, error) {
	if len(files) == 0 || len(files) > 20 {
		return nil, fmt.Errorf("select between 1 and 20 pictures")
	}
	results := make([]ProductPictureInfo, len(files))
	errs := make([]error, len(files))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 3)
	for i, header := range files {
		wg.Add(1)
		go func(i int, header *multipart.FileHeader) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			f, err := header.Open()
			if err != nil {
				errs[i] = err
				return
			}
			defer f.Close()
			name, err := s.UploadPicture(productName, f, header)
			if err != nil {
				errs[i] = err
				return
			}
			results[i] = ProductPictureInfo{FileName: name, Size: header.Size, ContentType: header.Header.Get("Content-Type"), ModifiedAt: time.Now()}
		}(i, header)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			for _, info := range results {
				if info.FileName != "" {
					if cleanupErr := s.DeletePicture(productName, info.FileName); cleanupErr != nil {
						log.Printf("[PICTURES] Partial upload cleanup failed")
					}
				}
			}
			return nil, err
		}
	}
	for _, info := range results {
		s.WarmPictureVariants(productName, info.FileName)
	}
	return results, nil
}

func (s *ProductPictureService) generateVariants(productName, fileName, format string) error {
	// Both sizes share a lock and a single original decode. Warm-up and browser
	// requests reuse one computation; at most two large images decode globally.
	key := productName + "/" + fileName
	lockValue, _ := s.variantLocks.LoadOrStore(key, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(s.cachedPath(productName, fileName, "thumb", format)); err == nil {
		if _, err := os.Stat(s.cachedPath(productName, fileName, "preview", format)); err == nil {
			return nil
		}
	}
	s.processing <- struct{}{}
	defer func() { <-s.processing }()
	original, _, err := s.downloadPictureLocked(productName, fileName)
	if err != nil {
		return err
	}
	defer original.Close()
	data, err := io.ReadAll(io.LimitReader(original, s.maxFileSize+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > s.maxFileSize {
		return fmt.Errorf("picture exceeds processing limit")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return fmt.Errorf("picture exceeds 40 megapixel processing limit")
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return err
	}
	for _, variant := range []struct {
		name string
		size int
	}{{"thumb", 480}, {"preview", 1200}} {
		target := s.cachedPath(productName, fileName, variant.name, format)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		resized := imaging.Fit(img, variant.size, variant.size, imaging.Linear)
		var buf bytes.Buffer
		if format == "webp" {
			err = webp.Encode(&buf, resized, &webp.Options{Quality: 82})
		} else {
			err = imaging.Encode(&buf, resized, imaging.JPEG, imaging.JPEGQuality(85))
		}
		if err != nil {
			return err
		}
		if err = writePictureCache(target, buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProductPictureService) originalFromCache(productName, fileName string) (io.ReadCloser, string, error) {
	data, err := os.ReadFile(s.cachedPath(productName, fileName, "original", ""))
	if err != nil {
		return nil, "", err
	}
	ct := pictureContentType(data)
	if !s.allowedTypes[ct] {
		return nil, "", fmt.Errorf("unsupported cached image content")
	}
	return io.NopCloser(bytes.NewReader(data)), ct, nil
}

func normalizePictureVariant(variant string) string {
	switch strings.ToLower(variant) {
	case "thumb", "thumbnail":
		return "thumb"
	case "preview", "medium":
		return "preview"
	default:
		return ""
	}
}

func pictureContentType(data []byte) string {
	detected := http.DetectContentType(data)
	if detected == "application/octet-stream" && len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
			return "image/heic"
		}
	}
	return detected
}

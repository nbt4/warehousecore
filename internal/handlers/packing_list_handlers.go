package handlers

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

const defaultPackingListDirectory = "/var/lib/warehousecore/packing-lists"

// GetJobPackingList serves the saved PDF and generates its first snapshot on demand.
func GetJobPackingList(w http.ResponseWriter, r *http.Request) {
	serveJobPackingList(w, r, false)
}

// RegenerateJobPackingList replaces the saved PDF with current job data.
func RegenerateJobPackingList(w http.ResponseWriter, r *http.Request) {
	serveJobPackingList(w, r, true)
}

func serveJobPackingList(w http.ResponseWriter, r *http.Request, regenerate bool) {
	jobID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || jobID < 1 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid job ID"})
		return
	}

	directory := strings.TrimSpace(os.Getenv("PACKING_LIST_DIR"))
	if directory == "" {
		directory = defaultPackingListDirectory
	}
	path := filepath.Join(directory, fmt.Sprintf("job-%d-packing-list.pdf", jobID))
	if regenerate || !regularFileExists(path) {
		if err := generateAndSavePackingList(jobID, directory, path); err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="job-%d-packliste.pdf"`, jobID))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func generateAndSavePackingList(jobID int, directory, destination string) error {
	packingList, err := services.LoadPackingList(repository.GetSQLDB(), jobID)
	if err != nil {
		return err
	}

	branding := services.NewBrandingService(repository.GetDB(), "warehouse").GetConfig()
	packingList.CompanyName = branding.CompanyName
	if packingList.CompanyName == "" {
		packingList.CompanyName = "Cores"
	}
	packingList.LogoDataURI = loadBrandingLogoDataURI(
		branding.CompanyAssets.Print,
		branding.CompanyAssets.HorizontalOnLight,
		branding.CompanyAssets.MarkOnLight,
	)
	packingList.BarcodeURI, err = services.NewLabelService().GenerateBarcode(packingList.JobCode, 600, 120)
	if err != nil {
		return fmt.Errorf("generate job barcode: %w", err)
	}

	html, err := services.RenderPackingListHTML(packingList)
	if err != nil {
		return err
	}
	pdf, err := services.NewLabelService().RenderHTMLToPDF(html, 210, 297)
	if err != nil {
		return fmt.Errorf("generate packing-list PDF: %w", err)
	}

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create packing-list directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".packing-list-*.pdf")
	if err != nil {
		return fmt.Errorf("create temporary packing list: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(pdf); err != nil {
		temporary.Close()
		return fmt.Errorf("write packing list: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync packing list: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close packing list: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("save packing list: %w", err)
	}
	return nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loadBrandingLogoDataURI(candidates ...string) string {
	for _, rawPath := range candidates {
		rawPath = strings.TrimSpace(rawPath)
		if rawPath == "" {
			continue
		}
		if parsed, err := url.Parse(rawPath); err == nil {
			rawPath = parsed.Path
		}
		name := filepath.Base(rawPath)
		if name == "." || name == string(filepath.Separator) {
			continue
		}
		for _, directory := range []string{"/var/lib/branding/logos", "./web/dist/logos"} {
			contents, err := os.ReadFile(filepath.Join(directory, name))
			if err != nil {
				continue
			}
			contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
			if contentType == "" || !strings.HasPrefix(contentType, "image/") {
				contentType = "image/png"
			}
			return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(contents)
		}
	}
	return ""
}

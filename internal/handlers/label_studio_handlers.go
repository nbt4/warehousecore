package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"warehousecore/internal/models"
	"warehousecore/internal/services"
)

func writeLabelJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// ListLabelTargets returns printable entities for the selected target type.
func ListLabelTargets(w http.ResponseWriter, r *http.Request) {
	targetType := strings.TrimSpace(r.URL.Query().Get("type"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	targets, err := labelService.ListTargets(targetType, r.URL.Query().Get("search"), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeLabelJSON(w, http.StatusOK, targets)
}

func GetLabelTarget(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	target, err := labelService.GetTarget(vars["target_type"], vars["target_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeLabelJSON(w, http.StatusOK, target)
}

func GetLabelFields(w http.ResponseWriter, r *http.Request) {
	targetType := mux.Vars(r)["target_type"]
	fields := services.LabelFields(targetType)
	if fields == nil {
		http.Error(w, "unsupported target type", http.StatusBadRequest)
		return
	}
	writeLabelJSON(w, http.StatusOK, fields)
}

func RenderTargetLabel(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		TemplateID int    `json:"template_id"`
		Save       bool   `json:"save"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if request.TargetID == "" || !services.ValidLabelTargetType(request.TargetType) {
		http.Error(w, "valid target_type and target_id are required", http.StatusBadRequest)
		return
	}
	result, err := labelService.RenderTargetLabel(request.TargetType, request.TargetID, request.TemplateID, request.Save)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeLabelJSON(w, http.StatusOK, result)
}

func RenderTargetLabels(w http.ResponseWriter, r *http.Request) {
	var request services.LabelBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	results, err := labelService.RenderTargetLabels(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if !request.IncludeImage {
		for _, result := range results {
			result.ImageData = ""
		}
	}
	writeLabelJSON(w, http.StatusOK, map[string]any{"results": results})
}

func ExportLabelsPDF(w http.ResponseWriter, r *http.Request) {
	var request services.LabelPDFRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	pdfData, err := labelService.ExportTargetsPDF(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="warehousecore-labels.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfData)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfData)
}

func ListLabelPrinters(w http.ResponseWriter, _ *http.Request) {
	printers, err := labelService.ListPrinters()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeLabelJSON(w, http.StatusOK, printers)
}

func CreateLabelPrinter(w http.ResponseWriter, r *http.Request) {
	var printer models.LabelPrinter
	if err := json.NewDecoder(r.Body).Decode(&printer); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := labelService.SavePrinter(&printer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeLabelJSON(w, http.StatusCreated, printer)
}

func UpdateLabelPrinter(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid printer ID", http.StatusBadRequest)
		return
	}
	var printer models.LabelPrinter
	if err := json.NewDecoder(r.Body).Decode(&printer); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	printer.ID = id
	if err := labelService.SavePrinter(&printer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeLabelJSON(w, http.StatusOK, printer)
}

func DeleteLabelPrinter(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		http.Error(w, "invalid printer ID", http.StatusBadRequest)
		return
	}
	if err := labelService.DeletePrinter(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func PrintLabels(w http.ResponseWriter, r *http.Request) {
	var request services.LabelPrintRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	jobs, err := labelService.PrintTargets(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeLabelJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func ListLabelPrintJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	jobs, err := labelService.ListPrintJobs(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeLabelJSON(w, http.StatusOK, jobs)
}

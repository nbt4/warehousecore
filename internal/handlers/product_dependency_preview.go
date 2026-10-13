package handlers

import (
	"github.com/gorilla/mux"
	"math"
	"net/http"
	"strconv"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

func GetProductDependencyPreview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	quantity, err2 := strconv.ParseFloat(r.URL.Query().Get("quantity"), 64)
	cases, err3 := strconv.Atoi(r.URL.Query().Get("cases"))
	if err != nil || err2 != nil || err3 != nil || math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity <= 0 || quantity > 1000 || cases < 0 || cases > 1000 {
		respondJSON(w, 400, map[string]string{"error": "Menge und Case-Anzahl sind ungültig"})
		return
	}
	graph, err := services.LoadMaterialGraph(repository.GetSQLDB())
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Packregeln konnten nicht geladen werden"})
		return
	}
	if _, ok := graph.Products[id]; !ok {
		respondJSON(w, 404, map[string]string{"error": "Aktives Produkt nicht gefunden"})
		return
	}
	preview, err := services.ExpandMaterial(graph, []services.MaterialRoot{{ProductID: id, Quantity: quantity, Cases: cases}})
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, 200, preview)
}

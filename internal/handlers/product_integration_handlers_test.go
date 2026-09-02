package handlers

import (
	"encoding/json"
	"testing"
)

func TestScoreCatalogProductUsesStableIdentifiers(t *testing.T) {
	warehouse := warehouseLinkProduct{EAN: "40 06380-133936", ManufacturerSKU: "ABC-42", Manufacturer: "Acme", Model: "Road One", Name: "Acme Road One"}
	procurement := procurementCatalogProduct{SKU: "ABC42", Manufacturer: "ACME", Model: "Road-One", Name: "Acme Road One", Attributes: json.RawMessage(`{"EAN":"4006380133936"}`)}

	score, reasons := scoreCatalogProduct(warehouse, procurement)
	if score != 320 {
		t.Fatalf("scoreCatalogProduct() score = %d, want 320 (reasons: %v)", score, reasons)
	}
}

func TestCatalogEANReadsParameters(t *testing.T) {
	product := procurementCatalogProduct{Parameters: json.RawMessage(`{"GTIN-13":"4006380133936"}`)}
	if got := catalogEAN(product); got != "4006380133936" {
		t.Fatalf("catalogEAN() = %q", got)
	}
}

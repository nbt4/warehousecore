package handlers

import (
	"reflect"
	"testing"
)

func TestWarehouseProductSearchTerms(t *testing.T) {
	want := []string{"ld", "systems", "stinger", "sub", "18a", "g3"}
	if got := warehouseProductSearchTerms(" LD Systems  Stinger SUB 18A G3 "); !reflect.DeepEqual(got, want) {
		t.Fatalf("warehouseProductSearchTerms() = %v, want %v", got, want)
	}
}

func intPointer(value int) *int { return &value }

func floatPointer(value float64) *float64 { return &value }

func TestNormalizeProductRequest(t *testing.T) {
	tests := []struct {
		name         string
		product      Product
		wantType     string
		wantTracking string
		wantErr      bool
	}{
		{
			name:         "defaults equipment to individual tracking",
			product:      Product{Name: "  Mixer  "},
			wantType:     "equipment",
			wantTracking: "individual",
		},
		{
			name:         "legacy accessory becomes quantity tracked",
			product:      Product{Name: "Clamp", IsAccessory: true, CountTypeID: intPointer(1)},
			wantType:     "accessory",
			wantTracking: "quantity",
		},
		{
			name:    "quantity tracking requires unit",
			product: Product{Name: "Tape", ProductType: "consumable", TrackingMode: "quantity"},
			wantErr: true,
		},
		{
			name:    "consumable cannot be individually tracked",
			product: Product{Name: "Fluid", ProductType: "consumable", TrackingMode: "individual", CountTypeID: intPointer(1)},
			wantErr: true,
		},
		{
			name:    "negative stock is rejected",
			product: Product{Name: "Fluid", ProductType: "consumable", TrackingMode: "quantity", CountTypeID: intPointer(1), StockQuantity: floatPointer(-1)},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := normalizeProductRequest(&test.product)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeProductRequest() error = %v, wantErr %v", err, test.wantErr)
			}
			if test.wantErr {
				return
			}
			if test.product.ProductType != test.wantType {
				t.Fatalf("product type = %q, want %q", test.product.ProductType, test.wantType)
			}
			if test.product.TrackingMode != test.wantTracking {
				t.Fatalf("tracking mode = %q, want %q", test.product.TrackingMode, test.wantTracking)
			}
			if test.product.Name != "Mixer" && test.name == "defaults equipment to individual tracking" {
				t.Fatalf("name was not trimmed: %q", test.product.Name)
			}
		})
	}
}

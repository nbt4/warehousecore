package handlers

import (
	"database/sql"
	"reflect"
	"testing"

	"warehousecore/internal/models"
)

func TestNormalizeAliases(t *testing.T) {
	got := normalizeAliases([]string{"  Sound L ", "sound l", "", "PA Set"})
	want := []string{"Sound L", "PA Set"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeAliases() = %#v, want %#v", got, want)
	}
}

func TestParseStringSlice(t *testing.T) {
	got := parseStringSlice(sql.NullString{Valid: true, String: `["front.jpg","front.jpg","back.jpg"]`})
	want := []string{"front.jpg", "back.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseStringSlice() = %#v, want %#v", got, want)
	}
}

func TestValidateProductPackageRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     productPackageRequest
		wantErr bool
	}{
		{name: "valid", req: productPackageRequest{Name: "Sound Set", Items: []models.ProductPackageItem{{ProductID: 1, Quantity: 2}}}},
		{name: "missing name", req: productPackageRequest{Items: []models.ProductPackageItem{{ProductID: 1, Quantity: 1}}}, wantErr: true},
		{name: "missing items", req: productPackageRequest{Name: "Empty"}, wantErr: true},
		{name: "invalid quantity", req: productPackageRequest{Name: "Empty", Items: []models.ProductPackageItem{{ProductID: 1, Quantity: 0}}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateProductPackageRequest(&test.req)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateProductPackageRequest() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

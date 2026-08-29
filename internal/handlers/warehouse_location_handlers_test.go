package handlers

import "testing"

func TestNormalizeWarehouseLocationBarcode(t *testing.T) {
	manual := "  EXISTING-4711  "
	empty := "  "
	tests := []struct {
		name    string
		barcode *string
		code    string
		want    string
	}{
		{name: "generates when omitted", code: "wdl-rg-01", want: "LOC-WDL-RG-01"},
		{name: "generates when empty", barcode: &empty, code: " WDL-F-02 ", want: "LOC-WDL-F-02"},
		{name: "preserves manual label", barcode: &manual, code: "WDL-F-03", want: "EXISTING-4711"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWarehouseLocationBarcode(tt.barcode, tt.code)
			if got == nil || *got != tt.want {
				t.Fatalf("normalizeWarehouseLocationBarcode() = %v, want %q", got, tt.want)
			}
		})
	}
}

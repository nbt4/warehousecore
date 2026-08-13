package handlers

import "testing"

func TestNormalizeCableTrackingMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "defaults to quantity", input: "", want: cableTrackingQuantity},
		{name: "normalizes individual", input: " Individual ", want: cableTrackingIndividual},
		{name: "accepts quantity", input: "quantity", want: cableTrackingQuantity},
		{name: "rejects unknown mode", input: "serial", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeCableTrackingMode(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeCableTrackingMode() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("normalizeCableTrackingMode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateCableCreateRequest(t *testing.T) {
	validMM2 := 2.5
	invalidMM2 := 0.0
	tests := []struct {
		name    string
		input   cableCreateRequest
		wantErr bool
	}{
		{
			name:  "valid quantity cable",
			input: cableCreateRequest{Connector1: 1, Connector2: 2, Typ: 1, Length: 10, MM2: &validMM2, TrackingMode: "quantity", Quantity: 5},
		},
		{
			name:  "valid individual cable",
			input: cableCreateRequest{Connector1: 1, Connector2: 2, Typ: 1, Length: 10, TrackingMode: "individual", Quantity: 3},
		},
		{
			name:    "missing connector",
			input:   cableCreateRequest{Connector2: 2, Typ: 1, Length: 10},
			wantErr: true,
		},
		{
			name:    "invalid length",
			input:   cableCreateRequest{Connector1: 1, Connector2: 2, Typ: 1, Length: 0},
			wantErr: true,
		},
		{
			name:    "invalid cross section",
			input:   cableCreateRequest{Connector1: 1, Connector2: 2, Typ: 1, Length: 10, MM2: &invalidMM2},
			wantErr: true,
		},
		{
			name:    "negative quantity",
			input:   cableCreateRequest{Connector1: 1, Connector2: 2, Typ: 1, Length: 10, Quantity: -1},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCableCreateRequest(&test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCableCreateRequest() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestBuildCableProductName(t *testing.T) {
	got := buildCableProductName("Audio", "XLR3 male", "XLR3 female", 10)
	want := "Audio (XLR3 male – XLR3 female) · 10 m"
	if got != want {
		t.Fatalf("buildCableProductName() = %q, want %q", got, want)
	}
}

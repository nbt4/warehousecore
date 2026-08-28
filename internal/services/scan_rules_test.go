package services

import (
	"database/sql"
	"strings"
	"testing"

	"warehousecore/internal/models"
)

func TestRequestedQuantity(t *testing.T) {
	one, err := requestedQuantity(nil)
	if err != nil || one != 1 {
		t.Fatalf("default quantity = %v, %v; want 1, nil", one, err)
	}
	invalid := 0.0
	if _, err := requestedQuantity(&invalid); err == nil {
		t.Fatal("expected zero quantity to fail")
	}
	valid := 2.5
	if got, err := requestedQuantity(&valid); err != nil || got != valid {
		t.Fatalf("quantity = %v, %v; want %v, nil", got, err, valid)
	}
}

func TestClosedJobStatus(t *testing.T) {
	for _, status := range []string{"Abgeschlossen", "Abgerechnet", "Storniert", "completed", "paid", "canceled"} {
		if !isClosedJobStatus(status) {
			t.Errorf("expected %q to be closed", status)
		}
	}
	if isClosedJobStatus("Aktiv") {
		t.Fatal("expected active job to remain open")
	}
}

func TestReturnPendingDeviceCannotBeIssuedAgain(t *testing.T) {
	device := &models.Device{
		DeviceID:          "SUB1001",
		Status:            "return_pending",
		CurrentJobID:      sql.NullInt64{Int64: 1148, Valid: true},
		CurrentJobCode:    "JOB001148",
		CurrentJobStatus:  "Abgerechnet",
		CurrentPackStatus: "issued",
	}
	err := validateDeviceForOuttake(device, 1200)
	if err == nil || !strings.Contains(err.Error(), "Rückgabe") {
		t.Fatalf("expected return guidance, got %v", err)
	}
}

func TestOnlyAvailableStoredStandaloneDeviceCanBeIssued(t *testing.T) {
	tests := []struct {
		name   string
		device models.Device
		want   string
	}{
		{name: "available stored", device: models.Device{DeviceID: "DEV-1", Status: "in_storage", ConditionStatus: "available"}},
		{name: "blocked", device: models.Device{DeviceID: "DEV-2", Status: "in_storage", ConditionStatus: "blocked"}, want: "Gesperrt"},
		{name: "unknown location", device: models.Device{DeviceID: "DEV-3", Status: "location_unknown", ConditionStatus: "available"}, want: "ungeklärt"},
		{name: "inside case", device: models.Device{DeviceID: "DEV-4", Status: "in_storage", ConditionStatus: "available", CaseID: sql.NullInt64{Int64: 7, Valid: true}}, want: "Case 7"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDeviceForOuttake(&test.device, 42)
			if test.want == "" && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("expected %q guidance, got %v", test.want, err)
			}
		})
	}
}

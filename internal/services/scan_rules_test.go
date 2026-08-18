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

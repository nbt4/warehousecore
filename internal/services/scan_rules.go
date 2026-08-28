package services

import (
	"fmt"
	"math"
	"strings"

	"warehousecore/internal/models"
)

func requestedQuantity(input *float64) (float64, error) {
	if input == nil {
		return 1, nil
	}
	if *input <= 0 || math.IsNaN(*input) || math.IsInf(*input, 0) {
		return 0, fmt.Errorf("Menge muss größer als 0 sein")
	}
	return *input, nil
}

func isClosedJobStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "abgeschlossen", "abgerechnet", "storniert", "completed", "paid", "canceled", "cancelled":
		return true
	default:
		return false
	}
}

func validateDeviceForOuttake(device *models.Device, requestedJobID int64) error {
	if device.ConditionStatus != "" && device.ConditionStatus != "available" {
		return fmt.Errorf("%s kann mit Betriebszustand %s nicht ausgegeben werden", device.DeviceID, deviceConditionLabel(device.ConditionStatus))
	}
	if device.CaseID.Valid {
		return fmt.Errorf("%s befindet sich in Case %d; bitte das Case ausgeben oder das Gerät zuerst auspacken", device.DeviceID, device.CaseID.Int64)
	}
	jobLabel := device.CurrentJobCode
	if jobLabel == "" && device.CurrentJobID.Valid {
		jobLabel = fmt.Sprintf("Job %d", device.CurrentJobID.Int64)
	}

	if device.CurrentJobID.Valid && device.CurrentPackStatus == "issued" {
		if isClosedJobStatus(device.CurrentJobStatus) || device.Status == "return_pending" {
			return fmt.Errorf("%s wartet noch auf Rückgabe aus %s; bitte zuerst einlagern", device.DeviceID, jobLabel)
		}
		if device.CurrentJobID.Int64 != requestedJobID {
			return fmt.Errorf("%s ist bereits für %s ausgegeben", device.DeviceID, jobLabel)
		}
	}

	switch device.Status {
	case "return_pending":
		return fmt.Errorf("%s wartet auf Rückgabe; bitte zuerst einlagern", device.DeviceID)
	case "location_unknown":
		return fmt.Errorf("Standort von %s ist ungeklärt; bitte zuerst einlagern", device.DeviceID)
	case "on_job":
		if !device.CurrentJobID.Valid {
			return fmt.Errorf("%s ist als ausgegeben markiert, hat aber keinen gültigen Job; bitte zuerst einlagern", device.DeviceID)
		}
	}
	if device.Status != "in_storage" {
		return fmt.Errorf("%s ist nicht als physisch eingelagert bestätigt", device.DeviceID)
	}
	return nil
}

func deviceConditionLabel(status string) string {
	switch status {
	case "available":
		return "Einsatzbereit"
	case "blocked":
		return "Gesperrt"
	case "defective":
		return "Defekt"
	case "maintenance":
		return "In Wartung"
	case "retired":
		return "Ausgemustert"
	default:
		return "Unbekannt"
	}
}

func deviceStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "free":
		return "Frei"
	case "in_storage":
		return "Im Lager"
	case "on_job", "rented":
		return "Ausgegeben"
	case "return_pending":
		return "Rückgabe offen"
	case "location_unknown":
		return "Standort ungeklärt"
	case "defective":
		return "Defekt"
	case "repair", "maintenance":
		return "In Wartung"
	case "blocked":
		return "Gesperrt"
	case "retired":
		return "Ausgemustert"
	default:
		if status == "" {
			return "Unbekannt"
		}
		return status
	}
}

func decorateDeviceStatus(device *models.DeviceWithDetails) {
	device.StatusLabel = deviceStatusLabel(device.Status)
	jobCode := device.JobNumber
	if jobCode == "" {
		jobCode = device.CurrentJobCode
	}

	switch device.Status {
	case "return_pending":
		device.NeedsReturn = true
		if jobCode != "" {
			device.StatusDetail = fmt.Sprintf("%s ist %s; Einlagerung wurde noch nicht gescannt", jobCode, device.CurrentJobStatus)
		} else {
			device.StatusDetail = "Einlagerung wurde noch nicht gescannt"
		}
	case "location_unknown":
		device.StatusDetail = "Kein aktiver Job und kein bestätigter Lagerplatz"
	case "on_job", "rented":
		if jobCode != "" {
			device.StatusDetail = fmt.Sprintf("Ausgegeben an %s (%s)", jobCode, device.CurrentJobStatus)
		}
	case "in_storage":
		if device.ZoneName != "" {
			device.StatusDetail = "Lagerplatz: " + device.ZoneName
		} else {
			device.StatusDetail = "Im Lager, aber ohne erfassten Lagerplatz"
		}
	}
	if device.ConditionStatus != "" && device.ConditionStatus != "available" {
		condition := "Betriebszustand: " + deviceConditionLabel(device.ConditionStatus)
		if device.StatusDetail == "" {
			device.StatusDetail = condition
		} else {
			device.StatusDetail += " · " + condition
		}
	}
}

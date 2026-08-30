package jobstatus

import "strings"

const (
	PlanningID  = 1
	ConfirmedID = 2
	CompletedID = 4
	CancelledID = 6

	Planning  = "Planung"
	Confirmed = "Bestätigt"
	Completed = "Abgeschlossen"
	Cancelled = "Storniert"
)

func IsClosed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "abgeschlossen", "storniert", "completed", "paid", "canceled", "cancelled", "abgerechnet":
		return true
	default:
		return false
	}
}

func IsDispatchable(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), Confirmed)
}

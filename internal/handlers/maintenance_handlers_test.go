package handlers

import "testing"

func TestValidMaintenanceTransition(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		expected bool
	}{
		{name: "start open work", from: "open", to: "in_progress", expected: true},
		{name: "start planned work", from: "planned", to: "in_progress", expected: true},
		{name: "wait for parts", from: "in_progress", to: "waiting_parts", expected: true},
		{name: "resume after parts", from: "waiting_parts", to: "in_progress", expected: true},
		{name: "complete active work", from: "in_progress", to: "completed", expected: true},
		{name: "cannot skip execution", from: "open", to: "completed", expected: false},
		{name: "completed is terminal", from: "completed", to: "in_progress", expected: false},
		{name: "cancelled is terminal", from: "cancelled", to: "open", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validMaintenanceTransition(tt.from, tt.to); got != tt.expected {
				t.Fatalf("validMaintenanceTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.expected)
			}
		})
	}
}

func TestValidateMaintenanceOrderInput(t *testing.T) {
	tests := []struct {
		name    string
		input   maintenanceOrderInput
		wantErr bool
	}{
		{name: "valid defect", input: maintenanceOrderInput{DeviceID: " DEV-1 ", OrderType: "defect", Priority: "critical", Title: " Netzteil defekt ", DueAt: "2026-09-01"}},
		{name: "defaults priority", input: maintenanceOrderInput{DeviceID: "DEV-1", OrderType: "inspection", Title: "Prüfung", DueAt: "2026-09-01"}},
		{name: "requires device", input: maintenanceOrderInput{OrderType: "defect", Priority: "normal", Title: "Defekt"}, wantErr: true},
		{name: "rejects type", input: maintenanceOrderInput{DeviceID: "DEV-1", OrderType: "cleaning", Priority: "normal", Title: "Reinigung"}, wantErr: true},
		{name: "rejects priority", input: maintenanceOrderInput{DeviceID: "DEV-1", OrderType: "defect", Priority: "urgent", Title: "Defekt"}, wantErr: true},
		{name: "rejects negative cost", input: maintenanceOrderInput{DeviceID: "DEV-1", OrderType: "defect", Priority: "normal", Title: "Defekt", Cost: maintenanceFloatPointer(-1)}, wantErr: true},
		{name: "rejects invalid date", input: maintenanceOrderInput{DeviceID: "DEV-1", OrderType: "defect", Priority: "normal", Title: "Defekt", DueAt: "01.09.2026"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateMaintenanceOrderInput(&tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMaintenanceOrderInput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.name == "defaults priority" && tt.input.Priority != "normal" {
				t.Fatalf("validateMaintenanceOrderInput() priority = %q, want normal", tt.input.Priority)
			}
		})
	}
}

func TestValidateMaintenancePlanInput(t *testing.T) {
	tests := []struct {
		name    string
		input   maintenancePlanInput
		wantErr bool
	}{
		{name: "valid inspection", input: maintenancePlanInput{DeviceID: "DEV-1", Name: "DGUV", MaintenanceType: "inspection", IntervalDays: 365, LeadTimeDays: 30, NextDueAt: "2027-01-01"}},
		{name: "rejects defect plan", input: maintenancePlanInput{DeviceID: "DEV-1", Name: "Defekt", MaintenanceType: "defect", IntervalDays: 1, NextDueAt: "2027-01-01"}, wantErr: true},
		{name: "requires next date", input: maintenancePlanInput{DeviceID: "DEV-1", Name: "Wartung", MaintenanceType: "preventive", IntervalDays: 365}, wantErr: true},
		{name: "rejects zero interval", input: maintenancePlanInput{DeviceID: "DEV-1", Name: "Wartung", MaintenanceType: "preventive", NextDueAt: "2027-01-01"}, wantErr: true},
		{name: "rejects excessive lead time", input: maintenancePlanInput{DeviceID: "DEV-1", Name: "Wartung", MaintenanceType: "preventive", IntervalDays: 365, LeadTimeDays: 366, NextDueAt: "2027-01-01"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateMaintenancePlanInput(&tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMaintenancePlanInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func maintenanceFloatPointer(value float64) *float64 { return &value }

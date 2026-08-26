package services

import "testing"

func TestLocationCapacityTotal(t *testing.T) {
	capacity := LocationCapacity{Devices: 2, Cases: 1, Products: 3.5}
	capacity.Total = capacity.Devices + capacity.Cases + capacity.Products
	if capacity.Total != 6.5 {
		t.Fatalf("total = %v, want 6.5", capacity.Total)
	}
}

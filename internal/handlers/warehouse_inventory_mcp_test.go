package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestInventoryBlindLinesNeverExposeExpectedOrMutableReferences(t *testing.T) {
	lines := []map[string]any{{"line_id": float64(1), "item_type": "product", "item_key": "3", "expected_quantity": float64(9), "counted_quantity": float64(2), "record": map[string]any{"stock_quantity": float64(9)}}}
	blind := inventoryMCPHideBlind(lines, true)
	if _, ok := blind[0]["expected_quantity"]; ok {
		t.Fatal("blind expected quantity leaked")
	}
	if _, ok := blind[0]["record"]; ok {
		t.Fatal("stock reference leaked")
	}
	if blind[0]["counted_quantity"] != float64(2) {
		t.Fatal("recorded quantity missing")
	}
	blind[0]["counted_quantity"] = float64(5)
	if lines[0]["counted_quantity"] != float64(2) {
		t.Fatal("redaction mutated internal baseline/draft")
	}
	review := inventoryMCPHideBlind(lines, false)
	if review[0]["expected_quantity"] != float64(9) {
		t.Fatal("review hides expected quantity")
	}
}

func TestInventoryContextRoundTripAndVersionChanges(t *testing.T) {
	value := map[string]any{"items": []map[string]any{{"quantity": float64(3.125), "versions": map[string]int64{"dependencies": 2}}}, "updated_at": "2026-10-01T12:00:00.000001Z"}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if inventoryMCPHash(value) != inventoryMCPHash(decoded) {
		t.Fatal("stored JSON baseline changes fingerprint")
	}
	decoded.(map[string]any)["updated_at"] = "2026-10-01T12:00:00.000002Z"
	if inventoryMCPHash(value) == inventoryMCPHash(decoded) {
		t.Fatal("precise version change not detected")
	}
}

func TestInventoryProjectedCapacityIncludesPackedContents(t *testing.T) {
	zone := map[string]any{"capacity_mode": "item_count", "capacity": float64(10), "max_weight_kg": float64(20), "max_volume_m3": float64(1), "allow_devices": true, "allow_quantity_products": true, "allow_cases": true, "allow_mixed_products": true}
	lines := []map[string]any{{"item_type": "case", "item_key": "1", "counted_quantity": float64(1)}, {"item_type": "product", "item_key": "3", "counted_quantity": float64(2)}}
	refs := map[string]any{"case:1": map[string]any{"width": float64(100), "height": float64(50), "depth": float64(50), "case_tree": []map[string]any{{"weight": float64(4)}, {"weight": float64(2)}}, "packed_devices": []map[string]any{{"product_id": float64(1), "weight": float64(3)}}, "product_contents": []map[string]any{{"product_id": float64(3), "quantity": float64(2), "weight": float64(1)}}}, "product:3": map[string]any{"weight": float64(1), "width": float64(10), "height": float64(10), "depth": float64(10)}}
	load, required := inventoryMCPCapacity(lines, refs, zone)
	if len(required) != 0 || load["item_count"] != float64(3) || load["weight_kg"] != float64(13) || load["volume_m3"] != float64(.252) {
		t.Fatalf("wrong physical projection: %#v %#v", load, required)
	}
	for _, test := range []struct {
		name, key string
		value     any
		want      string
	}{
		{"count", "capacity", float64(2), "projected_capacity"}, {"weight", "max_weight_kg", float64(12), "projected_weight_capacity"}, {"volume", "max_volume_m3", float64(.2), "projected_volume_capacity"}, {"case profile", "allow_cases", false, "location_profile_item_types"}, {"packed product mixture", "allow_mixed_products", false, "location_profile_mixed_products"},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := map[string]any{}
			for key, value := range zone {
				copy[key] = value
			}
			copy[test.key] = test.value
			_, blocked := inventoryMCPCapacity(lines, refs, copy)
			found := false
			for _, b := range blocked {
				found = found || b == test.want
			}
			if !found {
				t.Fatal("physical limit ignored", blocked)
			}
		})
	}
	refs["product:3"].(map[string]any)["weight"] = nil
	load, required = inventoryMCPCapacity(lines, refs, zone)
	if load["weight_kg"] != nil || !reflect.DeepEqual(required, []string{"complete_item_and_case_content_weights"}) {
		t.Fatal("missing weight treated as zero", load, required)
	}
}

func TestInventoryBlocksAssignedAndPackedSerializedStock(t *testing.T) {
	base := map[string]any{"lifecycle_status": "active", "product_lifecycle": "active", "status": "in_storage", "condition_status": "available", "current_case_id": nil, "dependencies": map[string]int64{}}
	if inventoryMCPBlockedItem("device", base, false) {
		t.Fatal("available direct inventory blocked")
	}
	base["dependencies"] = map[string]int64{"jobs": 1}
	if !inventoryMCPBlockedItem("device", base, false) {
		t.Fatal("active assignment can be reconciled")
	}
	base["dependencies"] = map[string]int64{"cases": 1}
	if !inventoryMCPBlockedItem("device", base, false) || inventoryMCPBlockedItem("device", base, true) {
		t.Fatal("packed membership semantics lost")
	}
	base["condition_status"] = "retired"
	if !inventoryMCPBlockedItem("device", base, true) {
		t.Fatal("retired packed content can move")
	}
}

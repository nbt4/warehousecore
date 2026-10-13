package services

import (
	"math"
	"testing"
)

func TestMaterialRulesScopeTypesPrecisionAndFreeze(t *testing.T) {
	graph := MaterialGraph{Products: map[int]MaterialProduct{1: {ID: 1, Name: "Head", Tracking: "individual", Unit: "Stk"}, 2: {ID: 2, Name: "Cable", Tracking: "individual", Unit: "Stk"}, 3: {ID: 3, Name: "Tape", Tracking: "quantity", Unit: "m"}, 4: {ID: 4, Name: "Optional", Tracking: "individual", Unit: "Stk"}}, Rules: map[int][]MaterialRule{1: {{ID: 1, Source: 1, Target: 2, Type: "required", Scope: "case", Quantity: 2}, {ID: 2, Source: 1, Target: 4, Type: "recommended", Scope: "device", Quantity: 99}}, 2: {{ID: 3, Source: 2, Target: 3, Type: "consumes", Scope: "case", Quantity: 0.0004}}}}
	roots := []MaterialRoot{{ProductID: 1, Quantity: 4, Cases: 2}}
	result, err := ExpandMaterialWithCases(graph, roots, map[int]int{1: 2, 2: 1})
	if err != nil {
		t.Fatal(err)
	}
	quantities := map[int]float64{}
	for _, item := range result.Items {
		quantities[item.ProductID] = item.Quantity
	}
	if quantities[1] != 4 || quantities[2] != 4 || quantities[3] != 0.001 || quantities[4] != 0 || len(result.Warnings) != 0 {
		t.Fatalf("scope/types/precision: %+v", result)
	}
	result, err = ExpandMaterial(graph, roots)
	if err != nil || len(result.Warnings) != 1 {
		t.Fatalf("nested case assignment must not inherit root cases: %+v %v", result, err)
	}
	frozen := FreezeMaterialGraph(graph, roots, MaterialGraph{})
	graph.Rules[1][0].Quantity = 99
	frozen = FreezeMaterialGraph(graph, roots, frozen)
	if frozen.Rules[1][0].Quantity != 2 {
		t.Fatal("existing job rules changed")
	}
	graph.Rules[2] = []MaterialRule{{Target: 1, Type: "required", Quantity: 1}}
	result, err = ExpandMaterial(graph, roots)
	if err != nil || len(result.Warnings) == 0 {
		t.Fatal("cycle not reported")
	}
	_, err = ExpandMaterial(graph, []MaterialRoot{{ProductID: 1, Quantity: math.Inf(1)}})
	if err == nil {
		t.Fatal("infinite quantity accepted")
	}
}

func TestMaterialSharedAccessoryCountsItsCaseRuleOnce(t *testing.T) {
	graph := MaterialGraph{Products: map[int]MaterialProduct{1: {ID: 1, Tracking: "individual"}, 2: {ID: 2, Tracking: "individual"}, 3: {ID: 3, Tracking: "individual"}, 4: {ID: 4, Tracking: "individual"}}, Rules: map[int][]MaterialRule{1: {{ID: 1, Target: 3, Type: "required", Quantity: 1}}, 2: {{ID: 2, Target: 3, Type: "required", Quantity: 1}}, 3: {{ID: 3, Target: 4, Type: "required", Scope: "case", Quantity: 1}}}}
	result, err := ExpandMaterialWithCases(graph, []MaterialRoot{{ProductID: 1, Quantity: 1}, {ProductID: 2, Quantity: 1}}, map[int]int{3: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Items {
		if item.ProductID == 3 && (item.Quantity != 2 || len(item.Origins) != 2) {
			t.Fatal("lost shared origins", item)
		}
		if item.ProductID == 4 && item.Quantity != 1 {
			t.Fatal("same case counted twice", item)
		}
	}
}

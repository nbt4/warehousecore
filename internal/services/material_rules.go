package services

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
)

type MaterialProduct struct {
	Lifecycle string `json:"lifecycle_status,omitempty"`
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Unit      string `json:"unit"`
	Tracking  string `json:"tracking_mode"`
}
type MaterialRule struct {
	ID       int     `json:"id"`
	Source   int     `json:"source"`
	Target   int     `json:"target"`
	Type     string  `json:"type"`
	Scope    string  `json:"scope"`
	Quantity float64 `json:"quantity"`
}
type MaterialGraph struct {
	Products map[int]MaterialProduct `json:"products"`
	Rules    map[int][]MaterialRule  `json:"rules"`
}
type MaterialRoot struct {
	ProductID int     `json:"product_id"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	Cases     int     `json:"cases"`
}
type MaterialOrigin struct {
	ProductID int     `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  float64 `json:"quantity"`
	Type      string  `json:"relation_type"`
	Scope     string  `json:"assignment_scope"`
	Path      []int   `json:"path"`
}
type MaterialItem struct {
	ProductID int              `json:"product_id"`
	Name      string           `json:"name"`
	Quantity  float64          `json:"quantity"`
	Unit      string           `json:"unit"`
	Tracking  string           `json:"tracking_mode"`
	Origins   []MaterialOrigin `json:"origins"`
}
type MaterialPreview struct {
	Items    []MaterialItem `json:"items"`
	Warnings []string       `json:"warnings"`
}

type materialQuerier interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

func LoadMaterialGraph(db materialQuerier) (MaterialGraph, error) {
	graph := MaterialGraph{Products: map[int]MaterialProduct{}, Rules: map[int][]MaterialRule{}}
	rows, err := db.Query(`SELECT p.productid,p.name,COALESCE(NULLIF(ct.abbreviation,''),'Stk'),COALESCE(to_jsonb(p)->>'tracking_mode','individual'),p.lifecycle_status FROM products p LEFT JOIN count_types ct ON ct.count_type_id=p.count_type_id`)
	if err != nil {
		return graph, err
	}
	for rows.Next() {
		var product MaterialProduct
		if err = rows.Scan(&product.ID, &product.Name, &product.Unit, &product.Tracking, &product.Lifecycle); err != nil {
			rows.Close()
			return graph, err
		}
		graph.Products[product.ID] = product
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return graph, err
	}
	rows, err = db.Query(`SELECT pd.id,pd.product_id,pd.dependency_product_id,COALESCE(to_jsonb(pd)->>'relation_type',CASE WHEN COALESCE((to_jsonb(pd)->>'is_optional')::boolean,false) THEN 'recommended' ELSE 'required' END),COALESCE(to_jsonb(pd)->>'assignment_scope','product'),pd.default_quantity FROM product_dependencies pd WHERE pd.lifecycle_status='active' ORDER BY pd.id`)
	if err != nil {
		return graph, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule MaterialRule
		if err = rows.Scan(&rule.ID, &rule.Source, &rule.Target, &rule.Type, &rule.Scope, &rule.Quantity); err != nil {
			return graph, err
		}
		if source, ok := graph.Products[rule.Source]; ok && source.Lifecycle == "active" {
			if target, ok := graph.Products[rule.Target]; ok && target.Lifecycle == "active" {
				graph.Rules[rule.Source] = append(graph.Rules[rule.Source], rule)
			}
		}
	}
	return graph, rows.Err()
}

// Freeze retains the rules already captured for a job while adding newly booked
// roots. Only the reachable catalog subset is retained in the job snapshot.
func FreezeMaterialGraph(current MaterialGraph, roots []MaterialRoot, previous MaterialGraph) MaterialGraph {
	if previous.Products == nil {
		previous.Products = map[int]MaterialProduct{}
	}
	if previous.Rules == nil {
		previous.Rules = map[int][]MaterialRule{}
	}
	visited := map[int]bool{}
	var visit func(int, int)
	visit = func(id, depth int) {
		if visited[id] || depth > 5 {
			return
		}
		visited[id] = true
		if _, exists := previous.Products[id]; !exists {
			if product, ok := current.Products[id]; ok {
				previous.Products[id] = product
				previous.Rules[id] = append([]MaterialRule(nil), current.Rules[id]...)
			}
		}
		for _, rule := range previous.Rules[id] {
			if mandatoryMaterialRule(rule.Type) {
				visit(rule.Target, depth+1)
			}
		}
	}
	for _, root := range roots {
		visit(root.ProductID, 0)
	}
	return previous
}
func mandatoryMaterialRule(kind string) bool {
	return kind == "required" || kind == "consumes" || kind == "included"
}

func ExpandMaterial(graph MaterialGraph, roots []MaterialRoot) (MaterialPreview, error) {
	cases := map[int]int{}
	for _, root := range roots {
		cases[root.ProductID] = root.Cases
	}
	return ExpandMaterialWithCases(graph, roots, cases)
}
func ExpandMaterialWithCases(graph MaterialGraph, roots []MaterialRoot, caseCounts map[int]int) (MaterialPreview, error) {
	result := MaterialPreview{Items: []MaterialItem{}, Warnings: []string{}}
	items := map[int]*MaterialItem{}
	caseRules := map[[3]int]bool{}
	visits := 0
	var visit func(int, float64, int, MaterialOrigin, []int, int) error
	visit = func(id int, quantity float64, cases int, origin MaterialOrigin, path []int, depth int) error {
		visits++
		if visits > 10000 {
			return fmt.Errorf("Packregel ist zu komplex; maximal 10000 Materialpfade")
		}
		product, exists := graph.Products[id]
		if !exists {
			return nil
		}
		if product.Tracking == "none" {
			return nil
		}
		if math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity < 0 || quantity > 9999999999 {
			return fmt.Errorf("material quantity exceeds supported range")
		}
		if quantity == 0 {
			return nil
		}
		if product.Tracking == "individual" {
			quantity = math.Ceil(quantity)
		} else {
			quantity = math.Ceil(quantity*1000-0.0000001) / 1000
		}
		origin.Quantity = quantity
		origin.Path = append(append([]int(nil), path...), id)
		item := items[id]
		if item == nil {
			item = &MaterialItem{ProductID: id, Name: product.Name, Unit: product.Unit, Tracking: product.Tracking, Origins: []MaterialOrigin{}}
			items[id] = item
		}
		item.Quantity += quantity
		if item.Quantity > 9999999999 {
			return fmt.Errorf("aggregated material quantity exceeds supported range")
		}
		item.Origins = append(item.Origins, origin)
		for _, rule := range graph.Rules[id] {
			if !mandatoryMaterialRule(rule.Type) {
				continue
			}
			if depth >= 5 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Packregel für %s überschreitet die maximale Tiefe", product.Name))
				continue
			}
			cycle := false
			for _, ancestor := range origin.Path {
				if rule.Target == ancestor {
					cycle = true
				}
			}
			if cycle {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Kreis in der Packregel für %s", product.Name))
				continue
			}
			factor := quantity
			if rule.Scope == "case" {
				key := [3]int{id, rule.ID, rule.Target}
				if caseRules[key] {
					continue
				}
				caseRules[key] = true
				factor = float64(cases)
				if cases == 0 {
					result.Warnings = append(result.Warnings, fmt.Sprintf("%s benötigt eine Case-Zuordnung für seine Packregel", product.Name))
				}
			}
			next := MaterialOrigin{ProductID: id, Name: product.Name, Type: rule.Type, Scope: rule.Scope}
			if err := visit(rule.Target, factor*rule.Quantity, caseCounts[rule.Target], next, origin.Path, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := visit(root.ProductID, root.Quantity, root.Cases, MaterialOrigin{ProductID: root.ProductID, Name: graph.Products[root.ProductID].Name, Type: "booking", Scope: "product"}, nil, 0); err != nil {
			return result, err
		}
	}
	ids := make([]int, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return graph.Products[ids[i]].Name < graph.Products[ids[j]].Name })
	for _, id := range ids {
		item := items[id]
		item.Quantity = math.Round(item.Quantity*1000) / 1000
		result.Items = append(result.Items, *item)
	}
	return result, nil
}

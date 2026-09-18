package handlers

import (
	"strings"
	"testing"
)

func TestProductDependenciesBootstrapCreatesTableBeforeIndexes(t *testing.T) {
	if len(productDependenciesBootstrapStatements) != 3 {
		t.Fatalf("expected table plus two indexes, got %d statements", len(productDependenciesBootstrapStatements))
	}
	if !strings.Contains(productDependenciesBootstrapStatements[0], "CREATE TABLE IF NOT EXISTS product_dependencies") {
		t.Fatalf("first bootstrap statement must create product_dependencies: %s", productDependenciesBootstrapStatements[0])
	}
	for i, statement := range productDependenciesBootstrapStatements[1:] {
		if !strings.Contains(statement, "CREATE INDEX IF NOT EXISTS") {
			t.Errorf("bootstrap statement %d must create an idempotent index: %s", i+1, statement)
		}
	}
}

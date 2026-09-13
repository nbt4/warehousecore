package handlers

import "testing"

func TestNormalizeLifecycleFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "defaults to active", want: "active"},
		{name: "trims archived", input: " archived ", want: "archived"},
		{name: "accepts all", input: "all", want: "all"},
		{name: "rejects unknown state", input: "deleted", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeLifecycleFilter(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeLifecycleFilter() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("normalizeLifecycleFilter() = %q, want %q", got, test.want)
			}
		})
	}
}

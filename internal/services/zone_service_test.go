package services

import "testing"

func TestNextZoneSequence(t *testing.T) {
	tests := []struct {
		name   string
		codes  []string
		prefix string
		want   int
	}{
		{name: "first child", prefix: "WDL-RG-", want: 1},
		{name: "next child", codes: []string{"WDL-RG-01", "WDL-RG-02"}, prefix: "WDL-RG-", want: 3},
		{name: "keeps sequence after gaps", codes: []string{"WDL-RG-01", "WDL-RG-07"}, prefix: "WDL-RG-", want: 8},
		{name: "ignores other types", codes: []string{"WDL-F-12", "WDL-RG-03"}, prefix: "WDL-RG-", want: 4},
		{name: "ignores malformed suffix", codes: []string{"WDL-RG-XX", "WDL-RG-04-extra"}, prefix: "WDL-RG-", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextZoneSequence(tt.codes, tt.prefix); got != tt.want {
				t.Fatalf("nextZoneSequence() = %d, want %d", got, tt.want)
			}
		})
	}
}

package validation

import "testing"

func TestValidatePattern(t *testing.T) {
	valid := []string{"solid", "breathe", "blink"}
	invalid := []string{"pulse", "", "flash", "bre ath"}

	for _, v := range valid {
		if !ValidatePattern(v) {
			t.Fatalf("expected pattern %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if ValidatePattern(v) {
			t.Fatalf("expected pattern %q to be invalid", v)
		}
	}
}

func TestValidateColorHex(t *testing.T) {
	valid := []string{"#FF7A00", "#ff7a00", "#CCFF7A00"}
	invalid := []string{"FF7A00", "#FFF", "#GGGGGG", "#123456789", ""}

	for _, v := range valid {
		if !ValidateColorHex(v) {
			t.Fatalf("expected color %q to be valid", v)
		}
	}
	for _, v := range invalid {
		if ValidateColorHex(v) {
			t.Fatalf("expected color %q to be invalid", v)
		}
	}
}

func TestValidateIntensity(t *testing.T) {
	if !ValidateIntensity(0) {
		t.Fatal("0 should be valid")
	}
	if !ValidateIntensity(180) {
		t.Fatal("180 should be valid")
	}
	if !ValidateIntensity(255) {
		t.Fatal("255 should be valid")
	}
}

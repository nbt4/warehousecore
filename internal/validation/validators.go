package validation

import (
	"regexp"
)

var (
	hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// ValidatePattern ensures the LED pattern is one of the allowed values
func ValidatePattern(p string) bool {
	switch p {
	case "solid", "breathe", "blink":
		return true
	default:
		return false
	}
}

// ValidateColorHex checks if a color string is in hex format (#RRGGBB or #AARRGGBB)
func ValidateColorHex(c string) bool {
	return hexColorRe.MatchString(c)
}

// ValidateIntensity checks if intensity is between 0 and 255
func ValidateIntensity(i uint8) bool {
	return i <= 255
}

package util

import "testing"

func TestValidateSSLMode(t *testing.T) {
	// Valid modes
	validModes := []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}
	for _, mode := range validModes {
		if err := ValidateSSLMode(mode); err != nil {
			t.Errorf("ValidateSSLMode(%q) returned error: %v", mode, err)
		}
	}

	// Invalid modes
	invalidModes := []string{"", "reqiure", "DISABLE", "ssl", "none"}
	for _, mode := range invalidModes {
		if err := ValidateSSLMode(mode); err == nil {
			t.Errorf("ValidateSSLMode(%q) should have returned error", mode)
		}
	}
}

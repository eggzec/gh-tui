package core

import "testing"

func TestEnterpriseSupported(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{MinEnterprise, true},
		{MinEnterprise + ".2", true},
		{"3.15.9", false},
		{"3.12.0", false},
		{"3.22.1", true},
		{"3.100.0", true},
		{"4.0.0", true},
		{"2.22.30", false},
		{"", true},
		{"nightly", true},
	}
	for _, tt := range tests {
		if got := EnterpriseSupported(tt.version); got != tt.want {
			t.Errorf("EnterpriseSupported(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

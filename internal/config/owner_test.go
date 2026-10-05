package config

import (
	"strings"
	"testing"
)

func TestOwnerDefaultTab(t *testing.T) {
	if got := Default().Owner.DefaultTab; got != OwnerTabRepositories {
		t.Errorf("default_tab defaults to %q, want %q", got, OwnerTabRepositories)
	}
	tests := []struct {
		value string
		ok    bool
	}{
		{OwnerTabRepositories, true},
		{OwnerTabReadme, true},
		{OwnerTabPeople, true},
		{"", false},
		{"stars", false},
		{"Readme", false},
	}
	for _, tt := range tests {
		cfg := Default()
		cfg.Owner.DefaultTab = tt.value
		err := cfg.Validate()
		if (err == nil) != tt.ok {
			t.Errorf("default_tab %q: Validate() = %v, want ok = %v", tt.value, err, tt.ok)
		}
		if err != nil && !strings.Contains(err.Error(), "owner.default_tab") {
			t.Errorf("default_tab %q: Validate() = %v, want it to name the field", tt.value, err)
		}
	}
}

package config

import (
	"fmt"
	"slices"
)

// Owner configures the page of a user or an organization.
type Owner struct {
	// DefaultTab is the tab the page opens on: OwnerTabRepositories,
	// OwnerTabReadme or OwnerTabPeople.
	DefaultTab string `yaml:"default_tab"`
}

// The tabs the page of a user or an organization may open on.
const (
	OwnerTabRepositories = "repositories"
	OwnerTabReadme       = "readme"
	OwnerTabPeople       = "people"
)

func (o Owner) validate() error {
	if !slices.Contains([]string{OwnerTabRepositories, OwnerTabReadme, OwnerTabPeople}, o.DefaultTab) {
		return fmt.Errorf("owner.default_tab: must be %s, %s or %s, got %q", OwnerTabRepositories, OwnerTabReadme, OwnerTabPeople, o.DefaultTab)
	}
	return nil
}

package filterform

import "testing"

func sortSpec(def Sort, opts ...SortOption) Spec {
	return Spec{
		Fields: []Field{{Key: "state", Label: "State", Kind: Choice, Qualifier: "is"}},
		Sort:   &SortField{Options: opts, Default: def},
	}
}

func TestSpecSortOf(t *testing.T) {
	t.Parallel()
	updated := SortOption{Label: "Updated", Value: "updated"}
	created := SortOption{Label: "Created", Value: "created"}
	best := SortOption{Label: "Best match"}
	tests := []struct {
		name string
		spec Spec
		q    string
		want Sort
	}{
		{"token", sortSpec(Sort{By: "updated", Desc: true}, updated, created), "is:open sort:created-asc", Sort{By: "created"}},
		{"no direction is descending", sortSpec(Sort{By: "updated", Desc: true}, updated, created), "sort:created", Sort{By: "created", Desc: true}},
		{"none is the default", sortSpec(Sort{By: "updated", Desc: true}, updated, created), "is:open", Sort{By: "updated", Desc: true}},
		{"none with best match is none", sortSpec(Sort{Desc: true}, best, updated), "words", Sort{Desc: true}},
		{"unknown token is no sort", sortSpec(Sort{By: "updated", Desc: true}, updated), "sort:stars-asc", Sort{By: "updated", Desc: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.spec.SortOf(tt.q); got != tt.want {
				t.Errorf("SortOf(%q) = %+v, want %+v", tt.q, got, tt.want)
			}
		})
	}
}

func TestSpecWithSort(t *testing.T) {
	t.Parallel()
	spec := sortSpec(Sort{By: "updated", Desc: true}, SortOption{Label: "Updated", Value: "updated"}, SortOption{Label: "Created", Value: "created"})
	tests := []struct {
		name string
		q    string
		so   Sort
		want string
	}{
		{"adds", "is:open label:\"a b\" fix", Sort{By: "created", Desc: true}, "is:open label:\"a b\" fix sort:created-desc"},
		{"replaces", "sort:updated-desc is:open", Sort{By: "created"}, "is:open sort:created-asc"},
		{"none drops it", "is:open sort:created-asc", Sort{}, "is:open"},
		{"keeps a sort it doesn't know", "sort:stars-asc is:open", Sort{By: "created", Desc: true}, "sort:stars-asc is:open sort:created-desc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := spec.WithSort(tt.q, tt.so); got != tt.want {
				t.Errorf("WithSort(%q) = %q, want %q", tt.q, got, tt.want)
			}
		})
	}
	if got := (Spec{}).WithSort("a sort:x-asc", Sort{By: "x"}); got != "a sort:x-asc" {
		t.Errorf("a spec without a sort changed the query: %q", got)
	}
}

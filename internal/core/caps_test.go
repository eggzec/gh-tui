package core

import (
	"slices"
	"testing"
)

func TestPermissionAtLeast(t *testing.T) {
	tests := []struct {
		p, q Permission
		want bool
	}{
		{PermissionAdmin, PermissionWrite, true},
		{PermissionWrite, PermissionWrite, true},
		{PermissionMaintain, PermissionTriage, true},
		{PermissionTriage, PermissionWrite, false},
		{PermissionRead, PermissionTriage, false},
		{PermissionRead, PermissionRead, true},
		{"", PermissionRead, false},
	}
	for _, tt := range tests {
		if got := tt.p.AtLeast(tt.q); got != tt.want {
			t.Errorf("%q.AtLeast(%q) = %v, want %v", tt.p, tt.q, got, tt.want)
		}
	}
}

func TestRepoCaps(t *testing.T) {
	known := func(p Permission) RepoCaps { return RepoCaps{Known: true, Permission: p} }
	archived := known(PermissionAdmin)
	archived.Archived = true
	locked := known(PermissionWrite)
	locked.Locked = true
	tests := []struct {
		name                          string
		caps                          RepoCaps
		readOnly, canTriage, canWrite bool
	}{
		{name: "unknown", caps: RepoCaps{}, canTriage: true, canWrite: true},
		{name: "read", caps: known(PermissionRead), readOnly: true},
		{name: "triage", caps: known(PermissionTriage), canTriage: true},
		{name: "write", caps: known(PermissionWrite), canTriage: true, canWrite: true},
		{name: "admin", caps: known(PermissionAdmin), canTriage: true, canWrite: true},
		{name: "unknown permission", caps: known(""), canTriage: true, canWrite: true},
		{name: "archived", caps: archived, readOnly: true},
		{name: "locked", caps: locked, readOnly: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.caps
			if got := c.ReadOnly(); got != tt.readOnly {
				t.Errorf("ReadOnly = %v, want %v", got, tt.readOnly)
			}
			if got := c.CanTriage(); got != tt.canTriage {
				t.Errorf("CanTriage = %v, want %v", got, tt.canTriage)
			}
			if got := c.CanWrite(); got != tt.canWrite {
				t.Errorf("CanWrite = %v, want %v", got, tt.canWrite)
			}
		})
	}
}

func TestMergeMethod(t *testing.T) {
	rebaseOnly := RepoCaps{Known: true, Rebase: true, DefaultMerge: MergeRebase}
	tests := []struct {
		name      string
		caps      RepoCaps
		preferred MergeMethod
		want      MergeMethod
		methods   []MergeMethod
		ok        bool
	}{
		{
			name: "unknown caps keep the preferred method", caps: RepoCaps{}, preferred: MergeSquash,
			want: MergeSquash, methods: []MergeMethod{MergeCommit, MergeSquash, MergeRebase}, ok: true,
		},
		{
			name: "an allowed preferred method stays", preferred: MergeSquash,
			caps: RepoCaps{Known: true, MergeCommit: true, Squash: true, DefaultMerge: MergeCommit},
			want: MergeSquash, methods: []MergeMethod{MergeCommit, MergeSquash}, ok: true,
		},
		{
			name: "a refused preferred method falls back to the viewer's default", preferred: MergeSquash,
			caps: rebaseOnly, want: MergeRebase, methods: []MergeMethod{MergeRebase}, ok: true,
		},
		{
			name: "a refused default falls back to the first allowed", preferred: MergeSquash,
			caps: RepoCaps{Known: true, MergeCommit: true, Rebase: true, DefaultMerge: MergeSquash},
			want: MergeCommit, methods: []MergeMethod{MergeCommit, MergeRebase}, ok: true,
		},
		{name: "none allowed", preferred: MergeSquash, caps: RepoCaps{Known: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.caps.MergeMethod(tt.preferred)
			if got != tt.want || ok != tt.ok {
				t.Errorf("MergeMethod(%q) = %q, %v; want %q, %v", tt.preferred, got, ok, tt.want, tt.ok)
			}
			if m := tt.caps.MergeMethods(); !slices.Equal(m, tt.methods) {
				t.Errorf("MergeMethods = %v, want %v", m, tt.methods)
			}
		})
	}
}

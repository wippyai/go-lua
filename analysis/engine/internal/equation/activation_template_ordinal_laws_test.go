package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// ordinalLawRoles is the declared role vector under test. Every role is
// control-only, so the formal normalization admits exactly one boundary point
// per role and nothing else competes for a target ordinal.
func ordinalLawRoles() []TemplateRole {
	return []TemplateRole{
		{Role: boundaryKey(41), Mode: PortExport},
		{Role: boundaryKey(42), Mode: PortExport},
		{Role: boundaryKey(43), Mode: PortExport},
		{Role: boundaryKey(44), Mode: PortExport},
		{Role: boundaryKey(45), Mode: PortExport},
		{Role: boundaryKey(46), Mode: PortExport},
	}
}

// publishedRoleOrdinals reads the sealed target point catalog and returns the
// published point key in ordinal order plus the key each declared role owns.
func publishedRoleOrdinals(t *testing.T, roles []TemplateRole) (published []composition.Key, declared []composition.Key) {
	t.Helper()
	_, batch, ports, ok := normalizeTemplateFormal(Template{Roles: roles})
	if !ok {
		t.Fatal("normalize template formal")
	}
	points, _, _, _, _, rowsOK := batch.TargetRows()
	if !rowsOK {
		t.Fatal("target rows")
	}
	if len(points) != len(roles) {
		t.Fatalf("point count %d, roles %d", len(points), len(roles))
	}
	published = make([]composition.Key, len(points))
	for index, point := range points {
		key := point.Site.Key()
		if !key.Available() {
			t.Fatalf("point %d key", index)
		}
		published[index] = key
	}
	declared = make([]composition.Key, len(roles))
	for index, role := range roles {
		port, present := ports[role.Role]
		if !present {
			t.Fatalf("role %d port", index)
		}
		key := port.Site().Key()
		if !key.Available() {
			t.Fatalf("role %d key", index)
		}
		declared[index] = key
	}
	return published, declared
}

// TestFormalPointOrdinalsFollowDeclaredRoleOrder proves the published boundary
// point ordinals are a total function of the declared role order. Repeated
// in-process normalization of the same template exercises Go's per-range map
// order randomization, so an ordinal derived from a map walk diverges here.
func TestFormalPointOrdinalsFollowDeclaredRoleOrder(t *testing.T) {
	roles := ordinalLawRoles()
	for repeat := 0; repeat < 64; repeat++ {
		published, declared := publishedRoleOrdinals(t, roles)
		for index := range declared {
			if published[index] != declared[index] {
				t.Fatalf("repeat %d ordinal %d: published %v, declared %v", repeat, index, published[index], declared[index])
			}
		}
	}
}

// TestFormalPointOrdinalsStableAcrossBuilds proves the full published ordinal
// vector is byte-identical across repeated builds of one authored template.
func TestFormalPointOrdinalsStableAcrossBuilds(t *testing.T) {
	roles := ordinalLawRoles()
	expected, _ := publishedRoleOrdinals(t, roles)
	for repeat := 1; repeat < 64; repeat++ {
		published, _ := publishedRoleOrdinals(t, roles)
		for index := range expected {
			if published[index] != expected[index] {
				t.Fatalf("repeat %d ordinal %d: %v, first build %v", repeat, index, published[index], expected[index])
			}
		}
	}
}

// TestFormalPointOrdinalsRejectUndeclaredRole proves the normalization still
// rejects a template whose role vector cannot be admitted as a formal port.
func TestFormalPointOrdinalsRejectUndeclaredRole(t *testing.T) {
	roles := append(ordinalLawRoles(), TemplateRole{Role: composition.Key{}, Mode: PortExport})
	if _, _, _, ok := normalizeTemplateFormal(Template{Roles: roles}); ok {
		t.Fatal("unavailable role admitted")
	}
}

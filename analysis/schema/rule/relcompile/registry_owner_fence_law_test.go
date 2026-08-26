package relcompile

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// ownerFenceRelation installs one ordinary addressed relation under one axis
// owner. The relation, column, and key intentionally have distinct member
// names: that is a legitimate same-owner binding the fences below must retain.
func ownerFenceRelation(t *testing.T, registry *Registry, axis schema.EntryReference, scope Name, stem string) (Name, Name, Name) {
	t.Helper()
	typeName := NewName(axis, schema.Key(stem+"/type"))
	if err := registry.InstallType(typeName, issue(t, stem+"/type")); err != nil {
		t.Fatalf("install type: %v", err)
	}
	relation := NewName(axis, schema.Key(stem+"/relation"))
	if err := registry.InstallRelation(relation, issue(t, stem+"/relation"), scope); err != nil {
		t.Fatalf("install relation: %v", err)
	}
	column := NewName(axis, schema.Key(stem+"/column"))
	if err := registry.InstallColumn(column, issue(t, stem+"/column"), relation, typeName); err != nil {
		t.Fatalf("install column: %v", err)
	}
	key := NewName(axis, schema.Key(stem+"/key"))
	if err := registry.InstallKey(key, issue(t, stem+"/key"), relation, column); err != nil {
		t.Fatalf("install key: %v", err)
	}
	return relation, column, key
}

func ownerFenceAxis(t *testing.T, registry *Registry, key schema.Key) (schema.EntryReference, Name) {
	t.Helper()
	axis := axisEntry(key)
	if err := registry.InstallOwner(axis, issue(t, "owner-fence/owner/"+string(key))); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	// The axis owns its decision scope too.  The member name may differ from
	// the axis key; ownership, not spelling equality, is what matters here.
	scope := NewName(axis, schema.Key("owner-fence/scope/"+string(key)))
	if err := registry.InstallScope(scope, issue(t, "owner-fence/scope/"+string(key)), region.True()); err != nil {
		t.Fatalf("install scope: %v", err)
	}
	return axis, scope
}

// TestInstallFactorRejectsAForeignAxisBinding closes the owner fence around a
// Factor. A denominator may be named by any surface, but the axis asking for
// a Factor must own both the relation and key it exposes under it.
func TestInstallFactorRejectsAForeignAxisBinding(t *testing.T) {
	registry := NewRegistry()
	heap, heapScope := ownerFenceAxis(t, registry, "heap")
	value, _ := ownerFenceAxis(t, registry, "value")
	heapRelation, _, heapKey := ownerFenceRelation(t, registry, heap, heapScope, "heap/factor")
	denominator := EntryName(schema.SurfaceKindDenominator, "owner-fence/denominator")
	if err := registry.InstallDenominator(denominator, heapRelation, heapKey); err != nil {
		t.Fatalf("install denominator: %v", err)
	}
	if err := registry.InstallFactor(value, denominator, heapRelation, heapKey); err == nil {
		t.Fatal("foreign axis installed a Factor relation/key")
	} else if refusal := refusalOf(t, err); refusal.Reason != ReasonForeign {
		t.Fatalf("foreign factor refusal = %+v, want foreign", refusal)
	}
}

// TestInstallFactorKeepsSameOwnerDifferentMembersLegal proves the fence is
// ownership, not accidental member-name equality.
func TestInstallFactorKeepsSameOwnerDifferentMembersLegal(t *testing.T) {
	registry := NewRegistry()
	value, scope := ownerFenceAxis(t, registry, "value")
	relation, _, key := ownerFenceRelation(t, registry, value, scope, "value/factor")
	denominator := EntryName(schema.SurfaceKindDenominator, "owner-fence/value-denominator")
	if err := registry.InstallDenominator(denominator, relation, key); err != nil {
		t.Fatalf("install denominator: %v", err)
	}
	if err := registry.InstallFactor(value, denominator, relation, key); err != nil {
		t.Fatalf("same-owner Factor relation/key was refused: %v", err)
	}
}

// TestInstallOutputRejectsAForeignWriterBinding closes the corresponding
// output fence: the OutputRef's entry, writer relation, writer fact column,
// and publication key must all be issued by one owner.
func TestInstallOutputRejectsAForeignWriterBinding(t *testing.T) {
	registry := NewRegistry()
	heap, _ := ownerFenceAxis(t, registry, "heap")
	value, valueScope := ownerFenceAxis(t, registry, "value")
	writer, column, key := ownerFenceRelation(t, registry, value, valueScope, "value/writer")
	output := NewName(heap, "heap/facts")
	if err := registry.InstallOutput(output, writer, column, key, carrier.Key("owner-fence/result")); err == nil {
		t.Fatal("foreign writer relation/column/key installed for output")
	} else if refusal := refusalOf(t, err); refusal.Reason != ReasonForeign {
		t.Fatalf("foreign output refusal = %+v, want foreign", refusal)
	}
}

// TestInstallOutputKeepsSameOwnerDifferentMembersLegal proves an OutputRef
// may write a distinct relation/member of its own axis. Only the owner fence
// is required; no member-name equality is inferred.
func TestInstallOutputKeepsSameOwnerDifferentMembersLegal(t *testing.T) {
	registry := NewRegistry()
	heap, scope := ownerFenceAxis(t, registry, "heap")
	writer, column, key := ownerFenceRelation(t, registry, heap, scope, "heap/writer")
	output := NewName(heap, "heap/facts")
	if err := registry.InstallOutput(output, writer, column, key, carrier.Key("owner-fence/result")); err != nil {
		t.Fatalf("same-owner writer relation/column/key was refused: %v", err)
	}
}

package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestTypedFingerprintCollisionsRemainStructurallyAuthoritative(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	arena.fingerprintMask = 0 // every node shares one bucket

	root0 := Root{Kind: RootParam, Index: 0}
	root1 := Root{Kind: RootParam, Index: 1}
	v0, v1 := arena.Root(root0), arena.Root(root1)
	if v0 == v1 || arena.Root(root0) != v0 {
		t.Fatalf("colliding value roots aliased or failed to intern: %d/%d", v0, v1)
	}
	c0 := arena.Constant(typevalue.LiteralString(reg, "left"))
	c1 := arena.Constant(typevalue.LiteralString(reg, "right"))
	if c0 == c1 || arena.Constant(typevalue.LiteralString(reg, "left")) != c0 {
		t.Fatalf("colliding constants aliased or failed to intern: %d/%d", c0, c1)
	}
	if j0, j1 := arena.JoinValue(v0, c0), arena.JoinValue(v1, c1); j0 == j1 || arena.JoinValue(c0, v0) != j0 {
		t.Fatalf("colliding value DAGs aliased or failed to intern: %d/%d", j0, j1)
	}

	p0 := arena.Path(root0, segment.Segment{Kind: segment.SegmentField, Name: "left"})
	p1 := arena.Path(root1, segment.Segment{Kind: segment.SegmentField, Name: "right"})
	if p0 == p1 || arena.Path(root0, segment.Segment{Kind: segment.SegmentField, Name: "left"}) != p0 {
		t.Fatalf("colliding paths aliased or failed to intern: %d/%d", p0, p1)
	}
	g0, g1 := arena.Truthy(v0), arena.Falsy(v0)
	if g0 == g1 || arena.Truthy(v0) != g0 {
		t.Fatalf("colliding guards aliased or failed to intern: %d/%d", g0, g1)
	}

	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create allocation template rejected")
	}
	op0, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{Owner: 41, Template: template.Root, Ordinal: 7}, template)
	op1, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{Owner: 41, Template: template.Root, Ordinal: 8}, template)
	a0, a1 := arena.AllocationTemplate(op0), arena.AllocationTemplate(op1)
	if a0 == a1 || arena.AllocationTemplate(op0) != a0 {
		t.Fatalf("colliding allocations aliased or failed to intern: %d/%d", a0, a1)
	}

	effects := NewEffectArena(arena)
	inv0 := InvalidatePathConfig{Target: PathEffectTarget(p0), Scope: InvalidationScopeDescendants}
	inv1 := InvalidatePathConfig{Target: PathEffectTarget(p1), Scope: InvalidationScopeDescendants}
	e0, err := effects.InvalidatePath(inv0)
	if err != nil {
		t.Fatal(err)
	}
	e1, err := effects.InvalidatePath(inv1)
	if err != nil {
		t.Fatal(err)
	}
	if e0 == e1 {
		t.Fatalf("colliding invalidation effects aliased: %d/%d", e0, e1)
	}
	mutation := IndexMutationConfig{
		Invalidation: inv0, Table: PathEffectTarget(p0), Key: c0, Value: c1,
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 41, Ordinal: 9},
	}
	m0, err := effects.IndexMutation(mutation)
	if err != nil {
		t.Fatal(err)
	}
	m1, err := effects.IndexMutation(mutation)
	if err != nil || m0 != m1 || m0 == e0 || m0 == e1 {
		t.Fatalf("colliding mutation effect identity = %d/%d invalidations=%d/%d err=%v", m0, m1, e0, e1, err)
	}
}

func TestTypedFingerprintPreservesCanonicalRelationSpelling(t *testing.T) {
	makeRowKey := func(mask uint64) string {
		reg := standard.Registry()
		arena := NewArena(reg)
		arena.fingerprintMask = mask
		effects := NewEffectArena(arena)
		root := Root{Kind: RootParam, Index: 0}
		value := arena.Root(root)
		constant := arena.Constant(typevalue.LiteralString(reg, "value"))
		path := arena.Path(root, segment.Segment{Kind: segment.SegmentField, Name: "member"})
		effect, err := effects.InvalidatePath(InvalidatePathConfig{Target: PathEffectTarget(path), Scope: InvalidationScopeDescendants})
		if err != nil {
			t.Fatal(err)
		}
		return rowKey(arena, effects, Row{
			Guard:   arena.And(arena.Truthy(value), arena.Falsy(constant)),
			Ops:     []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Slot: 0, Value: arena.JoinValue(value, constant)}},
			Effects: []EffectTerm{effect},
		})
	}
	regular := makeRowKey(^uint64(0))
	forcedCollision := makeRowKey(0)
	if forcedCollision != regular {
		t.Fatalf("canonical relation spelling changed under collisions:\nregular: %s\ncollision: %s", regular, forcedCollision)
	}
}

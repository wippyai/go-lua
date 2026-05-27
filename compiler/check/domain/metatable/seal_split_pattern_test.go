package metatable

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestSealClassFamily_SplitPatternPreservesPrototypeMethods exercises the
// split-pattern OOP class shape where the metatable's __index points to a
// SEPARATE prototype allocation that carries the method surface. The
// recursive cycle runs through the metatable (class_mt.__index = Class;
// Class.method.self.metatable = class_mt), not through Class directly. The
// seal must not fold __index to the recursion variable in this pattern: doing
// so would erase Class's method surface, leaving cross-procedural callers
// with a metatable that resolves __index to the instance and reports "no
// method" for legitimate prototype methods.
//
// This is the structural condition behind
// TestExternalLint_LocalMetatableInstanceKeepsLaterMethods (no method
// is_empty / no method has_cycles).
func TestSealClassFamily_SplitPatternPreservesPrototypeMethods(t *testing.T) {
	// Build the class_mt instance whose metatable is the prototype table:
	// instance = setmetatable({nodes={}}, class_mt) where class_mt =
	// {__index = Class} and Class has methods whose self type is the
	// instance shape with metatable = class_mt.
	classMt := typ.NewRecord()
	instance := typ.NewRecord().Field("nodes", typ.NewRecord().Build())

	// Class with method whose self has metatable = class_mt (cycle).
	instanceSelf := typ.NewRecord().Field("nodes", typ.NewRecord().Build())
	classRec := typ.NewRecord().
		Field("is_empty", typ.Func().Param("self", instanceSelf.Build()).Returns(typ.Boolean).Build()).
		Field("has_cycles", typ.Func().Param("self", instanceSelf.Build()).Returns(typ.Boolean, typ.Nil).Build()).
		Build()

	// class_mt = {__index = Class}
	classMtBuilt := classMt.Field("__index", classRec).Build()

	// instance = {nodes={}} with metatable = class_mt
	instanceBuilt := instance.Metatable(classMtBuilt).Build()

	sealed := SealClassInstanceReturn(instanceBuilt)
	if sealed == nil {
		t.Fatal("seal must not return nil")
	}

	// The sealed instance must preserve the metatable's __index target with
	// its method surface intact. Folding __index to a recursion variable
	// (the old over-firing seal behavior) would produce {__index: rec} with
	// no method evidence and break cross-procedural method resolution.
	sealedRec, ok := sealed.(*typ.Record)
	if !ok {
		// Could be a Recursive node; pick its body.
		rec, ok := sealed.(*typ.Recursive)
		if !ok {
			t.Fatalf("sealed must be a record or recursive, got %T: %s", sealed, sealed)
		}
		if rec.Body == nil {
			t.Fatal("sealed family body must be set")
		}
		bodyRec, ok := rec.Body.(*typ.Record)
		if !ok {
			t.Fatalf("sealed family body must be a record, got %T", rec.Body)
		}
		sealedRec = bodyRec
	}
	if sealedRec.Metatable == nil {
		t.Fatal("sealed instance must carry the class metatable")
	}
	mt, ok := sealedRec.Metatable.(*typ.Record)
	if !ok {
		// May be a recursive node wrapping the metatable; check its body.
		recMt, ok := sealedRec.Metatable.(*typ.Recursive)
		if !ok {
			t.Fatalf("metatable must be a record or recursive, got %T: %s", sealedRec.Metatable, sealedRec.Metatable)
		}
		if recMt.Body == nil {
			t.Fatal("recursive metatable body must be set")
		}
		bodyMt, ok := recMt.Body.(*typ.Record)
		if !ok {
			t.Fatalf("recursive metatable body must be a record, got %T", recMt.Body)
		}
		mt = bodyMt
	}

	// The metatable must keep an __index field pointing at the prototype
	// methods, not be collapsed to a back-edge reference.
	idx := fieldByName(mt, "__index")
	if idx == nil {
		t.Fatal("sealed metatable must keep __index field")
	}
	idxRec, ok := idx.Type.(*typ.Record)
	if !ok {
		// Could be a Recursive that wraps the prototype methods; that is
		// also acceptable as long as the methods survive.
		recIdx, ok := idx.Type.(*typ.Recursive)
		if !ok {
			t.Fatalf("__index target must be a record or recursive, got %T: %s", idx.Type, idx.Type)
		}
		bodyIdx, ok := recIdx.Body.(*typ.Record)
		if !ok {
			t.Fatalf("recursive __index body must be a record, got %T", recIdx.Body)
		}
		idxRec = bodyIdx
	}

	if fieldByName(idxRec, "is_empty") == nil {
		t.Fatalf("sealed metatable __index must preserve is_empty method, got: %s", typ.FormatShort(idx.Type))
	}
	if fieldByName(idxRec, "has_cycles") == nil {
		t.Fatalf("sealed metatable __index must preserve has_cycles method, got: %s", typ.FormatShort(idx.Type))
	}
}

// TestSealClassFamily_CanonicalizationStableAcrossBuilds verifies that two
// independently-built class allocations carrying the same split-pattern shape
// canonicalize to one representative across fixpoint iterations. Convergence
// detects a fixed point only when every observation of one class collapses to
// a single canonical form regardless of how many times it is rebuilt.
func TestSealClassFamily_CanonicalizationStableAcrossBuilds(t *testing.T) {
	build := func() typ.Type {
		instanceSelf := typ.NewRecord().Field("nodes", typ.NewRecord().Build()).Build()
		classRec := typ.NewRecord().
			Field("is_empty", typ.Func().Param("self", instanceSelf).Returns(typ.Boolean).Build()).
			Build()
		classMt := typ.NewRecord().Field("__index", classRec).Build()
		return typ.NewRecord().Field("nodes", typ.NewRecord().Build()).Metatable(classMt).Build()
	}

	a := SealClassInstanceReturn(build())
	b := SealClassInstanceReturn(build())
	// Two independent builds must seal to structurally-equal types so the
	// inter-procedural fixpoint reaches a fixed point on the constructor
	// return summary.
	if !typ.TypeEquals(a, b) {
		t.Fatalf("same-structure split-pattern seal must canonicalize to one representative:\n a=%s\n b=%s",
			typ.FormatShort(a), typ.FormatShort(b))
	}
}

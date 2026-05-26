package metatable

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// buildCyclicClass builds a class record shaped like a setmetatable OOP class:
// __index holds the prototype, new returns an instance whose metatable is the
// prototype, and run takes a self whose metatable is the prototype. Each
// back-edge is a finite, acyclic copy, mirroring how the inter-procedural
// fixpoint observes the class one unfolding at a time.
func buildCyclicClass() *typ.Record {
	proto := typ.NewRecord().
		Field("new", typ.Func().Returns(typ.NewRecord().SetOpen(true).Build()).Build()).
		Field("run", typ.Func().Param("self", typ.NewRecord().SetOpen(true).Build()).Returns(typ.Nil).Build()).
		Field("pending_ops", typ.Number).
		Field("stopping", typ.Boolean).
		Build()
	selfInstance := typ.NewRecord().SetOpen(true).Metatable(proto).Build()
	newInstance := typ.NewRecord().SetOpen(true).Field("pending_ops", typ.Number).Metatable(proto).Build()
	return typ.NewRecord().
		Field(indexField, proto).
		Field("new", typ.Func().Returns(newInstance).Build()).
		Field("run", typ.Func().Param("self", selfInstance).Returns(typ.Nil).Build()).
		Field("pending_ops", typ.Number).
		Field("stopping", typ.Boolean).
		Build()
}

func TestSealClassFamily_TiesBackEdgesToOneRecursive(t *testing.T) {
	class := buildCyclicClass()
	sealed := SealClassFamily(class, "g:1")
	rec, ok := sealed.(*typ.Recursive)
	if !ok {
		t.Fatalf("seal must produce a recursive family, got %T: %s", sealed, typ.FormatShort(sealed))
	}
	if rec.Body == nil {
		t.Fatal("sealed family body must be set")
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("sealed body must be the class record, got %T", rec.Body)
	}

	// The __index back-edge must be the recursion variable, not a nested record.
	idxField := fieldByName(body, indexField)
	if idxField == nil {
		t.Fatal("sealed body must keep __index field")
	}
	if !typ.IsRecursiveRef(idxField.Type, rec) {
		t.Fatalf("__index must bind to the recursion variable, got %s", typ.FormatShort(idxField.Type))
	}

	// The run method self metatable must bind to the recursion variable, so the
	// self-param no longer carries a fresh class copy each iteration.
	runField := fieldByName(body, "run")
	if runField == nil {
		t.Fatal("sealed body must keep run field")
	}
	runFn, ok := runField.Type.(*typ.Function)
	if !ok || len(runFn.Params) == 0 {
		t.Fatalf("run must stay a function with params, got %s", typ.FormatShort(runField.Type))
	}
	selfRec, ok := runFn.Params[0].Type.(*typ.Record)
	if !ok || selfRec.Metatable == nil {
		t.Fatalf("run self must keep its metatable, got %s", typ.FormatShort(runFn.Params[0].Type))
	}
	if !typ.IsRecursiveRef(selfRec.Metatable, rec) {
		t.Fatalf("run self metatable must bind to the recursion variable, got %s", typ.FormatShort(selfRec.Metatable))
	}
}

func TestSealClassFamily_SameStructureCanonicalizesAcrossBuilds(t *testing.T) {
	a := SealClassFamily(buildCyclicClass(), "g:1")
	b := SealClassFamily(buildCyclicClass(), "g:1")
	if _, ok := a.(*typ.Recursive); !ok {
		t.Fatalf("seal A must produce recursive family, got %T", a)
	}
	if _, ok := b.(*typ.Recursive); !ok {
		t.Fatalf("seal B must produce recursive family, got %T", b)
	}
	// Two builds of the same class produce structurally equal sealed families so
	// the value-domain canonicalizer maps them to one representative and the
	// fixpoint detects a fixed point.
	if !typ.TypeEquals(a, b) {
		t.Fatalf("same-structure seals must be Equal:\n a=%s\n b=%s", typ.FormatShort(a), typ.FormatShort(b))
	}
}

func TestSealClassFamily_NoBackEdgeReturnsUnchanged(t *testing.T) {
	plain := typ.NewRecord().Field("x", typ.Number).Field("y", typ.String).Build()
	got := SealClassFamily(plain, "g:1")
	if !typ.SameNode(got, plain) {
		t.Fatalf("a plain record with no class back-edge must be returned unchanged, got %s", typ.FormatShort(got))
	}
}

func fieldByName(rec *typ.Record, name string) *typ.Field {
	for i := range rec.Fields {
		if rec.Fields[i].Name == name {
			return &rec.Fields[i]
		}
	}
	return nil
}

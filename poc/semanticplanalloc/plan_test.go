package semanticplanalloc

import (
	"reflect"
	"sort"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

func TestPackedTransformerDeterministicAndFailClosed(t *testing.T) {
	target := pathdom.NewPath(1, "target").Field("field")
	source := pathdom.NewPath(2, "source").Field("nested")
	r := DefaultRegistry()
	a, ok := r.CompilePathAssignment(target, source)
	if !ok {
		t.Fatal("default registry unexpectedly contextual")
	}
	b, ok := r.CompilePathAssignment(target, source)
	if !ok || !reflect.DeepEqual(a, b) {
		t.Fatal("compilation is not deterministic")
	}

	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	specs := make([]LaneSpec, 0, len(lanes)-1)
	for _, lane := range lanes[1:] {
		specs = append(specs, LaneSpec{Lane: lane, Covered: true})
	}
	incomplete, err := NewRegistry(lanes, specs)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := incomplete.CompilePathAssignment(target, source); ok {
		t.Fatal("missing lane adapter did not fail closed")
	}
}

func TestBindReturnsBorrowedRebasedViews(t *testing.T) {
	transformer, ok := DefaultRegistry().CompilePathAssignment(
		pathdom.NewPath(1, "target").Field("field"),
		pathdom.NewPath(2, "source").Field("nested"),
	)
	if !ok {
		t.Fatal("compile failed")
	}
	bindings := Bindings{
		Roots:  []pathdom.Path{pathdom.NewPath(10, "caller-target"), pathdom.NewPath(11, "caller-source")},
		Values: make([]product.Value, termCount), ValueMask: uint64(1) << sourceTerm,
	}
	cursor, ok := transformer.Bind(&bindings)
	if !ok {
		t.Fatal("bind failed")
	}
	var phases []uint8
	for {
		effect, more := cursor.Next()
		if !more {
			break
		}
		phases = append(phases, effect.Phase)
		if got := effect.Target.Base.Symbol; got != 10 {
			t.Fatalf("target root = %d", got)
		}
		if got := effect.Target.Suffix[0].Name; got != "field" {
			t.Fatalf("target suffix = %q", got)
		}
	}
	if !sort.SliceIsSorted(phases, func(i, j int) bool { return phases[i] < phases[j] }) {
		t.Fatalf("phases not sorted: %v", phases)
	}
}

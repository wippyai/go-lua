package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEffectPathStoreRetainsIndependentAssignmentAndStaticWrites(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 4}
	terms := NewArena(reg)
	effects := NewEffectArena(terms)
	paths := make([]PathTerm, 4)
	values := make([]ValueTerm, 4)
	for index := range paths {
		root := Root{Kind: RootParam, Index: uint32(index)}
		paths[index] = terms.Path(root)
		values[index] = terms.Root(root)
	}
	config := PathStoreConfig{
		HasAssignment: true,
		Assignment: PathStoreWriteConfig{
			Target: paths[0], Value: values[1], SourcePath: paths[1], SuppressProof: true,
		},
		HasStatic: true,
		Static: PathStoreWriteConfig{
			Target: paths[2], Value: values[3], SourcePath: paths[3], SuppressProof: false,
		},
		StaticHasAnnotation: true,
		Site:                EffectSite{Owner: 77, Ordinal: 9},
	}
	term, err := effects.PathStore(config)
	if err != nil || !effects.Valid(term, shape) {
		t.Fatalf("independent path store = %d/%v", term, err)
	}
	again, err := effects.PathStore(config)
	if err != nil || again != term {
		t.Fatal("identical independent path store did not intern canonically")
	}
	different := config
	different.Static.SuppressProof = true
	other, err := effects.PathStore(different)
	if err != nil || other == term || effects.canonical(effects.nodes[other]) == effects.canonical(effects.nodes[term]) {
		t.Fatal("independent static proof policy was absent from effect identity")
	}

	cursor, err := NewBindingCursor(shape,
		[]product.Value{
			typevalue.LiteralString(reg, "assignment-target"),
			typevalue.LiteralString(reg, "assignment-value"),
			typevalue.LiteralString(reg, "static-target"),
			typevalue.LiteralString(reg, "static-value"),
		},
		[]pathdom.Path{
			pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1), pathdom.NewPlaceholder(2), pathdom.NewPlaceholder(3),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := effects.resolve(term, cursor, SpecializationContext{})
	if !ok || resolved.Kind != EffectPathStore || !resolved.PathStore.HasAssignment || !resolved.PathStore.HasStatic ||
		!resolved.PathStore.Assignment.Target.Equal(pathdom.NewPlaceholder(0)) ||
		!resolved.PathStore.Assignment.SourcePath.Equal(pathdom.NewPlaceholder(1)) ||
		!resolved.PathStore.Static.Target.Equal(pathdom.NewPlaceholder(2)) ||
		!resolved.PathStore.Static.SourcePath.Equal(pathdom.NewPlaceholder(3)) ||
		!resolved.PathStore.Assignment.SuppressProof || resolved.PathStore.Static.SuppressProof ||
		!product.Equal(reg, resolved.PathStore.Assignment.Value, typevalue.LiteralString(reg, "assignment-value")) ||
		!product.Equal(reg, resolved.PathStore.Static.Value, typevalue.LiteralString(reg, "static-value")) {
		t.Fatalf("resolved independent path store = %#v/%v", resolved.PathStore, ok)
	}

	callerTerms := NewArena(reg)
	callerEffects := NewEffectArena(callerTerms)
	boundValues, boundPaths := make([]ValueTerm, 4), make([]PathTerm, 4)
	for index := range boundValues {
		root := Root{Kind: RootParam, Index: uint32(index)}
		boundValues[index], boundPaths[index] = callerTerms.Root(root), callerTerms.Path(root)
	}
	bindings, err := NewTermRootBindings(shape, shape, boundValues, boundPaths)
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseEffectDAGs(callerEffects, effects, bindings, []EffectTerm{term})
	if err != nil || len(rebased.Effects) != 1 || !callerEffects.Valid(rebased.Effects[0], shape) {
		t.Fatalf("independent path store rebase = %#v/%v", rebased, err)
	}
	rebasedResolved, ok := callerEffects.resolve(rebased.Effects[0], cursor, SpecializationContext{})
	if !ok || !rebasedResolved.PathStore.HasAssignment || !rebasedResolved.PathStore.HasStatic ||
		!rebasedResolved.PathStore.Assignment.Target.Equal(resolved.PathStore.Assignment.Target) ||
		!rebasedResolved.PathStore.Static.Target.Equal(resolved.PathStore.Static.Target) ||
		rebasedResolved.PathStore.Assignment.SuppressProof != resolved.PathStore.Assignment.SuppressProof ||
		rebasedResolved.PathStore.Static.SuppressProof != resolved.PathStore.Static.SuppressProof {
		t.Fatalf("rebased independent path store = %#v/%v", rebasedResolved.PathStore, ok)
	}
}

func TestEffectPathStoreSupportsSingleIndependentWriteWithoutDuplicateKind(t *testing.T) {
	reg := standard.Registry()
	terms := NewArena(reg)
	effects := NewEffectArena(terms)
	target := terms.Path(Root{Kind: RootParam, Index: 0})
	value := terms.Root(Root{Kind: RootParam, Index: 1})
	source := terms.Path(Root{Kind: RootParam, Index: 1})
	assignment, err := effects.PathStore(PathStoreConfig{
		HasAssignment: true, Assignment: PathStoreWriteConfig{Target: target, Value: value, SourcePath: source},
		Site: EffectSite{Owner: 88, Ordinal: 1},
	})
	if err != nil || effects.Kind(assignment) != EffectPathStore || !effects.Valid(assignment, Shape{Params: 2}) {
		t.Fatalf("assignment-only path store = %d/%v", assignment, err)
	}
	static, err := effects.PathStore(PathStoreConfig{
		HasStatic: true, Static: PathStoreWriteConfig{Target: target, Value: value, SourcePath: source},
		Site: EffectSite{Owner: 88, Ordinal: 2},
	})
	if err != nil || effects.Kind(static) != EffectPathStore || !effects.Valid(static, Shape{Params: 2}) || static == assignment {
		t.Fatalf("static-only path store = %d/%v", static, err)
	}
}

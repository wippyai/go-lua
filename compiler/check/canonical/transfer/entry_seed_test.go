package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEntrySeedRefinesDeclaredAnyArrayWithExactEntry(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	got := entrySeedValue(
		typ.NewArray(typ.Any),
		product.FromType(typ.NewArray(entry)),
		paramevidence.ParamContractDomain.Bottom(),
	)
	want := typ.NewArray(entry)
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("entry seed = %v, want %v", got.ProjectValue(), want)
	}
}

func TestEntrySeedKeepsBareDeclaredAnyAsDynamicTop(t *testing.T) {
	got := entrySeedValue(
		typ.Any,
		product.FromType(typ.NewRecord().Field("id", typ.String).Build()),
		paramevidence.ParamContractDomain.Bottom(),
	)
	if !typ.TypeEquals(got.ProjectValue(), typ.Any) {
		t.Fatalf("bare any seed = %v, want any", got.ProjectValue())
	}
}

func TestEntrySeedOpenGenericDeclaredParamUsesClosedEntryValue(t *testing.T) {
	got := entrySeedValue(
		typ.NewTypeParam("T", nil),
		product.FromType(typ.String),
		paramevidence.ParamContractDomain.Bottom(),
	)
	if !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("generic entry seed = %v, want string", got.ProjectValue())
	}
}

func TestEntrySeedOpenGenericInstantiationUsesClosedInstantiatedEntryValue(t *testing.T) {
	tParam := typ.NewTypeParam("T", nil)
	boxParam := typ.NewTypeParam("X", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typ.NewRecord().Field("value", boxParam).Build(),
	)
	envelope := typ.NewRecord().Field("id", typ.String).Build()

	got := entrySeedValue(
		typ.Instantiate(box, tParam),
		product.FromType(typ.Instantiate(box, envelope)),
		paramevidence.ParamContractDomain.Bottom(),
	)

	if !typ.TypeEquals(got.ProjectValue(), typ.Instantiate(box, envelope)) {
		t.Fatalf("generic instantiated entry seed = %v, want Box<Envelope>", got.ProjectValue())
	}
}

func TestEntrySeedClosesGenericDeclaredParamsFromWholeEntryVector(t *testing.T) {
	tParam := typ.NewTypeParam("T", nil)
	uParam := typ.NewTypeParam("U", nil)
	boxParam := typ.NewTypeParam("X", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typ.NewRecord().Field("value", boxParam).Build())
	envelope := typ.NewRecord().Field("id", typ.String).Build()
	view := typ.NewRecord().Field("label", typ.String).Build()

	outerT := typ.NewTypeParam("T", nil)
	declared := map[int]typ.Type{
		0: typ.Instantiate(box, tParam),
		1: typ.Func().Param("value", tParam).Returns(uParam).Build(),
	}
	closed := closeDeclaredParamTypes(declared, []*typ.TypeParam{tParam, uParam}, []typ.Type{
		typ.Instantiate(box, outerT),
		typ.Func().Param("env", envelope).Returns(view).Build(),
	})

	if !typ.TypeEquals(closed[0], typ.Instantiate(box, envelope)) {
		t.Fatalf("closed slot 0 = %v, want Box<Envelope>", closed[0])
	}
	fn, ok := closed[1].(*typ.Function)
	if !ok || len(fn.Params) != 1 || len(fn.Returns) != 1 {
		t.Fatalf("closed slot 1 = %v, want one-param function", closed[1])
	}
	if !typ.TypeEquals(fn.Params[0].Type, envelope) || !typ.TypeEquals(fn.Returns[0], view) {
		t.Fatalf("closed callback = %v, want Envelope -> View", fn)
	}
}

func TestEntrySeedBodyContractDoesNotRefineDeclaredAny(t *testing.T) {
	contract := typ.NewRecord().ReadonlyField("id", typ.String).Build()
	got := entrySeedValue(
		typ.Any,
		product.AbstractValue{},
		paramevidence.DemandFromType(contract),
	)
	if !typ.TypeEquals(got.ProjectValue(), typ.Any) {
		t.Fatalf("contract seed = %v, want declared any", got.ProjectValue())
	}
}

func TestEntrySeedBodyContractSeedsUnannotatedParam(t *testing.T) {
	contract := typ.NewRecord().ReadonlyField("id", typ.String).Build()
	want := typ.NewRecord().SetOpen(true).ReadonlyField("id", typ.String).Build()
	got := entrySeedValue(
		nil,
		product.AbstractValue{},
		paramevidence.DemandFromType(contract),
	)
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("contract seed = %v, want %v", got.ProjectValue(), want)
	}
}

func TestEntrySeedBodyContractReplacesGradualEntry(t *testing.T) {
	contract := typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build())
	want := typ.NewArray(typ.NewRecord().SetOpen(true).ReadonlyField("id", typ.String).Build())
	got := entrySeedValue(
		nil,
		product.GradualAny(),
		paramevidence.DemandFromType(contract),
	)
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("gradual entry seed = %v, want body contract %v", got.ProjectValue(), want)
	}
}

func TestEntrySeedExactEntryIsNotRefinedByBodyContract(t *testing.T) {
	entry := typ.NewArray(typ.Any)
	contract := typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build())
	got := entrySeedValue(
		typ.NewArray(typ.Any),
		product.FromType(entry),
		paramevidence.DemandFromType(contract),
	)
	if !typ.TypeEquals(got.ProjectValue(), entry) {
		t.Fatalf("exact entry seed = %v, want %v", got.ProjectValue(), entry)
	}
}

func TestLengthContextIsLengthableNotStringOnly(t *testing.T) {
	ctx := lengthContextType()
	if !ops.MayHaveLength(ctx) {
		t.Fatalf("length context = %v, want lengthable", ctx)
	}
	if typ.TypeEquals(ctx, typ.String) {
		t.Fatalf("length context regressed to string-only")
	}
}

func TestEntrySeedEffectWritesRefinedDeclaredContainer(t *testing.T) {
	const sym = cfg.SymbolID(21)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}
	entry := typ.NewRecord().Field("id", typ.String).Build()

	tr.applyEntrySeedEffect(&out, EntrySeedEffect{
		Symbol:   sym,
		Declared: typ.NewArray(typ.Any),
		Entry:    product.FromType(typ.NewArray(entry)),
	})

	got, ok := tr.symbolValue(&out, sym)
	want := typ.NewArray(entry)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("seeded symbol = %v/%v, want %v", got.ProjectValue(), ok, want)
	}
}

func TestLocalSoftContainerAnnotationDoesNotEraseKnownEmptyInitializer(t *testing.T) {
	const sym = cfg.SymbolID(31)
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: typ.NewArray(typ.Any)}

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Symbol: sym, Name: "xs"}},
		Sources: []ast.Expr{&ast.TableExpr{}},
	}, nil)

	got, ok := tr.symbolValue(&out, sym)
	if !ok {
		t.Fatal("soft container initializer did not write a value")
	}
	if typ.TypeEquals(got.ProjectValue(), typ.NewArray(typ.Any)) {
		t.Fatalf("soft container annotation erased known initializer: %v", got.ProjectValue())
	}
	entry := typ.NewRecord().Field("id", typ.String).Build()
	appended := product.AppendElement(got, product.FromType(entry))
	want := typ.NewArray(entry)
	if !typ.TypeEquals(appended.ProjectValue(), want) {
		t.Fatalf("append after soft initializer = %v, want %v", appended.ProjectValue(), want)
	}
}

func TestLocalConcreteContainerAnnotationStillSeedsUnknownInitializer(t *testing.T) {
	const sym = cfg.SymbolID(32)
	declared := typ.NewMap(typ.String, typ.String)
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: declared}

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Symbol: sym, Name: "m"}},
		Sources: []ast.Expr{&ast.IdentExpr{Value: "unresolved"}},
	}, nil)

	got, ok := tr.symbolValue(&out, sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), declared) {
		t.Fatalf("concrete container declaration = %v/%v, want %v", got.ProjectValue(), ok, declared)
	}
}

func TestLocalObjectAnnotationSeedsConstructorInitializer(t *testing.T) {
	const sym = cfg.SymbolID(33)
	declared := typ.NewRecord().
		Field("sessions", typ.NewMap(typ.String, typ.String)).
		Build()
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: declared}

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		IsLocal: true,
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Symbol: sym,
			Name:   "self",
		}},
		Sources: []ast.Expr{&ast.TableExpr{}},
		TypeAnnotations: []ast.TypeExpr{
			&ast.TypeRefExpr{Path: []string{"Store"}},
		},
	}, nil)

	got, ok := tr.symbolValue(&out, sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), declared) {
		t.Fatalf("object declaration = %v/%v, want %v", got.ProjectValue(), ok, declared)
	}
}

func TestLocalUnionAnnotationKeepsInitializerDiscriminant(t *testing.T) {
	const sym = cfg.SymbolID(34)
	a := typ.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	b := typ.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	declared := typ.NewUnion(a, b)
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: declared}

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		IsLocal: true,
		Targets: []cfg.AssignTarget{{
			Kind:   cfg.TargetIdent,
			Symbol: sym,
			Name:   "event",
		}},
		Sources: []ast.Expr{&ast.TableExpr{Fields: []*ast.Field{{
			Key:       &ast.StringExpr{Value: "tag"},
			KeySyntax: ast.AttrKeyDot,
			Value:     &ast.StringExpr{Value: "a"},
		}}}},
		TypeAnnotations: []ast.TypeExpr{
			&ast.UnionTypeExpr{Types: []ast.TypeExpr{
				&ast.TypeRefExpr{Path: []string{"A"}},
				&ast.TypeRefExpr{Path: []string{"B"}},
			}},
		},
	}, nil)

	got, ok := tr.symbolValue(&out, sym)
	if !ok {
		t.Fatal("union initializer did not write a value")
	}
	if typ.TypeEquals(got.ProjectValue(), declared) {
		t.Fatalf("union annotation erased initializer discriminant: %v", got.ProjectValue())
	}
	if !typ.TypeEquals(got.ProjectValue(), a) {
		t.Fatalf("union initializer = %v, want %v", got.ProjectValue(), a)
	}
}

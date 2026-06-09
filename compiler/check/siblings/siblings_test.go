package siblings

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/types/constraint"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestBuild_Empty(t *testing.T) {
	conf := BuildConfig{}
	result := Build(conf)
	if result != nil {
		t.Error("empty config should return nil")
	}
}

func TestBuild_WithFuncs(t *testing.T) {
	fnType := typ.Func().Returns(typ.String).Build()
	conf := BuildConfig{
		Funcs: []FuncEntry{
			{Symbol: 1, IsLocal: true},
		},
		FunctionFacts: api.FunctionFacts{1: {Public: api.FunctionPublicProjection{Signature: fnType}}},
	}
	result := Build(conf)
	if result == nil {
		t.Fatal("should return result")
	}
	if result[1] == nil {
		t.Error("should include function type")
	}
}

func TestBuild_WithPrev(t *testing.T) {
	conf := BuildConfig{
		Funcs: []FuncEntry{
			{Symbol: 1, IsLocal: true},
		},
		FunctionFacts: api.FunctionFacts{1: {Public: api.FunctionPublicProjection{Signature: typ.Func().Build()}}},
		SiblingTypesPrev: map[cfg.SymbolID]typ.Type{
			2: typ.String,
		},
	}
	result := Build(conf)
	if result[2] != typ.String {
		t.Error("should include prev types")
	}
}

func TestBuild_FieldFunctionDefinitionsContributeToOwnerSurface(t *testing.T) {
	const (
		ownerSym cfg.SymbolID = 10
		fnSym    cfg.SymbolID = 11
	)
	methodType := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	conf := BuildConfig{
		Funcs: []FuncEntry{
			{
				Symbol:  fnSym,
				Point:   20,
				IsLocal: false,
				TargetPath: constraint.Path{
					Symbol: ownerSym,
					Segments: []constraint.Segment{
						{Kind: constraint.SegmentField, Name: "name"},
					},
				},
			},
		},
		FunctionFacts: api.FunctionFacts{fnSym: {Public: api.FunctionPublicProjection{Signature: methodType}}},
		Services: BuildServicesFuncs{
			TypeAtPointFn: func(point cfg.Point, sym cfg.SymbolID) typ.Type {
				if sym == ownerSym {
					return typ.NewRecord().Field("__index", typ.Any).Build()
				}
				return nil
			},
		},
	}

	result := Build(conf)
	owner := result[ownerSym]
	got, ok := querycore.Field(owner, "name")
	if !ok {
		t.Fatalf("owner surface missing field-defined method: %s", typ.FormatShort(owner))
	}
	if !typ.TypeEquals(got, methodType) {
		t.Fatalf("owner method type = %v, want %v", got, methodType)
	}
}

func TestBuild_FieldFunctionSurfaceUsesDeclaredSeedBeforeSolvedFacts(t *testing.T) {
	const (
		ownerSym cfg.SymbolID = 10
		fnSym    cfg.SymbolID = 11
	)
	seed := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	conf := BuildConfig{
		Funcs: []FuncEntry{
			{
				Symbol: fnSym,
				Point:  20,
				TargetPath: constraint.Path{
					Symbol: ownerSym,
					Segments: []constraint.Segment{
						{Kind: constraint.SegmentField, Name: "get"},
					},
				},
			},
		},
		Services: BuildServicesFuncs{
			TypeAtPointFn: func(point cfg.Point, sym cfg.SymbolID) typ.Type {
				switch sym {
				case ownerSym:
					return typ.NewRecord().Build()
				case fnSym:
					return seed
				default:
					return nil
				}
			},
		},
	}

	result := Build(conf)
	owner := result[ownerSym]
	got, ok := querycore.Field(owner, "get")
	if !ok {
		t.Fatalf("owner surface missing declared method seed: %s", typ.FormatShort(owner))
	}
	if !typ.TypeEquals(got, seed) {
		t.Fatalf("owner method seed = %v, want %v", got, seed)
	}
}

func TestReceiverSelfType_ComposesBaseWithSiblingMethodSurface(t *testing.T) {
	base := typ.NewRecord().
		Field("__index", typ.Any).
		Field("id", typ.String).
		Build()
	method := typ.Func().
		Param("self", base).
		Param("payload", typ.Any).
		Returns(typ.Boolean, typ.NewOptional(typ.String)).
		Build()
	surface := typ.NewRecord().
		Field("dispatch", method).
		Build()

	selfType := ReceiverSelfType(base, surface)
	if _, ok := querycore.Field(selfType, "id"); !ok {
		t.Fatalf("receiver self lost base field: %s", typ.FormatShort(selfType))
	}
	mt, ok := querycore.Method(selfType, "dispatch")
	if !ok {
		t.Fatalf("receiver self missing sibling method surface: %s", typ.FormatShort(selfType))
	}
	fn := unwrap.Function(mt)
	if fn == nil || len(fn.Returns) == 0 || !typ.TypeEquals(fn.Returns[0], typ.Boolean) {
		t.Fatalf("dispatch method = %s, want boolean first return", typ.FormatShort(mt))
	}
}

func TestMergeSiblingType_BothNilViaBuildAPI(t *testing.T) {
	result := functionfact.MergeType(nil, nil)
	if result != nil {
		t.Error("both nil should return nil")
	}
}

func TestMergeSiblingType_PrevNilViaBuildAPI(t *testing.T) {
	result := functionfact.MergeType(nil, typ.String)
	if result != typ.String {
		t.Error("prev nil should return next")
	}
}

func TestMergeSiblingType_NextNilViaBuildAPI(t *testing.T) {
	result := functionfact.MergeType(typ.String, nil)
	if result != typ.String {
		t.Error("next nil should return prev")
	}
}

func TestMergeSiblingType_FunctionsViaBuildAPI(t *testing.T) {
	prevFn := typ.Func().Build()
	nextFn := typ.Func().Returns(typ.String).Build()
	result := functionfact.MergeType(prevFn, nextFn)
	if result == nil {
		t.Fatal("should return merged function")
	}
	fn, ok := result.(*typ.Function)
	if !ok {
		t.Fatal("should be function type")
	}
	if len(fn.Returns) == 0 {
		t.Error("should prefer function with returns")
	}
}

func TestMergeSiblingType_FunctionAliasesViaBuildAPI(t *testing.T) {
	prevFn := typ.NewAlias("Prev", typ.Func().Build())
	nextFn := typ.NewAlias("Next", typ.Func().Returns(typ.String).Build())
	result := functionfact.MergeType(prevFn, nextFn)
	if !typ.TypeEquals(result, nextFn) {
		t.Fatalf("expected function alias with returns to be preferred, got %v", result)
	}
}

func TestCompute_Nil(t *testing.T) {
	result := Compute(nil, 0)
	if result != nil {
		t.Error("nil store should return nil")
	}
}

func TestCompute_Found(t *testing.T) {
	store := map[uint64]map[cfg.SymbolID]typ.Type{
		1: {1: typ.String},
	}
	result := Compute(store, 1)
	if result == nil {
		t.Error("should find stored siblings")
	}
}

func TestCompute_NotFound(t *testing.T) {
	store := map[uint64]map[cfg.SymbolID]typ.Type{
		1: {1: typ.String},
	}
	result := Compute(store, 2)
	if result != nil {
		t.Error("should return nil for missing hash")
	}
}

func TestCopy_Nil(t *testing.T) {
	result := Copy(nil)
	if result != nil {
		t.Error("copy of nil should be nil")
	}
}

func TestCopy(t *testing.T) {
	original := map[cfg.SymbolID]typ.Type{1: typ.String}
	copied := Copy(original)
	if copied == nil {
		t.Fatal("copy should not be nil")
	}
	if copied[1] != typ.String {
		t.Error("copy should have same values")
	}
	copied[2] = typ.Number
	if original[2] != nil {
		t.Error("modifying copy should not affect original")
	}
}

func TestFuncEntry(t *testing.T) {
	entry := FuncEntry{
		Symbol:  1,
		Point:   10,
		IsLocal: true,
	}
	if entry.Symbol != 1 {
		t.Error("Symbol should be set")
	}
	if entry.Point != 10 {
		t.Error("Point should be set")
	}
}

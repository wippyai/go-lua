package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type exportProjectionGraphStub struct {
	names map[cfg.SymbolID]string
	paths map[cfg.SymbolID]constraint.Path
}

func (g exportProjectionGraphStub) NameOf(sym cfg.SymbolID) string {
	return g.names[sym]
}

func (g exportProjectionGraphStub) FuncDefPathForSymbol(sym cfg.SymbolID) (constraint.Path, bool) {
	path, ok := g.paths[sym]
	return path, ok
}

func TestProjectExportType_DoesNotRewriteReturnedDataFieldsWithPrivateFunctionFacts(t *testing.T) {
	const parseSym cfg.SymbolID = 1
	const privateSym cfg.SymbolID = 2
	graph := exportProjectionGraphStub{
		names: map[cfg.SymbolID]string{
			parseSym:   "registry.parse_id",
			privateSym: "id_mt.ns",
		},
		paths: map[cfg.SymbolID]constraint.Path{
			parseSym: {
				Root:     "registry",
				Symbol:   10,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "parse_id"}},
			},
			privateSym: {
				Root:   "id_mt",
				Symbol: 11,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "ns"},
					{Kind: constraint.SegmentField, Name: "private"},
				},
			},
		},
	}
	parseReturn := typ.NewRecord().Field("ns", typ.String).Build()
	parseFn := typ.Func().Param("_raw", typ.Any).Returns(parseReturn).Build()
	privateFn := typ.Func().Param("self", typ.Any).Returns(typ.LiteralString("method")).Build()
	export := typ.NewRecord().Field("parse_id", typ.Func().Param("_raw", typ.Any).Returns(typ.Unknown).Build()).Build()
	facts := api.FunctionFacts{
		parseSym: {
			Signature: parseFn,
			Summary:   product.LiftVector([]typ.Type{parseReturn}),
			Narrow:    product.LiftVector([]typ.Type{parseReturn}),
		},
		privateSym: {
			Signature: privateFn,
			Summary:   product.LiftVector([]typ.Type{typ.LiteralString("method")}),
			Narrow:    product.LiftVector([]typ.Type{typ.LiteralString("method")}),
		},
	}

	got := projectExportTypeForNames(export, "", facts, graph)
	parseType, ok := core.Field(got, "parse_id")
	if !ok {
		t.Fatalf("projected export dropped parse_id: %v", got)
	}
	fn := unwrap.Function(parseType)
	if fn == nil || len(fn.Returns) != 1 {
		t.Fatalf("parse_id = %v, want one-return function", parseType)
	}
	rec, ok := fn.Returns[0].(*typ.Record)
	if !ok {
		t.Fatalf("parse_id return = %T, want record", fn.Returns[0])
	}
	field := rec.GetField("ns")
	if field == nil {
		t.Fatalf("parse_id return dropped ns field: %v", rec)
	}
	if !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("returned ns field = %v, want string", field.Type)
	}
}

func TestProjectExportType_UsesExactStructuralExportPath(t *testing.T) {
	const topSym cfg.SymbolID = 1
	const nestedSym cfg.SymbolID = 2
	graph := exportProjectionGraphStub{
		names: map[cfg.SymbolID]string{
			topSym:    "registry.run",
			nestedSym: "registry.api.run",
		},
		paths: map[cfg.SymbolID]constraint.Path{
			topSym: {
				Root:     "registry",
				Symbol:   10,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "run"}},
			},
			nestedSym: {
				Root:   "registry",
				Symbol: 10,
				Segments: []constraint.Segment{
					{Kind: constraint.SegmentField, Name: "api"},
					{Kind: constraint.SegmentField, Name: "run"},
				},
			},
		},
	}
	baseFn := typ.Func().Returns(typ.Unknown).Build()
	topFn := typ.Func().Returns(typ.String).Build()
	nestedFn := typ.Func().Returns(typ.Number).Build()
	export := typ.NewRecord().
		Field("run", baseFn).
		Field("api", typ.NewRecord().Field("run", baseFn).Build()).
		Build()
	facts := api.FunctionFacts{
		topSym: {
			Signature: topFn,
			Summary:   product.LiftVector([]typ.Type{typ.String}),
			Narrow:    product.LiftVector([]typ.Type{typ.String}),
		},
		nestedSym: {
			Signature: nestedFn,
			Summary:   product.LiftVector([]typ.Type{typ.Number}),
			Narrow:    product.LiftVector([]typ.Type{typ.Number}),
		},
	}

	got := projectExportTypeForNames(export, "", facts, graph)
	rec := unwrap.Alias(got).(*typ.Record)
	top := unwrap.Function(rec.GetField("run").Type)
	if top == nil || len(top.Returns) != 1 || !typ.TypeEquals(top.Returns[0], typ.String) {
		t.Fatalf("top run = %v, want string return", top)
	}
	apiRec := unwrap.Alias(rec.GetField("api").Type).(*typ.Record)
	nested := unwrap.Function(apiRec.GetField("run").Type)
	if nested == nil || len(nested.Returns) != 1 || !typ.TypeEquals(nested.Returns[0], typ.Number) {
		t.Fatalf("api.run = %v, want number return", nested)
	}
}

func TestProjectExportType_DoesNotRewriteSameLeafElsewhere(t *testing.T) {
	const topSym cfg.SymbolID = 1
	graph := exportProjectionGraphStub{
		names: map[cfg.SymbolID]string{topSym: "registry.run"},
		paths: map[cfg.SymbolID]constraint.Path{
			topSym: {
				Root:     "registry",
				Symbol:   10,
				Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "run"}},
			},
		},
	}
	baseFn := typ.Func().Returns(typ.Unknown).Build()
	topFn := typ.Func().Returns(typ.String).Build()
	export := typ.NewRecord().
		Field("run", baseFn).
		Field("api", typ.NewRecord().Field("run", baseFn).Build()).
		Build()
	facts := api.FunctionFacts{
		topSym: {
			Signature: topFn,
			Summary:   product.LiftVector([]typ.Type{typ.String}),
			Narrow:    product.LiftVector([]typ.Type{typ.String}),
		},
	}

	got := projectExportTypeForNames(export, "", facts, graph)
	rec := unwrap.Alias(got).(*typ.Record)
	apiRec := unwrap.Alias(rec.GetField("api").Type).(*typ.Record)
	nested := unwrap.Function(apiRec.GetField("run").Type)
	if nested == nil || len(nested.Returns) != 1 || !typ.TypeEquals(nested.Returns[0], typ.Unknown) {
		t.Fatalf("api.run = %v, want original unknown return", nested)
	}
}

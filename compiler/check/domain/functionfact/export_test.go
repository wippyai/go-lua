package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type exportProjectionGraphStub struct {
	names map[cfg.SymbolID]string
}

func (g exportProjectionGraphStub) NameOf(sym cfg.SymbolID) string {
	return g.names[sym]
}

func TestProjectExportType_DoesNotRewriteReturnedDataFieldsWithPrivateFunctionFacts(t *testing.T) {
	const parseSym cfg.SymbolID = 1
	const privateSym cfg.SymbolID = 2
	graph := exportProjectionGraphStub{
		names: map[cfg.SymbolID]string{
			parseSym:   "registry.parse_id",
			privateSym: "id_mt.ns",
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

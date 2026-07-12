package symboliccall

import (
	"context"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

var benchmarkValues []product.Value

func BenchmarkApplyHundredWrapperChain(b *testing.B) {
	reg := standard.Registry()
	constant := testValue(runtimekind.String, 0)
	defs := make([]Definition, 100)
	byID := make(map[FunctionID]Definition, len(defs))
	for i := range defs {
		id := FunctionID(fmt.Sprintf("f%03d", i))
		expr := Param(0)
		if i != 0 {
			expr = Call(FunctionID(fmt.Sprintf("f%03d", i-1)), 0, Param(0))
		}
		expr = Join(expr, Const(constant))
		defs[i] = Definition{ID: id, Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{expr}}
		byID[id] = defs[i]
	}
	compiled, err := Compile(context.Background(), defs, nil)
	if err != nil {
		b.Fatal(err)
	}
	last := defs[len(defs)-1]
	arg := testValue(runtimekind.Table, 1)
	b.Run("compiled", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkValues, err = compiled[last.ID].Instantiate(reg, []product.Value{arg})
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("sequential-call-tree", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			value, evalErr := evalSequential(reg, last.Returns[0], []product.Value{arg}, byID)
			if evalErr != nil {
				b.Fatal(evalErr)
			}
			benchmarkValues = []product.Value{value}
		}
	})
}

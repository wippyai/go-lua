package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPlanCompilerRejectsExpressionFunctionWithoutClosureEnvironmentProof(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	ref := factflow.ExprRef(1)
	functionSymbol := symbol.ID(41)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
	if !ok {
		t.Fatal("function expression source rejected")
	}
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.Func().Param("value", typ.Boolean).Returns(typ.Boolean).Build())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	value = product.Set(reg, value, identity.Key, identity.Singleton(identity.LuaFunction(uint64(functionSymbol))))
	compile := func(sidecar symbol.ID) Relation {
		t.Helper()
		plan := operationplan.New(graph, factflow.FactsInput{
			Returns:             map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{source})},
			ExpressionValues:    map[factflow.ExprRef]product.Value{ref: value},
			ExpressionFunctions: map[factflow.ExprRef]symbol.ID{ref: sidecar},
		})
		return NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	}

	for _, sidecar := range []symbol.ID{functionSymbol, functionSymbol + 1, 0} {
		relation := compile(sidecar)
		if reason := relation.ContextualReason(); !strings.Contains(reason, "ExpressionFunction") || relation.Rows() != 0 {
			t.Fatalf("function sidecar %d admitted without closure-environment proof: rows:%d reason:%q", sidecar, relation.Rows(), reason)
		}
	}
}

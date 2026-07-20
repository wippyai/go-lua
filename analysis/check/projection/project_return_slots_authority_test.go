package projection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type returnSlotAuthorityResult struct {
	reg         *axis.Registry
	graph       cfg.Graph
	exit        state.State
	point       cfg.Point
	sources     []factflow.ValueSource
	declared    []product.Value
	sourceValue product.Value
	reads       int
}

func (r *returnSlotAuthorityResult) Registry() *axis.Registry { return r.reg }
func (r *returnSlotAuthorityResult) Graph() cfg.Graph         { return r.graph }
func (r *returnSlotAuthorityResult) ExitState() (state.State, bool) {
	return r.exit, true
}
func (r *returnSlotAuthorityResult) ReturnPoints() []cfg.Point { return []cfg.Point{r.point} }
func (r *returnSlotAuthorityResult) KeySpace() *keyspace.KeySpace {
	return keyspace.New()
}
func (r *returnSlotAuthorityResult) DiagnosticOutput() callpayload.DiagnosticOutput {
	return callpayload.DiagnosticOutput{}
}
func (r *returnSlotAuthorityResult) ReturnTypeValues() []product.Value {
	return append([]product.Value(nil), r.declared...)
}
func (r *returnSlotAuthorityResult) ReturnValueSources(point cfg.Point) ([]factflow.ValueSource, bool) {
	if point != r.point {
		return nil, false
	}
	return append([]factflow.ValueSource(nil), r.sources...), true
}
func (r *returnSlotAuthorityResult) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	r.reads++
	if point != r.point || len(r.sources) == 0 || source != r.sources[0] {
		return product.Value{}, false
	}
	return r.sourceValue, true
}

func TestProjectReturnSlotsUsesOnlyStabilizedN5SlotsForReturnValues(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)

	stabilized := typevalue.LiteralString(reg, "stabilized")
	conflictingSource := typevalue.LiteralInt(reg, 41)
	declaredNumber := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}
	result := &returnSlotAuthorityResult{
		reg: reg, graph: graph, point: point,
		exit:        state.Reachable(state.State{}).WriteReturnSlot(reg, 0, stabilized),
		sources:     []factflow.ValueSource{source},
		declared:    []product.Value{product.Top(), declaredNumber},
		sourceValue: conflictingSource,
	}

	returns := projectReturnSlots(reg, result, result.exit, 3, result.declared)
	if len(returns) != 3 {
		t.Fatalf("return slots = %d, want 3", len(returns))
	}
	if !product.Equal(reg, returns[0], stabilized) {
		t.Fatalf("slot 0 = %#v, want stabilized N5 slot %#v", returns[0], stabilized)
	}
	if product.Equal(reg, returns[0], conflictingSource) || result.reads != 0 {
		t.Fatalf("source-side return authority was consulted: slot=%#v reads=%d", returns[0], result.reads)
	}
	if got, ok := typevalue.TypeOf(reg, returns[1]); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("declared slot enrichment = %v/%v, want number", got, ok)
	}
	if got := product.PresenceOf(returns[2]); !presence.Equal(got, presence.Absent()) {
		t.Fatalf("omitted slot presence = %s, want absent", got)
	}
	if product.Equal(reg, result.exit.ReadValue(reg, key.ReturnSlot(0)), product.Bottom(reg)) {
		t.Fatal("test fixture lost stabilized return slot")
	}
}

func TestProjectReturnSlotsPreservesUnknownCoordinateArity(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	result := &returnSlotAuthorityResult{
		reg: reg, graph: graph, point: point,
		exit:    state.Reachable(state.State{}).WriteReturnSlot(reg, 0, product.Top()),
		sources: []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: 1, HasExpr: true}},
	}

	returns := projectReturnSlots(reg, result, result.exit, 1, nil)
	if len(returns) != 1 || !product.Equal(reg, returns[0], product.Top()) {
		t.Fatalf("unknown return tuple = %#v, want one Top coordinate", returns)
	}
}

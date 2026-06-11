package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFactsNodeTransferAppliesLocalAssignmentThroughResolver(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(10), HasExpr: true}
	target := symbol.ID(101)
	assigned := presentValue(reg)
	resolver := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: assigned},
	}

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(NewFacts(FactsInput{
			LocalAssignments: map[cfg.Point]LocalAssignment{
				assign: NewLocalAssignment(target, path.NewPath(target, "local"), source),
			},
		}), resolver),
	})

	assertValue(t, reg, got[assign], key.SymbolValue(target), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferAppliesOrdinaryAssignmentThroughResolver(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(11), HasExpr: true}
	target := symbol.ID(102)
	assigned := absentValue(reg)
	resolver := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: assigned},
	}

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(NewFacts(FactsInput{
			OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
				assign: NewOrdinaryAssignment(target, path.NewPath(target, "ordinary"), source),
			},
		}), resolver),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferAppliesReturnSlotsThroughSourceValues(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	expr := ExprRef(20)
	exprValue := presentValue(reg)
	sources := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: exprValue,
		},
	})

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(NewFacts(FactsInput{
			Returns: map[cfg.Point]Return{
				ret: NewReturn([]ValueSource{
					{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true},
					{Kind: ValueSourceNil},
				}),
			},
		}), sources),
	})

	assertValue(t, reg, got[ret], key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), exprValue)
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(1), absentValue(reg))
}

func TestFactsNodeTransferUnresolvedReturnSourceLeavesSlotUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(21)
	slotValue := presentValue(reg)
	in := state.State{}.WriteReturnSlot(reg, 0, slotValue)
	sources := NewSourceValues(SourceValuesConfig{Registry: reg})

	got := NewFactsNodeTransfer(NewFacts(FactsInput{
		Returns: map[cfg.Point]Return{
			point: NewReturn([]ValueSource{
				{Kind: ValueSourceExpression, ExprRef: ExprRef(21), HasExpr: true},
			}),
		},
	}), sources)(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), slotValue)
}

func TestFactsNodeTransferReturnCallSourceReadsReturnSlotThroughRead(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	callValue := presentValue(reg)
	sources := NewSourceValues(SourceValuesConfig{Registry: reg})

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		Initial: func(point cfg.Point) (state.State, bool) {
			if point == call {
				return state.State{}.WriteReturnSlot(reg, 2, callValue), true
			}
			return state.State{}, false
		},
		NodeTransfer: NewFactsNodeTransfer(NewFacts(FactsInput{
			Returns: map[cfg.Point]Return{
				ret: NewReturn([]ValueSource{
					{
						Kind:         ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  2,
					},
				}),
			},
		}), sources),
	})

	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), callValue)
}

func TestFactsNodeTransferMissingResolverValueLeavesStateUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(12)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(12), HasExpr: true}
	target := symbol.ID(103)
	unchangedValue := presentValue(reg)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), unchangedValue)

	got := NewFactsNodeTransfer(NewFacts(FactsInput{
		LocalAssignments: map[cfg.Point]LocalAssignment{
			point: NewLocalAssignment(target, path.NewPath(target, "local"), source),
		},
	}), &recordingSourceValues{})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
}

func TestFactsNodeTransferAbsentFactsAndNilResolverLeaveStateUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(13)
	target := symbol.ID(104)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))

	gotNoResolver := NewFactsNodeTransfer(Facts{}, nil)(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)
	assertStateEqual(t, reg, gotNoResolver, in)

	gotNoFacts := NewFactsNodeTransfer(Facts{}, panicSourceValues{})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)
	assertStateEqual(t, reg, gotNoFacts, in)
}

func TestFactsNodeTransferIgnoresNonRootAssignmentFacts(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(14)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(14), HasExpr: true}
	target := symbol.ID(105)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))
	resolver := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(NewFacts(FactsInput{
		OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
			point: NewOrdinaryAssignment(target, path.NewPath(target, "ordinary").Field("member"), source),
		},
	}), resolver)(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
	if len(resolver.calls) != 0 {
		t.Fatalf("non-root assignment resolved source %d times, want zero", len(resolver.calls))
	}
}

type sourceValueCall struct {
	point  cfg.Point
	source ValueSource
}

type recordingSourceValues struct {
	values map[ValueSource]product.Value
	calls  []sourceValueCall
}

func (r *recordingSourceValues) ValueOfSource(
	point cfg.Point,
	source ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if read == nil {
		panic("nil read function")
	}
	_ = read(point)
	r.calls = append(r.calls, sourceValueCall{point: point, source: source})
	value, ok := r.values[source]
	return value, ok
}

type panicSourceValues struct{}

func (panicSourceValues) ValueOfSource(
	cfg.Point,
	ValueSource,
	state.State,
	func(cfg.Point) state.State,
) (product.Value, bool) {
	panic("ValueOfSource should not be called")
}

func assertResolverCall(t *testing.T, resolver *recordingSourceValues, point cfg.Point, source ValueSource) {
	t.Helper()
	if len(resolver.calls) != 1 {
		t.Fatalf("resolver calls = %d, want 1", len(resolver.calls))
	}
	if got := resolver.calls[0]; got.point != point || got.source != source {
		t.Fatalf("resolver call = %#v, want point %d source %#v", got, point, source)
	}
}

func assertStateEqual(t *testing.T, reg *axis.Registry, got state.State, want state.State) {
	t.Helper()
	if !state.Domain(reg).Equal(got, want) {
		t.Fatalf("state changed")
	}
}

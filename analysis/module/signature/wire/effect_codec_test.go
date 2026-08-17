package wire

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/type/projection"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

func TestWireEffectLabelRoundTripPreservesRowsAndSelectors(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	cases := []struct {
		name  string
		label effect.Label
	}{
		{"dispatch module load", dispatch.ModuleLoad{}},
		{"iteration iterator", iteration.Iterator{Source: p0, Kind: iteration.IterateIndexed}},
		{"mutation mutate", mutation.Mutate{Target: p0, Transform: mutation.ElementUnion{Source: p1}, LengthDelta: expr.Add(expr.PL(0), expr.C(1))}},
		{"mutation length change", mutation.LengthChange{Target: p1, Delta: -2}},
		{"mutation table mutator", mutation.TableMutator{Target: p0, Value: p1}},
		{"ownership borrow", ownership.Borrow{Param: p0}},
		{"ownership retain", ownership.Retain{Param: p0}},
		{"ownership store", ownership.Store{Param: p0, Into: p1}},
		{"ownership borrow all", ownership.BorrowAll{}},
		{"ownership send", ownership.Send{FromParam: 1}},
		{"ownership send param", ownership.SendParam{Param: p2}},
		{"postcondition normal return present", postcondition.NormalReturnRefinement{Target: p0, Refinement: postcondition.Present{}}},
		{"postcondition normal return absent", postcondition.NormalReturnRefinement{Target: p1, Refinement: postcondition.Absent{}}},
		{"returns return", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}}},
		{"returns error return", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			row := effect.Open("rho", tt.label)
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
			if got.Hash() != row.Hash() {
				t.Fatalf("roundtrip hash = %d, want %d", got.Hash(), row.Hash())
			}
			if !rowHasLabel(got, tt.label) {
				t.Fatalf("roundtrip row missing %T in %v", tt.label, got)
			}
		})
	}
}

func rowHasLabel(row effect.Row, want effect.Label) bool {
	want = effect.NormalizeLabel(want)
	return row.Has(func(got effect.Label) bool {
		return got != nil && got.Equals(want)
	})
}

func TestWireEffectLabelRoundTripCoversNestedKinds(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	rows := []effect.Row{
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.Unchanged{}}),
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.ContainerElementUnion{Container: p1, Value: p2}}),
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.ToArray{Element: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
			Source: p0,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("payload"),
				projection.CallableReturn(),
				projection.GenericArg(0),
				projection.InstantiateGeneric(typ.String),
			}},
		}}),
	}

	for _, row := range rows {
		t.Run(row.String(), func(t *testing.T) {
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
		})
	}
}

func TestWireEffectLabelRoundTripCoversActiveReturnMatrix(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	tests := []struct {
		name   string
		status string
		label  effect.Label
	}{
		{"actively lowered", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}}},
		{"actively lowered optional", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p0}}},
		{"actively lowered callback", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p1}}},
		{"actively lowered array callback", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p1}}},
		{"actively lowered same as", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p0}}},
		{"actively lowered projection", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
			Source: p0,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("payload"),
				projection.CallableReturn(),
			}},
		}}},
		{"actively lowered conditional type", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
			Source: p1,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("message"),
			}},
			When: typ.LiteralBool(true),
			Then: typ.String,
		}}},
		{"error return", "actively lowered by effectlowering", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}},
		{"lifecycle acquire", "actively lowered by effectlowering", lifecycle.Acquire{
			Target:   p0,
			Protocol: typestate.Protocol("transaction"),
			State:    typestate.State("active"),
			Obligation: typestate.Obligation{
				Final: typestate.State("finished"),
			},
		}},
		{"lifecycle transition", "actively lowered by effectlowering", lifecycle.Transition{
			Target:   p0,
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("finished"),
		}},
		{"lifecycle escape", "actively lowered by effectlowering", lifecycle.Escape{
			Target:   p0,
			Protocol: typestate.Protocol("transaction"),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.status+" / "+tt.name, func(t *testing.T) {
			row := effect.Open("rho", tt.label)
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
			if !rowHasLabel(got, tt.label) {
				t.Fatalf("roundtrip row missing %T in %v", tt.label, got)
			}
		})
	}
}

func TestWireRejectsInactiveEffectLabels(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	tests := []struct {
		name  string
		label effect.Label
	}{
		{"control throw", control.Throw{}},
		{"control io", control.IO{}},
		{"ownership export", ownership.Export{Param: p0}},
		{"ownership opaque", ownership.Opaque{Param: p1}},
		{"ownership freeze", ownership.Freeze{Param: p2}},
		{"return length", returns.ReturnLength{ReturnIndex: 0, Length: expr.MinExpr(expr.PL(0), expr.C(3))}},
		{"correlated return", returns.CorrelatedReturn{Indices: []int{0, 2}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := effect.Empty.With(tt.label)
			if _, err := encodeEffectRow(row); err == nil {
				t.Fatalf("encodeEffectRow(%v) succeeded, want inactive-label rejection", row)
			} else if !strings.Contains(err.Error(), "inactive effect label") {
				t.Fatalf("encodeEffectRow error = %v, want inactive-label rejection", err)
			}
		})
	}
}

func TestWireRejectsInactiveDecodedEffectLabels(t *testing.T) {
	tests := []struct {
		name string
		wire effectLabelWire
	}{
		{"control throw", effectLabelWire{Kind: "control.throw"}},
		{"control io", effectLabelWire{Kind: "control.io"}},
		{"ownership export", effectLabelWire{Kind: "ownership.export", Param: encodeParamRef(effect.ParamRef{Index: 0})}},
		{"ownership opaque", effectLabelWire{Kind: "ownership.opaque", Param: encodeParamRef(effect.ParamRef{Index: 0})}},
		{"ownership freeze", effectLabelWire{Kind: "ownership.freeze", Param: encodeParamRef(effect.ParamRef{Index: 0})}},
		{"return length", effectLabelWire{Kind: "returns.returnLength", Length: encodeExprForTest(t, expr.C(1))}},
		{"correlated return", effectLabelWire{Kind: "returns.correlatedReturn", Indices: []int{0, 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil {
				t.Fatal("decodeEffectRow succeeded, want inactive-label rejection")
			}
			if !strings.Contains(err.Error(), "inactive effect label") {
				t.Fatalf("decodeEffectRow error = %v, want inactive-label rejection", err)
			}
		})
	}
}

func TestWireRejectsMalformedLifecycleEffectLabels(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	encodeTests := []struct {
		name  string
		label effect.Label
		want  string
	}{
		{"acquire missing protocol", lifecycle.Acquire{Target: p0, State: typestate.State("active")}, "missing protocol"},
		{"acquire missing state", lifecycle.Acquire{Target: p0, Protocol: typestate.Protocol("transaction")}, "missing state"},
		{"transition missing protocol", lifecycle.Transition{Target: p0, To: typestate.State("finished")}, "missing protocol"},
		{"transition missing target", lifecycle.Transition{Target: p0, Protocol: typestate.Protocol("transaction")}, "missing target state"},
		{"transition missing source", lifecycle.Transition{Target: p0, Protocol: typestate.Protocol("transaction"), To: typestate.State("finished")}, "missing source state"},
		{"escape missing protocol", lifecycle.Escape{Target: p0}, "missing protocol"},
	}
	for _, tt := range encodeTests {
		t.Run("encode "+tt.name, func(t *testing.T) {
			_, err := encodeEffectRow(effect.Empty.With(tt.label))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("encodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}

	decodeTests := []struct {
		name string
		wire effectLabelWire
		want string
	}{
		{"acquire missing protocol", effectLabelWire{Kind: "lifecycle.acquire", Target: encodeParamRef(p0), To: "active"}, "missing protocol"},
		{"acquire missing state", effectLabelWire{Kind: "lifecycle.acquire", Target: encodeParamRef(p0), Protocol: "transaction"}, "missing state"},
		{"transition missing protocol", effectLabelWire{Kind: "lifecycle.transition", Target: encodeParamRef(p0), To: "finished"}, "missing protocol"},
		{"transition missing target", effectLabelWire{Kind: "lifecycle.transition", Target: encodeParamRef(p0), Protocol: "transaction"}, "missing target state"},
		{"transition missing source", effectLabelWire{Kind: "lifecycle.transition", Target: encodeParamRef(p0), Protocol: "transaction", To: "finished"}, "missing source state"},
		{"escape missing protocol", effectLabelWire{Kind: "lifecycle.escape", Target: encodeParamRef(p0)}, "missing protocol"},
	}
	for _, tt := range decodeTests {
		t.Run("decode "+tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWireRejectsEffectLabelsMissingParamRefs(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	tests := []struct {
		name string
		wire effectLabelWire
		want string
	}{
		{
			name: "iterator source",
			wire: effectLabelWire{Kind: "iteration.iterator", IteratorKind: "indexed"},
			want: "iterator source missing param ref",
		},
		{
			name: "mutation target",
			wire: effectLabelWire{Kind: "mutation.lengthChange"},
			want: "length change target missing param ref",
		},
		{
			name: "table mutator value",
			wire: effectLabelWire{Kind: "mutation.tableMutator", Target: encodeParamRef(p0)},
			want: "table mutator value missing param ref",
		},
		{
			name: "mutation transform source",
			wire: effectLabelWire{
				Kind:      "mutation.mutate",
				Target:    encodeParamRef(p0),
				Transform: &effectTransformWire{Kind: "mutation.elementUnion"},
				Length:    encodeExprForTest(t, expr.C(0)),
			},
			want: "mutation.elementUnion source missing param ref",
		},
		{
			name: "lifecycle target",
			wire: effectLabelWire{Kind: "lifecycle.acquire", Protocol: "transaction", To: "active"},
			want: "lifecycle acquire target missing param ref",
		},
		{
			name: "ownership store target",
			wire: effectLabelWire{Kind: "ownership.store", Param: encodeParamRef(p0)},
			want: "store target missing param ref",
		},
		{
			name: "ownership send fromParam",
			wire: effectLabelWire{Kind: "ownership.send"},
			want: "send fromParam missing",
		},
		{
			name: "param ref index",
			wire: effectLabelWire{Kind: "ownership.borrow", Param: &paramRefWire{}},
			want: "param ref index missing",
		},
		{
			name: "return transform source",
			wire: effectLabelWire{
				Kind:       "returns.return",
				ReturnType: &effectReturnWire{Kind: "returns.elementOf"},
			},
			want: "returns.elementOf source missing param ref",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWireRejectsEffectLabelsMissingScalarFields(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	tests := []struct {
		name string
		wire effectLabelWire
		want string
	}{
		{
			name: "length change delta",
			wire: effectLabelWire{Kind: "mutation.lengthChange", Target: encodeParamRef(p0)},
			want: "length change delta missing",
		},
		{
			name: "return index",
			wire: effectLabelWire{
				Kind:       "returns.return",
				ReturnType: &effectReturnWire{Kind: "returns.elementOf", Source: encodeParamRef(p0)},
			},
			want: "return index missing",
		},
		{
			name: "error return value index",
			wire: effectLabelWire{Kind: "returns.errorReturn", ErrorIndex: encodeInt(1)},
			want: "error return value index missing",
		},
		{
			name: "error return error index",
			wire: effectLabelWire{Kind: "returns.errorReturn", ValueIndex: encodeInt(0)},
			want: "error return error index missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWireEffectLabelSendEncodesZeroFromParamExplicitly(t *testing.T) {
	wire, err := encodeEffectRow(effect.Empty.With(ownership.Send{FromParam: 0}))
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	if wire == nil || len(wire.Labels) != 1 || wire.Labels[0].FromParam == nil || *wire.Labels[0].FromParam != 0 {
		t.Fatalf("send label wire = %#v, want explicit fromParam 0", wire)
	}
}

func TestWireParamRefEncodesZeroIndexExplicitly(t *testing.T) {
	wire := encodeParamRef(effect.ParamRef{Index: 0})
	if wire == nil || wire.Index == nil || *wire.Index != 0 {
		t.Fatalf("param ref wire = %#v, want explicit index 0", wire)
	}
}

func TestWireEffectLabelsEncodeZeroScalarFieldsExplicitly(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	tests := []struct {
		name  string
		label effect.Label
		check func(t *testing.T, wire effectLabelWire)
	}{
		{
			name:  "length change delta",
			label: mutation.LengthChange{Target: p0, Delta: 0},
			check: func(t *testing.T, wire effectLabelWire) {
				t.Helper()
				if wire.Delta == nil || *wire.Delta != 0 {
					t.Fatalf("length change wire = %#v, want explicit delta 0", wire)
				}
			},
		},
		{
			name:  "return index",
			label: returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}},
			check: func(t *testing.T, wire effectLabelWire) {
				t.Helper()
				if wire.ReturnIndex == nil || *wire.ReturnIndex != 0 {
					t.Fatalf("return wire = %#v, want explicit returnIndex 0", wire)
				}
			},
		},
		{
			name:  "error return indices",
			label: returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 0},
			check: func(t *testing.T, wire effectLabelWire) {
				t.Helper()
				if wire.ValueIndex == nil || *wire.ValueIndex != 0 || wire.ErrorIndex == nil || *wire.ErrorIndex != 0 {
					t.Fatalf("error return wire = %#v, want explicit valueIndex/errorIndex 0", wire)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := encodeEffectRow(effect.Empty.With(tt.label))
			if err != nil {
				t.Fatalf("encodeEffectRow: %v", err)
			}
			if wire == nil || len(wire.Labels) != 1 {
				t.Fatalf("effect row wire = %#v, want one label", wire)
			}
			tt.check(t, wire.Labels[0])
		})
	}
}

func TestWireEffectPointerLabelsNormalizeToValues(t *testing.T) {
	row := effect.Row{Labels: []effect.Label{
		&iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed},
		&mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: &mutation.ToArray{Element: effect.ParamRef{Index: 1}}},
		&ownership.Borrow{Param: effect.ParamRef{Index: 2}},
		&returns.Return{ReturnIndex: 0, Transform: &returns.ElementOf{Source: effect.ParamRef{Index: 0}}},
	}}

	got := mustRoundTripEffectRow(t, row)
	if !got.Equals(row) {
		t.Fatalf("roundtrip pointer row = %v, want %v", got, row)
	}
	if got.Hash() != row.Hash() {
		t.Fatalf("roundtrip pointer hash = %d, want %d", got.Hash(), row.Hash())
	}
	for _, want := range row.Labels {
		if !rowHasLabel(got, want) {
			t.Fatalf("roundtrip pointer row missing %T in %v", want, got)
		}
	}
	for _, label := range got.Labels {
		if effect.NormalizeLabel(label) != label {
			t.Fatalf("decoded label %T was not value-owned", label)
		}
	}
}

func TestWireExprPointerRoundTrip(t *testing.T) {
	original := &expr.BinOp{
		Op:    expr.OpAdd,
		Left:  &expr.ParamLen{Index: 0},
		Right: &expr.Const{Value: 2},
	}

	wire, err := encodeExpr(original)
	if err != nil {
		t.Fatalf("encodeExpr(pointer): %v", err)
	}
	got, err := decodeExpr(wire)
	if err != nil {
		t.Fatalf("decodeExpr(pointer roundtrip): %v", err)
	}
	if got.String() != original.String() {
		t.Fatalf("roundtrip expr = %s, want %s", got, original)
	}
}

func TestWireExprRejectsTypedNilPointer(t *testing.T) {
	var typedNil *expr.Const
	if _, err := encodeExpr(typedNil); err == nil {
		t.Fatal("encodeExpr(typed nil) succeeded, want error")
	} else if !strings.Contains(err.Error(), "nil constraint expr") {
		t.Fatalf("encodeExpr(typed nil) error = %v", err)
	}
}

func TestWireExprRejectsMissingCompoundOperands(t *testing.T) {
	one := encodeExprForTest(t, expr.C(1))
	tests := []struct {
		name string
		wire *exprWire
		want string
	}{
		{
			name: "binop missing left",
			wire: &exprWire{Kind: "binop", Op: "+", Right: one},
			want: "binop left",
		},
		{
			name: "binop missing right",
			wire: &exprWire{Kind: "binop", Op: "+", Left: one},
			want: "binop right",
		},
		{
			name: "min missing left",
			wire: &exprWire{Kind: "min", Right: one},
			want: "min left",
		},
		{
			name: "min missing right",
			wire: &exprWire{Kind: "min", Left: one},
			want: "min right",
		},
		{
			name: "max missing left",
			wire: &exprWire{Kind: "max", Right: one},
			want: "max left",
		},
		{
			name: "max missing right",
			wire: &exprWire{Kind: "max", Left: one},
			want: "max right",
		},
	}

	if got, err := decodeExpr(nil); err != nil || got != nil {
		t.Fatalf("decodeExpr(nil) = %#v/%v, want nil/nil for optional top-level field", got, err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeExpr(tt.wire); err == nil {
				t.Fatal("decodeExpr succeeded, want missing operand error")
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeExpr error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestWireExprRejectsMissingScalarIndex(t *testing.T) {
	tests := []struct {
		name string
		wire *exprWire
		want string
	}{
		{name: "param", wire: &exprWire{Kind: "param"}, want: "param index missing"},
		{name: "ret", wire: &exprWire{Kind: "ret"}, want: "ret index missing"},
		{name: "paramLen", wire: &exprWire{Kind: "paramLen"}, want: "paramLen index missing"},
		{name: "retLen", wire: &exprWire{Kind: "retLen"}, want: "retLen index missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeExpr(tt.wire); err == nil {
				t.Fatal("decodeExpr succeeded, want missing index error")
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeExpr error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestWireExprEncodesZeroIndexExplicitly(t *testing.T) {
	tests := []struct {
		name string
		expr expr.Expr
	}{
		{name: "param", expr: expr.P(0)},
		{name: "ret", expr: expr.R(0)},
		{name: "paramLen", expr: expr.PL(0)},
		{name: "retLen", expr: expr.RL(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := encodeExpr(tt.expr)
			if err != nil {
				t.Fatalf("encodeExpr: %v", err)
			}
			if wire == nil || wire.Index == nil || *wire.Index != 0 {
				t.Fatalf("expr wire = %#v, want explicit index 0", wire)
			}
		})
	}
}

func TestWireProjectionGenericArgRequiresExplicitIndex(t *testing.T) {
	if _, err := decodeProjectionSteps([]projectionStepWire{{Kind: "genericArg"}}); err == nil {
		t.Fatal("decodeProjectionSteps succeeded, want missing index error")
	} else if !strings.Contains(err.Error(), "projection genericArg index missing") {
		t.Fatalf("decodeProjectionSteps error = %v, want missing genericArg index", err)
	}
}

func TestWireProjectionGenericArgEncodesZeroIndexExplicitly(t *testing.T) {
	wire, err := encodeProjectionSteps([]projection.Step{projection.GenericArg(0)})
	if err != nil {
		t.Fatalf("encodeProjectionSteps: %v", err)
	}
	if len(wire) != 1 || wire[0].Index == nil || *wire[0].Index != 0 {
		t.Fatalf("projection wire = %#v, want explicit index 0", wire)
	}
}

func TestWireExprRejectsInvalidEncodeOp(t *testing.T) {
	_, err := encodeExpr(expr.BinOp{Op: expr.Op(99), Left: expr.C(1), Right: expr.C(2)})
	if err == nil {
		t.Fatal("encodeExpr(invalid op) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unsupported expr op") {
		t.Fatalf("encodeExpr(invalid op) error = %v", err)
	}
}

func encodeExprForTest(t *testing.T, e expr.Expr) *exprWire {
	t.Helper()
	wire, err := encodeExpr(e)
	if err != nil {
		t.Fatalf("encodeExpr: %v", err)
	}
	return wire
}

func mustRoundTripEffectRow(t *testing.T, row effect.Row) effect.Row {
	t.Helper()
	wire, err := encodeEffectRow(row)
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	got, err := decodeEffectRow(wire)
	if err != nil {
		t.Fatalf("decodeEffectRow: %v", err)
	}
	return got
}

func TestWireDecodeIteratorRequiresExplicitKind(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{Kind: "iteration.iterator"})
	if err == nil || !strings.Contains(err.Error(), `unknown iterator kind ""`) {
		t.Fatalf("decodeEffectLabel error = %v, want unknown iterator kind", err)
	}
}

func TestWireDecodePostconditionRefinementRequiresKnownKind(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{
		Kind:       postcondition.NormalReturnRefinementKind,
		Target:     &paramRefWire{Index: encodeInt(0)},
		Refinement: &effectRefinementWire{Kind: "future"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown effect refinement kind "future"`) {
		t.Fatalf("decodeEffectLabel error = %v, want unknown effect refinement kind", err)
	}
}

func TestWireDecodePostconditionRefinementRequiresRefinement(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{
		Kind:   postcondition.NormalReturnRefinementKind,
		Target: &paramRefWire{Index: encodeInt(0)},
	})
	if err == nil || !strings.Contains(err.Error(), "missing effect refinement") {
		t.Fatalf("decodeEffectLabel error = %v, want missing effect refinement", err)
	}
}

package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestExactTypePredicateEvidencePresenceTruthTable(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeBranch)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), true)
	graph.AddEdge(point, graph.Exit(), false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar source shape rejected")
	}
	conditionSource, ok := factflow.NewPathValueSource("condition", 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("condition source rejected")
	}
	conditionDescriptor, ok := factflow.NewBranchCondition(conditionSource, true)
	if !ok {
		t.Fatal("condition descriptor rejected")
	}

	const valueSymbol, otherSymbol = symbol.ID(10), symbol.ID(11)
	valuePath := pathdom.NewPath(valueSymbol, "value")
	otherPath := pathdom.NewPath(otherSymbol, "other")
	tests := []struct {
		name      string
		operator  string
		tag       string
		proof     factflow.BranchPathEvidence
		wantError string
	}{
		{name: "equal non-nil true", operator: "==", tag: "string", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(valuePath, presence.Present(), true)},
		{name: "not-equal non-nil false", operator: "~=", tag: "string", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(valuePath, presence.Present(), false)},
		{name: "equal nil false", operator: "==", tag: "nil", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(valuePath, presence.Present(), false)},
		{name: "not-equal nil true", operator: "~=", tag: "nil", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(valuePath, presence.Present(), true)},
		{name: "wrong predicate outcome", operator: "~=", tag: "string", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(valuePath, presence.Present(), true), wantError: "branch: contextual-path-evidence-polarity"},
		{name: "mismatched evidence path", operator: "~=", tag: "string", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(otherPath, presence.Present(), false), wantError: "branch: contextual-path-evidence-path"},
		{name: "absent is not entailed", operator: "~=", tag: "string", proof: factflow.NewBranchPathPresenceEvidenceOnEdge(valuePath, presence.Absent(), false), wantError: "branch: contextual-path-evidence-presence"},
		{name: "truthy is not entailed", operator: "~=", tag: "string", proof: factflow.NewBranchPathTruthyEvidenceOnEdge(valuePath, false), wantError: "branch: contextual-path-evidence-kind 4"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := factflow.FactsInput{
				BranchConditionSources: map[cfg.Point]factflow.BranchCondition{point: conditionDescriptor},
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					point: factflow.NewBranchPathEvidenceSet(test.proof),
				},
			}
			plan := operationplan.New(graph, input).WithBoundaryParams([]symbol.ID{valueSymbol, otherSymbol})
			builder := NewBuilder(reg, Shape{Params: 2}, nil, plan)
			value := builder.Arena().Root(Root{Kind: RootParam, Index: 0})
			other := builder.Arena().Root(Root{Kind: RootParam, Index: 1})
			typeName := builder.Arena().LuaTypeNameValue(value)
			condition, exact := builder.Arena().ScalarBinaryValue(test.operator, typeName, builder.Arena().Constant(typevalue.LiteralString(reg, test.tag)))
			if !exact {
				t.Fatal("type predicate term rejected")
			}
			ctx := planCompileContext{
				registry: reg,
				plan:     plan,
				facts:    plan.Facts(),
				builder:  builder,
				locals:   map[symbol.ID]ValueTerm{valueSymbol: value, otherSymbol: other},
			}
			err := validateRepresentedBranchEvidence(ctx, factapply.NewBranchAlgebra(plan.Facts(), point), condition)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("exact predicate evidence rejected: %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestSingleActiveEvidenceEdgeRejectsAmbiguousPolarity(t *testing.T) {
	for _, test := range []struct {
		name                    string
		activeTrue, activeFalse bool
		wantEdge, wantExact     bool
	}{
		{name: "true", activeTrue: true, wantEdge: true, wantExact: true},
		{name: "false", activeFalse: true, wantExact: true},
		{name: "neither"},
		{name: "both", activeTrue: true, activeFalse: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotEdge, gotExact := singleActiveEvidenceEdge(test.activeTrue, test.activeFalse)
			if gotEdge != test.wantEdge || gotExact != test.wantExact {
				t.Fatalf("single active edge = %v/%v, want %v/%v", gotEdge, gotExact, test.wantEdge, test.wantExact)
			}
		})
	}
}

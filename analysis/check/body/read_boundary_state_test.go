package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestNeedsBoundaryNodeOutputCoversNodeTransferFactLanes(t *testing.T) {
	point := cfg.Point(7)
	rootPath := path.NewPath(symbol.ID(1), "root")
	memberPath := rootPath.Field("field")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1), HasExpr: true}

	cases := []struct {
		name        string
		input       factflow.FactsInput
		callOutcome bool
	}{
		{
			name: "root assignment",
			input: factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, symbol.ID(1), rootPath, source),
			}},
		},
		{
			name: "path assignment",
			input: factflow.FactsInput{PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(memberPath, source),
			}},
		},
		{
			name: "path descendant invalidation",
			input: factflow.FactsInput{PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
				point: factflow.NewPathDescendantInvalidation(rootPath),
			}},
		},
		{
			name: "dynamic index write",
			input: factflow.FactsInput{DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(rootPath, source, source, dynamicindex.AdmissionUnknown, factflow.DynamicIndexReadbackNone),
			}},
		},
		{
			name: "path static member write",
			input: factflow.FactsInput{PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(memberPath, source),
			}},
		},
		{
			name: "return",
			input: factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
				point: factflow.NewReturn([]factflow.ValueSource{source}),
			}},
		},
		{
			name: "producer eligible call site",
			input: factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextAssignmentSource,
					ResultTargets: []factflow.CallResultTarget{
						factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, symbol.ID(2), path.NewPath(symbol.ID(2), "target")),
					},
				}),
			}},
		},
		{
			name: "call site with outcome provider",
			input: factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
			}},
			callOutcome: true,
		},
		{
			name: "no normal return",
			input: factflow.FactsInput{NoNormalReturns: map[cfg.Point]struct{}{
				point: struct{}{},
			}},
		},
		{
			name: "fixed call result value",
			input: factflow.FactsInput{CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
				point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, product.Top())),
			}},
		},
		{
			name: "channel select",
			input: factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
					SelectID: "select-1",
					Kind:     factflow.ChannelSelectSelect,
				})),
			}},
		},
		{
			name: "covariant exposure",
			input: factflow.FactsInput{CovariantExposures: map[cfg.Point][]factflow.CovariantExposure{
				point: []factflow.CovariantExposure{
					factflow.NewCovariantExposure(rootPath, product.Top(), factflow.CovariantExposureRecord),
				},
			}},
		},
		{
			name: "postcondition refinement",
			input: factflow.FactsInput{PostconditionRefinements: map[cfg.Point]factflow.PostconditionRefinementSet{
				point: factflow.NewPostconditionRefinementSet(
					factflow.NewPostconditionRefinement(rootPath, factflow.NewValueConstraint(product.Top())),
				),
			}},
		},
		{
			name: "postcondition path relation",
			input: factflow.FactsInput{PostconditionPathRelations: map[cfg.Point][]factflow.PostconditionPathRelation{
				point: {factflow.NewPostconditionPathEquality(rootPath, memberPath)},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &Result{facts: factflow.NewFacts(tc.input)}
			if tc.callOutcome {
				result.callOutcome = func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
					return callpayload.CallOutcome{}
				}
			}
			if !result.needsBoundaryNodeOutput(point) {
				t.Fatalf("needsBoundaryNodeOutput(%d) = false, want true", point)
			}
		})
	}
}

func TestNeedsBoundaryNodeOutputIgnoresStatementCallWithoutOutcome(t *testing.T) {
	point := cfg.Point(11)
	result := &Result{facts: factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		},
	})}
	if result.needsBoundaryNodeOutput(point) {
		t.Fatalf("needsBoundaryNodeOutput(%d) = true, want false for non-producer statement call without outcome provider", point)
	}
}

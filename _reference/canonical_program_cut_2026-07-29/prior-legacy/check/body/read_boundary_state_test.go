package body

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestBoundaryPathResolutionReturnsSealedValueDuringSelfReentry(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	sym := symbol.ID(19)
	p := path.NewPath(sym, "recursive")
	sealed := typevalue.FromType(reg, typ.String)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 19, HasExpr: true}
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "recursive")

	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		facts: factflow.NewFacts(factflow.FactsInput{PathAssignments: map[cfg.Point]factflow.PathAssignment{
			point: factflow.NewPathAssignment(p, source),
		}}),
		published: PublishedFacts{nodeOutputs: map[cfg.Point]state.State{
			point: state.State{}.WriteValue(reg, statekey.SymbolValue(sym), sealed),
		}},
	}
	if !result.needsBoundaryNodeOutput(point) {
		t.Fatal("path assignment must require the boundary node output")
	}
	key, ok := result.pathValueCacheKey(sourceValueReadBoundary, point, p)
	if !ok {
		t.Fatal("pathValueCacheKey returned false")
	}

	got, ok := result.queries.boundaryPathValue(key, func() (product.Value, bool) {
		return result.computePathValue(sourceValueReadBoundary, point, p, result.boundaryStateAt)
	}, func() (product.Value, bool) {
		// This mirrors a boundary path refinement whose source reads the same
		// boundary node output again. Before the cycle guard this re-entered
		// indefinitely because the cache was populated only after resolve.
		return result.queries.boundaryPathValue(key, func() (product.Value, bool) {
			t.Fatal("self-reentrant resolution must use the in-progress sealed value")
			return product.Value{}, false
		}, func() (product.Value, bool) {
			t.Fatal("self-reentrant resolution must not recompute")
			return product.Value{}, false
		})
	})
	if !ok {
		t.Fatal("boundaryPathValue returned false")
	}
	if !product.Equal(reg, got, sealed) {
		t.Fatalf("boundaryPathValue = %v, want sealed value %v", got, sealed)
	}
}

func TestNeedsBoundaryNodeOutputCoversNodeTransferFactLanes(t *testing.T) {
	point := cfg.Point(7)
	rootPath := path.NewPath(symbol.ID(1), "root")
	memberPath := rootPath.Field("field")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1), HasExpr: true}

	cases := []struct {
		name  string
		input factflow.FactsInput
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
				point: factflow.NewDynamicIndexWrite(factflow.NewDynamicIndexTarget(rootPath, source, nil), source, dynamicindex.AdmissionUnknown, factflow.DynamicIndexReadbackNone),
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
			if !result.needsBoundaryNodeOutput(point) {
				t.Fatalf("needsBoundaryNodeOutput(%d) = false, want true", point)
			}
		})
	}
}

func TestNeedsBoundaryNodeOutputIncludesEveryLexicalCall(t *testing.T) {
	point := cfg.Point(11)
	result := &Result{facts: factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		},
	})}
	if !result.needsBoundaryNodeOutput(point) {
		t.Fatalf("needsBoundaryNodeOutput(%d) = false, want true for exact call publication", point)
	}
}

package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFactsNodeTransferKeepsStaticMemberWritesDistinctFromPathAssignments(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(401)
	target := symbol.ID(401)
	targetPath := pathdom.NewPath(target, "table").Field("field")
	targetKey := pathdom.PathKey("sym401@1.field")
	assignmentSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(401), HasExpr: true}
	staticSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(402), HasExpr: true}
	assigned := presentValue(reg)
	proofValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			assignmentSource: assigned,
			staticSource:     proofValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	assignedState := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, assignmentSource),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, assignedState, targetKey, assigned)
	if got, ok := assignedState.ReadPathStaticMember(targetKey); ok {
		t.Fatalf("path assignment wrote static-member proof %s, want none", formatValue(reg, got))
	}

	staticState := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(targetPath, staticSource),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, staticState, targetKey, product.Bottom(reg))
	gotProof, ok := staticState.ReadPathStaticMember(targetKey)
	if !ok || !product.Equal(reg, gotProof, proofValue) {
		t.Fatalf("static-member proof = %s/%v, want %s/true", formatValue(reg, gotProof), ok, formatValue(reg, proofValue))
	}
}

func TestFactsNodeTransferAppliesDynamicIndexWriteKeyValueAdmission(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(402)
	table := symbol.ID(402)
	tablePath := pathdom.NewPath(table, "table").Field("items")
	tableKey := pathdom.PathKey("sym402@1.items")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(403), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(404), HasExpr: true}
	keyValue := presentValue(reg)
	writeValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   keyValue,
			valueSource: writeValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "table")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					factflow.DynamicIndexAdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	gotFact := got.ReadDynamicIndexFact(reg, state.DynamicIndexKey{Table: tableKey, Site: dynamicIndexSite(point)})
	if !presence.Equal(gotFact.KeyPresence, presence.Present()) ||
		!product.Equal(reg, gotFact.KeyValue, keyValue) ||
		!product.Equal(reg, gotFact.Value, writeValue) ||
		gotFact.Admission != state.DynamicIndexAdmissionAdmitted {
		t.Fatalf("dynamic-index fact = %#v, want key/value/admitted mapping", gotFact)
	}
	if len(sources.calls) != 2 || sources.calls[0].source != keySource || sources.calls[1].source != valueSource {
		t.Fatalf("dynamic-index source calls = %#v, want key then value", sources.calls)
	}
}

func TestFactsEdgeTransferAddsPointLevelBranchProofsOnBothBranchOutputs(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	err := symbol.ID(403)
	left := symbol.ID(404)
	right := symbol.ID(405)
	errPath := pathdom.NewPath(err, "err")
	leftPath := pathdom.NewPath(left, "left").Field("value")
	rightPath := pathdom.NewPath(right, "right").Field("value")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, err, "err")
	visibilityBuilder.Define(branch, left, "left")
	visibilityBuilder.Define(branch, right, "right")
	wantPresence := state.BranchProof{
		Kind:     state.BranchProofPathPresence,
		Path:     pathdom.PathKey("sym403@1"),
		Presence: presence.Present(),
	}
	wantEquality := state.BranchProof{
		Kind:  state.BranchProofPathEqual,
		Path:  pathdom.PathKey("sym404@1.value"),
		Other: pathdom.PathKey("sym405@1.value"),
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchProofs: map[cfg.Point]factflow.BranchProofSet{
					branch: factflow.NewBranchProofSet(
						factflow.NewBranchPathPresenceProof(errPath, presence.Present()),
						factflow.NewBranchPathEqualityProof(leftPath, rightPath),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	if !got[thenPoint].HasBranchProof(wantPresence) || !got[thenPoint].HasBranchProof(wantEquality) {
		t.Fatalf("true branch missing point-level branch proofs")
	}
	if !got[elsePoint].HasBranchProof(wantPresence) || !got[elsePoint].HasBranchProof(wantEquality) {
		t.Fatalf("false branch missing point-level branch proofs")
	}
}

func TestFactsNodeTransferAppliesChannelSelectFactsWithPathKeys(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(403)
	result := symbol.ID(406)
	selectedCase := symbol.ID(407)
	resultPath := pathdom.NewPath(result, "select").Field("result")
	casePath := pathdom.NewPath(selectedCase, "select").Field("case")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "select")
	visibilityBuilder.Define(point, selectedCase, "select")
	want := state.ChannelSelectFact{
		Select: state.ChannelSelectID("select-1"),
		Kind:   state.ChannelSelectFactReceive,
		Result: pathdom.PathKey("sym406@1.result"),
		Case:   pathdom.PathKey("sym407@1.case"),
		Index:  2,
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(factflow.NewChannelSelect(factflow.ChannelSelectConfig{
					SelectID:      factflow.ChannelSelectID("select-1"),
					Kind:          factflow.ChannelSelectReceive,
					ResultPath:    resultPath,
					HasResultPath: true,
					CasePath:      casePath,
					HasCasePath:   true,
					Index:         2,
				})),
			},
		}),
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if !got.HasChannelSelectFact(want) {
		t.Fatalf("channel-select fact missing: %#v", want)
	}
}

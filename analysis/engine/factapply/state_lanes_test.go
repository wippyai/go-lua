package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
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

	gotFact := got.ReadDynamicIndexFact(reg, dynamicindex.Key{Table: tableKey, Site: dynamicIndexSite(point)})
	if !presence.Equal(gotFact.KeyPresence, presence.Present()) ||
		!product.Equal(reg, gotFact.KeyValue, keyValue) ||
		!product.Equal(reg, gotFact.Value, writeValue) ||
		gotFact.Admission != dynamicindex.AdmissionAdmitted {
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
	wantPresence := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     pathdom.PathKey("sym403@1"),
		Presence: presence.Present(),
	}
	wantEquality := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
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

func TestFactsEdgeTransferBranchProofsRespectEdgesAndJoinByIntersection(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, join, false)
	graph.AddEdge(elsePoint, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	err := symbol.ID(430)
	left := symbol.ID(431)
	right := symbol.ID(432)
	errPath := pathdom.NewPath(err, "err")
	leftPath := pathdom.NewPath(left, "left").Field("value")
	rightPath := pathdom.NewPath(right, "right").Field("value")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, err, "err")
	visibilityBuilder.Define(branch, left, "left")
	visibilityBuilder.Define(branch, right, "right")
	oneSided := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     pathdom.PathKey("sym430@1"),
		Presence: presence.Present(),
	}
	twoSided := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  pathdom.PathKey("sym431@1.value"),
		Other: pathdom.PathKey("sym432@1.value"),
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchProofs: map[cfg.Point]factflow.BranchProofSet{
					branch: factflow.NewBranchProofSet(
						factflow.NewBranchPathPresenceProofOnEdge(errPath, presence.Present(), true),
						factflow.NewBranchPathEqualityProof(leftPath, rightPath),
					),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	if !got[thenPoint].HasBranchProof(oneSided) || !got[thenPoint].HasBranchProof(twoSided) {
		t.Fatalf("true branch proofs missing one-sided or two-sided proof")
	}
	if got[elsePoint].HasBranchProof(oneSided) {
		t.Fatalf("false branch kept true-edge-only proof")
	}
	if !got[elsePoint].HasBranchProof(twoSided) {
		t.Fatalf("false branch dropped two-sided proof")
	}
	if got[join].HasBranchProof(oneSided) {
		t.Fatalf("one-sided proof survived join")
	}
	if !got[join].HasBranchProof(twoSided) {
		t.Fatalf("two-sided proof did not survive join")
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

func TestFactsNodeTransferCallOutcomeRebasesPathRefinement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(501)
	arg := symbol.ID(501)
	argPath := pathdom.NewPath(arg, "arg")
	argFieldKey := pathdom.PathKey("sym501@1.field")
	placeholderKey := pathdom.NewPlaceholder(0).Field("field").Key()
	argExpr := factflow.ExprRef(501)
	refinement := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, arg, "arg")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				PathRefinements: []CallPathRefinement{
					{Path: pathdom.NewPlaceholder(0).Field("field"), Value: refinement},
				},
			}
		},
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WritePathKey(reg, argFieldKey, product.Top()))

	assertPathValue(t, reg, got, argFieldKey, refinement)
	assertPathValue(t, reg, got, placeholderKey, product.Bottom(reg))
}

func TestFactsNodeTransferStatementCallOutcomeDoesNotWriteReturnSlots(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(502)
	arg := symbol.ID(502)
	argPath := pathdom.NewPath(arg, "arg")
	argKey := pathdom.PathKey("sym502@1.side")
	argExpr := factflow.ExprRef(502)
	returnValue := absentValue(reg)
	sideValue := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, arg, "arg")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				Results: []CallResult{{Index: 0, Value: returnValue}},
				PathStaticMembers: []CallPathStaticMember{
					{Path: pathdom.NewPlaceholder(0).Field("side"), Value: sideValue},
				},
			}
		},
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue := got.ReadReturnSlot(reg, 0); !product.Equal(reg, gotValue, product.Bottom(reg)) {
		t.Fatalf("return slot 0 = %s, want bottom for statement call", formatValue(reg, gotValue))
	}
	if gotValue, ok := got.ReadPathStaticMember(argKey); !ok || !product.Equal(reg, gotValue, sideValue) {
		t.Fatalf("statement side fact = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, sideValue))
	}
}

func TestFactsNodeTransferCallOutcomeBindsReceiverBeforeExplicitArgs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(503)
	receiver := symbol.ID(503)
	arg := symbol.ID(504)
	receiverPath := pathdom.NewPath(receiver, "receiver")
	argPath := pathdom.NewPath(arg, "arg")
	receiverKey := pathdom.PathKey("sym503@1.self")
	argKey := pathdom.PathKey("sym504@1.value")
	argExpr := factflow.ExprRef(503)
	receiverValue := presentValue(reg)
	argValue := absentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, receiver, "receiver")
	visibilityBuilder.Define(point, arg, "arg")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:         factflow.CallSiteContextStatement,
					ReceiverPath:    receiverPath,
					HasReceiverPath: true,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				PathStaticMembers: []CallPathStaticMember{
					{Path: pathdom.NewPlaceholder(0).Field("self"), Value: receiverValue},
					{Path: pathdom.NewPlaceholder(1).Field("value"), Value: argValue},
				},
			}
		},
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue, ok := got.ReadPathStaticMember(receiverKey); !ok || !product.Equal(reg, gotValue, receiverValue) {
		t.Fatalf("receiver static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, receiverValue))
	}
	if gotValue, ok := got.ReadPathStaticMember(argKey); !ok || !product.Equal(reg, gotValue, argValue) {
		t.Fatalf("arg static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, argValue))
	}
}

func TestFactsNodeTransferCallOutcomeRebasesStateLaneFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(504)
	first := symbol.ID(505)
	second := symbol.ID(506)
	firstPath := pathdom.NewPath(first, "first")
	secondPath := pathdom.NewPath(second, "second")
	firstExpr := factflow.ExprRef(504)
	secondExpr := factflow.ExprRef(505)
	present := presentValue(reg)
	absent := absentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, first, "first")
	visibilityBuilder.Define(point, second, "second")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: firstExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: secondExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				firstExpr:  firstPath,
				secondExpr: secondPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) CallOutcome {
			return CallOutcome{
				DynamicIndexFacts: []CallDynamicIndexFact{
					{
						Table: pathdom.NewPlaceholder(0).Field("items"),
						Site:  "callee.dynamic",
						Value: dynamicindex.Fact{
							KeyPresence: presence.Present(),
							KeyValue:    present,
							Value:       absent,
							Admission:   dynamicindex.AdmissionAdmitted,
						},
					},
				},
				BranchProofs: []CallBranchProof{
					{
						Kind:  CallBranchProofPathEqual,
						Path:  pathdom.NewPlaceholder(0).Field("left"),
						Other: pathdom.NewPlaceholder(1).Field("right"),
					},
				},
				ChannelSelects: []CallChannelSelectFact{
					{
						Select: "callee.select",
						Kind:   CallChannelSelectFactReceive,
						Result: pathdom.NewPlaceholder(0).Field("result"),
						Case:   pathdom.NewPlaceholder(1).Field("case"),
						Index:  3,
					},
				},
				EffectDeltas: []CallEffectDelta{
					{
						Target: pathdom.NewPlaceholder(0).Field("items"),
						Site:   "callee.effect",
						Kind:   effectdelta.Mutation,
						Value: effectdelta.Value{
							Before: present,
							After:  absent,
							Change: effectdelta.ChangeChanged,
						},
					},
				},
			}
		},
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	dynamicKey := dynamicindex.Key{Table: pathdom.PathKey("sym505@1.items"), Site: dynamicindex.Site("callee.dynamic")}
	gotDynamic := got.ReadDynamicIndexFact(reg, dynamicKey)
	if !presence.Equal(gotDynamic.KeyPresence, presence.Present()) ||
		!product.Equal(reg, gotDynamic.KeyValue, present) ||
		!product.Equal(reg, gotDynamic.Value, absent) ||
		gotDynamic.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic-index fact = %#v, want rebased fact", gotDynamic)
	}

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  pathdom.PathKey("sym505@1.left"),
		Other: pathdom.PathKey("sym506@1.right"),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("branch proof missing: %#v", proof)
	}

	selectFact := state.ChannelSelectFact{
		Select: state.ChannelSelectID("callee.select"),
		Kind:   state.ChannelSelectFactReceive,
		Result: pathdom.PathKey("sym505@1.result"),
		Case:   pathdom.PathKey("sym506@1.case"),
		Index:  3,
	}
	if !got.HasChannelSelectFact(selectFact) {
		t.Fatalf("channel-select fact missing: %#v", selectFact)
	}

	effectKey := effectdelta.Key{
		Target: pathdom.PathKey("sym505@1.items"),
		Site:   "callee.effect",
		Kind:   effectdelta.Mutation,
	}
	gotEffect := got.ReadEffectDelta(effectKey)
	if !product.Equal(reg, gotEffect.Before, present) ||
		!product.Equal(reg, gotEffect.After, absent) ||
		gotEffect.Change != effectdelta.ChangeChanged {
		t.Fatalf("effect delta = %#v, want rebased delta", gotEffect)
	}
}

package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	ks := resolver.KeySpace()

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

	assertPathValue(t, reg, ks, assignedState, targetKey, assigned)
	if got, ok := assignedState.ReadPathStaticMember(ks, targetKey); ok {
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

	assertPathValue(t, reg, ks, staticState, targetKey, product.Bottom(reg))
	gotProof, ok := staticState.ReadPathStaticMember(ks, targetKey)
	if !ok || !product.Equal(reg, gotProof, proofValue) {
		t.Fatalf("static-member proof = %s/%v, want %s/true", formatValue(reg, gotProof), ok, formatValue(reg, proofValue))
	}
}

func TestFactsNodeTransferKeepsSamePointStaticMemberWriteWithPathAssignment(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4011)
	target := symbol.ID(4011)
	targetPath := pathdom.NewPath(target, "provider").IndexInt(1)
	targetKey := pathdom.PathKey("sym4011@1[1]")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4011), HasExpr: true}
	value := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: value},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "provider")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, ks, got, targetKey, value)
	gotProof, ok := got.ReadPathStaticMember(ks, targetKey)
	if !ok || !product.Equal(reg, gotProof, value) {
		t.Fatalf("same-point static-member proof = %s/%v, want %s/true", formatValue(reg, gotProof), ok, formatValue(reg, value))
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	gotFact := got.ReadDynamicIndexFact(reg, dynamicindex.Key{Table: mustStateKey(t, resolver.KeySpace(), tableKey), Site: dynamicindex.SiteForPoint(int(point))})
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

func TestFactsNodeTransferDynamicIndexWritePublishesFirstHeapDynamicFact(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(403)
	table := symbol.ID(403)
	tablePath := pathdom.NewPath(table, "table")
	tableKey := pathdom.PathKey("sym403@1")
	tableID := identity.ID{Kind: "test.table", Site: "dynamic", Index: 1}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(405), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(406), HasExpr: true}
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(table), tableValue).
		WriteHeapTableObject(reg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableValue})))

	dynamicKey := dynamicindex.Key{Table: mustStateKey(t, resolver.KeySpace(), tableKey), Site: dynamicindex.SiteForPoint(int(point))}
	object := got.ReadHeapTableObject(reg, tableID)
	heapFact, ok := object.DynamicIndexFact(dynamicKey)
	if !ok {
		t.Fatalf("heap dynamic fact missing for %v", dynamicKey)
	}
	if !presence.Equal(heapFact.KeyPresence, presence.Present()) ||
		!product.Equal(reg, heapFact.KeyValue, keyValue) ||
		!product.Equal(reg, heapFact.Value, writeValue) ||
		heapFact.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("heap dynamic-index fact = %#v, want key/value/admitted mapping", heapFact)
	}
}

func TestResolvePathValueReadsHeapDynamicFactAcrossPathKeyContexts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(404)
	root := symbol.ID(404)
	rootPath := pathdom.NewPath(root, "batch")
	itemsPath := rootPath.Field("items")
	itemPath := itemsPath.IndexStr("route-1")
	rootID := identity.ID{Kind: "test.table", Site: "root", Index: 1}
	itemsID := identity.ID{Kind: "test.table", Site: "items", Index: 1}
	itemID := identity.ID{Kind: "test.table", Site: "item", Index: 1}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	routeKeyType := typ.LiteralString("route-1")
	routeKeyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, routeKeyType), routeKeyType)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, root, "batch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	itemsKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
	}
	oldDynamicKey := dynamicindex.Key{
		Table: mustStateKey(t, ks, pathdom.PathKey("callee.items")),
		Site:  dynamicindex.Site("callee.write"),
	}
	st := state.State{}.
		WriteValue(reg, key.SymbolValue(root), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemsValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				oldDynamicKey: {
					KeyPresence: presence.Present(),
					KeyValue:    routeKeyValue,
					Value:       itemValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		}))

	got, ok := resolvePathValueAt(reg, resolver, point, st, itemPath, nil)
	if !ok {
		t.Fatalf("resolvePathValueAt(%s) returned false", itemPath)
	}
	if !product.Equal(reg, got.value, itemValue) {
		t.Fatalf("resolved value = %s, want item identity", formatValue(reg, got.value))
	}
}

func TestDynamicIndexStaticProjectionRequiresExactKey(t *testing.T) {
	reg := standard.Registry()
	exactType := typ.LiteralString("value")
	exact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    typevalue.WithWitness(reg, typevalue.FromType(reg, exactType), exactType),
		Value:       presentValue(reg),
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	broad := exact
	broad.KeyValue = typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	wrongType := typ.LiteralString("other")
	wrong := exact
	wrong.KeyValue = typevalue.WithWitness(reg, typevalue.FromType(reg, wrongType), wrongType)

	seg := fieldSuffix("value").Segments[0]
	if !dynamicIndexFactDefinitelyMatchesSegment(reg, exact, seg) {
		t.Fatalf("exact literal key did not prove static segment")
	}
	if dynamicIndexFactDefinitelyMatchesSegment(reg, broad, seg) {
		t.Fatalf("broad string key proved static segment")
	}
	if dynamicIndexFactDefinitelyMatchesSegment(reg, wrong, seg) {
		t.Fatalf("wrong literal key proved static segment")
	}
}

func TestFactsEdgeTransferAddsPointLevelBranchPathEvidenceOnBothBranchOutputs(t *testing.T) {
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	wantPresence := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     mustStateKey(t, ks, pathdom.PathKey("sym403@1")),
		Presence: presence.Present(),
	}
	wantEquality := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey("sym404@1.value")),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym405@1.value")),
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathPresenceEvidence(errPath, presence.Present()),
						factflow.NewBranchPathEqualityEvidence(leftPath, rightPath),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	if !got[thenPoint].HasBranchProof(wantPresence) || !got[thenPoint].HasBranchProof(wantEquality) {
		t.Fatalf("true branch missing point-level branch proofs")
	}
	if !got[elsePoint].HasBranchProof(wantPresence) || !got[elsePoint].HasBranchProof(wantEquality) {
		t.Fatalf("false branch missing point-level branch proofs")
	}
}

func TestFactsEdgeTransferBranchPathEvidenceRespectEdgesAndJoinByIntersection(t *testing.T) {
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	oneSided := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     mustStateKey(t, ks, pathdom.PathKey("sym430@1")),
		Presence: presence.Present(),
	}
	twoSided := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey("sym431@1.value")),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym432@1.value")),
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathPresenceEvidenceOnEdge(errPath, presence.Present(), true),
						factflow.NewBranchPathEqualityEvidence(leftPath, rightPath),
					),
				},
			}),
			Visibility: resolver,
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
	want := channelselectfact.Fact{
		Select: channelselectfact.ID("select-1"),
		Kind:   channelselectfact.FactReceive,
		Result: pathdom.PathKey("sym406@1.result"),
		Case:   pathdom.PathKey("sym407@1.case"),
		Index:  2,
	}
	wantSelect := channelselectfact.Fact{
		Select:     channelselectfact.ID("select-1"),
		Kind:       channelselectfact.FactSelect,
		Result:     pathdom.PathKey("sym406@1.result"),
		Index:      0,
		HasDefault: true,
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      factflow.ChannelSelectID("select-1"),
						Kind:          factflow.ChannelSelectSelect,
						ResultPath:    resultPath,
						HasResultPath: true,
						HasDefault:    true,
						Index:         0,
					}),
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      factflow.ChannelSelectID("select-1"),
						Kind:          factflow.ChannelSelectReceive,
						ResultPath:    resultPath,
						HasResultPath: true,
						CasePath:      casePath,
						HasCasePath:   true,
						Index:         2,
					}),
				),
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
	if !got.HasChannelSelectFact(wantSelect) {
		t.Fatalf("channel-select default fact missing: %#v", wantSelect)
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathRefinements: []callboundary.PathValueFact{
						{Path: pathdom.NewPlaceholder(0).Field("field"), Value: refinement},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WritePathKey(reg, ks, argFieldKey, product.Top()))

	assertPathValue(t, reg, ks, got, argFieldKey, refinement)
	assertPathValue(t, reg, ks, got, placeholderKey, product.Bottom(reg))
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				Results: []callpayload.CallResult{{Index: 0, Value: returnValue}},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathStaticMembers: []callboundary.PathStaticMemberFact{
						{Path: pathdom.NewPlaceholder(0).Field("side"), Value: sideValue},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue := got.ReadReturnSlot(reg, 0); !product.Equal(reg, gotValue, product.Bottom(reg)) {
		t.Fatalf("return slot 0 = %s, want bottom for statement call", formatValue(reg, gotValue))
	}
	if gotValue, ok := got.ReadPathStaticMember(ks, argKey); !ok || !product.Equal(reg, gotValue, sideValue) {
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathStaticMembers: []callboundary.PathStaticMemberFact{
						{Path: pathdom.NewPlaceholder(0).Field("self"), Value: receiverValue},
						{Path: pathdom.NewPlaceholder(1).Field("value"), Value: argValue},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue, ok := got.ReadPathStaticMember(ks, receiverKey); !ok || !product.Equal(reg, gotValue, receiverValue) {
		t.Fatalf("receiver static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, receiverValue))
	}
	if gotValue, ok := got.ReadPathStaticMember(ks, argKey); !ok || !product.Equal(reg, gotValue, argValue) {
		t.Fatalf("arg static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, argValue))
	}
}

func TestFactsNodeTransferCallOutcomeRebasesBoundaryFacts(t *testing.T) {
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					DynamicIndexFacts: []callboundary.DynamicIndexFact{
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
					BranchProofs: []callboundary.BranchProof{
						{
							Kind:  pathevidence.BranchProofPathEqual,
							Path:  pathdom.NewPlaceholder(0).Field("left"),
							Other: pathdom.NewPlaceholder(1).Field("right"),
						},
					},
					ChannelSelects: []callboundary.ChannelSelectFact{
						{
							Select: channelselectfact.ID("callee.select"),
							Kind:   channelselectfact.FactReceive,
							Result: pathdom.NewPlaceholder(0).Field("result"),
							Case:   pathdom.NewPlaceholder(1).Field("case"),
							Index:  3,
						},
					},
					EffectDeltas: []callboundary.EffectDelta{
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
					EscapeEvents: []callboundary.EscapeEventFact{
						{
							Target:    pathdom.NewPlaceholder(0).Field("sent"),
							Kind:      callboundary.EscapeEventSend,
							Recursive: true,
						},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	dynamicKey := dynamicindex.Key{Table: mustStateKey(t, ks, pathdom.PathKey("sym505@1.items")), Site: dynamicindex.Site("callee.dynamic")}
	gotDynamic := got.ReadDynamicIndexFact(reg, dynamicKey)
	if !presence.Equal(gotDynamic.KeyPresence, presence.Present()) ||
		!product.Equal(reg, gotDynamic.KeyValue, present) ||
		!product.Equal(reg, gotDynamic.Value, absent) ||
		gotDynamic.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic-index fact = %#v, want rebased fact", gotDynamic)
	}

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey("sym505@1.left")),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym506@1.right")),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("branch proof missing: %#v", proof)
	}

	selectFact := channelselectfact.Fact{
		Select: channelselectfact.ID("callee.select"),
		Kind:   channelselectfact.FactReceive,
		Result: pathdom.PathKey("sym505@1.result"),
		Case:   pathdom.PathKey("sym506@1.case"),
		Index:  3,
	}
	if !got.HasChannelSelectFact(selectFact) {
		t.Fatalf("channel-select fact missing: %#v", selectFact)
	}

	effectKey := effectdelta.Key{
		Target: mustStateKey(t, ks, pathdom.PathKey("sym505@1.items")),
		Site:   "callee.effect",
		Kind:   effectdelta.Mutation,
	}
	gotEffect := got.ReadEffectDelta(effectKey)
	if !product.Equal(reg, gotEffect.Before, present) ||
		!product.Equal(reg, gotEffect.After, absent) ||
		gotEffect.Change != effectdelta.ChangeChanged {
		t.Fatalf("effect delta = %#v, want rebased delta", gotEffect)
	}

	escapeKey := effectdelta.Key{
		Target: mustStateKey(t, ks, pathdom.PathKey("sym505@1.sent")),
		Site:   callboundary.EscapeEventEffectSite(callboundary.EscapeEventSend, true),
		Kind:   effectdelta.Escape,
	}
	gotEscape := got.ReadEffectDelta(escapeKey)
	if gotEscape.Change != effectdelta.ChangeUnknown {
		t.Fatalf("escape event delta = %#v, want rebased escape event", gotEscape)
	}
}

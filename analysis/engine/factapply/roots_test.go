package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFactsNodeTransferAppliesLocalAssignmentThroughResolver(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(10), HasExpr: true}
	target := symbol.ID(101)
	assigned := presentValue(reg)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[assign], key.SymbolValue(target), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferAppliesOrdinaryAssignmentThroughResolver(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(11), HasExpr: true}
	target := symbol.ID(102)
	assigned := absentValue(reg)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "ordinary"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestRootAssignmentRefinesGuardedStaticPathSourcePresence(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	sourceExpr := factflow.ExprRef(120)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true}
	target := symbol.ID(1201)
	current := symbol.ID(1202)
	currentPath := path.NewPath(current, "current")
	sourcePath := currentPath.Field("next")
	sourceValue := product.Join(reg, typevalue.FromType(reg, typ.String), nilSourceValue(reg))
	resolverValues := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: sourceValue},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(assign, target, "current")
	visibilityBuilder.Define(assign, current, "current")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	sourceKey, ok := visibility.AddressAt(resolver, assign, sourcePath).VisibleLocalKeyspaceKey()
	if !ok {
		t.Fatal("missing source path key")
	}
	in := state.State{}.AddBranchProof(pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     sourceKey,
		Presence: presence.Present(),
	})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "current"), source),
			},
			ExpressionPaths: map[factflow.ExprRef]path.Path{
				sourceExpr: sourcePath,
			},
		}),
		Sources:    resolverValues,
		Visibility: resolver,
	})(transfer.NodeContext{
		Graph:    graph,
		Registry: reg,
		Point:    assign,
		Node:     graph.Node(assign),
	}, in)

	written := got.ReadValue(reg, key.SymbolValue(target))
	if gotPresence := product.PresenceOf(written); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("written presence = %s in %s, want present", gotPresence, formatValue(reg, written))
	}
	writtenType, ok := typevalue.TypeOf(reg, written)
	if !ok {
		t.Fatalf("written value has no projected type: %s", formatValue(reg, written))
	}
	if typevalue.ProjectionHasNil(writtenType) {
		t.Fatalf("written value type = %s, want nil removed", writtenType)
	}
	assertResolverCall(t, resolverValues, assign, source)
}

func TestFactsNodeTransferFreshContainerRootSeedsClosedDynamicAllValueInvariant(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1111)
	container := symbol.ID(1111)
	table := symbol.ID(1112)
	containerPath := path.NewPath(container, "channel_to_id")
	tablePath := path.NewPath(table, "registered_channels")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1111), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(source.ExprRef)))
	sources := &recordingSourceValues{values: map[factflow.ValueSource]product.Value{source: rootValue}}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, container, "channel_to_id")
	visibilityBuilder.Define(point, table, "registered_channels")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	containerKey := resolver.KeySpace().FromPath(containerPath)
	tableStateKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, point, tablePath)
	if !ok {
		t.Fatal("missing table state key")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, container, containerPath, source),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				source.ExprRef: factflow.NewObjectLiteral(nil),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
		ClosedDynamicAllValues: []ClosedDynamicAllValueInvariant{
			{Container: containerPath, Table: tablePath},
		},
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	tables := got.DynamicIndexAllValuesKeyMembershipTables(containerKey)
	if len(tables) != 1 || tables[0] != tableStateKey {
		t.Fatalf("all-value memberships = %#v, want %s", tables, tableStateKey)
	}
}

func TestFactsNodeTransferRootAssignmentAddsPathEqualityProofForPathSource(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(12), HasExpr: true}
	target := symbol.ID(112)
	sourceSymbol := symbol.ID(113)
	assigned := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "alias")
	visibilityBuilder.Define(point, sourceSymbol, "box")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "alias"), source),
			},
			ExpressionPaths: map[factflow.ExprRef]path.Path{
				source.ExprRef: path.NewPath(sourceSymbol, "box"),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, path.PathKey("sym112@1")),
		Other: mustStateKey(t, ks, path.PathKey("sym113@1")),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("missing path equality proof %#v", proof)
	}
}

func TestFactsNodeTransferRootAssignmentAddsPathEqualityProofForWIRPathSource(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(120)
	target := symbol.ID(120)
	sourceSymbol := symbol.ID(121)
	targetPath := path.NewPath(target, "alias")
	sourcePath := path.NewPath(sourceSymbol, "box")
	source, ok := factflow.NewPathValueSource(sourcePath.Key(), 0, 0, 0, factflow.ValueSourceShape{})
	if !ok {
		t.Fatalf("NewPathValueSource(%q) failed", sourcePath.Key())
	}
	assigned := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "alias")
	visibilityBuilder.Define(point, sourceSymbol, "box")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, path.PathKey("sym120@1")),
		Other: mustStateKey(t, ks, path.PathKey("sym121@1")),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("missing path equality proof %#v", proof)
	}
}

func TestFactsNodeTransferRootAssignmentAddsPathEqualityProofForKnownDynamicIndexSource(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(121)
	target := symbol.ID(121)
	table := symbol.ID(122)
	keyExpr := factflow.ExprRef(121)
	sourceExpr := factflow.ExprRef(122)
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true}
	keyType := typ.LiteralString("tx")
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, keyType), keyType)
	assigned := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource: keyValue,
			source:    assigned,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "alias")
	visibilityBuilder.Define(point, table, "holder")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	dyn, ok := factflow.NewDynamicIndexExpression(path.NewPath(table, "holder"), keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "alias"), source),
			},
			DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
				sourceExpr: dyn,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, path.PathKey("sym121@1")),
		Other: mustStateKey(t, ks, path.PathKey(`sym122@1["tx"]`)),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("missing dynamic-index source path equality proof %#v", proof)
	}
}

func TestFactsNodeTransferRootAssignmentPropagatesNumericFloorThroughIncrement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	target := symbol.ID(113)
	targetPath := path.NewPath(target, "i")
	leftRef := factflow.ExprRef(131)
	oneRef := factflow.ExprRef(132)
	incRef := factflow.ExprRef(133)
	left := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: leftRef, HasExpr: true}
	one := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: oneRef, HasExpr: true}
	inc := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: incRef, HasExpr: true}
	op, ok := factflow.NewBinaryExpressionOperation("+", left, one)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	oneType := typ.LiteralInt(1)
	oneValue := typevalue.WithWitness(reg, typevalue.FromType(reg, oneType), oneType)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "i")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	targetKey, targetKeyOK := visibility.RootOrVisibleStateKeyAt(resolver, point, targetPath)
	if !targetKeyOK {
		t.Fatal("RootOrVisibleStateKeyAt(target) failed")
	}
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{inc: typevalue.FromType(reg, typ.Integer)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, inc),
			},
			ExpressionPaths: map[factflow.ExprRef]path.Path{
				leftRef: targetPath,
			},
			ExpressionValues: map[factflow.ExprRef]product.Value{
				oneRef: oneValue,
			},
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{
				incRef: op,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteNumFloor(ks, targetKey, 1))

	floor, ok := got.ReadNumFloor(ks, targetKey)
	if !ok || floor != 2 {
		t.Fatalf("numeric floor for i = %d/%v, want 2/true", floor, ok)
	}
}

func TestFactsNodeTransferRootAssignmentClearsNumericFloorWhenSourceIsUnresolved(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	target := symbol.ID(114)
	targetPath := path.NewPath(target, "i")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(141), HasExpr: true}
	declared := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "i")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	targetKey, targetKeyOK := visibility.RootOrVisibleStateKeyAt(resolver, point, targetPath)
	if !targetKeyOK {
		t.Fatal("RootOrVisibleStateKeyAt(target) failed")
	}
	sources := &recordingSourceValues{}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, target, targetPath, source, declared),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteNumFloor(ks, targetKey, 9))

	if floor, ok := got.ReadNumFloor(ks, targetKey); ok || floor != 0 {
		t.Fatalf("numeric floor for i = %d/%v, want 0/false", floor, ok)
	}
	assertValue(t, reg, got, key.SymbolValue(target), declared)
	assertResolverCall(t, sources, point, source)
}

func TestFactsNodeTransferRootAssignmentUsesDeclaredContractBeforeSource(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(12), HasExpr: true}
	target := symbol.ID(103)
	declared := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.String))

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: panicSourceValues{},
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), declared)
	assertRuntimeKind(t, reg, got[graph.Exit()].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.String))
}

func TestFactsNodeTransferDeclaredContractPreservesObjectLiteralIdentity(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(121), HasExpr: true}
	target := symbol.ID(121)
	tableID := identity.ID{Kind: "test.table", Site: "declared-contract", Index: 1}
	sourceType := typetable.NewRecord().Build()
	declaredType := typetable.NewRecord().Field("items", typetable.NewMap(typ.String, typ.String)).Build()
	assigned := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, sourceType), sourceType), identity.Key, identity.Singleton(tableID))
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
				ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
					source.ExprRef: factflow.NewObjectLiteral(nil),
				},
			}),
			Sources: resolver,
		}),
	})

	written := got[graph.Exit()].ReadValue(reg, key.SymbolValue(target))
	writtenType, ok := typevalue.TypeOf(reg, written)
	if !ok || !typ.TypeEquals(writtenType, declaredType) {
		t.Fatalf("written type = %v/%v, want declared contract %v", writtenType, ok, declaredType)
	}
	if gotID, ok := identityvalue.ExactID(reg, written); !ok || gotID != tableID {
		t.Fatalf("written identity = %v/%v, want source table identity %v", gotID, ok, tableID)
	}
}

func TestFactsNodeTransferRootAssignmentOverlayUsesSourceBeforeDeclaredFallback(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(13), HasExpr: true}
	target := symbol.ID(104)
	declared := product.Top()
	assigned := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertRuntimeKind(t, reg, got[graph.Exit()].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Number))
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferRootAssignmentOverlayUsesDeclaredValueWhenSourceIsBottom(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceCall, ExprRef: factflow.ExprRef(14), HasExpr: true}
	target := symbol.ID(105)
	declared := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: product.Bottom(reg)},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), declared)
	assertRuntimeKind(t, reg, got[graph.Exit()].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.String))
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferOrdinaryRootWriteFromAnyOriginTableSourceOverwritesAny(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1301), HasExpr: true}
	target := symbol.ID(1041)
	assigned := product.Set(reg, typevalue.FromType(reg, typetable.BuiltinTopMarker()), evidence.Key, evidence.ExplicitTop())
	assigned = typevalue.WithWitness(reg, assigned, typetable.BuiltinTopMarker())
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), typevalue.FromType(reg, typ.Any)),
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "bindings"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	written := got[graph.Exit()].ReadValue(reg, key.SymbolValue(target))
	writtenType, ok := typevalue.TypeOf(reg, written)
	if !ok || !typ.TypeEquals(writtenType, typetable.BuiltinTopMarker()) {
		t.Fatalf("written type = %v/%v in %s, want builtin table marker", writtenType, ok, formatValue(reg, written))
	}
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferRootAssignmentOverlaysDeclaredContractOnSource(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(14), HasExpr: true}
	target := symbol.ID(105)
	sourceType := typetable.NewRecord().Field("id", typ.LiteralString("u1")).Build()
	declaredType := typetable.NewRecord().Field("id", typ.String).Field("name", typ.String).Build()
	assigned := typevalue.WithWitness(reg, typevalue.FromType(reg, sourceType), sourceType)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: resolver,
		}),
	})

	written := got[graph.Exit()].ReadValue(reg, key.SymbolValue(target))
	writtenType, ok := typevalue.TypeOf(reg, written)
	if !ok || !typ.TypeEquals(writtenType, declaredType) {
		t.Fatalf("written type = %v/%v, want declared contract %v", writtenType, ok, declaredType)
	}
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferRootAssignmentOverlayCarriesDeclaredTypeClaim(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(114), HasExpr: true}
	target := symbol.ID(115)
	tableID := identity.ID{Kind: "test.table", Site: "declared-overlay-claim", Index: 1}
	sourceType := typetable.NewRecord().Build()
	declaredType := typ.NewArray(typetable.NewRecord().Field("role", typ.LiteralString("system")).Build())
	assigned := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, sourceType), sourceType), identity.Key, identity.Singleton(tableID))
	declared := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType), assertion.Key, assertion.Type())
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: resolver,
		}),
	})

	written := got[graph.Exit()].ReadValue(reg, key.SymbolValue(target))
	if gotID, ok := identityvalue.ExactID(reg, written); !ok || gotID != tableID {
		t.Fatalf("written identity = %v/%v, want preserved exact table identity %v", gotID, ok, tableID)
	}
	if gotClaim := product.Get(reg, written, assertion.Key); !gotClaim.Has(assertion.TypeClaim) {
		t.Fatalf("written assertion = %s, want declared type claim", gotClaim)
	}
}

func TestFactsNodeTransferRootAssignmentOverlayAdoptsDeclaredPresenceAndKeepsEvidence(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(15), HasExpr: true}
	target := symbol.ID(106)
	assigned := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()), evidence.Key, evidence.ExplicitTop())
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	resolver := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "local"), source, declared),
				},
			}),
			Sources: resolver,
		}),
	})

	value := got[graph.Exit()].ReadValue(reg, key.SymbolValue(target))
	if gotPresence := product.PresenceOf(value); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present from declared overlay", gotPresence)
	}
	if gotEvidence := product.Get(reg, value, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("evidence = %s, want explicit-top source evidence preserved", gotEvidence)
	}
}

func TestFactsNodeTransferRootAssignmentInvalidatesVisiblePathSubtree(t *testing.T) {
	tests := []struct {
		name string
		fact func(cfg.Point, symbol.ID, factflow.ValueSource) factflow.FactsInput
	}{
		{
			name: "local",
			fact: func(point cfg.Point, target symbol.ID, source factflow.ValueSource) factflow.FactsInput {
				return factflow.FactsInput{
					RootAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
		{
			name: "ordinary",
			fact: func(point cfg.Point, target symbol.ID, source factflow.ValueSource) factflow.FactsInput {
				return factflow.FactsInput{
					RootAssignments: map[cfg.Point]factflow.RootAssignment{
						point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(60)
			target := symbol.ID(120)
			source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(60), HasExpr: true}
			assigned := absentValue(reg)
			stale := presentValue(reg)
			rootKey := path.PathKey("sym120@1")
			childKey := path.PathKey("sym120@1.field")
			deepKey := path.PathKey("sym120@1.field.deep")
			otherVersionKey := path.PathKey("sym120@2.field")
			otherSymbolKey := path.PathKey("sym121@1.field")
			sources := &recordingSourceValues{
				values: map[factflow.ValueSource]product.Value{source: assigned},
			}
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(point, target, "obj")
			resolver := visibility.NewResolver(visibilityBuilder.Build())
			ks := resolver.KeySpace()

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      factflow.NewFacts(tc.fact(point, target, source)),
				Sources:    sources,
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.
				WritePathKey(reg, ks, rootKey, stale).
				WritePathKey(reg, ks, childKey, stale).
				WritePathKey(reg, ks, deepKey, stale).
				WritePathKey(reg, ks, otherVersionKey, stale).
				WritePathKey(reg, ks, otherSymbolKey, stale))

			assertValue(t, reg, got, key.SymbolValue(target), assigned)
			assertPathValue(t, reg, ks, got, rootKey, product.Bottom(reg))
			assertPathValue(t, reg, ks, got, childKey, product.Bottom(reg))
			assertPathValue(t, reg, ks, got, deepKey, product.Bottom(reg))
			assertPathValue(t, reg, ks, got, otherVersionKey, stale)
			assertPathValue(t, reg, ks, got, otherSymbolKey, stale)
			assertResolverCall(t, sources, point, source)
		})
	}
}

func TestFactsNodeTransferRootAssignmentInvalidatesTargetImplication(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(61)
	trigger := symbol.ID(161)
	target := symbol.ID(162)
	triggerPath := path.NewPath(trigger, "use_template")
	targetPath := path.NewPath(target, "executor")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(61), HasExpr: true}
	present := presentValue(reg)
	absent := absentValue(reg)

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, trigger, "use_template")
	visibilityBuilder.Define(point, target, "executor")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	triggerKey, triggerOK := factKeyspaceKeyAt(resolver, point, triggerPath)
	targetKey, targetOK := factKeyspaceKeyAt(resolver, point, targetPath)
	if !triggerOK || !targetOK {
		t.Fatalf("missing keyspace keys: trigger=%v target=%v", triggerOK, targetOK)
	}
	implication := pathevidence.NewPathValuePresenceImplication(
		triggerKey,
		typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False),
		targetKey,
		presence.Present(),
	)

	run := func(assigned product.Value) state.State {
		sources := &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{source: assigned},
		}
		return NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					point: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, source),
				},
			}),
			Sources:    sources,
			Visibility: resolver,
		})(transfer.NodeContext{
			Registry: reg,
			Point:    point,
		}, state.State{}.AddPathPresenceImplication(implication))
	}

	if got := run(present); got.HasPathPresenceImplication(implication) {
		t.Fatalf("present root assignment preserved stale target path-presence implication")
	}
	if got := run(absent); got.HasPathPresenceImplication(implication) {
		t.Fatalf("absent root assignment preserved invalid path-presence implication")
	}
}

func TestPathPresenceImplicationActivationAcceptsRootTarget(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(62)
	trigger := symbol.ID(261)
	target := symbol.ID(262)
	triggerPath := path.NewPath(trigger, "use_template")
	targetPath := path.NewPath(target, "executor")

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, trigger, "use_template")
	visibilityBuilder.Define(point, target, "executor")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	triggerKey := resolver.KeySpace().FromPath(triggerPath)
	targetKey := resolver.KeySpace().FromPath(targetPath)
	falseValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(trigger), falseValue).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathValuePresenceImplication(
			triggerKey,
			falseValue,
			targetKey,
			presence.Present(),
		))
	got := activatePathPresenceImplications(reg, resolver, point, in)

	assertValue(t, reg, got, key.SymbolValue(target), presentValue(reg))
}

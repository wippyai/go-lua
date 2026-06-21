package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFactsNodeTransferAppliesPathAssignmentThroughVisibility(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(15), HasExpr: true}
	target := symbol.ID(106)
	targetPath := path.NewPath(target, "table").Field("field")
	assigned := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(assign, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				PathAssignments: map[cfg.Point]factflow.PathAssignment{
					assign: factflow.NewPathAssignment(targetPath, source),
				},
			}),
			Sources:    sources,
			Visibility: resolver,
		}),
	})

	assertPathValue(t, reg, ks, got[assign], path.PathKey("sym106@1.field"), product.Bottom(reg))
	assertPathValue(t, reg, ks, got[graph.Exit()], path.PathKey("sym106@1.field"), assigned)
	assertResolverCall(t, sources, assign, source)
}

func TestFactsNodeTransferPathAssignmentInvalidatesSubtreeBeforeWriting(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(16)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(16), HasExpr: true}
	target := symbol.ID(107)
	targetPath := path.NewPath(target, "table").Field("field")
	childKey := path.PathKey("sym107@1.field.deep")
	siblingKey := path.PathKey("sym107@1.other")
	assigned := absentValue(reg)
	present := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	in := state.State{}.
		WritePathKey(reg, ks, childKey, present).
		WritePathKey(reg, ks, siblingKey, present)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, ks, got, path.PathKey("sym107@1.field"), assigned)
	assertPathValue(t, reg, ks, got, childKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got, siblingKey, present)
}

func TestFactsNodeTransferPathAssignmentInvalidatesEquivalentPathProofs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(18), HasExpr: true}
	alias := symbol.ID(108)
	original := symbol.ID(109)
	aliasPath := path.NewPath(alias, "alias").Field("value")
	aliasKey := path.PathKey("sym108@1.value")
	originalKey := path.PathKey("sym109@1.value")
	originalChildKey := path.PathKey("sym109@1.value.deep")
	siblingKey := path.PathKey("sym109@1.other")
	assigned := absentValue(reg)
	present := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, alias, "alias")
	visibilityBuilder.Define(point, original, "original")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	in := state.State{}.
		WritePathKey(reg, ks, aliasKey, present).
		WritePathKey(reg, ks, originalKey, present).
		WritePathKey(reg, ks, originalChildKey, present).
		WritePathKey(reg, ks, siblingKey, present).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  path.PathKey("sym108@1"),
			Other: path.PathKey("sym109@1"),
		})
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(aliasPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, ks, got, aliasKey, assigned)
	assertPathValue(t, reg, ks, got, originalKey, assigned)
	assertPathValue(t, reg, ks, got, originalChildKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got, siblingKey, present)
}

func TestFactsNodeTransferPathAssignmentSharesDotAndBracketStringKeys(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(19), HasExpr: true}
	slots := symbol.ID(110)
	targetPath := path.NewPath(slots, "slots").IndexStr("active").Field("value")
	bracketKey := path.PathKey(`sym110@1["active"].value`)
	dotKey := path.PathKey("sym110@1.active.value")
	dotChildKey := path.PathKey("sym110@1.active.value.path")
	assigned := absentValue(reg)
	present := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, slots, "slots")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	in := state.State{}.
		WritePathKey(reg, ks, dotKey, present).
		WritePathKey(reg, ks, dotChildKey, present)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, ks, got, bracketKey, assigned)
	assertPathValue(t, reg, ks, got, dotKey, assigned)
	assertPathValue(t, reg, ks, got, dotChildKey, product.Bottom(reg))
}

func TestFactsNodeTransferPathAssignmentInvalidatesEquivalentOriginsAndHeapMembers(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(21)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(21), HasExpr: true}
	alias := symbol.ID(113)
	slots := symbol.ID(114)
	aliasPath := path.NewPath(alias, "alias").Field("value")
	aliasKey := path.PathKey("sym113@1.value")
	slotsKey := path.PathKey("sym114@1.active.value")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, alias, "alias")
	visibilityBuilder.Define(point, slots, "slots")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	staleAliasMember := suffixStaticKey(t, ks, []segment.Segment{
		{Kind: segment.SegmentField, Name: "value"},
		{Kind: segment.SegmentField, Name: "path"},
	})
	staleSlotsMember := suffixStaticKey(t, ks, []segment.Segment{
		{Kind: segment.SegmentField, Name: "active"},
		{Kind: segment.SegmentField, Name: "value"},
		{Kind: segment.SegmentField, Name: "path"},
	})
	siblingSlotsMember := suffixStaticKey(t, ks, []segment.Segment{
		{Kind: segment.SegmentField, Name: "active"},
		{Kind: segment.SegmentField, Name: "other"},
	})
	aliasID := identity.ID{Kind: "test.table", Site: "alias", Index: 1}
	slotsID := identity.ID{Kind: "test.table", Site: "slots", Index: 2}
	assigned := absentValue(reg)
	present := presentValue(reg)
	aliasRoot := product.Set(reg, present, identity.Key, identity.Singleton(aliasID))
	aliasRoot = product.Set(reg, aliasRoot, variantorigin.Key, variantorigin.Singleton(7, 0))
	slotsRoot := product.Set(reg, present, identity.Key, identity.Singleton(slotsID))
	slotsRoot = product.Set(reg, slotsRoot, variantorigin.Key, variantorigin.Singleton(8, 1))
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(alias), aliasRoot).
		WriteValue(reg, key.SymbolValue(slots), slotsRoot).
		WriteHeapTableObject(reg, aliasID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          aliasRoot,
			StaticMembers: map[keyspace.Key]product.Value{staleAliasMember: present},
		})).
		WriteHeapTableObject(reg, slotsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: slotsRoot,
			StaticMembers: map[keyspace.Key]product.Value{
				staleSlotsMember:   present,
				siblingSlotsMember: present,
			},
		})).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  path.PathKey("sym113@1"),
			Other: path.PathKey("sym114@1.active"),
		})
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(aliasPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, ks, got, aliasKey, assigned)
	assertPathValue(t, reg, ks, got, slotsKey, assigned)
	assertRootVariantOriginTop(t, reg, got, alias)
	assertRootVariantOriginTop(t, reg, got, slots)
	if _, ok := got.ReadHeapTableObject(reg, aliasID).StaticMember(staleAliasMember); ok {
		t.Fatalf("alias heap static member %s survived path assignment", ks.Format(staleAliasMember))
	}
	slotsObject := got.ReadHeapTableObject(reg, slotsID)
	if _, ok := slotsObject.StaticMember(staleSlotsMember); ok {
		t.Fatalf("slots heap static member %s survived alias path assignment", ks.Format(staleSlotsMember))
	}
	if gotMember, ok := slotsObject.StaticMember(siblingSlotsMember); !ok || !product.Equal(reg, gotMember, present) {
		t.Fatalf("slots sibling heap member = %s/%v, want present/true", formatValue(reg, gotMember), ok)
	}
}

func TestFactsNodeTransferPathDescendantInvalidationKeepsContainer(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	target := symbol.ID(111)
	containerPath := path.NewPath(target, "item")
	containerKey := path.PathKey("sym111@1")
	countKey := path.PathKey("sym111@1.count")
	nameKey := path.PathKey("sym111@1.name")
	unrelatedKey := path.PathKey("sym112@1.count")
	present := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "item")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	in := state.State{}.
		WritePathKey(reg, ks, containerKey, present).
		WritePathKey(reg, ks, countKey, present).
		WritePathKey(reg, ks, nameKey, present).
		WritePathKey(reg, ks, unrelatedKey, present)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
				point: factflow.NewPathDescendantInvalidation(containerPath),
			},
		}),
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, ks, got, containerKey, present)
	assertPathValue(t, reg, ks, got, countKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got, nameKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got, unrelatedKey, present)
}

func TestFactsNodeTransferPathDescendantInvalidationClearsRootStructuralWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	target := symbol.ID(115)
	containerPath := path.NewPath(target, "slots").Field("active")
	targetID := identity.ID{Kind: "test.table", Site: "slots", Index: 1}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(targetID))
	rootValue = product.Set(reg, rootValue, typewitness.Key, typewitness.Of(typ.String))
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), rootValue)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "slots")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
				point: factflow.NewPathDescendantInvalidation(containerPath),
			},
		}),
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	gotRoot := got.ReadValue(reg, key.SymbolValue(target))
	if gotID, ok := product.Get(reg, gotRoot, identity.Key).ID(); !ok || gotID != targetID {
		t.Fatalf("root identity = %v/%v, want preserved %v", gotID, ok, targetID)
	}
	if witness := product.Get(reg, gotRoot, typewitness.Key); !witness.IsTop() {
		t.Fatalf("root type witness = %v, want top after unresolved descendant write", witness)
	}
}

func TestFactsNodeTransferDirectDynamicIndexWriteKeepsRootStructuralWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(23)
	target := symbol.ID(116)
	tablePath := path.NewPath(target, "item")
	targetID := identity.ID{Kind: "test.table", Site: "item", Index: 1}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(23), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(24), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(targetID))
	rootValue = product.Set(reg, rootValue, typewitness.Key, typewitness.Of(typ.String))
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   presentValue(reg),
			valueSource: presentValue(reg),
		},
	}
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), rootValue)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "item")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathDescendantInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
				point: factflow.NewPathDescendantInvalidation(tablePath),
			},
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionUnknown,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	gotRoot := got.ReadValue(reg, key.SymbolValue(target))
	if witness := product.Get(reg, gotRoot, typewitness.Key); witness.IsTop() {
		t.Fatalf("root type witness was cleared for direct dynamic-index write")
	}
}

func TestFactsNodeTransferPathAssignmentRequiresVisibility(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(17), HasExpr: true}
	target := symbol.ID(108)
	targetPath := path.NewPath(target, "table").Field("field")
	pathKey := path.PathKey("sym108@1.field")
	ks := keyspace.New()
	in := state.State{}.WritePathKey(reg, ks, pathKey, presentValue(reg))
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources: sources,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
	if len(sources.calls) != 0 {
		t.Fatalf("path assignment without visibility resolved source %d times, want zero", len(sources.calls))
	}
}

func TestFactsNodeTransferPathAssignmentWithUnresolvedVersionIsNoop(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(18), HasExpr: true}
	target := symbol.ID(109)
	targetPath := path.NewPath(target, "table").Field("field")
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	ks := resolver.KeySpace()
	in := state.State{}.WritePathKey(reg, ks, path.PathKey("sym109@1.field"), presentValue(reg))
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
}

func TestFactsNodeTransferIgnoresRootPathAssignment(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(19), HasExpr: true}
	target := symbol.ID(110)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: absentValue(reg)},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(path.NewPath(target, "table"), source),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertStateEqual(t, reg, got, state.State{})
	if len(sources.calls) != 0 {
		t.Fatalf("root path assignment resolved source %d times, want zero", len(sources.calls))
	}
}

func suffixStaticKey(t *testing.T, ks *keyspace.KeySpace, segments []segment.Segment) keyspace.Key {
	t.Helper()
	k, ok := heapidentity.StaticMemberSuffixKey(ks, segments)
	if !ok {
		t.Fatalf("StaticMemberSuffixKey(%v) failed", segments)
	}
	return k
}

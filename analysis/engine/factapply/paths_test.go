package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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

	assertPathValue(t, reg, got[assign], path.PathKey("sym106@1.field"), product.Bottom(reg))
	assertPathValue(t, reg, got[graph.Exit()], path.PathKey("sym106@1.field"), assigned)
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
	in := state.State{}.
		WritePathKey(reg, childKey, present).
		WritePathKey(reg, siblingKey, present)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

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

	assertPathValue(t, reg, got, path.PathKey("sym107@1.field"), assigned)
	assertPathValue(t, reg, got, childKey, product.Bottom(reg))
	assertPathValue(t, reg, got, siblingKey, present)
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
	in := state.State{}.
		WritePathKey(reg, aliasKey, present).
		WritePathKey(reg, originalKey, present).
		WritePathKey(reg, originalChildKey, present).
		WritePathKey(reg, siblingKey, present).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  path.PathKey("sym108@1"),
			Other: path.PathKey("sym109@1"),
		})
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, alias, "alias")
	visibilityBuilder.Define(point, original, "original")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(aliasPath, source),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, got, aliasKey, assigned)
	assertPathValue(t, reg, got, originalKey, assigned)
	assertPathValue(t, reg, got, originalChildKey, product.Bottom(reg))
	assertPathValue(t, reg, got, siblingKey, present)
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
	in := state.State{}.
		WritePathKey(reg, containerKey, present).
		WritePathKey(reg, countKey, present).
		WritePathKey(reg, nameKey, present).
		WritePathKey(reg, unrelatedKey, present)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "item")

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

	assertPathValue(t, reg, got, containerKey, present)
	assertPathValue(t, reg, got, countKey, product.Bottom(reg))
	assertPathValue(t, reg, got, nameKey, product.Bottom(reg))
	assertPathValue(t, reg, got, unrelatedKey, present)
}

func TestFactsNodeTransferPathAssignmentRequiresVisibility(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(17), HasExpr: true}
	target := symbol.ID(108)
	targetPath := path.NewPath(target, "table").Field("field")
	pathKey := path.PathKey("sym108@1.field")
	in := state.State{}.WritePathKey(reg, pathKey, presentValue(reg))
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
	in := state.State{}.WritePathKey(reg, path.PathKey("sym109@1.field"), presentValue(reg))
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
		Visibility: visibility.NewResolver(visibility.NewTable(nil)),
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

package effectlowering

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type signatureMap map[string]signature.Function

func (m signatureMap) Lookup(name string) (signature.Function, bool) {
	sig, ok := m[name]
	return sig, ok
}

func TestSignatureProviderMaterializesDeclaredReturns(t *testing.T) {
	reg := standard.Registry()
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: StaticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: symbol.ID(17),
	}), state.State{}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureProviderMaterializesOptionalDeclaredReturn(t *testing.T) {
	reg := standard.Registry()
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.NewOptional(typ.String)).Build()},
		},
		NameFor: StaticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol: symbol.ID(18),
	}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertPresence(t, reg, got[0].Value, presence.Maybe())
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestWithSignatureRelationsLowersErrorReturnToBranchPresenceRelations(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	value := symbol.ID(701)
	err := symbol.ID(702)
	valuePath := path.NewPath(value, "value")
	errPath := path.NewPath(err, "err")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, value, valuePath),
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, err, errPath),
				},
			}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  0,
				ResultIndex:  0,
				CallPoint:    call,
				HasCallPoint: true,
			}),
			assignErr: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, err, errPath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  1,
				ResultIndex:  1,
				CallPoint:    call,
				HasCallPoint: true,
			}),
		},
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				factflow.NewBranchRefinement(
					errPath,
					factflow.NewValueConstraint(product.NewWithPresence(standard.Registry(), product.ShapeTop, presence.Absent())), true,
					factflow.NewValueConstraint(product.NewWithPresence(standard.Registry(), product.ShapeTop, presence.Present())), true,
				),
			),
		},
	})

	got := WithSignatureRelations(SignatureRelationConfig{
		Graph: graph,
		Signatures: signatureMap{
			"f": {Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})},
		},
		NameFor: StaticName("f"),
		Facts:   facts,
	})

	relations := got.BranchPresenceRelations(branch)
	if len(relations) != 2 {
		t.Fatalf("branch relations = %d, want 2: %#v", len(relations), relations)
	}
	assertBranchPresenceRelation(t, relations, errPath, presence.Present(), valuePath, presence.Absent())
	assertBranchPresenceRelation(t, relations, errPath, presence.Absent(), valuePath, presence.Present())
}

func TestWithSignatureRelationsStopsAtErrorReturnTargetReassignment(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assignValue := graph.AddNode(cfg.NodeAssign)
	assignErr := graph.AddNode(cfg.NodeAssign)
	reassignErr := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assignValue, false)
	graph.AddEdge(assignValue, assignErr, false)
	graph.AddEdge(assignErr, reassignErr, false)
	graph.AddEdge(reassignErr, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	value := symbol.ID(703)
	err := symbol.ID(704)
	valuePath := path.NewPath(value, "value")
	errPath := path.NewPath(err, "err")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextAssignmentSource,
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, value, valuePath),
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, err, errPath),
				},
			}),
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			assignValue: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, value, valuePath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  0,
				ResultIndex:  0,
				CallPoint:    call,
				HasCallPoint: true,
			}),
			assignErr: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, err, errPath, factflow.ValueSource{
				Kind:         factflow.ValueSourceCall,
				TargetIndex:  1,
				ResultIndex:  1,
				CallPoint:    call,
				HasCallPoint: true,
			}),
			reassignErr: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, err, errPath, factflow.ValueSource{Kind: factflow.ValueSourceNil}),
		},
		BranchRefinements: map[cfg.Point]factflow.BranchRefinementSet{
			branch: factflow.NewBranchRefinementSet(
				factflow.NewBranchRefinement(
					errPath,
					factflow.NewValueConstraint(product.NewWithPresence(standard.Registry(), product.ShapeTop, presence.Absent())), true,
					factflow.NewValueConstraint(product.NewWithPresence(standard.Registry(), product.ShapeTop, presence.Present())), true,
				),
			),
		},
	})

	got := WithSignatureRelations(SignatureRelationConfig{
		Graph: graph,
		Signatures: signatureMap{
			"f": {Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})},
		},
		NameFor: StaticName("f"),
		Facts:   facts,
	})

	if relations := got.BranchPresenceRelations(branch); len(relations) != 0 {
		t.Fatalf("branch relations after reassignment = %#v, want none", relations)
	}
}

func TestWithSignaturePostconditionsLowersCallSiteArgumentPathAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(801)
	argExpr := factflow.ExprRef(802)
	argSymbol := symbol.ID(803)
	argPath := path.NewPath(argSymbol, "x")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	got := WithSignaturePostconditions(SignaturePostconditionConfig{
		Graph:    graph,
		Registry: reg,
		Signatures: signatureMap{
			"assertLike": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Present{},
				}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "assertLike", true
		},
		Facts: facts,
	})

	refinements := got.PostconditionRefinements(call)
	if len(refinements) != 1 {
		t.Fatalf("postcondition refinements = %d, want 1: %#v", len(refinements), refinements)
	}
	if !refinements[0].TargetPath().Equal(argPath) {
		t.Fatalf("target path = %s, want %s", refinements[0].TargetPath(), argPath)
	}
	value, ok := refinements[0].Value().Constraint()
	if !ok {
		t.Fatalf("missing value constraint")
	}
	assertPresence(t, reg, value, presence.Present())

	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:   got,
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Present())
}

func TestWithSignaturePostconditionsSkipsArgumentsWithoutExpressionPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(811)
	argSymbol := symbol.ID(812)
	existing := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(813), HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
	})

	got := WithSignaturePostconditions(SignaturePostconditionConfig{
		Graph:    graph,
		Registry: reg,
		Signatures: signatureMap{
			"assertLike": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(
					postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{}},
					postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 1}, Refinement: postcondition.Present{}},
				),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "assertLike", true
		},
		Facts: facts,
	})

	if refinements := got.PostconditionRefinements(call); len(refinements) != 0 {
		t.Fatalf("postcondition refinements = %#v, want none", refinements)
	}
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), existing),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:   got,
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Maybe())
}

func TestWithSignatureMutationsLowersTableMutatorToDescendantInvalidation(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	argExpr := factflow.ExprRef(901)
	argPath := path.NewPath(symbol.ID(902), "items")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	got := WithSignatureMutations(SignatureMutationConfig{
		Graph: graph,
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
					mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
				),
			},
		},
		NameFor: StaticName("table.insert"),
		Facts:   facts,
	})

	invalidation, ok := got.PathDescendantInvalidation(call)
	if !ok {
		t.Fatalf("missing path descendant invalidation")
	}
	if !invalidation.ContainerPath().Equal(argPath) {
		t.Fatalf("invalidation path = %s, want %s", invalidation.ContainerPath(), argPath)
	}
	if assignment, ok := got.PathAssignment(call); ok {
		t.Fatalf("unexpected path assignment: %#v", assignment)
	}
}

func TestWithSignatureMutationsLowersStoreIntoContainerArgument(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	containerExpr := factflow.ExprRef(911)
	insertedExpr := factflow.ExprRef(912)
	containerPath := path.NewPath(symbol.ID(913), "container")
	insertedPath := path.NewPath(symbol.ID(914), "inserted")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: containerExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: insertedExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			containerExpr: containerPath,
			insertedExpr:  insertedPath,
		},
	})

	got := WithSignatureMutations(SignatureMutationConfig{
		Graph: graph,
		Signatures: signatureMap{
			"store": {
				Effect: effect.Empty.With(ownership.Store{
					Param: effect.ParamRef{Index: -1},
					Into:  effect.ParamRef{Index: 0},
				}),
			},
		},
		NameFor: StaticName("store"),
		Facts:   facts,
	})

	invalidation, ok := got.PathDescendantInvalidation(call)
	if !ok {
		t.Fatalf("missing path descendant invalidation")
	}
	if !invalidation.ContainerPath().Equal(containerPath) {
		t.Fatalf("invalidation path = %s, want container path %s", invalidation.ContainerPath(), containerPath)
	}
	if invalidation.ContainerPath().Equal(insertedPath) {
		t.Fatalf("invalidation used inserted value path %s", insertedPath)
	}
}

func TestWithSignatureMutationsSkipsStoreWithoutKnownDestination(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	firstExpr := factflow.ExprRef(916)
	lastExpr := factflow.ExprRef(917)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: firstExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: lastExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			firstExpr: path.NewPath(symbol.ID(918), "first"),
			lastExpr:  path.NewPath(symbol.ID(919), "last"),
		},
	})

	got := WithSignatureMutations(SignatureMutationConfig{
		Graph: graph,
		Signatures: signatureMap{
			"store": {
				Effect: effect.Empty.With(ownership.Store{
					Param: effect.ParamRef{Index: 0},
					Into:  effect.ParamRef{Index: -1},
				}),
			},
		},
		NameFor: StaticName("store"),
		Facts:   facts,
	})

	if invalidation, ok := got.PathDescendantInvalidation(call); ok {
		t.Fatalf("unexpected path descendant invalidation for store without destination: %#v", invalidation)
	}
}

func TestWithSignatureMutationsSkipsTargetWithoutExpressionPath(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(921), HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
	})

	got := WithSignatureMutations(SignatureMutationConfig{
		Graph: graph,
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(mutation.TableMutator{
					Target: effect.ParamRef{Index: 0},
					Value:  effect.ParamRef{Index: -1},
				}),
			},
		},
		NameFor: StaticName("table.insert"),
		Facts:   facts,
	})

	if invalidation, ok := got.PathDescendantInvalidation(call); ok {
		t.Fatalf("unexpected path descendant invalidation: %#v", invalidation)
	}
}

func TestWithSignatureNoNormalReturnsMarksNeverReturnCallAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	target := symbol.ID(820)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
			}),
		},
	})

	got := WithSignatureNoNormalReturns(SignatureNoNormalReturnConfig{
		Graph:    graph,
		Registry: reg,
		Signatures: signatureMap{
			"error": {Type: typ.Func().Param("message", typ.Any).Returns(typ.Never).Build()},
		},
		NameFor: StaticName("error"),
		Facts:   facts,
	})

	if !got.NoNormalReturn(call) {
		t.Fatalf("NoNormalReturn(%d) = false, want true", call)
	}
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:   got,
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		}),
	})
	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(target), product.Bottom(reg))
}

func TestCallTargetForResultUsesExplicitTargetResultIndex(t *testing.T) {
	target := symbol.ID(705)
	targetPath := path.NewPath(target, "value")
	call := factflow.NewCallProducer(factflow.CallProducerConfig{
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 9, 1, target, targetPath),
		},
	})

	got, ok := callTargetForResult(call, 1)
	if !ok || got.TargetSymbol() != target || got.ResultIndex() != 1 {
		t.Fatalf("target for result 1 = %#v/%v, want explicit slot target", got, ok)
	}
	if got, ok := callTargetForResult(call, 0); ok {
		t.Fatalf("target for recomputed result 0 = %#v, want none", got)
	}
}

func TestSignatureProviderSameAsReturnsArgumentValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4)
	argRef := factflow.ExprRef(7)
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("value", typ.Any).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{argValue})
}

func TestSignatureProviderSameAsResolvesNegativeParamRef(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(5)
	firstRef := factflow.ExprRef(8)
	lastRef := factflow.ExprRef(9)
	firstValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	lastValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("first", typ.Any).Param("last", typ.Any).Returns(typ.String).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: firstRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: lastRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				firstRef: firstValue,
				lastRef:  lastValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{lastValue})
}

func TestSignatureProviderSameAsFallsBackToDeclaredReturnTypeWhenArgumentUnresolved(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(10), HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderElementOfArrayReturnsElementRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureProviderElementOfMapReturnsValueRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewMap(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderElementOfTupleReturnsElementUnionRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewTuple(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Join(
		runtimekind.Singleton(runtimekind.String),
		runtimekind.Singleton(runtimekind.Number),
	))
}

func TestSignatureProviderOptionalElementOfArrayKeepsMaybePresence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	if gotPresence := product.PresenceOf(got[0].Value); !presence.Equal(gotPresence, presence.Top()) {
		t.Fatalf("presence = %s, want maybe/top", gotPresence)
	}
}

func TestSignatureProviderElementOfFallsBackToDeclaredReturnTypeWhenParamRefUnresolved(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderCallbackReturnProjectsFirstReturnRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Integer).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderCallbackReturnResolvesNegativeParamRef(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", typ.String).
					Param("callback", typ.Func().Returns(typ.Boolean).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression},
			{Kind: factflow.ValueSourceExpression},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureProviderArrayOfCallbackReturnProjectsTableRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(15)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Table))
}

func TestSignatureProviderCallbackReturnFallsBackToDeclaredReturnType(t *testing.T) {
	reg := standard.Registry()

	tests := []struct {
		name      string
		point     cfg.Point
		paramType typ.Type
		ref       effect.ParamRef
		args      []factflow.ValueSource
		want      runtimekind.Value
	}{
		{
			name:      "non-callable callback parameter",
			point:     cfg.Point(16),
			paramType: typ.String,
			ref:       effect.ParamRef{Index: 0},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
		{
			name:      "out-of-range callback parameter",
			point:     cfg.Point(17),
			paramType: typ.Func().Returns(typ.Number).Build(),
			ref:       effect.ParamRef{Index: 1},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureProvider(SignatureProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("callback", tc.paramType).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: tc.ref}}),
					},
				},
				NameFor: StaticName("f"),
				Facts:   signatureProviderFacts(tc.point, tc.args),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: tc.point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, tc.want)
		})
	}
}

func TestSignatureProviderTypeProjectionFieldReturnsFieldRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	record := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", record).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.Field("name")}},
				}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureProviderTypeProjectionCallableReturnReturnsFirstReturnRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Boolean, typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.CallableReturn()}},
				}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureProviderTypeProjectionGenericArgReturnsArgRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)
	stringBox := typ.NewAlias("StringBox", typ.Instantiate(box, typ.String))
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", stringBox).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.GenericArg(0)}},
				}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureProviderTypeProjectionFallsBackToDeclaredReturnType(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(21)
	record := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	provider := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", record).
					Returns(typ.Number).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.Field("missing")}},
				}}),
			},
		},
		NameFor: StaticName("f"),
		Facts:   signatureProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureProviderReservedReturnTransformsUseOnlyDeclaredReturnType(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name: "deep element",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "string unpack",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select case",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select result",
			label: returns.Return{
				ReturnIndex: 0,
				Transform: returns.SelectResultOfCases{
					Cases:   effect.ParamRef{Index: 0},
					Default: effect.ParamRef{Index: 1},
				},
			},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureProvider(SignatureProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("items", typ.NewArray(typ.String)).
							Param("default", typ.Number).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(tc.label),
					},
				},
				NameFor: StaticName("f"),
				Facts: signatureProviderFacts(point, []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression},
					{Kind: factflow.ValueSourceExpression},
				}),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1 declared result: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
		})
	}
}

func TestActiveReturnTransformIgnoresReservedReturnTransforms(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name: "deep element",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "string unpack",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select case",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select result",
			label: returns.Return{
				ReturnIndex: 0,
				Transform: returns.SelectResultOfCases{
					Cases:   effect.ParamRef{Index: 0},
					Default: effect.ParamRef{Index: 1},
				},
			},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if transform, ok := activeReturnTransform(signature.Function{Effect: effect.Empty.With(tc.label)}, 0); ok {
				t.Fatalf("active transform = %#v, want none", transform)
			}
		})
	}
}

func TestFallbackKeepsPrimarySlotsAndFillsMissingSignatureSlots(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	primary := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []factapply.CallResult {
		return []factapply.CallResult{{Index: 0, Value: primaryValue}}
	}
	signatures := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: StaticName("f"),
	})

	got := callresult.Fallback(primary, signatures)(transfer.NodeContext{Registry: reg}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	if got[0].Index != 0 || !product.Equal(reg, got[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got[0])
	}
	if got[1].Index != 1 {
		t.Fatalf("fallback slot index = %d, want 1", got[1].Index)
	}
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestFallbackKeepsPrimarySlotOverSignatureSameAs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	argRef := factflow.ExprRef(11)
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	primary := func(transfer.NodeContext, factflow.CallProducer, state.State, func(cfg.Point) state.State) []factapply.CallResult {
		return []factapply.CallResult{{Index: 0, Value: primaryValue}}
	}
	signatures := SignatureProvider(SignatureProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: StaticName("f"),
		Facts: signatureProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := callresult.Fallback(primary, signatures)(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallProducer(factflow.CallProducerConfig{}), state.State{}, nil)

	assertCallResults(t, reg, got, []product.Value{primaryValue})
}

func TestProductionImportsAreBounded(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list imports . error = %v", err)
	}
	allowed := map[string]bool{
		"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult":     true,
		"github.com/wippyai/go-lua/analysis/check/fixpoint/summary":        true,
		"github.com/wippyai/go-lua/analysis/domain/effect":                 true,
		"github.com/wippyai/go-lua/analysis/domain/effect/mutation":        true,
		"github.com/wippyai/go-lua/analysis/domain/effect/ownership":       true,
		"github.com/wippyai/go-lua/analysis/domain/effect/postcondition":   true,
		"github.com/wippyai/go-lua/analysis/domain/effect/returns":         true,
		"github.com/wippyai/go-lua/analysis/domain/effect/signature":       true,
		"github.com/wippyai/go-lua/analysis/domain/path":                   true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis":             true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/presence":    true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind": true,
		"github.com/wippyai/go-lua/analysis/domain/value/product":          true,
		"github.com/wippyai/go-lua/analysis/domain/value/typevalue":        true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":               true,
		"github.com/wippyai/go-lua/analysis/engine/factapply":              true,
		"github.com/wippyai/go-lua/analysis/engine/sourcevalue":            true,
		"github.com/wippyai/go-lua/analysis/engine/state":                  true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":               true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                        true,
		"github.com/wippyai/go-lua/analysis/lua/typeaccess":                true,
		"github.com/wippyai/go-lua/analysis/lua/typecall":                  true,
		"github.com/wippyai/go-lua/analysis/lua/typeprojection":            true,
		"github.com/wippyai/go-lua/analysis/symbol":                        true,
		"github.com/wippyai/go-lua/analysis/type/kind":                     true,
		"github.com/wippyai/go-lua/analysis/type/typ":                      true,
		"strings": true,
	}
	for _, dep := range strings.Fields(string(out)) {
		if !allowed[dep] {
			t.Fatalf("unexpected production import %q", dep)
		}
	}

	forbidden := []string{"/__old", "/adapter", "/query", "/compiler", "/analysis/lua", "/cfgbuild", "/semantics", "/diagnostic", "/diagnostics", "/store", "/session"}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "github.com/wippyai/go-lua/analysis/lua/typeaccess" ||
			dep == "github.com/wippyai/go-lua/analysis/lua/typecall" ||
			dep == "github.com/wippyai/go-lua/analysis/lua/typeprojection" {
			continue
		}
		for _, forbiddenPart := range forbidden {
			if strings.Contains(dep, forbiddenPart) {
				t.Fatalf("forbidden production import %q matched %q", dep, forbiddenPart)
			}
		}
	}
}

func assertCallResults(t *testing.T, reg *axis.Registry, got []factapply.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("got result[%d].Index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("got result[%d].Value = %v, want %v", i, got[i].Value, value)
		}
	}
}

func signatureProviderFacts(point cfg.Point, args []factflow.ValueSource) factflow.Facts {
	return factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{ArgumentSources: args}),
		},
	})
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := st.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("state[%s] = %v, want %v", slot, got, want)
	}
}

func assertStatePresence(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want presence.Value) {
	t.Helper()
	if got := product.PresenceOf(st.ReadValue(reg, slot)); !presence.Equal(got, want) {
		t.Fatalf("state[%s] presence = %s, want %s", slot, got, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtimekind = %s, want %s", kind, want)
	}
}

func assertPresence(t *testing.T, _ *axis.Registry, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertBranchPresenceRelation(
	t *testing.T,
	relations []factflow.BranchPresenceRelation,
	triggerPath path.Path,
	triggerPresence presence.Value,
	targetPath path.Path,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerPath().Equal(triggerPath) &&
			presence.Equal(relation.TriggerPresence(), triggerPresence) &&
			relation.TargetPath().Equal(targetPath) &&
			presence.Equal(relation.TargetPresence(), targetPresence) {
			return
		}
	}
	t.Fatalf("missing relation %s/%s -> %s/%s in %#v", triggerPath.String(), triggerPresence, targetPath.String(), targetPresence, relations)
}

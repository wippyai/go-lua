package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFactsNodeTransferObjectLiteralRootAssignmentsWriteStaticEntries(t *testing.T) {
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
			point := cfg.Point(61)
			target := symbol.ID(121)
			objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(61), HasExpr: true}
			entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(62), HasExpr: true}
			rootValue := presentValue(reg)
			entryValue := absentValue(reg)
			sources := &recordingSourceValues{
				values: map[factflow.ValueSource]product.Value{
					objectSource: rootValue,
					entrySource:  entryValue,
				},
			}
			input := tc.fact(point, target, objectSource)
			input.ObjectLiterals = map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			}
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(point, target, "obj")

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      factflow.NewFacts(input),
				Sources:    sources,
				Visibility: visibility.NewResolver(visibilityBuilder.Build()),
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{})

			assertValue(t, reg, got, key.SymbolValue(target), rootValue)
			assertPathValue(t, reg, got, path.PathKey("sym121@1.leaf"), entryValue)
			if len(sources.calls) != 2 {
				t.Fatalf("resolver calls = %d, want root and entry", len(sources.calls))
			}
			if sources.calls[0].source != objectSource || sources.calls[1].source != entrySource {
				t.Fatalf("resolver calls = %#v, want root then entry", sources.calls)
			}
		})
	}
}

func TestFactsNodeTransferObjectLiteralEntriesUsePreWriteInputState(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(62)
	target := symbol.ID(122)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(63), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(64), HasExpr: true}
	oldRootValue := presentValue(reg)
	newRootValue := absentValue(reg)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValue: func(point cfg.Point, expr factflow.ExprRef, source factflow.ValueSource, in state.State) (product.Value, bool) {
			switch expr {
			case objectSource.ExprRef:
				return newRootValue, true
			case entrySource.ExprRef:
				return in.ReadValue(reg, key.SymbolValue(target)), true
			default:
				return product.Value{}, false
			}
		},
	})
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "obj")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "obj"), objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("old"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), oldRootValue))

	assertValue(t, reg, got, key.SymbolValue(target), newRootValue)
	assertPathValue(t, reg, got, path.PathKey("sym122@1.old"), oldRootValue)
}

func TestFactsNodeTransferObjectLiteralMissingVisibilitySkipsEntriesKeepsRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(63)
	target := symbol.ID(123)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(65), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(66), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "obj"), objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibility.NewTable(nil)),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertValue(t, reg, got, key.SymbolValue(target), rootValue)
	assertPathValue(t, reg, got, path.PathKey("sym123@1.leaf"), product.Bottom(reg))
	assertResolverCall(t, sources, point, objectSource)
}

func TestFactsNodeTransferObjectLiteralPathAssignmentWritesStaticEntries(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(64)
	target := symbol.ID(124)
	targetPath := path.NewPath(target, "t").Field("child")
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(67), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(68), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "t")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, got, path.PathKey("sym124@1.child"), rootValue)
	assertPathValue(t, reg, got, path.PathKey("sym124@1.child.leaf"), entryValue)
	if len(sources.calls) != 2 {
		t.Fatalf("resolver calls = %d, want root and entry", len(sources.calls))
	}
}

func TestFactsNodeTransferObjectLiteralEntriesInvalidateSubtreeBeforeWrite(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(65)
	target := symbol.ID(125)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(69), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(70), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	staleValue := presentValue(reg)
	siblingValue := presentValue(reg)
	staleChildKey := path.PathKey("sym125@1.a.old")
	siblingKey := path.PathKey("sym125@1.b.old")
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "t")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "t"), objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("a"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WritePathKey(reg, staleChildKey, staleValue).
		WritePathKey(reg, siblingKey, siblingValue))

	assertValue(t, reg, got, key.SymbolValue(target), rootValue)
	assertPathValue(t, reg, got, path.PathKey("sym125@1.a"), entryValue)
	assertPathValue(t, reg, got, staleChildKey, product.Bottom(reg))
	assertPathValue(t, reg, got, siblingKey, product.Bottom(reg))
}

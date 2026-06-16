package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

const testObjectLiteralGraphID uint64 = 9001

func testTableLiteralID(expr factflow.ExprRef) identity.ID {
	return identity.LuaTableLiteral(testObjectLiteralGraphID, uint64(expr))
}

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
			rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
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
			assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".leaf", entryValue)
			if len(sources.calls) != 4 {
				t.Fatalf("resolver calls = %d, want assignment root plus heap/path entry reads", len(sources.calls))
			}
			if sources.calls[0].source != objectSource || sources.calls[1].source != objectSource ||
				sources.calls[2].source != entrySource || sources.calls[3].source != entrySource {
				t.Fatalf("resolver calls = %#v, want root, root, entry, entry", sources.calls)
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
	newRootValue := product.Set(reg, absentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
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
	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".old", oldRootValue)
}

func TestFactsNodeTransferObjectLiteralMissingVisibilitySkipsEntriesKeepsRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(63)
	target := symbol.ID(123)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(65), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(66), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
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
	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".leaf", entryValue)
	if len(sources.calls) != 3 {
		t.Fatalf("resolver calls = %d, want assignment root plus heap reads", len(sources.calls))
	}
	if sources.calls[0].source != objectSource || sources.calls[1].source != objectSource || sources.calls[2].source != entrySource {
		t.Fatalf("resolver calls = %#v, want root, root, entry", sources.calls)
	}
}

func TestFactsNodeTransferObjectLiteralPathAssignmentWritesStaticEntries(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(64)
	target := symbol.ID(124)
	targetPath := path.NewPath(target, "t").Field("child")
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(67), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(68), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
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
	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".leaf", entryValue)
	if len(sources.calls) != 4 {
		t.Fatalf("resolver calls = %d, want assignment root plus heap/path entry reads", len(sources.calls))
	}
	if sources.calls[0].source != objectSource || sources.calls[1].source != objectSource ||
		sources.calls[2].source != entrySource || sources.calls[3].source != entrySource {
		t.Fatalf("resolver calls = %#v, want root, root, entry, entry", sources.calls)
	}
}

func TestFactsNodeTransferObjectLiteralWritesNestedHeapObjects(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(66)
	target := symbol.ID(126)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(71), HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(72), HasExpr: true}
	leafSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(73), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
	nestedValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(nestedSource.ExprRef)))
	leafValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			nestedSource: nestedValue,
			leafSource:   leafValue,
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
					factflow.NewObjectEntry(fieldSuffix("child"), nestedSource),
					factflow.NewObjectEntry(path.Path{
						Segments: []segment.Segment{
							{Kind: segment.SegmentField, Name: "child"},
							{Kind: segment.SegmentField, Name: "id"},
						},
					}, leafSource),
				}).WithIdentity(testTableLiteralID(objectSource.ExprRef)),
				nestedSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("id"), leafSource),
				}).WithIdentity(testTableLiteralID(nestedSource.ExprRef)),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".child", nestedValue)
	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".child.id", leafValue)
	assertHeapStaticMember(t, reg, got, nestedSource.ExprRef, ".id", leafValue)
}

func TestFactsNodeTransferReturnObjectLiteralWritesHeapObject(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(67)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(74), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(75), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
	entryValue := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			Returns: map[cfg.Point]factflow.Return{
				point: factflow.NewReturn([]factflow.ValueSource{objectSource}),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			},
		}),
		Sources: sources,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertValue(t, reg, got, key.ReturnSlot(0), rootValue)
	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".leaf", entryValue)
}

func TestFactsNodeTransferCallArgumentObjectLiteralWritesHeapObject(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(68)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(76), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(77), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
	entryValue := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:         factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{objectSource},
				}),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			},
		}),
		Sources: sources,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertHeapStaticMember(t, reg, got, objectSource.ExprRef, ".leaf", entryValue)
}

func TestFactsNodeTransferObjectLiteralEntriesInvalidateSubtreeBeforeWrite(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(65)
	target := symbol.ID(125)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(69), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(70), HasExpr: true}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
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

func assertHeapStaticMember(t *testing.T, reg *axis.Registry, gotState state.State, expr factflow.ExprRef, suffix path.PathKey, want product.Value) {
	t.Helper()
	id := testTableLiteralID(expr)
	object := gotState.ReadHeapTableObject(reg, id)
	got, ok := object.StaticMember(path.PathKey(suffix))
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("heap object %v static %s = %s/%v, want %s", id, suffix, formatValue(reg, got), ok, formatValue(reg, want))
	}
	if rootID, ok := product.Get(reg, object.Root(), identity.Key).ID(); !ok || rootID != id {
		t.Fatalf("heap object %v root identity = %v/%v, want %v", id, rootID, ok, id)
	}
	if !heapidentity.ObjectDomain(reg).LessOrEq(object, heapidentity.TopObject()) {
		t.Fatalf("heap object %v not in domain", id)
	}
}

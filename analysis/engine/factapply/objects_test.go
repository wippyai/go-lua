package factapply

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
				}).WithIdentity(testTableLiteralID(objectSource.ExprRef)),
			}
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(point, target, "obj")
			resolver := visibility.NewResolver(visibilityBuilder.Build())
			ks := resolver.KeySpace()

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      factflow.NewFacts(input),
				Sources:    sources,
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{})

			assertValue(t, reg, got, key.SymbolValue(target), rootValue)
			assertPathValue(t, reg, ks, got, path.PathKey("sym121@1.leaf"), entryValue)
			assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".leaf", entryValue)
			assertPlacement(t, got, testTableLiteralID(objectSource.ExprRef), placement.Stack)
			if len(sources.calls) != 2 {
				t.Fatalf("resolver calls = %d, want assignment root plus one cached entry read", len(sources.calls))
			}
			if sources.calls[0].source != objectSource || sources.calls[1].source != entrySource {
				t.Fatalf("resolver calls = %#v, want root, entry", sources.calls)
			}
		})
	}
}

func TestFactsNodeTransferObjectLiteralHeapRootUsesResolvedTypedSourceValue(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	point := cfg.Point(65)
	target := symbol.ID(125)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(65), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(66), HasExpr: true}
	rootType := typetable.NewRecord().Field("leaf", typ.String).Build()
	rootValue := typeValues.FromTypeWithWitness(reg, rootType)
	rootValue = product.Set(reg, rootValue, identity.Key, identity.Singleton(testTableLiteralID(objectSource.ExprRef)))
	rootValue = product.Set(reg, rootValue, evidence.Key, evidence.ExplicitTop())
	entryValue := typeValues.FromTypeWithWitness(reg, typ.String)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	input := factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "obj"), objectSource),
		},
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
			objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
			}).WithIdentity(testTableLiteralID(objectSource.ExprRef)),
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "obj")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts:      factflow.NewFacts(input),
		Sources:    sources,
		Visibility: resolver,
		TypeValues: typeValues,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	object := got.ReadHeapTableObject(reg, testTableLiteralID(objectSource.ExprRef))
	if gotRoot := object.Root(); !product.Equal(reg, gotRoot, rootValue) {
		t.Fatalf("heap object root = %s, want resolved source value %s", formatValue(reg, gotRoot), formatValue(reg, rootValue))
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), oldRootValue))

	assertValue(t, reg, got, key.SymbolValue(target), newRootValue)
	assertPathValue(t, reg, ks, got, path.PathKey("sym122@1.old"), oldRootValue)
	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".old", oldRootValue)
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
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "obj"), objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}).WithIdentity(testTableLiteralID(objectSource.ExprRef)),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertValue(t, reg, got, key.SymbolValue(target), rootValue)
	assertPathValue(t, reg, ks, got, path.PathKey("sym123@1.leaf"), product.Bottom(reg))
	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".leaf", entryValue)
	if len(sources.calls) != 2 {
		t.Fatalf("resolver calls = %d, want assignment root plus one heap entry read", len(sources.calls))
	}
	if sources.calls[0].source != objectSource || sources.calls[1].source != entrySource {
		t.Fatalf("resolver calls = %#v, want root, entry", sources.calls)
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}).WithIdentity(testTableLiteralID(objectSource.ExprRef)),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, ks, got, path.PathKey("sym124@1.child"), rootValue)
	assertPathValue(t, reg, ks, got, path.PathKey("sym124@1.child.leaf"), entryValue)
	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".leaf", entryValue)
	if len(sources.calls) != 2 {
		t.Fatalf("resolver calls = %d, want assignment root plus one cached entry read", len(sources.calls))
	}
	if sources.calls[0].source != objectSource || sources.calls[1].source != entrySource {
		t.Fatalf("resolver calls = %#v, want root, entry", sources.calls)
	}
}

func TestFactsNodeTransferObjectLiteralEntryExpectedContractPreservesIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(70)
	target := symbol.ID(128)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(80), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(81), HasExpr: true}
	rootID := testTableLiteralID(objectSource.ExprRef)
	entryID := testTableLiteralID(entrySource.ExprRef)
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	entryValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(entryID))
	entryType := typetable.NewMap(typ.String, typ.String)
	entryExpected := typevalue.WithWitness(reg, typevalue.FromType(reg, entryType), entryType)
	entry := factflow.NewObjectEntry(fieldSuffix("processed"), entrySource).WithExpected(entryExpected)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "actor")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "actor"), objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{entry}),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	pathValue := got.ReadPathKey(reg, ks, path.PathKey("sym128@1.processed"))
	if gotID, ok := product.Get(reg, pathValue, identity.Key).ID(); !ok || gotID != entryID {
		t.Fatalf("path entry identity = %v/%v, want %v in %s", gotID, ok, entryID, formatValue(reg, pathValue))
	}
	if gotType, ok := typevalue.TypeOf(reg, pathValue); !ok || !typ.TypeEquals(gotType, entryType) {
		t.Fatalf("path entry type = %v/%v, want %v", gotType, ok, entryType)
	}
	object := got.ReadHeapTableObject(reg, rootID)
	heapValue, ok := object.StaticMember(staticSuffixKey(t, ks, ".processed"))
	if !ok {
		t.Fatalf("heap processed member missing")
	}
	if gotID, ok := product.Get(reg, heapValue, identity.Key).ID(); !ok || gotID != entryID {
		t.Fatalf("heap entry identity = %v/%v, want %v in %s", gotID, ok, entryID, formatValue(reg, heapValue))
	}
	if gotType, ok := typevalue.TypeOf(reg, heapValue); !ok || !typ.TypeEquals(gotType, entryType) {
		t.Fatalf("heap entry type = %v/%v, want %v", gotType, ok, entryType)
	}
}

func TestFactsNodeTransferObjectLiteralEntryExpectedContractDoesNotEraseIncompatibleWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(71)
	target := symbol.ID(129)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(82), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(83), HasExpr: true}
	rootID := testTableLiteralID(objectSource.ExprRef)
	entryID := testTableLiteralID(entrySource.ExprRef)
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	actualType := typ.Func().Param("self", typ.Any).Returns(typ.Number).Build()
	expectedType := typ.Func().Param("self", typ.Any).Returns(typ.String).Build()
	entryValue := product.Set(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, actualType), actualType), identity.Key, identity.Singleton(entryID))
	entryExpected := typevalue.WithWitness(reg, typevalue.FromType(reg, expectedType), expectedType)
	entry := factflow.NewObjectEntry(fieldSuffix("read"), entrySource).WithExpected(entryExpected)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "impl")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "impl"), objectSource),
			},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				objectSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{entry}),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	pathValue := got.ReadPathKey(reg, ks, path.PathKey("sym129@1.read"))
	if gotType, ok := typevalue.TypeOf(reg, pathValue); !ok || !typ.TypeEquals(gotType, actualType) {
		t.Fatalf("path entry type = %v/%v, want actual incompatible type %v", gotType, ok, actualType)
	}
	object := got.ReadHeapTableObject(reg, rootID)
	heapValue, ok := object.StaticMember(staticSuffixKey(t, ks, ".read"))
	if !ok {
		t.Fatalf("heap read member missing")
	}
	if gotType, ok := typevalue.TypeOf(reg, heapValue); !ok || !typ.TypeEquals(gotType, actualType) {
		t.Fatalf("heap entry type = %v/%v, want actual incompatible type %v", gotType, ok, actualType)
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".child", nestedValue)
	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".child.id", leafValue)
	assertHeapStaticMember(t, reg, ks, got, nestedSource.ExprRef, ".id", leafValue)
	assertPlacement(t, got, testTableLiteralID(objectSource.ExprRef), placement.Stack)
	assertPlacement(t, got, testTableLiteralID(nestedSource.ExprRef), placement.Stack)
}

func TestFactsNodeTransferObjectLiteralPlacementDoesNotDemotePromotedIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(69)
	target := symbol.ID(127)
	objectSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(78), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(79), HasExpr: true}
	id := testTableLiteralID(objectSource.ExprRef)
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(id))
	entryValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "obj")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WritePlacement(id, placement.SharedHeap))

	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".leaf", entryValue)
	assertPlacement(t, got, id, placement.SharedHeap)
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
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	ks := resolver.KeySpace()

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
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertValue(t, reg, got, key.ReturnSlot(0), rootValue)
	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".leaf", entryValue)
	if len(sources.calls) != 2 {
		t.Fatalf("resolver calls = %d, want return root plus one cached entry read", len(sources.calls))
	}
	if sources.calls[0].source != objectSource || sources.calls[1].source != entrySource {
		t.Fatalf("resolver calls = %#v, want root, entry", sources.calls)
	}
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
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	ks := resolver.KeySpace()

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
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertHeapStaticMember(t, reg, ks, got, objectSource.ExprRef, ".leaf", entryValue)
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
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

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
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WritePathKey(reg, ks, staleChildKey, staleValue).
		WritePathKey(reg, ks, siblingKey, siblingValue))

	assertValue(t, reg, got, key.SymbolValue(target), rootValue)
	assertPathValue(t, reg, ks, got, path.PathKey("sym125@1.a"), entryValue)
	assertPathValue(t, reg, ks, got, staleChildKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, got, siblingKey, product.Bottom(reg))
}

func dottedSuffixSegments(suffix string) []segment.Segment {
	trimmed := strings.TrimPrefix(suffix, ".")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ".")
	segments := make([]segment.Segment, 0, len(parts))
	for _, part := range parts {
		segments = append(segments, segment.Segment{Kind: segment.SegmentField, Name: part})
	}
	return segments
}

func staticSuffixKey(t *testing.T, ks *keyspace.KeySpace, suffix string) keyspace.Key {
	t.Helper()
	k, ok := heapidentity.StaticMemberSuffixKey(ks, dottedSuffixSegments(suffix))
	if !ok {
		t.Fatalf("StaticMemberSuffixKey(%q) failed", suffix)
	}
	return k
}

func assertHeapStaticMember(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, gotState state.State, expr factflow.ExprRef, suffix string, want product.Value) {
	t.Helper()
	id := testTableLiteralID(expr)
	object := gotState.ReadHeapTableObject(reg, id)
	got, ok := object.StaticMember(staticSuffixKey(t, ks, suffix))
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

func assertPlacement(t *testing.T, gotState state.State, id identity.ID, want placement.Value) {
	t.Helper()
	if got := gotState.ReadPlacement(id); got != want {
		t.Fatalf("placement[%v] = %s, want %s", id, got, want)
	}
}

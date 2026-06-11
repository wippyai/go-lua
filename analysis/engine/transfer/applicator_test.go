package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFactsNodeTransferAppliesLocalAssignmentThroughResolver(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(10), HasExpr: true}
	target := symbol.ID(101)
	assigned := presentValue(reg)
	resolver := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: assigned},
	}

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: NewFacts(FactsInput{
				LocalAssignments: map[cfg.Point]LocalAssignment{
					assign: NewLocalAssignment(target, path.NewPath(target, "local"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[assign], key.SymbolValue(target), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferAppliesAssertionSidecarsToAssignments(t *testing.T) {
	tests := []struct {
		name         string
		innerAbsent  bool
		claim        assertion.Value
		wantPresence presence.Value
	}{
		{
			name:         "type assertion attaches claim",
			claim:        assertion.Type(),
			wantPresence: presence.Present(),
		},
		{
			name:         "non-nil assertion does not refine absent presence",
			innerAbsent:  true,
			claim:        assertion.NonNil(),
			wantPresence: presence.Absent(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := product.DefaultRegistry()
			point := cfg.Point(51)
			target := symbol.ID(151)
			inner := ExprRef(1510)
			outer := ExprRef(1511)
			innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true}
			outerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}
			innerValue := presentValue(reg)
			if tc.innerAbsent {
				innerValue = absentValue(reg)
			}
			sources := NewSourceValues(SourceValuesConfig{
				Registry: reg,
				ExpressionValues: map[ExprRef]product.Value{
					inner: innerValue,
				},
			})

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts: NewFacts(FactsInput{
					LocalAssignments: map[cfg.Point]LocalAssignment{
						point: NewLocalAssignment(target, path.NewPath(target, "value"), outerSource),
					},
					Assertions: map[ExprRef]Assertion{
						outer: NewAssertion(innerSource, tc.claim),
					},
				}),
				Sources: sources,
			})(NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{})

			assigned := got.ReadValue(reg, key.SymbolValue(target))
			if gotPresence := product.PresenceOf(assigned); !presence.Equal(gotPresence, tc.wantPresence) {
				t.Fatalf("assigned presence = %s, want %s", gotPresence, tc.wantPresence)
			}
			if gotClaim := product.Get(reg, assigned, assertion.Key); !assertion.Equal(gotClaim, tc.claim) {
				t.Fatalf("assigned assertion = %s, want %s", gotClaim, tc.claim)
			}
		})
	}
}

func TestFactsNodeTransferAppliesOrdinaryAssignmentThroughResolver(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(11), HasExpr: true}
	target := symbol.ID(102)
	assigned := absentValue(reg)
	resolver := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: assigned},
	}

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: NewFacts(FactsInput{
				OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
					assign: NewOrdinaryAssignment(target, path.NewPath(target, "ordinary"), source),
				},
			}),
			Sources: resolver,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), assigned)
	assertResolverCall(t, resolver, assign, source)
}

func TestFactsNodeTransferRootAssignmentInvalidatesVisiblePathSubtree(t *testing.T) {
	tests := []struct {
		name string
		fact func(cfg.Point, symbol.ID, ValueSource) FactsInput
	}{
		{
			name: "local",
			fact: func(point cfg.Point, target symbol.ID, source ValueSource) FactsInput {
				return FactsInput{
					LocalAssignments: map[cfg.Point]LocalAssignment{
						point: NewLocalAssignment(target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
		{
			name: "ordinary",
			fact: func(point cfg.Point, target symbol.ID, source ValueSource) FactsInput {
				return FactsInput{
					OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
						point: NewOrdinaryAssignment(target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := product.DefaultRegistry()
			point := cfg.Point(60)
			target := symbol.ID(120)
			source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(60), HasExpr: true}
			assigned := absentValue(reg)
			stale := presentValue(reg)
			rootKey := path.PathKey("sym120@1")
			childKey := path.PathKey("sym120@1.field")
			deepKey := path.PathKey("sym120@1.field.deep")
			otherVersionKey := path.PathKey("sym120@2.field")
			otherSymbolKey := path.PathKey("sym121@1.field")
			sources := &recordingSourceValues{
				values: map[ValueSource]product.Value{source: assigned},
			}
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(point, target, "obj")

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      NewFacts(tc.fact(point, target, source)),
				Sources:    sources,
				Visibility: visibility.NewResolver(visibilityBuilder.Build()),
			})(NodeContext{
				Registry: reg,
				Point:    point,
			}, state.State{}.
				WritePathKey(reg, rootKey, stale).
				WritePathKey(reg, childKey, stale).
				WritePathKey(reg, deepKey, stale).
				WritePathKey(reg, otherVersionKey, stale).
				WritePathKey(reg, otherSymbolKey, stale))

			assertValue(t, reg, got, key.SymbolValue(target), assigned)
			assertPathValue(t, reg, got, rootKey, product.Bottom(reg))
			assertPathValue(t, reg, got, childKey, product.Bottom(reg))
			assertPathValue(t, reg, got, deepKey, product.Bottom(reg))
			assertPathValue(t, reg, got, otherVersionKey, stale)
			assertPathValue(t, reg, got, otherSymbolKey, stale)
			assertResolverCall(t, sources, point, source)
		})
	}
}

func TestFactsNodeTransferObjectLiteralRootAssignmentsWriteStaticEntries(t *testing.T) {
	tests := []struct {
		name string
		fact func(cfg.Point, symbol.ID, ValueSource) FactsInput
	}{
		{
			name: "local",
			fact: func(point cfg.Point, target symbol.ID, source ValueSource) FactsInput {
				return FactsInput{
					LocalAssignments: map[cfg.Point]LocalAssignment{
						point: NewLocalAssignment(target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
		{
			name: "ordinary",
			fact: func(point cfg.Point, target symbol.ID, source ValueSource) FactsInput {
				return FactsInput{
					OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
						point: NewOrdinaryAssignment(target, path.NewPath(target, "obj"), source),
					},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := product.DefaultRegistry()
			point := cfg.Point(61)
			target := symbol.ID(121)
			objectSource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(61), HasExpr: true}
			entrySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(62), HasExpr: true}
			rootValue := presentValue(reg)
			entryValue := absentValue(reg)
			sources := &recordingSourceValues{
				values: map[ValueSource]product.Value{
					objectSource: rootValue,
					entrySource:  entryValue,
				},
			}
			input := tc.fact(point, target, objectSource)
			input.ObjectLiterals = map[ExprRef]ObjectLiteral{
				objectSource.ExprRef: NewObjectLiteral([]ObjectEntry{
					NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			}
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(point, target, "obj")

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      NewFacts(input),
				Sources:    sources,
				Visibility: visibility.NewResolver(visibilityBuilder.Build()),
			})(NodeContext{
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
	reg := product.DefaultRegistry()
	point := cfg.Point(62)
	target := symbol.ID(122)
	objectSource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(63), HasExpr: true}
	entrySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(64), HasExpr: true}
	oldRootValue := presentValue(reg)
	newRootValue := absentValue(reg)
	sources := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValue: func(point cfg.Point, expr ExprRef, source ValueSource, in state.State) (product.Value, bool) {
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
		Facts: NewFacts(FactsInput{
			LocalAssignments: map[cfg.Point]LocalAssignment{
				point: NewLocalAssignment(target, path.NewPath(target, "obj"), objectSource),
			},
			ObjectLiterals: map[ExprRef]ObjectLiteral{
				objectSource.ExprRef: NewObjectLiteral([]ObjectEntry{
					NewObjectEntry(fieldSuffix("old"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), oldRootValue))

	assertValue(t, reg, got, key.SymbolValue(target), newRootValue)
	assertPathValue(t, reg, got, path.PathKey("sym122@1.old"), oldRootValue)
}

func TestFactsNodeTransferObjectLiteralMissingVisibilitySkipsEntriesKeepsRoot(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(63)
	target := symbol.ID(123)
	objectSource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(65), HasExpr: true}
	entrySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(66), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			LocalAssignments: map[cfg.Point]LocalAssignment{
				point: NewLocalAssignment(target, path.NewPath(target, "obj"), objectSource),
			},
			ObjectLiterals: map[ExprRef]ObjectLiteral{
				objectSource.ExprRef: NewObjectLiteral([]ObjectEntry{
					NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibility.NewTable(nil)),
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertValue(t, reg, got, key.SymbolValue(target), rootValue)
	assertPathValue(t, reg, got, path.PathKey("sym123@1.leaf"), product.Bottom(reg))
	assertResolverCall(t, sources, point, objectSource)
}

func TestFactsNodeTransferObjectLiteralPathAssignmentWritesStaticEntries(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(64)
	target := symbol.ID(124)
	targetPath := path.NewPath(target, "t").Field("child")
	objectSource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(67), HasExpr: true}
	entrySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(68), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "t")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			PathAssignments: map[cfg.Point]PathAssignment{
				point: NewPathAssignment(targetPath, objectSource),
			},
			ObjectLiterals: map[ExprRef]ObjectLiteral{
				objectSource.ExprRef: NewObjectLiteral([]ObjectEntry{
					NewObjectEntry(fieldSuffix("leaf"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(NodeContext{
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
	reg := product.DefaultRegistry()
	point := cfg.Point(65)
	target := symbol.ID(125)
	objectSource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(69), HasExpr: true}
	entrySource := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(70), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	staleValue := presentValue(reg)
	siblingValue := presentValue(reg)
	staleChildKey := path.PathKey("sym125@1.a.old")
	siblingKey := path.PathKey("sym125@1.b.old")
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{
			objectSource: rootValue,
			entrySource:  entryValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "t")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			LocalAssignments: map[cfg.Point]LocalAssignment{
				point: NewLocalAssignment(target, path.NewPath(target, "t"), objectSource),
			},
			ObjectLiterals: map[ExprRef]ObjectLiteral{
				objectSource.ExprRef: NewObjectLiteral([]ObjectEntry{
					NewObjectEntry(fieldSuffix("a"), entrySource),
				}),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(NodeContext{
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

func TestFactsNodeTransferAppliesPathAssignmentThroughVisibility(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(15), HasExpr: true}
	target := symbol.ID(106)
	targetPath := path.NewPath(target, "table").Field("field")
	assigned := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(assign, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: NewFacts(FactsInput{
				PathAssignments: map[cfg.Point]PathAssignment{
					assign: NewPathAssignment(targetPath, source),
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
	reg := product.DefaultRegistry()
	point := cfg.Point(16)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(16), HasExpr: true}
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
		values: map[ValueSource]product.Value{source: assigned},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			PathAssignments: map[cfg.Point]PathAssignment{
				point: NewPathAssignment(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertPathValue(t, reg, got, path.PathKey("sym107@1.field"), assigned)
	assertPathValue(t, reg, got, childKey, product.Bottom(reg))
	assertPathValue(t, reg, got, siblingKey, present)
}

func TestFactsNodeTransferAppliesReturnSlotsThroughSourceValues(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	expr := ExprRef(20)
	exprValue := presentValue(reg)
	sources := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: exprValue,
		},
	})

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: NewFacts(FactsInput{
				Returns: map[cfg.Point]Return{
					ret: NewReturn([]ValueSource{
						{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true},
						{Kind: ValueSourceNil},
					}),
				},
			}),
			Sources: sources,
		}),
	})

	assertValue(t, reg, got[ret], key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), exprValue)
	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(1), absentValue(reg))
}

func TestFactsNodeTransferUnresolvedReturnSourceLeavesSlotUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(21)
	slotValue := presentValue(reg)
	in := state.State{}.WriteReturnSlot(reg, 0, slotValue)
	sources := NewSourceValues(SourceValuesConfig{Registry: reg})

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			Returns: map[cfg.Point]Return{
				point: NewReturn([]ValueSource{
					{Kind: ValueSourceExpression, ExprRef: ExprRef(21), HasExpr: true},
				}),
			},
		}),
		Sources: sources,
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), slotValue)
}

func TestFactsNodeTransferReturnCallSourceReadsReturnSlotThroughRead(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)

	callValue := presentValue(reg)
	sources := NewSourceValues(SourceValuesConfig{Registry: reg})

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		Initial: func(point cfg.Point) (state.State, bool) {
			if point == call {
				return state.State{}.WriteReturnSlot(reg, 2, callValue), true
			}
			return state.State{}, false
		},
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: NewFacts(FactsInput{
				Returns: map[cfg.Point]Return{
					ret: NewReturn([]ValueSource{
						{
							Kind:         ValueSourceCall,
							CallPoint:    call,
							HasCallPoint: true,
							ResultIndex:  2,
						},
					}),
				},
			}),
			Sources: sources,
		}),
	})

	assertValue(t, reg, got[graph.Exit()], key.ReturnSlot(0), callValue)
}

func TestFactsNodeTransferCallProducerProviderWritesReturnSlots(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(22)
	target := symbol.ID(111)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))
	first := presentValue(reg)
	third := absentValue(reg)

	var providerCalled bool
	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			Calls: map[cfg.Point]CallProducer{
				point: NewCallProducer(CallProducerConfig{
					Context:      CallProducerContextAssignment,
					CalleeSymbol: symbol.ID(201),
					ExprIndex:    4,
				}),
			},
		}),
		CallResults: func(ctx NodeContext, call CallProducer, gotIn state.State, read func(cfg.Point) state.State) []CallResult {
			providerCalled = true
			if ctx.Point != point {
				t.Fatalf("provider point = %d, want %d", ctx.Point, point)
			}
			if call.Context() != CallProducerContextAssignment || call.CalleeSymbol() != symbol.ID(201) || call.ExprIndex() != 4 {
				t.Fatalf("provider call = %#v", call)
			}
			assertStateEqual(t, reg, gotIn, in)
			assertValue(t, reg, read(point), key.SymbolValue(target), presentValue(reg))
			return []CallResult{
				{Index: 0, Value: first},
				{Index: 2, Value: third},
				{Index: -1, Value: product.Top()},
			}
		},
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	if !providerCalled {
		t.Fatal("call result provider was not called")
	}
	assertValue(t, reg, got, key.ReturnSlot(0), first)
	assertValue(t, reg, got, key.ReturnSlot(1), product.Bottom(reg))
	assertValue(t, reg, got, key.ReturnSlot(2), third)
	assertValue(t, reg, got, key.SymbolValue(target), presentValue(reg))
}

func TestFactsNodeTransferAssignmentCallSourceConsumesProviderReturnSlotThroughRead(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	target := symbol.ID(112)
	callValue := presentValue(reg)
	sources := NewSourceValues(SourceValuesConfig{Registry: reg})

	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: NewFacts(FactsInput{
				Calls: map[cfg.Point]CallProducer{
					call: NewCallProducer(CallProducerConfig{
						Context: CallProducerContextAssignment,
					}),
				},
				LocalAssignments: map[cfg.Point]LocalAssignment{
					assign: NewLocalAssignment(target, path.NewPath(target, "local"), ValueSource{
						Kind:         ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  0,
					}),
				},
			}),
			Sources: sources,
			CallResults: func(ctx NodeContext, call CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
				return []CallResult{{Index: 0, Value: callValue}}
			},
		}),
		EdgeTransfer: func(ctx EdgeContext, out state.State) state.State {
			if ctx.Edge.From == call && ctx.Edge.To == assign {
				return out.WriteReturnSlot(reg, 0, product.Bottom(reg))
			}
			return out
		},
	})

	assertValue(t, reg, got[assign], key.ReturnSlot(0), product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), callValue)
}

func TestFactsNodeTransferCallResultTargetsDoNotDirectlyWriteTargets(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(23)
	target := symbol.ID(113)
	targetPath := path.NewPath(target, "table").Field("field")
	pathKey := path.PathKey("sym113@1.field")
	symbolValue := presentValue(reg)
	pathValue := presentValue(reg)
	resultValue := absentValue(reg)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(target), symbolValue).
		WritePathKey(reg, pathKey, pathValue)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			Calls: map[cfg.Point]CallProducer{
				point: NewCallProducer(CallProducerConfig{
					Context: CallProducerContextAssignment,
					ResultTargets: []CallResultTarget{
						NewCallResultTarget(CallResultTargetLocalAssignment, 0, target, path.NewPath(target, "local")),
						NewCallResultTarget(CallResultTargetOrdinaryAssignment, 0, target, targetPath),
					},
				}),
			},
		}),
		CallResults: func(ctx NodeContext, call CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
			return []CallResult{{Index: 0, Value: resultValue}}
		},
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertValue(t, reg, got, key.ReturnSlot(0), resultValue)
	assertValue(t, reg, got, key.SymbolValue(target), symbolValue)
	assertPathValue(t, reg, got, pathKey, pathValue)
}

func TestFactsNodeTransferMissingCallResultProviderOrNoResultsLeavesStateUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(24)
	target := symbol.ID(114)
	in := state.State{}.
		WriteReturnSlot(reg, 0, presentValue(reg)).
		WriteValue(reg, key.SymbolValue(target), absentValue(reg))

	facts := NewFacts(FactsInput{
		Calls: map[cfg.Point]CallProducer{
			point: NewCallProducer(CallProducerConfig{Context: CallProducerContextAssignment}),
		},
	})
	tests := []struct {
		name     string
		provider CallResultProvider
	}{
		{name: "nil provider"},
		{
			name: "nil results",
			provider: func(ctx NodeContext, call CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
				return nil
			},
		},
		{
			name: "empty results",
			provider: func(ctx NodeContext, call CallProducer, in state.State, read func(cfg.Point) state.State) []CallResult {
				return []CallResult{}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:       facts,
				CallResults: tc.provider,
			})(NodeContext{
				Registry: reg,
				Point:    point,
			}, in)

			assertStateEqual(t, reg, got, in)
		})
	}
}

func TestFactsNodeTransferMissingResolverValueLeavesStateUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(12)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(12), HasExpr: true}
	target := symbol.ID(103)
	unchangedValue := presentValue(reg)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), unchangedValue)

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			LocalAssignments: map[cfg.Point]LocalAssignment{
				point: NewLocalAssignment(target, path.NewPath(target, "local"), source),
			},
		}),
		Sources: &recordingSourceValues{},
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
}

func TestFactsNodeTransferAbsentFactsAndNilResolverLeaveStateUnchanged(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(13)
	target := symbol.ID(104)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))

	gotNoResolver := NewFactsNodeTransfer(FactsNodeTransferConfig{Facts: Facts{}})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)
	assertStateEqual(t, reg, gotNoResolver, in)

	gotNoFacts := NewFactsNodeTransfer(FactsNodeTransferConfig{Sources: panicSourceValues{}})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)
	assertStateEqual(t, reg, gotNoFacts, in)
}

func TestFactsNodeTransferIgnoresNonRootAssignmentFacts(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(14)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(14), HasExpr: true}
	target := symbol.ID(105)
	in := state.State{}.WriteValue(reg, key.SymbolValue(target), presentValue(reg))
	resolver := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			OrdinaryAssignments: map[cfg.Point]OrdinaryAssignment{
				point: NewOrdinaryAssignment(target, path.NewPath(target, "ordinary").Field("member"), source),
			},
		}),
		Sources: resolver,
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
	if len(resolver.calls) != 0 {
		t.Fatalf("non-root assignment resolved source %d times, want zero", len(resolver.calls))
	}
}

func TestFactsNodeTransferPathAssignmentRequiresVisibility(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(17)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(17), HasExpr: true}
	target := symbol.ID(108)
	targetPath := path.NewPath(target, "table").Field("field")
	pathKey := path.PathKey("sym108@1.field")
	in := state.State{}.WritePathKey(reg, pathKey, presentValue(reg))
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			PathAssignments: map[cfg.Point]PathAssignment{
				point: NewPathAssignment(targetPath, source),
			},
		}),
		Sources: sources,
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
	if len(sources.calls) != 0 {
		t.Fatalf("path assignment without visibility resolved source %d times, want zero", len(sources.calls))
	}
}

func TestFactsNodeTransferPathAssignmentWithUnresolvedVersionIsNoop(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(18)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(18), HasExpr: true}
	target := symbol.ID(109)
	targetPath := path.NewPath(target, "table").Field("field")
	in := state.State{}.WritePathKey(reg, path.PathKey("sym109@1.field"), presentValue(reg))
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: absentValue(reg)},
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			PathAssignments: map[cfg.Point]PathAssignment{
				point: NewPathAssignment(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibility.NewTable(nil)),
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	assertStateEqual(t, reg, got, in)
}

func TestFactsNodeTransferIgnoresRootPathAssignment(t *testing.T) {
	reg := product.DefaultRegistry()
	point := cfg.Point(19)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(19), HasExpr: true}
	target := symbol.ID(110)
	sources := &recordingSourceValues{
		values: map[ValueSource]product.Value{source: absentValue(reg)},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: NewFacts(FactsInput{
			PathAssignments: map[cfg.Point]PathAssignment{
				point: NewPathAssignment(path.NewPath(target, "table"), source),
			},
		}),
		Sources:    sources,
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertStateEqual(t, reg, got, state.State{})
	if len(sources.calls) != 0 {
		t.Fatalf("root path assignment resolved source %d times, want zero", len(sources.calls))
	}
}

func TestFactsEdgeTransferAppliesNilRefinementsOnRootValue(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(301)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithPresence(path.NewPath(target, "x"), presence.Absent(), true, presence.Present(), true),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), absentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), presentValue(reg))
}

func TestFactsEdgeTransferOneSidedTruthyFalsyRefinements(t *testing.T) {
	tests := []struct {
		name      string
		fact      BranchRefinement
		wantTrue  product.Value
		wantFalse product.Value
	}{
		{
			name:      "truthy refines true edge only",
			fact:      branchWithPresence(path.NewPath(symbol.ID(302), "x"), presence.Present(), true, presence.Bottom(), false),
			wantTrue:  presentValue(product.DefaultRegistry()),
			wantFalse: product.Top(),
		},
		{
			name:      "falsy refines false edge only",
			fact:      branchWithPresence(path.NewPath(symbol.ID(303), "x"), presence.Bottom(), false, presence.Present(), true),
			wantTrue:  product.Top(),
			wantFalse: presentValue(product.DefaultRegistry()),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := product.DefaultRegistry()
			graph := cfg.New()
			branch := graph.AddNode(cfg.NodeBranch)
			thenPoint := graph.AddNode(cfg.NodeNoop)
			elsePoint := graph.AddNode(cfg.NodeNoop)
			graph.AddEdge(graph.Entry(), branch, false)
			graph.AddEdge(branch, thenPoint, true)
			graph.AddEdge(branch, elsePoint, false)
			graph.AddEdge(thenPoint, graph.Exit(), false)
			graph.AddEdge(elsePoint, graph.Exit(), false)

			target := tc.fact.TargetPath().Symbol
			initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
			got := Run(Config{
				Graph:      graph,
				Registry:   reg,
				EntryState: initial,
				EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
					Facts: NewFacts(FactsInput{
						BranchRefinements: map[cfg.Point]BranchRefinement{
							branch: tc.fact,
						},
					}),
				}),
			})

			assertValue(t, reg, got[thenPoint], key.SymbolValue(target), tc.wantTrue)
			assertValue(t, reg, got[elsePoint], key.SymbolValue(target), tc.wantFalse)
		})
	}
}

func TestFactsEdgeTransferRefinesStaticMemberPathThroughVisibility(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(304)
	targetPath := path.NewPath(target, "t").Field("field")
	pathKey := path.PathKey("sym304@1.field")
	initial := state.State{}.WritePathKey(reg, pathKey, product.Top())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "t")

	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithPresence(targetPath, presence.Present(), true, presence.Absent(), true),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertPathValue(t, reg, got[thenPoint], pathKey, presentValue(reg))
	assertPathValue(t, reg, got[elsePoint], pathKey, absentValue(reg))
	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), product.Bottom(reg))
}

func TestFactsEdgeTransferRefinesRuntimeKindOnRootValue(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(308)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithRuntimeKind(path.NewPath(target, "x"), runtimekind.Singleton(runtimekind.Table), true, runtimekind.Value{}, false),
				},
			}),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Table))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Top())
}

func TestFactsEdgeTransferRefinesRuntimeKindOnStaticMemberPath(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(309)
	targetPath := path.NewPath(target, "t").Field("field")
	pathKey := path.PathKey("sym309@1.field")
	initial := state.State{}.WritePathKey(reg, pathKey, product.Top())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, target, "t")

	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithRuntimeKind(targetPath, runtimekind.Singleton(runtimekind.Function), true, runtimekind.Value{}, false),
				},
			}),
			Visibility: visibility.NewResolver(visibilityBuilder.Build()),
		}),
	})

	assertRuntimeKind(t, reg, got[thenPoint].ReadPathKey(reg, pathKey), runtimekind.Singleton(runtimekind.Function))
	assertRuntimeKind(t, reg, got[elsePoint].ReadPathKey(reg, pathKey), runtimekind.Top())
}

func TestFactsEdgeTransferRuntimeKindContradictionGoesBottom(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(310)
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), numberValue)
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithRuntimeKind(path.NewPath(target, "x"), runtimekind.Singleton(runtimekind.Table), true, runtimekind.Value{}, false),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), product.Bottom(reg))
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), runtimekind.Singleton(runtimekind.Number))
}

func TestFactsEdgeTransferAppliesGenericProductConstraintAxis(t *testing.T) {
	reg := wideningRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	target := symbol.ID(312)
	initialValue := wideningValue(reg, wideningExactMax)
	constraint := product.Set(reg, product.Top(), wideningKey, wideningOne)
	trueRefinement := NewValueRefinement().WithConstraint(reg, constraint)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), initialValue)
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: NewBranchRefinement(path.NewPath(target, "x"), trueRefinement, true, ValueRefinement{}, false),
				},
			}),
		}),
	})

	if gotValue := product.Get(reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), wideningKey); gotValue != wideningOne {
		t.Fatalf("true edge custom axis = %v, want %v", gotValue, wideningOne)
	}
	if gotValue := product.Get(reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), wideningKey); gotValue != wideningExactMax {
		t.Fatalf("false edge custom axis = %v, want %v", gotValue, wideningExactMax)
	}
}

func TestFactsEdgeTransferNoopsWithoutBranchConditionOrVisibility(t *testing.T) {
	t.Run("non-branch edge", func(t *testing.T) {
		reg := product.DefaultRegistry()
		graph := cfg.New()
		mid := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(graph.Entry(), mid, false)
		graph.AddEdge(mid, graph.Exit(), false)

		target := symbol.ID(305)
		initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
		got := Run(Config{
			Graph:      graph,
			Registry:   reg,
			EntryState: initial,
			EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
				Facts: NewFacts(FactsInput{
					BranchRefinements: map[cfg.Point]BranchRefinement{
						graph.Entry(): branchWithPresence(path.NewPath(target, "x"), presence.Absent(), true, presence.Present(), true),
					},
				}),
			}),
		})

		assertValue(t, reg, got[mid], key.SymbolValue(target), product.Top())
	})

	t.Run("missing visibility for static path", func(t *testing.T) {
		reg := product.DefaultRegistry()
		graph := cfg.New()
		branch := graph.AddNode(cfg.NodeBranch)
		thenPoint := graph.AddNode(cfg.NodeNoop)
		elsePoint := graph.AddNode(cfg.NodeNoop)
		graph.AddEdge(graph.Entry(), branch, false)
		graph.AddEdge(branch, thenPoint, true)
		graph.AddEdge(branch, elsePoint, false)
		graph.AddEdge(thenPoint, graph.Exit(), false)
		graph.AddEdge(elsePoint, graph.Exit(), false)

		target := symbol.ID(306)
		targetPath := path.NewPath(target, "t").Field("field")
		pathKey := path.PathKey("sym306@1.field")
		initial := state.State{}.WritePathKey(reg, pathKey, product.Top())
		got := Run(Config{
			Graph:      graph,
			Registry:   reg,
			EntryState: initial,
			EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
				Facts: NewFacts(FactsInput{
					BranchRefinements: map[cfg.Point]BranchRefinement{
						branch: branchWithPresence(targetPath, presence.Present(), true, presence.Absent(), true),
					},
				}),
			}),
		})

		assertPathValue(t, reg, got[thenPoint], pathKey, product.Top())
		assertPathValue(t, reg, got[elsePoint], pathKey, product.Top())
	})
}

func TestFactsEdgeTransferJoinRestoresMaybePresence(t *testing.T) {
	reg := product.DefaultRegistry()
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

	target := symbol.ID(307)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithPresence(path.NewPath(target, "x"), presence.Absent(), true, presence.Present(), true),
				},
			}),
		}),
	})

	assertValue(t, reg, got[thenPoint], key.SymbolValue(target), absentValue(reg))
	assertValue(t, reg, got[elsePoint], key.SymbolValue(target), presentValue(reg))
	assertValue(t, reg, got[join], key.SymbolValue(target), product.Top())
}

func TestFactsEdgeTransferJoinRestoresRuntimeKindUnion(t *testing.T) {
	reg := product.DefaultRegistry()
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

	target := symbol.ID(311)
	initial := state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top())
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: NewFacts(FactsInput{
				BranchRefinements: map[cfg.Point]BranchRefinement{
					branch: branchWithRuntimeKind(
						path.NewPath(target, "x"),
						runtimekind.Singleton(runtimekind.Table), true,
						runtimekind.Singleton(runtimekind.Function), true,
					),
				},
			}),
		}),
	})

	tableKind := runtimekind.Singleton(runtimekind.Table)
	functionKind := runtimekind.Singleton(runtimekind.Function)
	assertRuntimeKind(t, reg, got[thenPoint].ReadValue(reg, key.SymbolValue(target)), tableKind)
	assertRuntimeKind(t, reg, got[elsePoint].ReadValue(reg, key.SymbolValue(target)), functionKind)
	assertRuntimeKind(t, reg, got[join].ReadValue(reg, key.SymbolValue(target)), runtimekind.Join(tableKind, functionKind))
}

type sourceValueCall struct {
	point  cfg.Point
	source ValueSource
}

type recordingSourceValues struct {
	values map[ValueSource]product.Value
	calls  []sourceValueCall
}

func (r *recordingSourceValues) ValueOfSource(
	point cfg.Point,
	source ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if read == nil {
		panic("nil read function")
	}
	_ = read(point)
	r.calls = append(r.calls, sourceValueCall{point: point, source: source})
	value, ok := r.values[source]
	return value, ok
}

type panicSourceValues struct{}

func (panicSourceValues) ValueOfSource(
	cfg.Point,
	ValueSource,
	state.State,
	func(cfg.Point) state.State,
) (product.Value, bool) {
	panic("ValueOfSource should not be called")
}

func assertResolverCall(t *testing.T, resolver *recordingSourceValues, point cfg.Point, source ValueSource) {
	t.Helper()
	if len(resolver.calls) != 1 {
		t.Fatalf("resolver calls = %d, want 1", len(resolver.calls))
	}
	if got := resolver.calls[0]; got.point != point || got.source != source {
		t.Fatalf("resolver call = %#v, want point %d source %#v", got, point, source)
	}
}

func assertStateEqual(t *testing.T, reg *axis.Registry, got state.State, want state.State) {
	t.Helper()
	if !state.Domain(reg).Equal(got, want) {
		t.Fatalf("state changed")
	}
}

func assertPathValue(t *testing.T, reg *axis.Registry, gotState state.State, pathKey path.PathKey, want product.Value) {
	t.Helper()
	if got := gotState.ReadPathKey(reg, pathKey); !product.Equal(reg, got, want) {
		t.Fatalf("path %s = %s, want %s", pathKey, formatValue(reg, got), formatValue(reg, want))
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtime kind = %s in %s, want %s", kind, formatValue(reg, got), want)
	}
}

func branchWithPresence(
	targetPath path.Path,
	truePresence presence.Value,
	hasTrue bool,
	falsePresence presence.Value,
	hasFalse bool,
) BranchRefinement {
	var trueValue ValueRefinement
	if hasTrue {
		trueValue = NewValueConstraint(product.NewWithPresence(product.DefaultRegistry(), product.ShapeTop, truePresence))
	}
	var falseValue ValueRefinement
	if hasFalse {
		falseValue = NewValueConstraint(product.NewWithPresence(product.DefaultRegistry(), product.ShapeTop, falsePresence))
	}
	return NewBranchRefinement(targetPath, trueValue, hasTrue, falseValue, hasFalse)
}

func branchWithRuntimeKind(
	targetPath path.Path,
	trueRuntimeKind runtimekind.Value,
	hasTrue bool,
	falseRuntimeKind runtimekind.Value,
	hasFalse bool,
) BranchRefinement {
	var trueValue ValueRefinement
	if hasTrue {
		trueValue = NewValueConstraint(product.Set(product.DefaultRegistry(), product.Top(), runtimekind.Key, trueRuntimeKind))
	}
	var falseValue ValueRefinement
	if hasFalse {
		falseValue = NewValueConstraint(product.Set(product.DefaultRegistry(), product.Top(), runtimekind.Key, falseRuntimeKind))
	}
	return NewBranchRefinement(targetPath, trueValue, hasTrue, falseValue, hasFalse)
}

func fieldSuffix(name string) path.Path {
	return path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: name}}}
}

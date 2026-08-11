package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// stagedGuardChainRule is a real Rule declaration used by the staged-read
// integration laws below.  Its input arity is deliberately caller-selected:
// zero-input rules seed a source point, while one-input rules preserve a
// guarded predecessor region without adding a second observation surface.
func stagedGuardChainRule(tb testing.TB, composition *Composition, semantic SemanticKey, admission uint64, output Output[uint64], form WriteForm[uint64], inputs int, value uint64) (*Rule[uint64, ruleUnit], Write[uint64]) {
	tb.Helper()
	var write Write[uint64]
	rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: semantic, Output: output, Inputs: inputs, Admission: testTrustedTheorem[uint64](admission),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, value) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var written bool
		write, written = WriteTo(rule, form)
		return written
	})
	if !declared || rule == nil {
		tb.Fatalf("staged guard-chain rule declaration for %v", semantic)
	}
	return rule, write
}

// TestStagedReadDistinctTagsShareOnePhysicalExactRead proves that semantic
// routes remain distinct even when they select one physical exact Unit.  The
// proof checker sees one static predecessor and one dynamic selected Unit,
// while Product sees both canonical tags in its Selection.
func TestStagedReadDistinctTagsShareOnePhysicalExactRead(t *testing.T) {
	const base uint64 = 291_000
	composition := NewComposition()
	trigger := coldFactor(composition, coldKey(base+1))
	dynamic := coldFactor(composition, coldKey(base+2))
	output := coldFactor(composition, coldKey(base+3))
	triggerRead, triggerReadOK := ExactReadForm(trigger)
	triggerWrite, triggerWriteOK := ExactWriteForm(trigger)
	dynamicRead, dynamicReadOK := ExactReadForm(dynamic)
	dynamicWrite, dynamicWriteOK := ExactWriteForm(dynamic)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if trigger == nil || dynamic == nil || output == nil || !triggerReadOK || !triggerWriteOK || !dynamicReadOK || !dynamicWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("distinct-tag staged factors/forms")
	}

	triggerSeed, triggerSeedWrite := stagedGuardChainRule(t, composition, coldKey(base+4), base+5, trigger.Output(), triggerWrite, 0, 1)
	dynamicSeed, dynamicSeedWrite := stagedGuardChainRule(t, composition, coldKey(base+6), base+7, dynamic.Output(), dynamicWrite, 0, 7)

	locatorCalls, productRows, evidenceCalls := 0, 0, 0
	var predecessor Read[OrderedCells[uint64]]
	var selected Read[Selection[uint64, OrderedCells[uint64]]]
	var projectWrite Write[uint64]
	var dynamicRef Ref[uint64]
	project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(base + 8), Output: output.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(base+9), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			evidenceCalls++
			// The two semantic Selection routes must compact to exactly one
			// dynamic evidence read alongside the one declared predecessor.
			if derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, dispositionOK := derivation.DispositionAt(0)
			predecessorCells, predecessorOK := DerivationDispositionReadValue(derivation, disposition, predecessor)
			selection, selectionOK := DerivationDispositionReadValue(derivation, disposition, selected)
			if !dispositionOK || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 1 ||
				!predecessorOK || waveCCell(predecessorCells) != 1 || !selectionOK || selection.count == nil || selection.at == nil {
				return RuleEvidence{}, false
			}
			count, countOK := selection.count(disposition.ordinal)
			if !countOK || count != 2 {
				return RuleEvidence{}, false
			}
			firstTag, firstCells, firstOK := selection.at(disposition.ordinal, 0)
			secondTag, secondCells, secondOK := selection.at(disposition.ordinal, 1)
			if !firstOK || !secondOK || firstTag != 3 || secondTag != 8 || waveCCell(firstCells) != 7 || waveCCell(secondCells) != 7 {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				productRows++
				triggerCells, triggerOK := ReadValue(access, row, predecessor)
				selection, selectionOK := ReadValue(access, row, selected)
				count, countOK := SelectionCount(access, row, selection)
				firstTag, firstCells, firstOK := SelectionAt(access, row, selection, 0)
				secondTag, secondCells, secondOK := SelectionAt(access, row, selection, 1)
				return triggerOK && waveCCell(triggerCells) == 1 && selectionOK && countOK && count == 2 &&
					firstOK && secondOK && firstTag == 3 && secondTag == 8 && waveCCell(firstCells) == 7 && waveCCell(secondCells) == 7 &&
					StageValue(access, row, uint64(78))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var predecessorOK, selectedOK, writeOK bool
		predecessor, predecessorOK = ReadFrom(rule, input, triggerRead)
		selected, selectedOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, dynamicRead, []Dependency{ReadDependency(predecessor)}, func(context SelectorContext, _ ruleUnit) bool {
			locatorCalls++
			cells, readable := SelectorRead(context, predecessor)
			return readable && waveCCell(cells) == 1 &&
				SelectRoute(context, dynamicRef, uint64(8)) && SelectRoute(context, dynamicRef, uint64(3))
		})
		projectWrite, writeOK = WriteTo(rule, outputWrite)
		return inputOK && predecessorOK && selectedOK && writeOK
	})
	if !projectOK || project == nil {
		t.Fatal("distinct-tag staged projection declaration")
	}

	var outputToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(base + 10),
		Project: func(observation Observation) uint64 {
			rows, result := 0, uint64(0)
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputToken)
				if !readable || waveCCell(cells) != 78 {
					return false
				}
				rows++
				result = 1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(base + 11)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		outputToken, declared = QueryReadFrom(query, outputRead)
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("distinct-tag staged query/composition")
	}
	triggerRef, triggerRefOK := trigger.Ref(0)
	dynamicRef, dynamicRefOK := dynamic.Ref(0)
	outputRef, outputRefOK := output.Ref(0)
	if !triggerRefOK || !dynamicRefOK || !outputRefOK {
		t.Fatal("distinct-tag staged refs")
	}
	triggerInstance, triggerInstanceOK := NewRuleInstance(triggerSeed, ruleUnitForSemantic(coldKey(base+12)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, triggerSeedWrite, triggerRef)
	})
	dynamicInstance, dynamicInstanceOK := NewRuleInstance(dynamicSeed, ruleUnitForSemantic(coldKey(base+13)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, dynamicSeedWrite, dynamicRef)
	})
	projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(base+14)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, predecessor, triggerRef) &&
			InstanceSelectorRead(binding, selected, dynamicRead) &&
			InstanceWrite(binding, projectWrite, outputRef)
	})
	if !triggerInstanceOK || !dynamicInstanceOK || !projectInstanceOK {
		t.Fatal("distinct-tag staged instances")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(base+15).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	projectSite, projectSiteOK := batch.AdmitSite(coldKey(base+16).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	triggerOccurrence, triggerOccurred := batch.Relation(seedSite, coldKey(base+17).compositionKey())
	dynamicOccurrence, dynamicOccurred := batch.Relation(seedSite, coldKey(base+18).compositionKey())
	projectOccurrence, projectOccurred := batch.Relation(projectSite, coldKey(base+19).compositionKey())
	triggerOperand, triggerOperandOK := admitInstanceOperand(batch, triggerOccurrence, triggerInstance)
	dynamicOperand, dynamicOperandOK := admitInstanceOperand(batch, dynamicOccurrence, dynamicInstance)
	projectOperand, projectOperandOK := admitInstanceOperand(batch, projectOccurrence, projectInstance)
	if !scope.Available() || !seedSiteOK || !projectSiteOK || !triggerOccurred || !dynamicOccurred || !projectOccurred ||
		!triggerOperandOK || !dynamicOperandOK || !projectOperandOK || !batch.Seal() {
		t.Fatal("distinct-tag staged source batch")
	}
	boundary := equation.BoundaryInput(seedSite, projectSite, coldKey(base+20).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint, projectPoint := admitPoint(assembly, seedSite), admitPoint(assembly, projectSite)
		triggerMember := admitInstance(assembly, seedPoint, triggerOccurrence, triggerOperand, triggerInstance)
		dynamicMember := admitInstance(assembly, seedPoint, dynamicOccurrence, dynamicOperand, dynamicInstance)
		projectMember := admitInstance(assembly, projectPoint, projectOccurrence, projectOperand, projectInstance)
		seedGroup := admitGroup(assembly, seedPoint, triggerMember, dynamicMember)
		projectGroup := admitGroup(assembly, projectPoint, projectMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, outputToken, outputRef)
		})
		return seedPoint != nil && projectPoint != nil && triggerMember != nil && dynamicMember != nil && projectMember != nil &&
			seedGroup != nil && projectGroup != nil && boundary.Available() && admitBoundary(assembly, projectGroup, boundary) &&
			queryInstanceOK && admitQueryAt(assembly, projectPoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("distinct-tag staged solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 1 {
		t.Fatalf("distinct-tag staged solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if locatorCalls != 1 || productRows != 1 || evidenceCalls != 1 {
		t.Fatalf("distinct-tag staged callbacks = locator:%d rows:%d evidence:%d", locatorCalls, productRows, evidenceCalls)
	}
}

// TestStagedReadGuardedAlternativesKeepSelectionsOnTheirRows builds two
// disjoint source guards, joins them at one Product predecessor, and selects
// a different exact target on each branch.  The only accepted outputs are
// A->refA and B->refB; a Selection reused across Product rows would produce
// a mixed pair and reject the transfer or derivation.
func TestStagedReadGuardedAlternativesKeepSelectionsOnTheirRows(t *testing.T) {
	const base uint64 = 291_100
	composition := NewComposition()
	control := coldFactor(composition, coldKey(base+1))
	dynamicSpec := coldFactorSpec(coldKey(base + 2))
	dynamicSpec.KeyEnd = 2
	dynamic, dynamicOK := DeclareFactor(composition, dynamicSpec, func(*Factor[uint64, uint64]) bool { return true })
	output := coldFactor(composition, coldKey(base+3))
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	dynamicRead, dynamicReadOK := ExactReadForm(dynamic)
	dynamicWrite, dynamicWriteOK := ExactWriteForm(dynamic)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if control == nil || !dynamicOK || dynamic == nil || output == nil ||
		!controlReadOK || !controlWriteOK || !dynamicReadOK || !dynamicWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("guarded staged factors/forms")
	}

	controlA, controlAWrite := stagedGuardChainRule(t, composition, coldKey(base+4), base+5, control.Output(), controlWrite, 1, 1)
	controlB, controlBWrite := stagedGuardChainRule(t, composition, coldKey(base+6), base+7, control.Output(), controlWrite, 1, 2)
	dynamicA, dynamicAWrite := stagedGuardChainRule(t, composition, coldKey(base+8), base+9, dynamic.Output(), dynamicWrite, 1, 101)
	dynamicB, dynamicBWrite := stagedGuardChainRule(t, composition, coldKey(base+10), base+11, dynamic.Output(), dynamicWrite, 1, 202)

	locatorCalls, productRows, evidenceCalls := 0, 0, 0
	var predecessor Read[OrderedCells[uint64]]
	var selected Read[Selection[uint64, OrderedCells[uint64]]]
	var projectWrite Write[uint64]
	var dynamicARef, dynamicBRef Ref[uint64]
	project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(base + 12), Output: output.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(base+13), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			evidenceCalls++
			if derivation.ReadCount() != 3 || derivation.DispositionCount() != 2 {
				return RuleEvidence{}, false
			}
			seen := uint64(0)
			for index := 0; index < derivation.DispositionCount(); index++ {
				disposition, dispositionOK := derivation.DispositionAt(index)
				controlCells, controlOK := DerivationDispositionReadValue(derivation, disposition, predecessor)
				selection, selectionOK := DerivationDispositionReadValue(derivation, disposition, selected)
				if !dispositionOK || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 1 || !controlOK || !selectionOK || selection.count == nil || selection.at == nil {
					return RuleEvidence{}, false
				}
				count, countOK := selection.count(disposition.ordinal)
				tag, dynamicCells, routeOK := selection.at(disposition.ordinal, 0)
				if !countOK || count != 1 || !routeOK {
					return RuleEvidence{}, false
				}
				switch waveCCell(controlCells) {
				case 1:
					if tag != 11 || waveCCell(dynamicCells) != 101 {
						return RuleEvidence{}, false
					}
					seen |= 1
				case 2:
					if tag != 22 || waveCCell(dynamicCells) != 202 {
						return RuleEvidence{}, false
					}
					seen |= 2
				default:
					return RuleEvidence{}, false
				}
			}
			if seen != 3 {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				productRows++
				controlCells, controlOK := ReadValue(access, row, predecessor)
				selection, selectionOK := ReadValue(access, row, selected)
				count, countOK := SelectionCount(access, row, selection)
				tag, dynamicCells, routeOK := SelectionAt(access, row, selection, 0)
				if !controlOK || !selectionOK || !countOK || count != 1 || !routeOK {
					return false
				}
				switch waveCCell(controlCells) {
				case 1:
					return tag == 11 && waveCCell(dynamicCells) == 101 && StageValue(access, row, uint64(1101))
				case 2:
					return tag == 22 && waveCCell(dynamicCells) == 202 && StageValue(access, row, uint64(2202))
				default:
					return false
				}
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var predecessorOK, selectedOK, writeOK bool
		predecessor, predecessorOK = ReadFrom(rule, input, controlRead)
		selected, selectedOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, dynamicRead, []Dependency{ReadDependency(predecessor)}, func(context SelectorContext, _ ruleUnit) bool {
			locatorCalls++
			cells, readable := SelectorRead(context, predecessor)
			switch waveCCell(cells) {
			case 1:
				return readable && SelectRoute(context, dynamicARef, uint64(11))
			case 2:
				return readable && SelectRoute(context, dynamicBRef, uint64(22))
			default:
				return false
			}
		})
		projectWrite, writeOK = WriteTo(rule, outputWrite)
		return inputOK && predecessorOK && selectedOK && writeOK
	})
	if !projectOK || project == nil {
		t.Fatal("guarded staged projection declaration")
	}

	var outputToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(base + 14),
		Project: func(observation Observation) uint64 {
			rows, seen := 0, uint64(0)
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputToken)
				if !readable {
					return false
				}
				switch waveCCell(cells) {
				case 1101:
					seen |= 1
				case 2202:
					seen |= 2
				default:
					return false
				}
				rows++
				return true
			}) || rows != 2 || seen != 3 {
				return 0
			}
			return seen
		},
		Result: frozenColdResult(coldKey(base + 15)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		outputToken, declared = QueryReadFrom(query, outputRead)
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("guarded staged query/composition")
	}
	controlRef, controlRefOK := control.Ref(0)
	dynamicARef, dynamicARefOK := dynamic.Ref(0)
	dynamicBRef, dynamicBRefOK := dynamic.Ref(1)
	outputRef, outputRefOK := output.Ref(0)
	if !controlRefOK || !dynamicARefOK || !dynamicBRefOK || !outputRefOK {
		t.Fatal("guarded staged refs")
	}
	controlAInstance, controlAInstanceOK := NewRuleInstance(controlA, ruleUnitForSemantic(coldKey(base+16)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlAWrite, controlRef)
	})
	controlBInstance, controlBInstanceOK := NewRuleInstance(controlB, ruleUnitForSemantic(coldKey(base+17)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlBWrite, controlRef)
	})
	dynamicAInstance, dynamicAInstanceOK := NewRuleInstance(dynamicA, ruleUnitForSemantic(coldKey(base+18)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, dynamicAWrite, dynamicARef)
	})
	dynamicBInstance, dynamicBInstanceOK := NewRuleInstance(dynamicB, ruleUnitForSemantic(coldKey(base+19)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, dynamicBWrite, dynamicBRef)
	})
	projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(base+20)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, predecessor, controlRef) &&
			InstanceSelectorRead(binding, selected, dynamicRead) &&
			InstanceWrite(binding, projectWrite, outputRef)
	})
	if !controlAInstanceOK || !controlBInstanceOK || !dynamicAInstanceOK || !dynamicBInstanceOK || !projectInstanceOK {
		t.Fatal("guarded staged instances")
	}

	decision, decisionOK := equation.NewDecision(coldKey(base + 21).compositionKey())
	scope, scopeOK := equation.NewScope(decision)
	whenA, whenAOK := equation.DecisionExpr(decision)
	whenB, whenBOK := equation.NotExpr(whenA)
	batch := equation.NewBatch()
	sourceA, sourceAOK := batch.AdmitSite(coldKey(base+22).compositionKey(), scope, whenA, equation.InitPresent)
	sourceB, sourceBOK := batch.AdmitSite(coldKey(base+23).compositionKey(), scope, whenB, equation.InitPresent)
	middle, middleOK := batch.AdmitSite(coldKey(base+24).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	projectSite, projectSiteOK := batch.AdmitSite(coldKey(base+25).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	controlAOccurrence, controlAOccurred := batch.Relation(middle, coldKey(base+26).compositionKey())
	controlBOccurrence, controlBOccurred := batch.Relation(middle, coldKey(base+27).compositionKey())
	dynamicAOccurrence, dynamicAOccurred := batch.Relation(middle, coldKey(base+28).compositionKey())
	dynamicBOccurrence, dynamicBOccurred := batch.Relation(middle, coldKey(base+29).compositionKey())
	projectOccurrence, projectOccurred := batch.Relation(projectSite, coldKey(base+30).compositionKey())
	controlAOperand, controlAOperandOK := admitInstanceOperand(batch, controlAOccurrence, controlAInstance)
	controlBOperand, controlBOperandOK := admitInstanceOperand(batch, controlBOccurrence, controlBInstance)
	dynamicAOperand, dynamicAOperandOK := admitInstanceOperand(batch, dynamicAOccurrence, dynamicAInstance)
	dynamicBOperand, dynamicBOperandOK := admitInstanceOperand(batch, dynamicBOccurrence, dynamicBInstance)
	projectOperand, projectOperandOK := admitInstanceOperand(batch, projectOccurrence, projectInstance)
	if !decisionOK || !scopeOK || !whenAOK || !whenBOK || !scope.Available() ||
		!sourceAOK || !sourceBOK || !middleOK || !projectSiteOK ||
		!controlAOccurred || !controlBOccurred || !dynamicAOccurred || !dynamicBOccurred || !projectOccurred ||
		!controlAOperandOK || !controlBOperandOK || !dynamicAOperandOK || !dynamicBOperandOK || !projectOperandOK || !batch.Seal() {
		t.Fatal("guarded staged source batch")
	}
	boundaryA := equation.BoundaryInput(sourceA, middle, coldKey(base+31).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	boundaryB := equation.BoundaryInput(sourceB, middle, coldKey(base+32).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	boundaryProject := equation.BoundaryInput(middle, projectSite, coldKey(base+33).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourceAPoint, sourceBPoint := admitPoint(assembly, sourceA), admitPoint(assembly, sourceB)
		middlePoint, projectPoint := admitPoint(assembly, middle), admitPoint(assembly, projectSite)
		controlAMember := admitInstance(assembly, middlePoint, controlAOccurrence, controlAOperand, controlAInstance)
		controlBMember := admitInstance(assembly, middlePoint, controlBOccurrence, controlBOperand, controlBInstance)
		dynamicAMember := admitInstance(assembly, middlePoint, dynamicAOccurrence, dynamicAOperand, dynamicAInstance)
		dynamicBMember := admitInstance(assembly, middlePoint, dynamicBOccurrence, dynamicBOperand, dynamicBInstance)
		projectMember := admitInstance(assembly, projectPoint, projectOccurrence, projectOperand, projectInstance)
		groupA := admitGroup(assembly, middlePoint, controlAMember, dynamicAMember)
		groupB := admitGroup(assembly, middlePoint, controlBMember, dynamicBMember)
		projectGroup := admitGroup(assembly, projectPoint, projectMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, outputToken, outputRef)
		})
		return sourceAPoint != nil && sourceBPoint != nil && middlePoint != nil && projectPoint != nil &&
			controlAMember != nil && controlBMember != nil && dynamicAMember != nil && dynamicBMember != nil && projectMember != nil &&
			groupA != nil && groupB != nil && projectGroup != nil &&
			boundaryA.Available() && boundaryB.Available() && boundaryProject.Available() &&
			admitBoundary(assembly, groupA, boundaryA) && admitBoundary(assembly, groupB, boundaryB) && admitBoundary(assembly, projectGroup, boundaryProject) &&
			queryInstanceOK && admitQueryAt(assembly, projectPoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("guarded staged solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 3 {
		t.Fatalf("guarded staged solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if locatorCalls != 2 || productRows != 2 || evidenceCalls != 1 {
		t.Fatalf("guarded staged callbacks = locator:%d rows:%d evidence:%d", locatorCalls, productRows, evidenceCalls)
	}
}

// TestStagedReadLocatorChainReadsOnlyItsCurrentSelectionRow exercises the
// canonical Value -> Call -> Heap shape with two real staged reads.  The Heap
// locator declares only the preceding Call Selection as a dependency and
// resolves it through SelectorRead; it never receives a Value read or an
// ambient product/state handle from which it could cross rows.
func TestStagedReadLocatorChainReadsOnlyItsCurrentSelectionRow(t *testing.T) {
	const base uint64 = 291_200
	composition := NewComposition()
	valueFactor := coldFactor(composition, coldKey(base+1))
	callSpec := coldFactorSpec(coldKey(base + 2))
	callSpec.KeyEnd = 2
	callFactor, callFactorOK := DeclareFactor(composition, callSpec, func(*Factor[uint64, uint64]) bool { return true })
	heapSpec := coldFactorSpec(coldKey(base + 3))
	heapSpec.KeyEnd = 2
	heapFactor, heapFactorOK := DeclareFactor(composition, heapSpec, func(*Factor[uint64, uint64]) bool { return true })
	output := coldFactor(composition, coldKey(base+4))
	valueRead, valueReadOK := ExactReadForm(valueFactor)
	valueWrite, valueWriteOK := ExactWriteForm(valueFactor)
	callRead, callReadOK := ExactReadForm(callFactor)
	callWrite, callWriteOK := ExactWriteForm(callFactor)
	heapRead, heapReadOK := ExactReadForm(heapFactor)
	heapWrite, heapWriteOK := ExactWriteForm(heapFactor)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if valueFactor == nil || !callFactorOK || callFactor == nil || !heapFactorOK || heapFactor == nil || output == nil ||
		!valueReadOK || !valueWriteOK || !callReadOK || !callWriteOK || !heapReadOK || !heapWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("staged locator-chain factors/forms")
	}

	valueA, valueAWrite := stagedGuardChainRule(t, composition, coldKey(base+5), base+6, valueFactor.Output(), valueWrite, 1, 1)
	valueB, valueBWrite := stagedGuardChainRule(t, composition, coldKey(base+7), base+8, valueFactor.Output(), valueWrite, 1, 2)
	callA, callAWrite := stagedGuardChainRule(t, composition, coldKey(base+9), base+10, callFactor.Output(), callWrite, 1, 10)
	callB, callBWrite := stagedGuardChainRule(t, composition, coldKey(base+11), base+12, callFactor.Output(), callWrite, 1, 20)
	heapA, heapAWrite := stagedGuardChainRule(t, composition, coldKey(base+13), base+14, heapFactor.Output(), heapWrite, 1, 100)
	heapB, heapBWrite := stagedGuardChainRule(t, composition, coldKey(base+15), base+16, heapFactor.Output(), heapWrite, 1, 200)

	valueLocatorCalls, heapLocatorCalls, productRows, evidenceCalls := 0, 0, 0, 0
	var valueSelection Read[OrderedCells[uint64]]
	var callSelection Read[Selection[uint64, OrderedCells[uint64]]]
	var heapSelection Read[Selection[uint64, OrderedCells[uint64]]]
	var projectWrite Write[uint64]
	var callARef, callBRef, heapARef, heapBRef Ref[uint64]
	project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(base + 17), Output: output.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(base+18), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			evidenceCalls++
			// One static Value read plus the A/B Call and A/B Heap exact Units.
			if derivation.ReadCount() != 5 || derivation.DispositionCount() != 2 {
				return RuleEvidence{}, false
			}
			seen := uint64(0)
			for index := 0; index < derivation.DispositionCount(); index++ {
				disposition, dispositionOK := derivation.DispositionAt(index)
				valueCells, valueOK := DerivationDispositionReadValue(derivation, disposition, valueSelection)
				callResult, callOK := DerivationDispositionReadValue(derivation, disposition, callSelection)
				heapResult, heapOK := DerivationDispositionReadValue(derivation, disposition, heapSelection)
				if !dispositionOK || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 1 ||
					!valueOK || !callOK || !heapOK || callResult.count == nil || callResult.at == nil || heapResult.count == nil || heapResult.at == nil {
					return RuleEvidence{}, false
				}
				callCount, callCountOK := callResult.count(disposition.ordinal)
				heapCount, heapCountOK := heapResult.count(disposition.ordinal)
				callTag, callCells, callRouteOK := callResult.at(disposition.ordinal, 0)
				heapTag, heapCells, heapRouteOK := heapResult.at(disposition.ordinal, 0)
				if !callCountOK || !heapCountOK || callCount != 1 || heapCount != 1 || !callRouteOK || !heapRouteOK {
					return RuleEvidence{}, false
				}
				switch waveCCell(valueCells) {
				case 1:
					if callTag != 101 || waveCCell(callCells) != 10 || heapTag != 1001 || waveCCell(heapCells) != 100 {
						return RuleEvidence{}, false
					}
					seen |= 1
				case 2:
					if callTag != 202 || waveCCell(callCells) != 20 || heapTag != 2002 || waveCCell(heapCells) != 200 {
						return RuleEvidence{}, false
					}
					seen |= 2
				default:
					return RuleEvidence{}, false
				}
			}
			if seen != 3 {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				productRows++
				valueCells, valueOK := ReadValue(access, row, valueSelection)
				callResult, callOK := ReadValue(access, row, callSelection)
				heapResult, heapOK := ReadValue(access, row, heapSelection)
				callCount, callCountOK := SelectionCount(access, row, callResult)
				heapCount, heapCountOK := SelectionCount(access, row, heapResult)
				callTag, callCells, callRouteOK := SelectionAt(access, row, callResult, 0)
				heapTag, heapCells, heapRouteOK := SelectionAt(access, row, heapResult, 0)
				if !valueOK || !callOK || !heapOK || !callCountOK || !heapCountOK || callCount != 1 || heapCount != 1 || !callRouteOK || !heapRouteOK {
					return false
				}
				switch waveCCell(valueCells) {
				case 1:
					return callTag == 101 && waveCCell(callCells) == 10 && heapTag == 1001 && waveCCell(heapCells) == 100 && StageValue(access, row, uint64(11100))
				case 2:
					return callTag == 202 && waveCCell(callCells) == 20 && heapTag == 2002 && waveCCell(heapCells) == 200 && StageValue(access, row, uint64(22200))
				default:
					return false
				}
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var valueReadOK, callReadOK, heapReadOK, writeOK bool
		valueSelection, valueReadOK = ReadFrom(rule, input, valueRead)
		callSelection, callReadOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, callRead, []Dependency{ReadDependency(valueSelection)}, func(context SelectorContext, _ ruleUnit) bool {
			valueLocatorCalls++
			cells, readable := SelectorRead(context, valueSelection)
			switch waveCCell(cells) {
			case 1:
				return readable && SelectRoute(context, callARef, uint64(101))
			case 2:
				return readable && SelectRoute(context, callBRef, uint64(202))
			default:
				return false
			}
		})
		heapSelection, heapReadOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, heapRead, []Dependency{ReadDependency(callSelection)}, func(context SelectorContext, _ ruleUnit) bool {
			heapLocatorCalls++
			// This locator intentionally observes only its immediately preceding
			// Selection. SelectorRead scopes that value to this Product row.
			calls, readable := SelectorRead(context, callSelection)
			count, countOK := SelectorSelectionCount(context, calls)
			tag, cells, routeOK := SelectorSelectionAt(context, calls, 0)
			if !readable || !countOK || count != 1 || !routeOK {
				return false
			}
			switch {
			case tag == 101 && waveCCell(cells) == 10:
				return SelectRoute(context, heapARef, uint64(1001))
			case tag == 202 && waveCCell(cells) == 20:
				return SelectRoute(context, heapBRef, uint64(2002))
			default:
				return false
			}
		})
		projectWrite, writeOK = WriteTo(rule, outputWrite)
		return inputOK && valueReadOK && callReadOK && heapReadOK && writeOK
	})
	if !projectOK || project == nil {
		t.Fatal("staged locator-chain projection declaration")
	}

	var outputToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(base + 19),
		Project: func(observation Observation) uint64 {
			rows, seen := 0, uint64(0)
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputToken)
				if !readable {
					return false
				}
				switch waveCCell(cells) {
				case 11100:
					seen |= 1
				case 22200:
					seen |= 2
				default:
					return false
				}
				rows++
				return true
			}) || rows != 2 || seen != 3 {
				return 0
			}
			return seen
		},
		Result: frozenColdResult(coldKey(base + 20)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		outputToken, declared = QueryReadFrom(query, outputRead)
		return declared
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("staged locator-chain query/composition")
	}
	valueRef, valueRefOK := valueFactor.Ref(0)
	callARef, callARefOK := callFactor.Ref(0)
	callBRef, callBRefOK := callFactor.Ref(1)
	heapARef, heapARefOK := heapFactor.Ref(0)
	heapBRef, heapBRefOK := heapFactor.Ref(1)
	outputRef, outputRefOK := output.Ref(0)
	if !valueRefOK || !callARefOK || !callBRefOK || !heapARefOK || !heapBRefOK || !outputRefOK {
		t.Fatal("staged locator-chain refs")
	}
	valueAInstance, valueAInstanceOK := NewRuleInstance(valueA, ruleUnitForSemantic(coldKey(base+21)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, valueAWrite, valueRef)
	})
	valueBInstance, valueBInstanceOK := NewRuleInstance(valueB, ruleUnitForSemantic(coldKey(base+22)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, valueBWrite, valueRef)
	})
	callAInstance, callAInstanceOK := NewRuleInstance(callA, ruleUnitForSemantic(coldKey(base+23)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, callAWrite, callARef)
	})
	callBInstance, callBInstanceOK := NewRuleInstance(callB, ruleUnitForSemantic(coldKey(base+24)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, callBWrite, callBRef)
	})
	heapAInstance, heapAInstanceOK := NewRuleInstance(heapA, ruleUnitForSemantic(coldKey(base+25)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, heapAWrite, heapARef)
	})
	heapBInstance, heapBInstanceOK := NewRuleInstance(heapB, ruleUnitForSemantic(coldKey(base+26)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, heapBWrite, heapBRef)
	})
	projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(base+27)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, valueSelection, valueRef) &&
			InstanceSelectorRead(binding, callSelection, callRead) &&
			InstanceSelectorRead(binding, heapSelection, heapRead) &&
			InstanceWrite(binding, projectWrite, outputRef)
	})
	if !valueAInstanceOK || !valueBInstanceOK || !callAInstanceOK || !callBInstanceOK || !heapAInstanceOK || !heapBInstanceOK || !projectInstanceOK {
		t.Fatal("staged locator-chain instances")
	}

	decision, decisionOK := equation.NewDecision(coldKey(base + 28).compositionKey())
	scope, scopeOK := equation.NewScope(decision)
	whenA, whenAOK := equation.DecisionExpr(decision)
	whenB, whenBOK := equation.NotExpr(whenA)
	batch := equation.NewBatch()
	sourceA, sourceAOK := batch.AdmitSite(coldKey(base+29).compositionKey(), scope, whenA, equation.InitPresent)
	sourceB, sourceBOK := batch.AdmitSite(coldKey(base+30).compositionKey(), scope, whenB, equation.InitPresent)
	middle, middleOK := batch.AdmitSite(coldKey(base+31).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	projectSite, projectSiteOK := batch.AdmitSite(coldKey(base+32).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	valueAOccurrence, valueAOccurred := batch.Relation(middle, coldKey(base+33).compositionKey())
	valueBOccurrence, valueBOccurred := batch.Relation(middle, coldKey(base+34).compositionKey())
	callAOccurrence, callAOccurred := batch.Relation(middle, coldKey(base+35).compositionKey())
	callBOccurrence, callBOccurred := batch.Relation(middle, coldKey(base+36).compositionKey())
	heapAOccurrence, heapAOccurred := batch.Relation(middle, coldKey(base+37).compositionKey())
	heapBOccurrence, heapBOccurred := batch.Relation(middle, coldKey(base+38).compositionKey())
	projectOccurrence, projectOccurred := batch.Relation(projectSite, coldKey(base+39).compositionKey())
	valueAOperand, valueAOperandOK := admitInstanceOperand(batch, valueAOccurrence, valueAInstance)
	valueBOperand, valueBOperandOK := admitInstanceOperand(batch, valueBOccurrence, valueBInstance)
	callAOperand, callAOperandOK := admitInstanceOperand(batch, callAOccurrence, callAInstance)
	callBOperand, callBOperandOK := admitInstanceOperand(batch, callBOccurrence, callBInstance)
	heapAOperand, heapAOperandOK := admitInstanceOperand(batch, heapAOccurrence, heapAInstance)
	heapBOperand, heapBOperandOK := admitInstanceOperand(batch, heapBOccurrence, heapBInstance)
	projectOperand, projectOperandOK := admitInstanceOperand(batch, projectOccurrence, projectInstance)
	if !decisionOK || !scopeOK || !whenAOK || !whenBOK || !scope.Available() ||
		!sourceAOK || !sourceBOK || !middleOK || !projectSiteOK ||
		!valueAOccurred || !valueBOccurred || !callAOccurred || !callBOccurred || !heapAOccurred || !heapBOccurred || !projectOccurred ||
		!valueAOperandOK || !valueBOperandOK || !callAOperandOK || !callBOperandOK || !heapAOperandOK || !heapBOperandOK || !projectOperandOK || !batch.Seal() {
		t.Fatal("staged locator-chain source batch")
	}
	boundaryA := equation.BoundaryInput(sourceA, middle, coldKey(base+40).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	boundaryB := equation.BoundaryInput(sourceB, middle, coldKey(base+41).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	boundaryProject := equation.BoundaryInput(middle, projectSite, coldKey(base+42).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourceAPoint, sourceBPoint := admitPoint(assembly, sourceA), admitPoint(assembly, sourceB)
		middlePoint, projectPoint := admitPoint(assembly, middle), admitPoint(assembly, projectSite)
		valueAMember := admitInstance(assembly, middlePoint, valueAOccurrence, valueAOperand, valueAInstance)
		valueBMember := admitInstance(assembly, middlePoint, valueBOccurrence, valueBOperand, valueBInstance)
		callAMember := admitInstance(assembly, middlePoint, callAOccurrence, callAOperand, callAInstance)
		callBMember := admitInstance(assembly, middlePoint, callBOccurrence, callBOperand, callBInstance)
		heapAMember := admitInstance(assembly, middlePoint, heapAOccurrence, heapAOperand, heapAInstance)
		heapBMember := admitInstance(assembly, middlePoint, heapBOccurrence, heapBOperand, heapBInstance)
		projectMember := admitInstance(assembly, projectPoint, projectOccurrence, projectOperand, projectInstance)
		groupA := admitGroup(assembly, middlePoint, valueAMember, callAMember, heapAMember)
		groupB := admitGroup(assembly, middlePoint, valueBMember, callBMember, heapBMember)
		projectGroup := admitGroup(assembly, projectPoint, projectMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, outputToken, outputRef)
		})
		return sourceAPoint != nil && sourceBPoint != nil && middlePoint != nil && projectPoint != nil &&
			valueAMember != nil && valueBMember != nil && callAMember != nil && callBMember != nil && heapAMember != nil && heapBMember != nil && projectMember != nil &&
			groupA != nil && groupB != nil && projectGroup != nil &&
			boundaryA.Available() && boundaryB.Available() && boundaryProject.Available() &&
			admitBoundary(assembly, groupA, boundaryA) && admitBoundary(assembly, groupB, boundaryB) && admitBoundary(assembly, projectGroup, boundaryProject) &&
			queryInstanceOK && admitQueryAt(assembly, projectPoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("staged locator-chain solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 3 {
		t.Fatalf("staged locator-chain solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if valueLocatorCalls != 2 || heapLocatorCalls != 2 || productRows != 2 || evidenceCalls != 1 {
		t.Fatalf("staged locator-chain callbacks = value:%d heap:%d rows:%d evidence:%d", valueLocatorCalls, heapLocatorCalls, productRows, evidenceCalls)
	}
}

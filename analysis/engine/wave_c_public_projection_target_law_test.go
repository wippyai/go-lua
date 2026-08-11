package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestPublicSummaryReadObservesExplicitWholePlane(t *testing.T) {
	composition := NewComposition()
	seedCalls, projectCalls := 0, 0
	var summary ReadForm[uint64, uint64]
	sourceSpec := coldFactorSpec(coldKey(94_000))
	sourceSpec.KeyEnd = 3
	source, sourceOK := DeclareFactor(composition, sourceSpec, func(factor *Factor[uint64, uint64]) bool {
		normalizer, declared := DeclareNormalizer(factor, coldKey(94_001), func(cells OrderedCells[uint64]) uint64 {
			var total uint64
			for index := 0; index < cells.Count(); index++ {
				value, present, valid := cells.At(index)
				if !valid {
					return 0
				}
				if present {
					total += value
				}
			}
			return total
		}, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		if !declared {
			return false
		}
		summary, declared = SummaryReadForm(normalizer)
		return declared
	})
	sink, sinkOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_002)), func(*Factor[uint64, uint64]) bool { return true })
	if !sourceOK || source == nil || !sinkOK || sink == nil {
		t.Fatal("summary factors")
	}
	sourceWrite, sourceWriteOK := ExactWriteForm(source)
	sinkRead, sinkReadOK := ExactReadForm(sink)
	sinkWrite, sinkWriteOK := ExactWriteForm(sink)
	if !sourceWriteOK || !sinkReadOK || !sinkWriteOK {
		t.Fatal("summary forms")
	}
	var seedWrites [3]Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_003), Output: source.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_004),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			seedCalls++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		for index := 0; index < 3; index++ {
			write, written := WriteTo(rule, sourceWrite)
			if !written {
				return false
			}
			seedWrites[index] = write
		}
		return true
	})
	var whole Read[uint64]
	var projectWrite Write[uint64]
	project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_005), Output: sink.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](94_006),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			projectCalls++
			return Product(access, func(row Row) bool {
				value, readable := ReadValue(access, row, whole)
				return readable && StageValue(access, row, value)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK bool
		whole, readOK = ReadFrom(rule, input, summary)
		var writeOK bool
		projectWrite, writeOK = WriteTo(rule, sinkWrite)
		return inputOK && readOK && writeOK
	})
	var sinkToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_007), Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, sinkToken)
				if !ok {
					return false
				}
				result = waveCCell(cells)
				return result != 0
			}) {
				return 0
			}
			return result
		}, Result: frozenColdResult(coldKey(94_008)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		sinkToken, ok = QueryReadFrom(query, sinkRead)
		return ok
	})
	if !seedOK || seed == nil || !projectOK || project == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("summary declarations")
	}
	refs := source.NewClosedRefs()
	if refs == nil {
		t.Fatal("summary assembly")
	}
	for key := uint64(0); key < 3; key++ {
		ref, issued := source.Ref(key)
		if !issued || !refs.Append(ref) {
			t.Fatal("summary source reference")
		}
	}
	sinkRef, sinkIssued := sink.Ref(0)
	if !sinkIssued || !refs.Close() {
		t.Fatal("summary references close")
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(94_009).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	projectSite, projectSiteOK := batch.AdmitSite(coldKey(94_010).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurred := batch.Relation(seedSite, coldKey(94_011).compositionKey())
	projectOccurrence, projectOccurred := batch.Relation(projectSite, coldKey(94_012).compositionKey())
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(94_013)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		for key := uint64(0); key < 3; key++ {
			ref, issued := source.Ref(key)
			if !issued || !InstanceWrite(binding, seedWrites[key], ref) {
				return false
			}
		}
		return true
	})
	projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(94_014)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceSummaryRead(binding, whole, summary, refs) && InstanceWrite(binding, projectWrite, sinkRef)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	projectOperand, projectOperandOK := admitInstanceOperand(batch, projectOccurrence, projectInstance)
	if !scope.Available() || !seedSiteOK || !projectSiteOK || !seedOccurred || !projectOccurred || !seedInstanceOK || !projectInstanceOK || !seedOperandOK || !projectOperandOK || !batch.Seal() {
		t.Fatal("summary source batch")
	}
	boundary := equation.BoundaryInput(seedSite, projectSite, coldKey(94_015).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint, projectPoint := admitPoint(assembly, seedSite), admitPoint(assembly, projectSite)
		seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
		projectMember := admitInstance(assembly, projectPoint, projectOccurrence, projectOperand, projectInstance)
		seedGroup := admitGroup(assembly, seedPoint, seedMember)
		projectGroup := admitGroup(assembly, projectPoint, projectMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, sinkToken, sinkRef)
		})
		observation := admitQueryAt(assembly, projectPoint, queryInstance)
		if seedPoint == nil || projectPoint == nil || !seedInstanceOK || !projectInstanceOK || seedGroup == nil || projectGroup == nil ||
			!admitBoundary(assembly, projectGroup, boundary) || observation == nil || !queryInstanceOK {
			t.Fatal("summary assembly binding")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("summary solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 21 {
		t.Fatalf("summary solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if seedCalls != 1 || projectCalls != 1 {
		t.Fatalf("summary callbacks = seed:%d project:%d, want 1/1", seedCalls, projectCalls)
	}
}

func TestPublicSelectWriteExecutesStrongAndWeakSummaryTargets(t *testing.T) {
	composition := NewComposition()
	control := coldFactor(composition, coldKey(94_100))
	var outputSummary ReadForm[uint64, uint64]
	var writeSelector WriteForm[uint64]
	output, outputOK := DeclareFactor(composition, FactorSpec[uint64, uint64]{
		Semantic: coldKey(94_101), KeyEnd: 3, Lattice: coldUintLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(factor *Factor[uint64, uint64]) bool {
		normalizer, normalizerOK := DeclareNormalizer(factor, coldKey(94_102), func(cells OrderedCells[uint64]) uint64 { return uint64(cells.Count()) }, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		if !normalizerOK {
			return false
		}
		var summaryOK, selectorOK bool
		outputSummary, summaryOK = SummaryReadForm(normalizer)
		writeSelector, selectorOK = DeclareWriteSelector(factor, coldKey(94_103))
		return summaryOK && selectorOK
	})
	if control == nil || !outputOK || output == nil {
		t.Fatal("select-write factors")
	}
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	outputRead, outputReadOK := ExactReadForm(output)
	if !controlReadOK || !controlWriteOK || !outputReadOK || !outputSummary.valid() || !writeSelector.valid() {
		t.Fatal("select-write forms")
	}
	seedCalls, transferCalls, selectorCalls := 0, 0, 0
	var seedLeftWrite, seedRightWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_104), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_105),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			seedCalls++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var leftOK, rightOK bool
		seedLeftWrite, leftOK = WriteTo(rule, controlWrite)
		seedRightWrite, rightOK = WriteTo(rule, controlWrite)
		return leftOK && rightOK
	})
	var controlLeftRead, controlRightRead Read[OrderedCells[uint64]]
	var selectorWrite Write[uint64]
	selectorRule, selectorRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_106), Output: output.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](94_107),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transferCalls++
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(5)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var leftOK, rightOK bool
		controlLeftRead, leftOK = ReadFrom(rule, input, controlRead)
		controlRightRead, rightOK = ReadFrom(rule, input, controlRead)
		if !inputOK || !leftOK || !rightOK {
			return false
		}
		var selected bool
		selectorWrite, selected = SelectWrite(rule, writeSelector, []Read[OrderedCells[uint64]]{controlLeftRead, controlRightRead}, []Dependency{ReadDependency(controlLeftRead), ReadDependency(controlRightRead)}, func(context SelectorContext) bool {
			selectorCalls++
			leftCells, leftReadable := SelectorRead(context, controlLeftRead)
			rightCells, rightReadable := SelectorRead(context, controlRightRead)
			return leftReadable && rightReadable && waveCCell(leftCells) == 1 && waveCCell(rightCells) == 1
		})
		return selected
	})
	var queryReads [3]QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_108), Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				for index, token := range queryReads {
					cells, ok := QueryValue(row, token)
					if !ok || waveCCell(cells) != 5 {
						return false
					}
					result |= uint64(1) << index
				}
				return true
			}) {
				return 0
			}
			return result
		}, Result: frozenColdResult(coldKey(94_109)),
	}, func(query *Query[uint64]) bool {
		for index := range queryReads {
			var ok bool
			queryReads[index], ok = QueryReadFrom(query, outputRead)
			if !ok {
				return false
			}
		}
		return true
	})
	if !seedOK || seed == nil || !selectorRuleOK || selectorRule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("select-write declarations")
	}
	controlLeft, leftIssued := control.Ref(0)
	controlRight, rightIssued := control.Ref(1)
	strong, strongIssued := output.Ref(0)
	outputRefs := output.NewClosedRefs()
	if !leftIssued || !rightIssued || !strongIssued || outputRefs == nil {
		t.Fatal("select-write assembly")
	}
	for _, key := range []uint64{1, 2} {
		ref, issued := output.Ref(key)
		if !issued || !outputRefs.Append(ref) {
			t.Fatal("select-write summary reference")
		}
	}
	if !outputRefs.Close() {
		t.Fatal("select-write summary close")
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(94_110).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	selectorSite, selectorSiteOK := batch.AdmitSite(coldKey(94_111).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurred := batch.Relation(seedSite, coldKey(94_112).compositionKey())
	selectorOccurrence, selectorOccurred := batch.Relation(selectorSite, coldKey(94_113).compositionKey())
	var weak SelectorTarget
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(94_114)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedLeftWrite, controlLeft) && InstanceWrite(binding, seedRightWrite, controlRight)
	})
	selectorInstance, selectorInstanceOK := NewRuleInstance(selectorRule, ruleUnitForSemantic(coldKey(94_115)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, controlLeftRead, controlLeft) && InstanceRead(binding, controlRightRead, controlRight) &&
			InstanceSelectorWrite(binding, selectorWrite, writeSelector, []SelectorTarget{strong, weak}, nil)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	selectorOperand, selectorOperandOK := admitInstanceOperand(batch, selectorOccurrence, selectorInstance)
	if !scope.Available() || !seedSiteOK || !selectorSiteOK || !seedOccurred || !selectorOccurred || !seedInstanceOK || !selectorInstanceOK || !seedOperandOK || !selectorOperandOK || !batch.Seal() {
		t.Fatal("select-write source batch")
	}
	boundary := equation.BoundaryInput(seedSite, selectorSite, coldKey(94_116).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		weak = WeakTarget(assembly, outputSummary, outputRefs)
		seedPoint, selectorPoint := admitPoint(assembly, seedSite), admitPoint(assembly, selectorSite)
		seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
		selectorMember := admitInstance(assembly, selectorPoint, selectorOccurrence, selectorOperand, selectorInstance)
		seedGroup := admitGroup(assembly, seedPoint, seedMember)
		selectorGroup := admitGroup(assembly, selectorPoint, selectorMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			for key := uint64(0); key < 3; key++ {
				ref, issued := output.Ref(key)
				if !issued || !InstanceQueryRead(binding, queryReads[key], ref) {
					return false
				}
			}
			return true
		})
		observation := admitQueryAt(assembly, selectorPoint, queryInstance)
		if weak == nil || seedPoint == nil || selectorPoint == nil || !seedInstanceOK || !selectorInstanceOK || seedGroup == nil || selectorGroup == nil || !admitBoundary(assembly, selectorGroup, boundary) || observation == nil {
			t.Fatal("select-write group binding")
		}
		if !queryInstanceOK {
			t.Fatal("select-write query binding")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("select-write solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 7 {
		t.Fatalf("select-write solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if seedCalls != 1 || transferCalls != 1 || selectorCalls != 2 {
		t.Fatalf("select-write callbacks = seed:%d transfer:%d selector:%d, want 1/1/2", seedCalls, transferCalls, selectorCalls)
	}
}

func TestPublicStagedSelectorReadControlsWriteTarget(t *testing.T) {
	composition := NewComposition()
	control, controlOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_200)), func(*Factor[uint64, uint64]) bool { return true })
	var writeSelector WriteForm[uint64]
	output, outputOK := DeclareFactor(composition, coldFactorSpec(coldKey(94_201)), func(factor *Factor[uint64, uint64]) bool {
		var ok bool
		writeSelector, ok = DeclareWriteSelector(factor, coldKey(94_202))
		return ok
	})
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	outputRead, outputReadOK := ExactReadForm(output)
	if !controlOK || control == nil || !outputOK || output == nil || !controlReadOK || !controlWriteOK || !outputReadOK {
		t.Fatal("staged selector factors/forms")
	}
	var seedLeftWrite, seedRightWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_204), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_205),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var leftOK, rightOK bool
		seedLeftWrite, leftOK = WriteTo(rule, controlWrite)
		seedRightWrite, rightOK = WriteTo(rule, controlWrite)
		return leftOK && rightOK
	})
	readSelectorCalls, writeSelectorCalls, transferCalls := 0, 0, 0
	var controlLeft, controlRight Ref[uint64]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var left, right Read[OrderedCells[uint64]]
	var selectedWrite Write[uint64]
	rule, ruleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_206), Output: output.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](94_207),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			transferCalls++
			return Product(access, func(row Row) bool {
				selected, ok := ReadValue(access, row, selection)
				if !ok {
					return false
				}
				count, ok := SelectionCount(access, row, selected)
				tag, value, valueOK := SelectionAt(access, row, selected, 0)
				return ok && count == 1 && valueOK && tag == 1 && waveCCell(value) == 2 && StageValue(access, row, uint64(9))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var leftOK, rightOK bool
		left, leftOK = ReadFrom(rule, input, controlRead)
		right, rightOK = ReadFrom(rule, input, controlRead)
		if !inputOK || !leftOK || !rightOK {
			return false
		}
		var selected bool
		selection, selected = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, controlRead, []Dependency{ReadDependency(left), ReadDependency(right)}, func(context SelectorContext, _ ruleUnit) bool {
			readSelectorCalls++
			cells, read := SelectorRead(context, right)
			return read && waveCCell(cells) == 2 && SelectRoute(context, controlRight, uint64(1))
		})
		if !selected {
			return false
		}
		selectedWrite, selected = SelectWrite(rule, writeSelector, []Read[OrderedCells[uint64]]{left, right}, []Dependency{ReadDependency(selection)}, func(context SelectorContext) bool {
			writeSelectorCalls++
			_, selectionOK := SelectorRead(context, selection)
			return selectionOK && CurrentCandidate(context, right)
		})
		return selected
	})
	var leftToken, rightToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_208), Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				left, leftOK := QueryValue(row, leftToken)
				right, rightOK := QueryValue(row, rightToken)
				_, leftPresent := waveCOptionalCell(left)
				rightValue, rightPresent := waveCOptionalCell(right)
				if !leftOK || !rightOK || leftPresent || !rightPresent {
					return false
				}
				result = rightValue
				return true
			}) {
				return 0
			}
			return result
		}, Result: frozenColdResult(coldKey(94_209)),
	}, func(query *Query[uint64]) bool {
		var leftOK, rightOK bool
		leftToken, leftOK = QueryReadFrom(query, outputRead)
		rightToken, rightOK = QueryReadFrom(query, outputRead)
		return leftOK && rightOK
	})
	if !seedOK || seed == nil || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("staged selector declarations")
	}
	controlLeft, leftIssued := control.Ref(0)
	controlRight, rightIssued := control.Ref(1)
	outputLeft, outputLeftIssued := output.Ref(0)
	outputRight, outputRightIssued := output.Ref(1)
	if !leftIssued || !rightIssued || !outputLeftIssued || !outputRightIssued {
		t.Fatal("staged selector assembly")
	}
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(94_210).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	ruleSite, ruleSiteOK := batch.AdmitSite(coldKey(94_211).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurred := batch.Relation(seedSite, coldKey(94_212).compositionKey())
	ruleOccurrence, ruleOccurred := batch.Relation(ruleSite, coldKey(94_213).compositionKey())
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(94_214)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedLeftWrite, controlLeft) && InstanceWrite(binding, seedRightWrite, controlRight)
	})
	ruleInstance, ruleInstanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(94_215)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, left, controlLeft) && InstanceRead(binding, right, controlRight) &&
			InstanceSelectorRead(binding, selection, controlRead) &&
			InstanceSelectorWrite(binding, selectedWrite, writeSelector, []SelectorTarget{outputLeft, outputRight}, nil)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	ruleOperand, ruleOperandOK := admitInstanceOperand(batch, ruleOccurrence, ruleInstance)
	if !scope.Available() || !seedSiteOK || !ruleSiteOK || !seedOccurred || !ruleOccurred || !seedInstanceOK || !ruleInstanceOK || !seedOperandOK || !ruleOperandOK || !batch.Seal() {
		t.Fatal("staged selector source batch")
	}
	boundary := equation.BoundaryInput(seedSite, ruleSite, coldKey(94_216).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint, rulePoint := admitPoint(assembly, seedSite), admitPoint(assembly, ruleSite)
		seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
		ruleMember := admitInstance(assembly, rulePoint, ruleOccurrence, ruleOperand, ruleInstance)
		seedGroup := admitGroup(assembly, seedPoint, seedMember)
		ruleGroup := admitGroup(assembly, rulePoint, ruleMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, leftToken, outputLeft) && InstanceQueryRead(binding, rightToken, outputRight)
		})
		observation := admitQueryAt(assembly, rulePoint, queryInstance)
		if seedPoint == nil || rulePoint == nil || !seedInstanceOK || !ruleInstanceOK || seedGroup == nil || ruleGroup == nil || !admitBoundary(assembly, ruleGroup, boundary) || observation == nil ||
			!queryInstanceOK {
			t.Fatal("staged selector group binding")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("staged selector solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 9 {
		t.Fatalf("staged selector solve = state:%v status:%v result:%d readable:%t", state, status, result, readable)
	}
	if transferCalls != 1 || readSelectorCalls == 0 || writeSelectorCalls != 2 {
		t.Fatalf("staged selector callbacks = transfer:%d read:%d write:%d", transferCalls, readSelectorCalls, writeSelectorCalls)
	}
}

func TestRecursiveSelectionReplacementWakesOnlyCompatibleReaders(t *testing.T) {
	composition := NewComposition()
	rankedSpec := func(semantic SemanticKey) FactorSpec[uint64, uint64] {
		spec := coldFactorSpec(semantic)
		spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
		return spec
	}
	var targetSelector WriteForm[uint64]
	selected, selectedOK := DeclareFactor(composition, rankedSpec(coldKey(94_300)), func(factor *Factor[uint64, uint64]) bool {
		var ok bool
		targetSelector, ok = DeclareWriteSelector(factor, coldKey(94_301))
		return ok
	})
	leftSink, leftSinkOK := DeclareFactor(composition, rankedSpec(coldKey(94_302)), func(*Factor[uint64, uint64]) bool { return true })
	rightSink, rightSinkOK := DeclareFactor(composition, rankedSpec(coldKey(94_303)), func(*Factor[uint64, uint64]) bool { return true })
	if !selectedOK || selected == nil || !leftSinkOK || leftSink == nil || !rightSinkOK || rightSink == nil {
		t.Fatal("recursive selector factors")
	}
	selectedRead, selectedReadOK := ExactReadForm(selected)
	selectedWrite, selectedWriteOK := ExactWriteForm(selected)
	selectedCarry, selectedCarryOK := Carry(selected)
	leftSinkRead, leftSinkReadOK := ExactReadForm(leftSink)
	leftSinkWrite, leftSinkWriteOK := ExactWriteForm(leftSink)
	rightSinkRead, rightSinkReadOK := ExactReadForm(rightSink)
	rightSinkWrite, rightSinkWriteOK := ExactWriteForm(rightSink)
	if !selectedReadOK || !selectedWriteOK || !selectedCarryOK || !leftSinkReadOK || !leftSinkWriteOK || !rightSinkReadOK || !rightSinkWriteOK {
		t.Fatal("recursive selector forms")
	}

	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_304), Output: selected.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](94_305),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		seedWrite, ok = WriteTo(rule, selectedWrite)
		return ok
	})
	var leftValue, rightValue Read[OrderedCells[uint64]]
	var selectorWrite Write[uint64]
	selectorRule, selectorRuleOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
		Semantic: coldKey(94_306), Output: selected.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](94_307),
		OperandContent: ruleUnitContent,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				left, leftOK := ReadValue(access, row, leftValue)
				right, rightOK := ReadValue(access, row, rightValue)
				if !leftOK || !rightOK {
					return false
				}
				_, rightPresent := waveCOptionalCell(right)
				value := uint64(2)
				if rightPresent {
					value = 3
				}
				if waveCCell(left) == 0 {
					return NoCandidate(access, row)
				}
				return StageValue(access, row, value)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var leftOK, rightOK bool
		leftValue, leftOK = ReadFrom(rule, input, selectedRead)
		rightValue, rightOK = ReadFrom(rule, input, selectedRead)
		if !inputOK || !leftOK || !rightOK || !CarryFrom(rule, input, selectedCarry) {
			return false
		}
		var ok bool
		selectorWrite, ok = SelectWrite(rule, targetSelector, []Read[OrderedCells[uint64]]{leftValue, rightValue}, []Dependency{ReadDependency(leftValue), ReadDependency(rightValue)}, func(context SelectorContext) bool {
			right, rightOK := SelectorRead(context, rightValue)
			if !rightOK {
				return false
			}
			_, rightPresent := waveCOptionalCell(right)
			if rightPresent {
				return CurrentCandidate(context, leftValue)
			}
			return CurrentCandidate(context, rightValue)
		})
		return ok
	})
	declareReader := func(semantic SemanticKey, theorem uint64, sink *Factor[uint64, uint64], sinkWrite WriteForm[uint64]) (*Rule[uint64, ruleUnit], Read[OrderedCells[uint64]], Write[uint64]) {
		var observed Read[OrderedCells[uint64]]
		var written Write[uint64]
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily,
			Semantic: semantic, Output: sink.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](theorem),
			OperandContent: ruleUnitContent,
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool {
					cells, readable := ReadValue(access, row, observed)
					value, _ := waveCOptionalCell(cells)
					return readable && StageValue(access, row, value)
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			input, inputOK := rule.InputAt(0)
			var readOK bool
			observed, readOK = ReadFrom(rule, input, selectedRead)
			var writeOK bool
			written, writeOK = WriteTo(rule, sinkWrite)
			return inputOK && readOK && writeOK
		})
		if !declared {
			return nil, Read[OrderedCells[uint64]]{}, Write[uint64]{}
		}
		return rule, observed, written
	}
	leftReader, leftReaderRead, leftReaderWrite := declareReader(coldKey(94_308), 94_309, leftSink, leftSinkWrite)
	rightReader, rightReaderRead, rightReaderWrite := declareReader(coldKey(94_310), 94_311, rightSink, rightSinkWrite)

	var queryReads [4]QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(94_312), Project: func(observation Observation) uint64 {
			var values [4]uint64
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				for index, token := range queryReads {
					cells, readable := QueryValue(row, token)
					value, present := waveCOptionalCell(cells)
					if !readable || !present {
						return false
					}
					values[index] = value
				}
				rows++
				return true
			}) || rows != 1 {
				return 0
			}
			return values[0]<<24 | values[1]<<16 | values[2]<<8 | values[3]
		}, Result: frozenColdResult(coldKey(94_313)),
	}, func(query *Query[uint64]) bool {
		forms := []ReadForm[uint64, OrderedCells[uint64]]{selectedRead, selectedRead, leftSinkRead, rightSinkRead}
		for index, form := range forms {
			var ok bool
			queryReads[index], ok = QueryReadFrom(query, form)
			if !ok {
				return false
			}
		}
		return true
	})
	if !seedOK || seed == nil || !selectorRuleOK || selectorRule == nil || leftReader == nil || rightReader == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("recursive selector declarations")
	}
	selectedZero, selectedZeroIssued := selected.Ref(0)
	selectedOne, selectedOneIssued := selected.Ref(1)
	leftSinkZero, leftSinkIssued := leftSink.Ref(0)
	rightSinkZero, rightSinkIssued := rightSink.Ref(0)
	if !selectedZeroIssued || !selectedOneIssued || !leftSinkIssued || !rightSinkIssued {
		t.Fatal("recursive selector binding")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	site, siteOK := batch.AdmitSite(coldKey(94_314).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	if !scope.Available() || !siteOK {
		t.Fatal("recursive selector source site")
	}
	rules := []*Rule[uint64, ruleUnit]{seed, selectorRule, leftReader, rightReader}
	occurrences := make([]equation.Occurrence, len(rules))
	operands := make([]equation.Operand, len(rules))
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	instanceOK := make([]bool, len(rules))
	instances[0], instanceOK[0] = NewRuleInstance(rules[0], ruleUnitForSemantic(coldKey(94_320)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, selectedZero)
	})
	instances[1], instanceOK[1] = NewRuleInstance(rules[1], ruleUnitForSemantic(coldKey(94_321)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, leftValue, selectedZero) && InstanceRead(binding, rightValue, selectedOne) &&
			InstanceSelectorWrite(binding, selectorWrite, targetSelector, []SelectorTarget{selectedZero, selectedOne}, nil)
	})
	instances[2], instanceOK[2] = NewRuleInstance(rules[2], ruleUnitForSemantic(coldKey(94_322)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, leftReaderRead, selectedZero) && InstanceWrite(binding, leftReaderWrite, leftSinkZero)
	})
	instances[3], instanceOK[3] = NewRuleInstance(rules[3], ruleUnitForSemantic(coldKey(94_323)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, rightReaderRead, selectedOne) && InstanceWrite(binding, rightReaderWrite, rightSinkZero)
	})
	for index := range rules {
		occurrence, occurred := batch.Relation(site, coldKey(uint64(94_315+index)).compositionKey())
		operand, admitted := admitInstanceOperand(batch, occurrence, instances[index])
		if !occurred || !instanceOK[index] || !admitted {
			t.Fatal("recursive selector source operand")
		}
		occurrences[index], operands[index] = occurrence, operand
	}
	if !batch.Seal() {
		t.Fatal("recursive selector source seal")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		members := make([]*assemblyMember, len(rules))
		for index := range rules {
			members[index] = admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			if !instanceOK[index] || members[index] == nil {
				t.Fatal("recursive selector rule assembly")
			}
		}
		seedGroup := admitGroup(assembly, point, members[0])
		selectorGroup := admitGroup(assembly, point, members[1])
		leftGroup := admitGroup(assembly, point, members[2])
		rightGroup := admitGroup(assembly, point, members[3])
		for index, group := range []*assemblyGroup{selectorGroup, leftGroup, rightGroup} {
			boundary := equation.BoundaryInput(site, site, coldKey(uint64(94_330+index)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
			if group == nil || !admitBoundary(assembly, group, boundary) {
				t.Fatal("recursive selector boundary")
			}
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryReads[0], selectedZero) && InstanceQueryRead(binding, queryReads[1], selectedOne) &&
				InstanceQueryRead(binding, queryReads[2], leftSinkZero) && InstanceQueryRead(binding, queryReads[3], rightSinkZero)
		})
		observation := admitQueryAt(assembly, point, queryInstance)
		if seedGroup == nil || observation == nil ||
			!queryInstanceOK {
			t.Fatal("recursive selector query binding")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("recursive selector solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 0x03020302 {
		t.Fatalf("recursive selector solve = state:%v status:%v result:%#x readable:%t", state, status, result, readable)
	}
}

func waveCCell(cells OrderedCells[uint64]) uint64 {
	value, present, valid := cells.At(0)
	if cells.Count() != 1 || !valid || !present {
		return 0
	}
	return value
}

func waveCOptionalCell(cells OrderedCells[uint64]) (uint64, bool) {
	value, present, valid := cells.At(0)
	if cells.Count() != 1 || !valid {
		return 0, false
	}
	return value, present
}

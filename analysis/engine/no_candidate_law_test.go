package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestNoCandidatePreservesPriorAndStageDefaultIsDefined separates the two
// meanings that must never share a representation. NoCandidate contributes no
// patch to an already-defined strong target. StageValue(Default) is a staged
// disposition, while the Factor plane correctly re-sparsifies its Default
// value; Default is deliberately distinct from lattice Bottom.
func TestNoCandidatePreservesPriorAndStageDefaultIsDefined(t *testing.T) {
	composition := NewComposition()
	specification := func(semantic SemanticKey) FactorSpec[uint64, uint64] {
		spec := coldFactorSpec(semantic)
		spec.KeyEnd, spec.Default = 1, 7 // lattice Bottom remains 0.
		return spec
	}
	prior, priorOK := DeclareFactor(composition, specification(coldKey(95_001)), func(*Factor[uint64, uint64]) bool { return true })
	defaulted, defaultedOK := DeclareFactor(composition, specification(coldKey(95_002)), func(*Factor[uint64, uint64]) bool { return true })
	priorRead, priorReadOK := ExactReadForm(prior)
	priorWrite, priorWriteOK := ExactWriteForm(prior)
	defaultRead, defaultReadOK := ExactReadForm(defaulted)
	defaultWrite, defaultWriteOK := ExactWriteForm(defaulted)
	if !priorOK || prior == nil || !defaultedOK || defaulted == nil || !priorReadOK || !priorWriteOK || !defaultReadOK || !defaultWriteOK {
		t.Fatal("default distinction factors/forms")
	}

	var stagePriorWrite, omitPriorWrite, stageDefaultWrite Write[uint64]
	stagePrior, stagePriorOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(95_003), Output: prior.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](95_103),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(11)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		stagePriorWrite, ok = WriteTo(rule, priorWrite)
		return ok
	})
	omitPrior, omitPriorOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(95_004), Output: prior.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](95_104),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return NoCandidate(access, row) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		omitPriorWrite, ok = WriteTo(rule, priorWrite)
		return ok
	})
	stageDefault, stageDefaultOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(95_005), Output: defaulted.Output(), Inputs: 0, Admission: AdmitRuleByDerivation(coldKey(95_105), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			if derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, dispositionOK := derivation.DispositionAt(0)
			value, staged := disposition.Value()
			if !dispositionOK || disposition.Kind() != RuleDispositionStaged || !staged || value != 7 || disposition.TargetCount() != 1 {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		stageDefaultWrite, ok = WriteTo(rule, defaultWrite)
		return ok
	})

	var priorToken, defaultToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(95_006),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				priorCells, priorOK := QueryValue(row, priorToken)
				defaultCells, defaultOK := QueryValue(row, defaultToken)
				priorValue, priorPresent, priorCellOK := priorCells.At(0)
				defaultValue, defaultPresent, defaultCellOK := defaultCells.At(0)
				if !priorOK || !defaultOK || priorCells.Count() != 1 || defaultCells.Count() != 1 || !priorCellOK || !defaultCellOK || !priorPresent || defaultPresent {
					return false
				}
				result, rows = priorValue<<8|defaultValue, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(95_007)),
	}, func(query *Query[uint64]) bool {
		var priorOK, defaultOK bool
		priorToken, priorOK = QueryReadFrom(query, priorRead)
		defaultToken, defaultOK = QueryReadFrom(query, defaultRead)
		return priorOK && defaultOK
	})
	if !stagePriorOK || stagePrior == nil || !omitPriorOK || omitPrior == nil || !stageDefaultOK || stageDefault == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("default distinction declarations")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	site, siteOK := batch.AdmitSite(coldKey(95_008).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	rules := []*Rule[uint64, ruleUnit]{stagePrior, omitPrior, stageDefault}
	priorRef, priorIssued := prior.Ref(0)
	defaultRef, defaultIssued := defaulted.Ref(0)
	refs := []Ref[uint64]{priorRef, priorRef, defaultRef}
	writeTokens := []Write[uint64]{stagePriorWrite, omitPriorWrite, stageDefaultWrite}
	occurrences := make([]equation.Occurrence, len(rules))
	operands := make([]equation.Operand, len(rules))
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	for index := range rules {
		semantic := coldKey(uint64(95_020 + index))
		instance, instanceOK := NewRuleInstance(rules[index], ruleUnitForSemantic(semantic), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceWrite(binding, writeTokens[index], refs[index])
		})
		occurrence, occurrenceOK := batch.Relation(site, coldKey(uint64(95_010+index)).compositionKey())
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !instanceOK || !occurrenceOK || !operandOK {
			t.Fatal("default distinction source")
		}
		occurrences[index], operands[index], instances[index] = occurrence, operand, instance
	}
	if !scope.Available() || !siteOK || !batch.Seal() {
		t.Fatal("default distinction source seal")
	}
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, site)
		members := make([]*assemblyMember, len(rules))
		for index := range rules {
			members[index] = admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			if members[index] == nil {
				t.Fatal("default distinction member")
			}
		}
		if point == nil || !priorIssued || !defaultIssued ||
			admitGroup(assembly, point, members[0]) == nil || admitGroup(assembly, point, members[1]) == nil || admitGroup(assembly, point, members[2]) == nil {
			t.Fatal("default distinction assembly")
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, priorToken, priorRef) && InstanceQueryRead(binding, defaultToken, defaultRef)
		})
		observation := admitQueryAt(assembly, point, queryInstance)
		if !queryInstanceOK || observation == nil {
			t.Fatal("default distinction query")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("default distinction solver")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 0x0B07 {
		t.Fatalf("default distinction solve = state:%v status:%v result:%#x readable:%t", state, status, result, readable)
	}
}

// TestStageValueRejectsEmptyResolvedFactorTarget ensures a selector that
// chooses no Factor target cannot disguise that empty successor as a staged
// value merely because its Rule uses trusted-theorem admission.
func TestStageValueRejectsEmptyResolvedFactorTarget(t *testing.T) {
	composition := NewComposition()
	control := coldFactor(composition, coldKey(95_101))
	var selector WriteForm[uint64]
	output, outputOK := DeclareFactor(composition, coldFactorSpec(coldKey(95_102)), func(factor *Factor[uint64, uint64]) bool {
		var declared bool
		selector, declared = DeclareWriteSelector(factor, coldKey(95_103))
		return declared
	})
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	outputRead, outputReadOK := ExactReadForm(output)
	if control == nil || !outputOK || output == nil || !controlReadOK || !controlWriteOK || !outputReadOK {
		t.Fatal("empty target factors/forms")
	}
	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(95_104), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](95_204),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		seedWrite, ok = WriteTo(rule, controlWrite)
		return ok
	})
	var observed Read[OrderedCells[uint64]]
	var selected Write[uint64]
	emptyTarget, emptyTargetOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(95_105), Output: output.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](95_205),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				if _, readable := ReadValue(access, row, observed); !readable {
					return false
				}
				return StageValue(access, row, uint64(9))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, selectorOK bool
		observed, readOK = ReadFrom(rule, input, controlRead)
		selected, selectorOK = SelectWrite(rule, selector, []Read[OrderedCells[uint64]]{observed}, []Dependency{ReadDependency(observed)}, func(SelectorContext) bool { return false })
		return inputOK && readOK && selectorOK
	})
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(95_106), Project: func(observation Observation) uint64 {
			if !ProjectRows(observation, func(row QueryRow) bool {
				_, readable := QueryValue(row, token)
				return readable
			}) {
				return 0
			}
			return 1
		}, Result: frozenColdResult(coldKey(95_107)),
	}, func(query *Query[uint64]) bool { var ok bool; token, ok = QueryReadFrom(query, outputRead); return ok })
	if !seedOK || seed == nil || !emptyTargetOK || emptyTarget == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("empty target declarations")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	seedSite, seedSiteOK := batch.AdmitSite(coldKey(95_108).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(coldKey(95_109).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	seedOccurrence, seedOccurrenceOK := batch.Relation(seedSite, coldKey(95_110).compositionKey())
	targetOccurrence, targetOccurrenceOK := batch.Relation(targetSite, coldKey(95_111).compositionKey())
	controlRef, controlIssued := control.Ref(0)
	outputRef, outputIssued := output.Ref(0)
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(95_112)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrite, controlRef)
	})
	targetInstance, targetInstanceOK := NewRuleInstance(emptyTarget, ruleUnitForSemantic(coldKey(95_113)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, observed, controlRef) && InstanceSelectorWrite(binding, selected, selector, []SelectorTarget{outputRef}, nil)
	})
	seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
	targetOperand, targetOperandOK := admitInstanceOperand(batch, targetOccurrence, targetInstance)
	if !scope.Available() || !seedSiteOK || !targetSiteOK || !seedOccurrenceOK || !targetOccurrenceOK || !seedOperandOK || !targetOperandOK || !batch.Seal() {
		t.Fatal("empty target source")
	}
	boundary := equation.BoundaryInput(seedSite, targetSite, coldKey(95_114).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		seedPoint, targetPoint := admitPoint(assembly, seedSite), admitPoint(assembly, targetSite)
		seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
		targetMember := admitInstance(assembly, targetPoint, targetOccurrence, targetOperand, targetInstance)
		seedGroup := admitGroup(assembly, seedPoint, seedMember)
		targetGroup := admitGroup(assembly, targetPoint, targetMember)
		if seedPoint == nil || targetPoint == nil || !seedInstanceOK || !targetInstanceOK || seedMember == nil || targetMember == nil || !controlIssued || !outputIssued ||
			seedGroup == nil || targetGroup == nil || !boundary.Available() || !admitBoundary(assembly, targetGroup, boundary) {
			t.Fatal("empty target assembly")
		}
		queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, outputRef)
		})
		observation := admitQueryAt(assembly, targetPoint, queryInstance)
		if !queryInstanceOK || observation == nil {
			t.Fatal("empty target query")
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("empty target solver")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete {
		t.Fatalf("empty target StageValue = state:%v status:%v", state, status)
	}
}

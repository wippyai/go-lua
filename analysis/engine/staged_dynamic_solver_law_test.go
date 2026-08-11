package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// stagedDynamicSolverLawResult is the public semantic snapshot used by these
// laws.  It deliberately comes from a Query rather than from a runtime or
// demand implementation detail: result is the selected value, while the
// other fields prove the revisions that led to it completed.
type stagedDynamicSolverLawResult struct {
	result  uint64
	control uint64
	phase   uint64
	a       uint64
	b       uint64
}

type stagedDynamicSolverLawStats struct {
	projectionStages  []uint64
	evidenceStages    []uint64
	projectionAtAEdit int
	projectionAtBEdit int
	aEdited           bool
	bEdited           bool
	closureTransfers  int
}

type stagedDynamicSolverLawFixture struct {
	solver         *Solver
	query          *Query[stagedDynamicSolverLawResult]
	closureQuery   *Query[uint64]
	receipt        QueryReceipt[stagedDynamicSolverLawResult]
	closureReceipt QueryReceipt[uint64]
	stats          *stagedDynamicSolverLawStats
}

func stagedDynamicSolverLawRankedSpec(semantic SemanticKey, keyEnd uint64) FactorSpec[uint64, uint64] {
	spec := coldFactorSpec(semantic)
	spec.KeyEnd = keyEnd
	// Every main Factor belongs to one genuine self-recursive Point.  The
	// ranking is part of its normal Factor law; it is not a test work limit.
	spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	return spec
}

func stagedDynamicSolverLawCell(cells OrderedCells[uint64]) (uint64, bool) {
	value, present, valid := cells.At(0)
	return value, valid && present && cells.Count() == 1
}

func stagedDynamicSolverLawFrozen(semantic SemanticKey) FrozenResult[stagedDynamicSolverLawResult] {
	return FrozenResult[stagedDynamicSolverLawResult]{
		Semantic: semantic,
		Freeze:   func(value stagedDynamicSolverLawResult) stagedDynamicSolverLawResult { return value },
		Clone:    func(value stagedDynamicSolverLawResult) stagedDynamicSolverLawResult { return value },
		Equal:    func(left, right stagedDynamicSolverLawResult) bool { return left == right },
		Fingerprint: func(value stagedDynamicSolverLawResult) uint64 {
			return value.result ^ value.control<<7 ^ value.phase<<14 ^ value.a<<21 ^ value.b<<28
		},
	}
}

// newStagedDynamicSolverLawFixture builds the actual assembled Solver path
// used by both laws below.  Its one Point evolves as follows:
//
//	control=1 selects A=10; that result latches control=2 and selects B=20.
//	B=20 latches phase=4, which changes A to 11.  A=11 latches phase=8,
//	which changes B to 30.
//
// The projection has exactly one static read (control) and a real staged
// exact read of source.  Thus the A=11 publication can wake it only if the
// old dynamic route survived the A-to-B replacement; B=30 can wake it only
// if the B route was installed.
func newStagedDynamicSolverLawFixture(t testing.TB, fullSelectedClosure bool) stagedDynamicSolverLawFixture {
	t.Helper()
	stats := &stagedDynamicSolverLawStats{projectionAtAEdit: -1, projectionAtBEdit: -1}
	composition := NewComposition()

	control, controlOK := DeclareFactor(composition, stagedDynamicSolverLawRankedSpec(coldKey(160_000), 1), func(*Factor[uint64, uint64]) bool { return true })
	phase, phaseOK := DeclareFactor(composition, stagedDynamicSolverLawRankedSpec(coldKey(160_001), 1), func(*Factor[uint64, uint64]) bool { return true })
	source, sourceOK := DeclareFactor(composition, stagedDynamicSolverLawRankedSpec(coldKey(160_002), 2), func(*Factor[uint64, uint64]) bool { return true })
	result, resultOK := DeclareFactor(composition, stagedDynamicSolverLawRankedSpec(coldKey(160_003), 1), func(*Factor[uint64, uint64]) bool { return true })
	if !controlOK || control == nil || !phaseOK || phase == nil || !sourceOK || source == nil || !resultOK || result == nil {
		t.Fatal("staged dynamic solver Factors")
	}
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	phaseRead, phaseReadOK := ExactReadForm(phase)
	phaseWrite, phaseWriteOK := ExactWriteForm(phase)
	sourceRead, sourceReadOK := ExactReadForm(source)
	sourceWrite, sourceWriteOK := ExactWriteForm(source)
	resultRead, resultReadOK := ExactReadForm(result)
	resultWrite, resultWriteOK := ExactWriteForm(result)
	if !controlReadOK || !controlWriteOK || !phaseReadOK || !phaseWriteOK || !sourceReadOK || !sourceWriteOK || !resultReadOK || !resultWriteOK {
		t.Fatal("staged dynamic solver Factor forms")
	}

	var closure *Factor[uint64, uint64]
	var closureRead ReadForm[uint64, OrderedCells[uint64]]
	var closureWrite WriteForm[uint64]
	var closureOK, closureReadOK, closureWriteOK bool
	if fullSelectedClosure {
		closure, closureOK = DeclareFactor(composition, coldFactorSpec(coldKey(160_004)), func(*Factor[uint64, uint64]) bool { return true })
		if closureOK && closure != nil {
			closureRead, closureReadOK = ExactReadForm(closure)
			closureWrite, closureWriteOK = ExactWriteForm(closure)
		}
		if !closureOK || closure == nil || !closureReadOK || !closureWriteOK {
			t.Fatal("full selected closure Factor/forms")
		}
	}

	var controlSeedWrite Write[uint64]
	controlSeed, controlSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_010), Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](160_110),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		controlSeedWrite, declared = WriteTo(rule, controlWrite)
		return declared
	})

	var phaseSeedWrite Write[uint64]
	phaseSeed, phaseSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_011), Output: phase.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](160_111),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		phaseSeedWrite, declared = WriteTo(rule, phaseWrite)
		return declared
	})

	var sourceASeedWrite Write[uint64]
	sourceASeed, sourceASeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_012), Output: source.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](160_112),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(10)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		sourceASeedWrite, declared = WriteTo(rule, sourceWrite)
		return declared
	})

	var sourceBSeedWrite Write[uint64]
	sourceBSeed, sourceBSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_013), Output: source.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](160_113),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(20)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		sourceBSeedWrite, declared = WriteTo(rule, sourceWrite)
		return declared
	})

	var controlLatchControl, controlLatchResult Read[OrderedCells[uint64]]
	var controlLatchWrite Write[uint64]
	controlLatch, controlLatchOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_014), Output: control.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](160_114),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				controlCells, controlReadable := ReadValue(access, row, controlLatchControl)
				resultCells, resultReadable := ReadValue(access, row, controlLatchResult)
				controlValue, controlPresent := stagedDynamicSolverLawCell(controlCells)
				resultValue, resultPresent := stagedDynamicSolverLawCell(resultCells)
				if !controlReadable || !resultReadable {
					return false
				}
				if controlPresent && controlValue >= 2 || resultPresent && resultValue == 10 {
					return StageValue(access, row, uint64(2))
				}
				return NoCandidate(access, row)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var controlOK, resultOK, writeOK bool
		controlLatchControl, controlOK = ReadFrom(rule, input, controlRead)
		controlLatchResult, resultOK = ReadFrom(rule, input, resultRead)
		controlLatchWrite, writeOK = WriteTo(rule, controlWrite)
		return inputOK && controlOK && resultOK && writeOK
	})

	var phaseLatchPhase, phaseLatchResult Read[OrderedCells[uint64]]
	var phaseLatchWrite Write[uint64]
	phaseLatch, phaseLatchOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_015), Output: phase.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](160_115),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				phaseCells, phaseReadable := ReadValue(access, row, phaseLatchPhase)
				resultCells, resultReadable := ReadValue(access, row, phaseLatchResult)
				phaseValue, phasePresent := stagedDynamicSolverLawCell(phaseCells)
				resultValue, resultPresent := stagedDynamicSolverLawCell(resultCells)
				if !phaseReadable || !resultReadable {
					return false
				}
				if phasePresent && phaseValue >= 4 || resultPresent && resultValue == 20 {
					return StageValue(access, row, uint64(4))
				}
				return NoCandidate(access, row)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var phaseOK, resultOK, writeOK bool
		phaseLatchPhase, phaseOK = ReadFrom(rule, input, phaseRead)
		phaseLatchResult, resultOK = ReadFrom(rule, input, resultRead)
		phaseLatchWrite, writeOK = WriteTo(rule, phaseWrite)
		return inputOK && phaseOK && resultOK && writeOK
	})

	var sourceABumpPhase Read[OrderedCells[uint64]]
	var sourceABumpWrite Write[uint64]
	sourceABump, sourceABumpOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_016), Output: source.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](160_116),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				phaseCells, readable := ReadValue(access, row, sourceABumpPhase)
				phaseValue, present := stagedDynamicSolverLawCell(phaseCells)
				if !readable {
					return false
				}
				if !present || phaseValue < 4 {
					return NoCandidate(access, row)
				}
				if !stats.aEdited {
					stats.aEdited = true
					stats.projectionAtAEdit = len(stats.projectionStages)
				}
				return StageValue(access, row, uint64(11))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		sourceABumpPhase, readOK = ReadFrom(rule, input, phaseRead)
		sourceABumpWrite, writeOK = WriteTo(rule, sourceWrite)
		return inputOK && readOK && writeOK
	})

	var phaseLatePhase, phaseLateA Read[OrderedCells[uint64]]
	var phaseLateWrite Write[uint64]
	phaseLate, phaseLateOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_017), Output: phase.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](160_117),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				phaseCells, phaseReadable := ReadValue(access, row, phaseLatePhase)
				aCells, aReadable := ReadValue(access, row, phaseLateA)
				phaseValue, phasePresent := stagedDynamicSolverLawCell(phaseCells)
				aValue, aPresent := stagedDynamicSolverLawCell(aCells)
				if !phaseReadable || !aReadable {
					return false
				}
				if phasePresent && phaseValue >= 8 || aPresent && aValue == 11 {
					return StageValue(access, row, uint64(8))
				}
				return NoCandidate(access, row)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var phaseOK, aOK, writeOK bool
		phaseLatePhase, phaseOK = ReadFrom(rule, input, phaseRead)
		phaseLateA, aOK = ReadFrom(rule, input, sourceRead)
		phaseLateWrite, writeOK = WriteTo(rule, phaseWrite)
		return inputOK && phaseOK && aOK && writeOK
	})

	var sourceBBumpPhase Read[OrderedCells[uint64]]
	var sourceBBumpWrite Write[uint64]
	sourceBBump, sourceBBumpOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_018), Output: source.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](160_118),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				phaseCells, readable := ReadValue(access, row, sourceBBumpPhase)
				phaseValue, present := stagedDynamicSolverLawCell(phaseCells)
				if !readable {
					return false
				}
				if !present || phaseValue < 8 {
					return NoCandidate(access, row)
				}
				if !stats.bEdited {
					stats.bEdited = true
					stats.projectionAtBEdit = len(stats.projectionStages)
				}
				return StageValue(access, row, uint64(30))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		sourceBBumpPhase, readOK = ReadFrom(rule, input, phaseRead)
		sourceBBumpWrite, writeOK = WriteTo(rule, sourceWrite)
		return inputOK && readOK && writeOK
	})

	var projectionControl Read[OrderedCells[uint64]]
	var projectionSelection Read[Selection[uint64, OrderedCells[uint64]]]
	var projectionWrite Write[uint64]
	var sourceARef, sourceBRef Ref[uint64]
	projection, projectionOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(160_019), Output: result.Output(), Inputs: 1,
		Admission: AdmitRuleByDerivation(coldKey(160_119), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			disposition, dispositionOK := derivation.DispositionAt(0)
			selected, selectedOK := DerivationDispositionReadValue(derivation, disposition, projectionSelection)
			if !dispositionOK || !selectedOK {
				return RuleEvidence{}, false
			}
			// DerivationDispositionReadValue above proves this is the exact staged
			// read for this Product row. Its route cardinality/tag is checked by
			// the transfer while it owns the row-local Access capability.
			_ = selected
			if disposition.Kind() == RuleDispositionNoCandidate {
				return derivation.Accept()
			}
			if disposition.Kind() != RuleDispositionStaged {
				return RuleEvidence{}, false
			}
			// The staged value is checked against the transfer's public projection
			// trace before this evidence is accepted.
			value, staged := disposition.Value()
			if !staged || len(stats.projectionStages) == 0 || stats.projectionStages[len(stats.projectionStages)-1] != value {
				return RuleEvidence{}, false
			}
			stats.evidenceStages = append(stats.evidenceStages, value)
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				selected, readable := ReadValue(access, row, projectionSelection)
				if !readable {
					return false
				}
				count, counted := SelectionCount(access, row, selected)
				if !counted {
					return false
				}
				if count == 0 {
					return NoCandidate(access, row)
				}
				if count != 1 {
					return false
				}
				tag, cells, selectedOK := SelectionAt(access, row, selected, 0)
				value, present := stagedDynamicSolverLawCell(cells)
				if !selectedOK || !present || tag != 1 && tag != 2 {
					return false
				}
				stats.projectionStages = append(stats.projectionStages, value)
				return StageValue(access, row, value)
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var controlOK, selectionOK, writeOK bool
		projectionControl, controlOK = ReadFrom(rule, input, controlRead)
		projectionSelection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, input, sourceRead, []Dependency{ReadDependency(projectionControl)}, func(context SelectorContext, _ ruleUnit) bool {
			controlCells, readable := SelectorRead(context, projectionControl)
			controlValue, present := stagedDynamicSolverLawCell(controlCells)
			if !readable || !present {
				return true
			}
			if controlValue < 2 {
				return SelectRoute(context, sourceARef, uint64(1))
			}
			return SelectRoute(context, sourceBRef, uint64(2))
		})
		projectionWrite, writeOK = WriteTo(rule, resultWrite)
		return inputOK && controlOK && selectionOK && writeOK
	})

	var queryResult, queryControl, queryPhase, queryA, queryB QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[stagedDynamicSolverLawResult]{
		Semantic: coldKey(160_020),
		Project: func(observation Observation) stagedDynamicSolverLawResult {
			var resultValue stagedDynamicSolverLawResult
			rows := 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				resultCells, resultReadable := QueryValue(row, queryResult)
				controlCells, controlReadable := QueryValue(row, queryControl)
				phaseCells, phaseReadable := QueryValue(row, queryPhase)
				aCells, aReadable := QueryValue(row, queryA)
				bCells, bReadable := QueryValue(row, queryB)
				resultValue.result, resultReadable = stagedDynamicSolverLawCell(resultCells)
				resultValue.control, controlReadable = stagedDynamicSolverLawCell(controlCells)
				resultValue.phase, phaseReadable = stagedDynamicSolverLawCell(phaseCells)
				resultValue.a, aReadable = stagedDynamicSolverLawCell(aCells)
				resultValue.b, bReadable = stagedDynamicSolverLawCell(bCells)
				if !resultReadable || !controlReadable || !phaseReadable || !aReadable || !bReadable {
					return false
				}
				rows++
				return true
			}) || rows != 1 {
				return stagedDynamicSolverLawResult{}
			}
			return resultValue
		},
		Result: stagedDynamicSolverLawFrozen(coldKey(160_120)),
	}, func(query *Query[stagedDynamicSolverLawResult]) bool {
		var resultOK, controlOK, phaseOK, aOK, bOK bool
		queryResult, resultOK = QueryReadFrom(query, resultRead)
		queryControl, controlOK = QueryReadFrom(query, controlRead)
		queryPhase, phaseOK = QueryReadFrom(query, phaseRead)
		queryA, aOK = QueryReadFrom(query, sourceRead)
		queryB, bOK = QueryReadFrom(query, sourceRead)
		return resultOK && controlOK && phaseOK && aOK && bOK
	})

	var closureSeed *Rule[uint64, ruleUnit]
	var closureSeedWrite Write[uint64]
	var closureQuery *Query[uint64]
	var closureToken QueryRead[OrderedCells[uint64]]
	if fullSelectedClosure {
		var closureSeedOK, closureQueryOK bool
		closureSeed, closureSeedOK = DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
			OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(160_021), Output: closure.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](160_121),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				stats.closureTransfers++
				return Product(access, func(row Row) bool { return StageValue(access, row, uint64(71)) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var declared bool
			closureSeedWrite, declared = WriteTo(rule, closureWrite)
			return declared
		})
		closureQuery, closureQueryOK = DeclareQuery(composition, QuerySpec[uint64]{
			Semantic: coldKey(160_022),
			Project: func(observation Observation) uint64 {
				var value uint64
				rows := 0
				if !ProjectRows(observation, func(row QueryRow) bool {
					cells, readable := QueryValue(row, closureToken)
					value, readable = stagedDynamicSolverLawCell(cells)
					if !readable {
						return false
					}
					rows++
					return true
				}) || rows != 1 {
					return 0
				}
				return value
			},
			Result: frozenColdResult(coldKey(160_122)),
		}, func(query *Query[uint64]) bool {
			var declared bool
			closureToken, declared = QueryReadFrom(query, closureRead)
			return declared
		})
		if !closureOK || closure == nil || !closureReadOK || !closureWriteOK || !closureSeedOK || closureSeed == nil || !closureQueryOK || closureQuery == nil {
			t.Fatal("full selected closure declarations")
		}
	}

	if !controlSeedOK || controlSeed == nil || !phaseSeedOK || phaseSeed == nil || !sourceASeedOK || sourceASeed == nil || !sourceBSeedOK || sourceBSeed == nil ||
		!controlLatchOK || controlLatch == nil || !phaseLatchOK || phaseLatch == nil || !sourceABumpOK || sourceABump == nil || !phaseLateOK || phaseLate == nil || !sourceBBumpOK || sourceBBump == nil ||
		!projectionOK || projection == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("staged dynamic solver declarations")
	}

	controlRef, controlIssued := control.Ref(0)
	phaseRef, phaseIssued := phase.Ref(0)
	var sourceAIssued bool
	sourceARef, sourceAIssued = source.Ref(0)
	sourceBRef, sourceBIssued := source.Ref(1)
	resultRef, resultIssued := result.Ref(0)
	if !controlIssued || !phaseIssued || !sourceAIssued || !sourceBIssued || !resultIssued {
		t.Fatal("staged dynamic solver refs")
	}

	rules := []*Rule[uint64, ruleUnit]{
		controlSeed, phaseSeed, sourceASeed, sourceBSeed, controlLatch,
		phaseLatch, sourceABump, phaseLate, sourceBBump, projection,
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	instanceOK := make([]bool, len(rules))
	instances[0], instanceOK[0] = NewRuleInstance(rules[0], ruleUnitForSemantic(coldKey(160_200)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlSeedWrite, controlRef)
	})
	instances[1], instanceOK[1] = NewRuleInstance(rules[1], ruleUnitForSemantic(coldKey(160_201)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, phaseSeedWrite, phaseRef)
	})
	instances[2], instanceOK[2] = NewRuleInstance(rules[2], ruleUnitForSemantic(coldKey(160_202)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, sourceASeedWrite, sourceARef)
	})
	instances[3], instanceOK[3] = NewRuleInstance(rules[3], ruleUnitForSemantic(coldKey(160_203)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, sourceBSeedWrite, sourceBRef)
	})
	instances[4], instanceOK[4] = NewRuleInstance(rules[4], ruleUnitForSemantic(coldKey(160_204)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, controlLatchControl, controlRef) && InstanceRead(binding, controlLatchResult, resultRef) && InstanceWrite(binding, controlLatchWrite, controlRef)
	})
	instances[5], instanceOK[5] = NewRuleInstance(rules[5], ruleUnitForSemantic(coldKey(160_205)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, phaseLatchPhase, phaseRef) && InstanceRead(binding, phaseLatchResult, resultRef) && InstanceWrite(binding, phaseLatchWrite, phaseRef)
	})
	instances[6], instanceOK[6] = NewRuleInstance(rules[6], ruleUnitForSemantic(coldKey(160_206)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, sourceABumpPhase, phaseRef) && InstanceWrite(binding, sourceABumpWrite, sourceARef)
	})
	instances[7], instanceOK[7] = NewRuleInstance(rules[7], ruleUnitForSemantic(coldKey(160_207)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, phaseLatePhase, phaseRef) && InstanceRead(binding, phaseLateA, sourceARef) && InstanceWrite(binding, phaseLateWrite, phaseRef)
	})
	instances[8], instanceOK[8] = NewRuleInstance(rules[8], ruleUnitForSemantic(coldKey(160_208)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, sourceBBumpPhase, phaseRef) && InstanceWrite(binding, sourceBBumpWrite, sourceBRef)
	})
	instances[9], instanceOK[9] = NewRuleInstance(rules[9], ruleUnitForSemantic(coldKey(160_209)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, projectionControl, controlRef) && InstanceSelectorRead(binding, projectionSelection, sourceRead) && InstanceWrite(binding, projectionWrite, resultRef)
	})
	for index, accepted := range instanceOK {
		if !accepted || instances[index] == nil {
			t.Fatalf("staged dynamic solver instance %d", index)
		}
	}

	var closureRef Ref[uint64]
	var closureInstance *RuleInstance[uint64, ruleUnit]
	var closureInstanceOK bool
	if fullSelectedClosure {
		closureRef, closureInstanceOK = closure.Ref(0)
		if !closureInstanceOK {
			t.Fatal("full selected closure ref")
		}
		closureInstance, closureInstanceOK = NewRuleInstance(closureSeed, ruleUnitForSemantic(coldKey(160_210)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceWrite(binding, closureSeedWrite, closureRef)
		})
		if !closureInstanceOK || closureInstance == nil {
			t.Fatal("full selected closure instance")
		}
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	mainSite, mainSiteOK := batch.AdmitSite(coldKey(160_300).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	var closureSite equation.Site
	closureSiteOK := true
	if fullSelectedClosure {
		closureSite, closureSiteOK = batch.AdmitSite(coldKey(160_301).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	}
	occurrences := make([]equation.Occurrence, len(rules))
	operands := make([]equation.Operand, len(rules))
	admitted := scope.Available() && mainSiteOK && closureSiteOK
	for index, instance := range instances {
		occurrence, occurred := batch.Relation(mainSite, coldKey(uint64(160_400+index)).compositionKey())
		operand, operandOK := admitInstanceOperand(batch, occurrence, instance)
		if !occurred || !operandOK {
			admitted = false
		}
		occurrences[index], operands[index] = occurrence, operand
	}
	var closureOccurrence equation.Occurrence
	var closureOperand equation.Operand
	if fullSelectedClosure {
		var occurred, operandOK bool
		closureOccurrence, occurred = batch.Relation(closureSite, coldKey(160_410).compositionKey())
		closureOperand, operandOK = admitInstanceOperand(batch, closureOccurrence, closureInstance)
		admitted = admitted && occurred && operandOK
	}
	if !admitted || !batch.Seal() {
		t.Fatal("staged dynamic solver source batch")
	}

	var queryInstance *QueryInstance[stagedDynamicSolverLawResult]
	var closureQueryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		mainPoint := admitPoint(assembly, mainSite)
		if mainPoint == nil {
			return false
		}
		for index, instance := range instances {
			member := admitInstance(assembly, mainPoint, occurrences[index], operands[index], instance)
			group := admitGroup(assembly, mainPoint, member)
			if member == nil || group == nil {
				return false
			}
			if index >= 4 {
				boundary := equation.BoundaryInput(mainSite, mainSite, coldKey(uint64(160_500+index)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
				if !boundary.Available() || !admitBoundary(assembly, group, boundary) {
					return false
				}
			}
		}
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[stagedDynamicSolverLawResult]) bool {
			return InstanceQueryRead(binding, queryResult, resultRef) &&
				InstanceQueryRead(binding, queryControl, controlRef) &&
				InstanceQueryRead(binding, queryPhase, phaseRef) &&
				InstanceQueryRead(binding, queryA, sourceARef) &&
				InstanceQueryRead(binding, queryB, sourceBRef)
		})
		if !queryInstanceOK || admitQueryAt(assembly, mainPoint, queryInstance) == nil {
			return false
		}
		if !fullSelectedClosure {
			return true
		}
		closurePoint := admitPoint(assembly, closureSite)
		closureMember := admitInstance(assembly, closurePoint, closureOccurrence, closureOperand, closureInstance)
		closureGroup := admitGroup(assembly, closurePoint, closureMember)
		var closureQueryOK bool
		closureQueryInstance, closureQueryOK = NewQueryInstance(closureQuery, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, closureToken, closureRef)
		})
		return closurePoint != nil && closureMember != nil && closureGroup != nil && closureQueryOK && admitQueryAt(assembly, closurePoint, closureQueryInstance) != nil
	})
	if !compiled || solver == nil {
		t.Fatal("staged dynamic solver assembly")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("staged dynamic query receipt")
	}
	fixture := stagedDynamicSolverLawFixture{solver: solver, query: query, closureQuery: closureQuery, receipt: receipt, stats: stats}
	if fullSelectedClosure {
		fixture.closureReceipt, receiptOK = closureQueryInstance.Receipt()
		if !receiptOK {
			t.Fatal("staged dynamic closure receipt")
		}
	}
	return fixture
}

func stagedDynamicSolverLawSolve(t testing.TB, fixture stagedDynamicSolverLawFixture) stagedDynamicSolverLawResult {
	t.Helper()
	state, status := fixture.solver.Solve(context.Background())
	value, readable := QueryResult(fixture.receipt, state)
	if status != SolveComplete || state == nil || !readable {
		t.Fatalf("staged dynamic Solve = state:%v status:%v readable:%t", state, status, readable)
	}
	return value
}

func equalStagedDynamicSolverLawTrace(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// TestStagedDynamicSolverRouteReplacementDropsStaleAAndWakesB proves the
// route replacement through a real assembled Solver and its demand path.  A
// source revision of A occurs strictly after the projection has selected B;
// it must not run the projection again.  The later B revision must run it,
// and only the B=30 result/evidence may remain in the completed Query state.
func TestStagedDynamicSolverRouteReplacementDropsStaleAAndWakesB(t *testing.T) {
	fixture := newStagedDynamicSolverLawFixture(t, false)
	got := stagedDynamicSolverLawSolve(t, fixture)
	want := stagedDynamicSolverLawResult{result: 30, control: 2, phase: 8, a: 11, b: 30}
	if got != want {
		t.Fatalf("completed staged dynamic State/Query = %+v, want %+v", got, want)
	}
	stats := fixture.stats
	if stats == nil || !stats.aEdited || !stats.bEdited || stats.projectionAtAEdit < 0 || stats.projectionAtBEdit < 0 {
		t.Fatalf("missing staged source revisions: %+v", stats)
	}
	if stats.projectionAtAEdit != 2 || stats.projectionAtBEdit != stats.projectionAtAEdit {
		t.Fatalf("A revision woke/recomputed replaced route: at-A=%d at-B=%d stages=%v", stats.projectionAtAEdit, stats.projectionAtBEdit, stats.projectionStages)
	}
	wantStages := []uint64{10, 20, 30}
	if !equalStagedDynamicSolverLawTrace(stats.projectionStages, wantStages) {
		t.Fatalf("staged projection results = %v, want A/B/B %v", stats.projectionStages, wantStages)
	}
	if !equalStagedDynamicSolverLawTrace(stats.evidenceStages, wantStages) {
		t.Fatalf("staged projection evidence = %v, want only current A/B/B evidence %v", stats.evidenceStages, wantStages)
	}
}

// TestStagedDynamicDemandMatchesFullSelectedClosureAfterLateRouteDiscovery
// compares two independently assembled Solvers.  The first has only the
// terminal semantic query.  The second has that same query plus a genuine
// disconnected query, so its selected Point closure is the complete fixture
// rather than the terminal demand alone.  Neither test reaches into Demand;
// the only selection authority is the public Query/Assembly path.
func TestStagedDynamicDemandMatchesFullSelectedClosureAfterLateRouteDiscovery(t *testing.T) {
	demanded := newStagedDynamicSolverLawFixture(t, false)
	demandedResult := stagedDynamicSolverLawSolve(t, demanded)

	full := newStagedDynamicSolverLawFixture(t, true)
	fullState, fullStatus := full.solver.Solve(context.Background())
	fullResult, fullReadable := QueryResult(full.receipt, fullState)
	closureValue, closureReadable := QueryResult(full.closureReceipt, fullState)
	if fullStatus != SolveComplete || fullState == nil || !fullReadable || !closureReadable || closureValue != 71 || full.stats.closureTransfers == 0 {
		t.Fatalf("full selected closure Solve = state:%v status:%v main:%+v/%t closure:%d/%t transfers:%d", fullState, fullStatus, fullResult, fullReadable, closureValue, closureReadable, full.stats.closureTransfers)
	}
	if demandedResult != fullResult {
		t.Fatalf("demanded and full selected State/Query differ after late route discovery: demanded=%+v full=%+v", demandedResult, fullResult)
	}
	if !equalStagedDynamicSolverLawTrace(demanded.stats.projectionStages, full.stats.projectionStages) ||
		!equalStagedDynamicSolverLawTrace(demanded.stats.evidenceStages, full.stats.evidenceStages) {
		t.Fatalf("demanded/full staged route traces differ: demanded results/evidence=%v/%v full=%v/%v", demanded.stats.projectionStages, demanded.stats.evidenceStages, full.stats.projectionStages, full.stats.evidenceStages)
	}
}

package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestDerivationSelectionRefEvidenceKeepsEqualValueCoordinatesDistinct proves
// the proof-time route predicate carries information that Value facts alone
// cannot. Both dynamic coordinates hold the same Value, but the locator
// selected only the first one under tag 41; accepting the second Ref would
// make a homogeneous Factor's broad value admission an evidence forgery.
func TestDerivationSelectionRefEvidenceKeepsEqualValueCoordinatesDistinct(t *testing.T) {
	const base = 98_800
	composition := NewComposition()
	control := coldFactor(composition, coldKey(base+1))
	dynamic := coldFactor(composition, coldKey(base+2))
	output := coldFactor(composition, coldKey(base+3))
	foreign := coldFactor(composition, coldKey(base+4))
	if control == nil || dynamic == nil || output == nil || foreign == nil {
		t.Fatal("factors")
	}
	controlRead, controlReadOK := ExactReadForm(control)
	controlWrite, controlWriteOK := ExactWriteForm(control)
	dynamicRead, dynamicReadOK := ExactReadForm(dynamic)
	dynamicWrite, dynamicWriteOK := ExactWriteForm(dynamic)
	outputRead, outputReadOK := ExactReadForm(output)
	outputWrite, outputWriteOK := ExactWriteForm(output)
	if !controlReadOK || !controlWriteOK || !dynamicReadOK || !dynamicWriteOK || !outputReadOK || !outputWriteOK {
		t.Fatal("forms")
	}

	var controlSeedWrite, dynamicFirstSeedWrite, dynamicSecondSeedWrite Write[uint64]
	controlSeed, controlSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(base + 5), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: control.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](uint64(base + 6)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var ok bool
		controlSeedWrite, ok = WriteTo(rule, controlWrite)
		return ok
	})
	dynamicSeed, dynamicSeedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(base + 7), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: dynamic.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](uint64(base + 8)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			// The two instances below deliberately write this identical value.
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(7)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var firstOK, secondOK bool
		dynamicFirstSeedWrite, firstOK = WriteTo(rule, dynamicWrite)
		dynamicSecondSeedWrite, secondOK = WriteTo(rule, dynamicWrite)
		return firstOK && secondOK
	})
	if !controlSeedOK || controlSeed == nil || !dynamicSeedOK || dynamicSeed == nil {
		t.Fatal("seed rules")
	}

	checks := 0
	var trigger Read[OrderedCells[uint64]]
	var selection Read[Selection[uint64, OrderedCells[uint64]]]
	var projectionWrite Write[uint64]
	var selectedRef, siblingRef, foreignRef Ref[uint64]
	var staleDerivation RuleDerivation[uint64, ruleUnit]
	var staleDisposition RuleDisposition[uint64]
	projection, projectionOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(base + 9), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: output.Output(), Inputs: 2,
		Admission: AdmitRuleByDerivation(coldKey(base+10), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			if derivation.ReadCount() != 2 || derivation.DispositionCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, present := derivation.DispositionAt(0)
			tag, cells, selected := DerivationDispositionSelectionAt(derivation, disposition, selection, 0)
			value, valuePresent, valueOK := cells.At(0)
			if !present || !selected || disposition.Kind() != RuleDispositionStaged || disposition.TargetCount() != 1 ||
				tag != 41 || cells.Count() != 1 || !valueOK || !valuePresent || value != 7 ||
				!DerivationDispositionSelectionMatchesRef(derivation, disposition, selection, 0, selectedRef) ||
				DerivationDispositionSelectionMatchesRef(derivation, disposition, selection, 0, siblingRef) ||
				DerivationDispositionSelectionMatchesRef(derivation, disposition, selection, 0, foreignRef) ||
				DerivationDispositionSelectionMatchesRef(derivation, disposition, selection, 1, selectedRef) ||
				DerivationDispositionSelectionMatchesRef(derivation, disposition, Read[Selection[uint64, OrderedCells[uint64]]]{}, 0, selectedRef) {
				return RuleEvidence{}, false
			}
			staleDerivation, staleDisposition = derivation, disposition
			return derivation.Accept()
		}),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				controlCells, controlOK := ReadValue(access, row, trigger)
				selected, selectionOK := ReadValue(access, row, selection)
				count, countOK := SelectionCount(access, row, selected)
				tag, dynamicCells, routeOK := SelectionAt(access, row, selected, 0)
				control, controlPresent, controlValueOK := controlCells.At(0)
				dynamicValue, dynamicPresent, dynamicValueOK := dynamicCells.At(0)
				return controlOK && selectionOK && countOK && routeOK && controlCells.Count() == 1 && dynamicCells.Count() == 1 &&
					controlValueOK && controlPresent && control == 1 && dynamicValueOK && dynamicPresent && dynamicValue == 7 && tag == 41 && count == 1 &&
					StageValue(access, row, uint64(9))
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		controlIn, controlInputOK := rule.InputAt(0)
		dynamicIn, dynamicInputOK := rule.InputAt(1)
		var triggerOK, selectionOK, writeOK bool
		trigger, triggerOK = ReadFrom(rule, controlIn, controlRead)
		selection, selectionOK = SelectRead[uint64, ruleUnit, uint64, OrderedCells[uint64], uint64](rule, dynamicIn, dynamicRead, []Dependency{ReadDependency(trigger)}, func(context SelectorContext, _ ruleUnit) bool {
			cells, readable := SelectorRead(context, trigger)
			value, present, valueOK := cells.At(0)
			return readable && cells.Count() == 1 && valueOK && present && value == 1 && SelectRoute(context, selectedRef, uint64(41))
		})
		projectionWrite, writeOK = WriteTo(rule, outputWrite)
		return controlInputOK && dynamicInputOK && triggerOK && selectionOK && writeOK
	})
	if !projectionOK || projection == nil {
		t.Fatal("projection rule")
	}

	var outputQueryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(base + 11),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, outputQueryRead)
				value, present, valueOK := cells.At(0)
				if !readable || cells.Count() != 1 || !valueOK || !present || value != 9 {
					return false
				}
				result, rows = value, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(base + 12)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		outputQueryRead, ok = QueryReadFrom(query, outputRead)
		return ok
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("query or composition")
	}

	controlRef, controlIssued := control.Ref(0)
	selectedRef, selectedIssued := dynamic.Ref(0)
	siblingRef, siblingIssued := dynamic.Ref(1)
	foreignRef, foreignIssued := foreign.Ref(0)
	outputRef, outputIssued := output.Ref(0)
	if !controlIssued || !selectedIssued || !siblingIssued || !foreignIssued || !outputIssued {
		t.Fatal("refs")
	}

	controlInstance, controlInstanceOK := NewRuleInstance(controlSeed, ruleUnitForSemantic(coldKey(base+13)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, controlSeedWrite, controlRef)
	})
	dynamicInstance, dynamicInstanceOK := NewRuleInstance(dynamicSeed, ruleUnitForSemantic(coldKey(base+14)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, dynamicFirstSeedWrite, selectedRef) &&
			InstanceWrite(binding, dynamicSecondSeedWrite, siblingRef)
	})
	projectionInstance, projectionInstanceOK := NewRuleInstance(projection, ruleUnitForSemantic(coldKey(base+16)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, trigger, controlRef) &&
			InstanceSelectorRead(binding, selection, dynamicRead) &&
			InstanceWrite(binding, projectionWrite, outputRef)
	})
	if !controlInstanceOK || !dynamicInstanceOK || !projectionInstanceOK {
		t.Fatal("instances")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(base+17).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(coldKey(base+18).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	controlOccurrence, controlOccurrenceOK := batch.Relation(sourceSite, coldKey(base+19).compositionKey())
	dynamicOccurrence, dynamicOccurrenceOK := batch.Relation(sourceSite, coldKey(base+20).compositionKey())
	projectionOccurrence, projectionOccurrenceOK := batch.Relation(targetSite, coldKey(base+21).compositionKey())
	controlOperand, controlOperandOK := admitInstanceOperand(batch, controlOccurrence, controlInstance)
	dynamicOperand, dynamicOperandOK := admitInstanceOperand(batch, dynamicOccurrence, dynamicInstance)
	projectionOperand, projectionOperandOK := admitInstanceOperand(batch, projectionOccurrence, projectionInstance)
	if !sourceSiteOK || !targetSiteOK || !controlOccurrenceOK || !dynamicOccurrenceOK || !projectionOccurrenceOK ||
		!controlOperandOK || !dynamicOperandOK || !projectionOperandOK || !batch.Seal() {
		t.Fatal("batch")
	}
	controlBoundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(base+22).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	dynamicBoundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(base+23).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())

	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint, targetPoint := admitPoint(assembly, sourceSite), admitPoint(assembly, targetSite)
		controlMember := admitInstance(assembly, sourcePoint, controlOccurrence, controlOperand, controlInstance)
		dynamicMember := admitInstance(assembly, sourcePoint, dynamicOccurrence, dynamicOperand, dynamicInstance)
		projectionMember := admitInstance(assembly, targetPoint, projectionOccurrence, projectionOperand, projectionInstance)
		sourceGroup := admitGroup(assembly, sourcePoint, controlMember, dynamicMember)
		targetGroup := admitGroup(assembly, targetPoint, projectionMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, outputQueryRead, outputRef)
		})
		observation := admitQueryAt(assembly, targetPoint, queryInstance)
		controlBound := admitBoundary(assembly, targetGroup, controlBoundary)
		dynamicBound := admitBoundary(assembly, targetGroup, dynamicBoundary)
		return sourcePoint != nil && targetPoint != nil && controlMember != nil && dynamicMember != nil && projectionMember != nil &&
			sourceGroup != nil && targetGroup != nil && queryInstanceOK && observation != nil && controlBoundary.Available() && dynamicBoundary.Available() && controlBound && dynamicBound
	})
	if !assembled || solver == nil {
		t.Fatal("assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 9 || checks != 1 {
		t.Fatalf("solve state=%v status=%v result=%d readable=%t checks=%d", state, status, result, readable, checks)
	}
	if DerivationDispositionSelectionMatchesRef(staleDerivation, staleDisposition, selection, 0, selectedRef) {
		t.Fatal("stale derivation retained staged route identity")
	}
}

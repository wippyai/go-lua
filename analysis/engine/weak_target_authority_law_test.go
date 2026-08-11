package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestWeakTargetCannotCrossAssemblyAuthority proves that the disposable
// Assembly, not a coincident local surface ordinal, owns a weak target. Each
// transaction below issues its first Summary and first WeakTarget, so A and B
// deliberately occupy the same local summary/weak slots.
func TestWeakTargetCannotCrossAssemblyAuthority(t *testing.T) {
	composition := NewComposition()
	control := coldFactor(composition, coldKey(262_001))
	var outputSummary ReadForm[uint64, uint64]
	var targetSelector WriteForm[uint64]
	outputSpec := coldFactorSpec(coldKey(262_002))
	outputSpec.KeyEnd = 1
	output, outputOK := DeclareFactor(composition, outputSpec, func(factor *Factor[uint64, uint64]) bool {
		normalizer, normalizerOK := DeclareNormalizer(factor, coldKey(262_003), func(cells OrderedCells[uint64]) uint64 {
			return uint64(cells.Count())
		}, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		if !normalizerOK {
			return false
		}
		var summaryOK, selectorOK bool
		outputSummary, summaryOK = SummaryReadForm(normalizer)
		targetSelector, selectorOK = DeclareWriteSelector(factor, coldKey(262_004))
		return summaryOK && selectorOK
	})
	if control == nil || !outputOK || output == nil {
		t.Fatal("weak-target factors")
	}
	controlReadForm, controlReadOK := ExactReadForm(control)
	controlWriteForm, controlWriteOK := ExactWriteForm(control)
	outputReadForm, outputReadOK := ExactReadForm(output)
	if !controlReadOK || !controlWriteOK || !outputReadOK || !outputSummary.valid() || !targetSelector.valid() {
		t.Fatal("weak-target forms")
	}

	var seedWrite Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, Semantic: coldKey(262_005), Output: control.Output(), Inputs: 0,
		OperandContent: ruleUnitContent,
		Admission:      testTrustedTheorem[uint64](262_006),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		seedWrite, declared = WriteTo(rule, controlWriteForm)
		return declared
	})
	var controlRead Read[OrderedCells[uint64]]
	var selectedWrite Write[uint64]
	selector, selectorOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, Semantic: coldKey(262_007), Output: output.Output(), Inputs: 1,
		OperandContent: ruleUnitContent,
		Admission:      testTrustedTheorem[uint64](262_008),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK bool
		controlRead, readOK = ReadFrom(rule, input, controlReadForm)
		var writeOK bool
		selectedWrite, writeOK = SelectWrite(rule, targetSelector, []Read[OrderedCells[uint64]]{controlRead}, []Dependency{ReadDependency(controlRead)}, func(SelectorContext) bool { return true })
		return inputOK && readOK && writeOK
	})
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(262_009), Project: func(Observation) uint64 { return 0 }, Result: frozenColdResult(coldKey(262_010)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		queryRead, declared = QueryReadFrom(query, outputReadForm)
		return declared
	})
	if !seedOK || seed == nil || !selectorOK || selector == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatalf("weak-target declarations seed=%t/%t selector=%t/%t query=%t/%t", seedOK, seed != nil, selectorOK, selector != nil, queryOK, query != nil)
	}

	controlRef, controlRefOK := control.Ref(0)
	outputRef, outputRefOK := output.Ref(0)
	outputRefs := output.NewClosedRefs()
	if !controlRefOK || !outputRefOK || outputRefs == nil || !outputRefs.Append(outputRef) || !outputRefs.Close() {
		t.Fatal("weak-target references")
	}
	type result struct {
		solver   *Solver
		compiled bool
		local    SelectorTarget
		selected bool
		member   *assemblyMember
	}
	assemble := func(choose func(SelectorTarget) SelectorTarget) (result result) {
		batch := equation.NewBatch()
		scope := equation.EmptyScope()
		seedSite, seedSiteOK := batch.AdmitSite(coldKey(262_011).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
		selectorSite, selectorSiteOK := batch.AdmitSite(coldKey(262_012).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
		seedOccurrence, seedOccurrenceOK := batch.Relation(seedSite, coldKey(262_013).compositionKey())
		selectorOccurrence, selectorOccurrenceOK := batch.Relation(selectorSite, coldKey(262_014).compositionKey())
		var candidate SelectorTarget
		seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(262_015)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			return InstanceWrite(binding, seedWrite, controlRef)
		})
		selectorInstance, selectorInstanceOK := NewRuleInstance(selector, ruleUnitForSemantic(coldKey(262_016)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			if !InstanceRead(binding, controlRead, controlRef) {
				return false
			}
			result.selected = InstanceSelectorWrite(binding, selectedWrite, targetSelector, []SelectorTarget{candidate}, nil)
			return result.selected
		})
		seedOperand, seedOperandOK := admitInstanceOperand(batch, seedOccurrence, seedInstance)
		selectorOperand, selectorOperandOK := admitInstanceOperand(batch, selectorOccurrence, selectorInstance)
		if !scope.Available() || !seedSiteOK || !selectorSiteOK || !seedOccurrenceOK || !selectorOccurrenceOK ||
			!seedInstanceOK || !selectorInstanceOK || !seedOperandOK || !selectorOperandOK || !batch.Seal() {
			t.Fatal("weak-target source batch")
		}
		boundary := equation.BoundaryInput(seedSite, selectorSite, coldKey(262_017).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
		if !boundary.Available() {
			t.Fatal("weak-target boundary")
		}
		result.solver, result.compiled = assemble(composition, batch, func(assembly *Assembly) bool {
			result.local = WeakTarget(assembly, outputSummary, outputRefs)
			candidate = choose(result.local)
			seedPoint, selectorPoint := admitPoint(assembly, seedSite), admitPoint(assembly, selectorSite)
			seedMember := admitInstance(assembly, seedPoint, seedOccurrence, seedOperand, seedInstance)
			result.member = admitInstance(assembly, selectorPoint, selectorOccurrence, selectorOperand, selectorInstance)
			if result.local == nil || seedPoint == nil || selectorPoint == nil || !seedInstanceOK || !selectorInstanceOK || seedMember == nil || result.member == nil {
				return false
			}
			seedGroup := admitGroup(assembly, seedPoint, seedMember)
			selectorGroup := admitGroup(assembly, selectorPoint, result.member)
			queryInstance, queryInstanceOK := NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
				return InstanceQueryRead(binding, queryRead, outputRef)
			})
			return seedGroup != nil && selectorGroup != nil && admitBoundary(assembly, selectorGroup, boundary) && queryInstanceOK && admitQueryAt(assembly, selectorPoint, queryInstance) != nil
		})
		return result
	}

	assemblyA := assemble(func(local SelectorTarget) SelectorTarget { return local })
	if !assemblyA.compiled || assemblyA.solver == nil || assemblyA.local == nil || !assemblyA.selected || assemblyA.member == nil {
		t.Fatal("Assembly A did not accept its own weak target")
	}
	assemblyB := assemble(func(local SelectorTarget) SelectorTarget { return local })
	if !assemblyB.compiled || assemblyB.solver == nil || assemblyB.local == nil || !assemblyB.selected || assemblyB.member == nil {
		t.Fatal("Assembly B did not accept its own weak target")
	}
	stale := assemble(func(SelectorTarget) SelectorTarget { return assemblyA.local })
	if stale.compiled || stale.solver != nil || stale.local == nil || stale.selected || stale.member != nil {
		t.Fatal("Assembly B accepted Assembly A's stale weak target")
	}
}

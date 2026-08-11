package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// TestTransformedCarryMapsTheCarriedClosureBeforeItsStrongResultWrite is the
// allocation-recency shape in a minimal Factor-neutral form.  The source has
// two equal cells. The carrying rule maps both 2→1 (a monotone but
// non-extensive transfer), then strongly replaces
// only its selected result with 99.  If transform and write were separate
// publications, or if the map were applied after the write, this observable
// result would differ.
func TestTransformedCarryMapsTheCarriedClosureBeforeItsStrongResultWrite(t *testing.T) {
	solver, receipt := transformedCarryFixture(t, transformedCarryWrite, false)
	state, status := solver.Solve(context.Background())
	value, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || value != 9_901 {
		t.Fatalf("map then strong result = state:%v status:%v value:%d readable:%t", state, status, value, readable)
	}
}

// TestTransformedCarryCanBeTheOnlyRowEffect proves the map does not need a
// sentinel write.  Both carried coordinates change, and the checker observes
// the explicit transform-only disposition.
func TestTransformedCarrySupportsTransformOnlyRows(t *testing.T) {
	solver, receipt := transformedCarryFixture(t, transformedCarryOnly, false)
	state, status := solver.Solve(context.Background())
	value, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || value != 101 {
		t.Fatalf("transform-only result = state:%v status:%v value:%d readable:%t", state, status, value, readable)
	}
}

// TestTransformedCarryNoCandidateDoesNotApplyTheMap proves omission is not a
// hidden transform.  Ordinary carry still transports the immutable source
// root, so the destination observes 2,2 rather than 1,1.
func TestTransformedCarryNoCandidateDoesNotApplyTheMap(t *testing.T) {
	solver, receipt := transformedCarryFixture(t, transformedCarryOmit, false)
	state, status := solver.Solve(context.Background())
	value, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || value != 202 {
		t.Fatalf("no-candidate carry result = state:%v status:%v value:%d readable:%t", state, status, value, readable)
	}
}

// TestTransformedCarryRejectsBadEvidence proves no transformed candidate is
// published when its local checker rejects the declared carry form.
func TestTransformedCarryRejectsBadEvidence(t *testing.T) {
	solver, _ := transformedCarryFixture(t, transformedCarryWrite, true)
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete {
		t.Fatalf("rejected transformed carry = state:%v status:%v", state, status)
	}
}

type transformedCarryMode uint8

const (
	transformedCarryWrite transformedCarryMode = iota + 1
	transformedCarryOnly
	transformedCarryOmit
)

func transformedCarryFixture(t testing.TB, mode transformedCarryMode, rejectEvidence bool) (*Solver, QueryReceipt[uint64]) {
	t.Helper()
	carryOperand := ruleUnitForSemantic(coldKey(160_080 + uint64(mode)))
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(160_000 + uint64(mode)))
	spec.KeyEnd = 2
	spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	transformSemantic := coldKey(160_001 + uint64(mode))
	transform := func(operand ruleUnit, value uint64) (uint64, bool) {
		if value == 0 {
			return 0, true
		}
		if operand != carryOperand {
			return 0, false
		}
		return 1, true
	}
	factor, declared := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("transformed-carry factor")
	}
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	carryForm, carryFormOK := Carry(factor)
	if !readOK || !writeOK || !carryFormOK {
		t.Fatal("transformed-carry forms")
	}

	var sourceLeft, sourceRight, result Write[uint64]
	source, sourceOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(160_010 + uint64(mode)), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](160_020 + uint64(mode)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(2)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var leftOK, rightOK bool
		sourceLeft, leftOK = WriteTo(rule, write)
		sourceRight, rightOK = WriteTo(rule, write)
		return leftOK && rightOK
	})
	if !sourceOK || source == nil {
		t.Fatal("transformed-carry source")
	}

	admission := AdmitRuleByDerivation[uint64, ruleUnit](coldKey(160_030+uint64(mode)), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
		if derivation.DispositionCount() != 1 {
			return RuleEvidence{}, false
		}
		disposition, okay := derivation.DispositionAt(0)
		semantic, transformed := disposition.CarryTransform()
		if !okay {
			return RuleEvidence{}, false
		}
		switch mode {
		case transformedCarryWrite:
			value, staged := disposition.Value()
			if disposition.Kind() != RuleDispositionStaged || !transformed || semantic != transformSemantic || disposition.TransformOnly() || !staged || value != 99 || disposition.TargetCount() != 1 {
				return RuleEvidence{}, false
			}
		case transformedCarryOnly:
			if disposition.Kind() != RuleDispositionStaged || !transformed || semantic != transformSemantic || !disposition.TransformOnly() || disposition.TargetCount() != 0 {
				return RuleEvidence{}, false
			}
			if _, staged := disposition.Value(); staged {
				return RuleEvidence{}, false
			}
		case transformedCarryOmit:
			if disposition.Kind() != RuleDispositionNoCandidate || transformed || disposition.TransformOnly() {
				return RuleEvidence{}, false
			}
		default:
			return RuleEvidence{}, false
		}
		if rejectEvidence {
			return RuleEvidence{}, false
		}
		return derivation.Accept()
	})
	carry, carryOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		Semantic: coldKey(160_040 + uint64(mode)), OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Output: factor.Output(), Inputs: 1, Admission: admission,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool {
				switch mode {
				case transformedCarryWrite:
					return StageValue(access, row, uint64(99))
				case transformedCarryOnly:
					return StageTransform(access, row)
				case transformedCarryOmit:
					return NoCandidate(access, row)
				default:
					return false
				}
			})
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		if !inputOK || !TransformCarryFrom(rule, input, carryForm, transformSemantic, transform) {
			return false
		}
		if mode != transformedCarryWrite {
			return true
		}
		var written bool
		result, written = WriteTo(rule, write)
		return written
	})
	if !carryOK || carry == nil {
		t.Fatal("transformed-carry rule")
	}

	var leftToken, rightToken QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(160_050 + uint64(mode)),
		Project: func(observation Observation) uint64 {
			result, rows := uint64(0), 0
			if !ProjectRows(observation, func(row QueryRow) bool {
				left, leftOK := QueryValue(row, leftToken)
				right, rightOK := QueryValue(row, rightToken)
				leftValue, leftPresent, leftCell := left.At(0)
				rightValue, rightPresent, rightCell := right.At(0)
				if !leftOK || !rightOK || !leftCell || !rightCell || !leftPresent || !rightPresent {
					return false
				}
				result, rows = leftValue*100+rightValue, rows+1
				return true
			}) || rows != 1 {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(160_060 + uint64(mode))),
	}, func(query *Query[uint64]) bool {
		var leftOK, rightOK bool
		leftToken, leftOK = QueryReadFrom(query, read)
		rightToken, rightOK = QueryReadFrom(query, read)
		return leftOK && rightOK
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("transformed-carry query/composition")
	}

	leftRef, leftIssued := factor.Ref(0)
	rightRef, rightIssued := factor.Ref(1)
	sourceInstance, sourceInstanceOK := NewRuleInstance(source, ruleUnitForSemantic(coldKey(160_070+uint64(mode))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, sourceLeft, leftRef) && InstanceWrite(binding, sourceRight, rightRef)
	})
	carryInstance, carryInstanceOK := NewRuleInstance(carry, carryOperand, func(binding *RuleBinding[uint64, ruleUnit]) bool {
		if mode != transformedCarryWrite {
			return true
		}
		return InstanceWrite(binding, result, leftRef)
	})
	if !leftIssued || !rightIssued || !sourceInstanceOK || !carryInstanceOK {
		t.Fatal("transformed-carry instances")
	}

	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(160_090+uint64(mode)).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	targetSite, targetSiteOK := batch.AdmitSite(coldKey(160_100+uint64(mode)).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	sourceOccurrence, sourceOccurred := batch.Relation(sourceSite, coldKey(160_110+uint64(mode)).compositionKey())
	targetOccurrence, targetOccurred := batch.Relation(targetSite, coldKey(160_120+uint64(mode)).compositionKey())
	sourceOperand, sourceOperandOK := admitInstanceOperand(batch, sourceOccurrence, sourceInstance)
	targetOperand, targetOperandOK := admitInstanceOperand(batch, targetOccurrence, carryInstance)
	if !scope.Available() || !sourceSiteOK || !targetSiteOK || !sourceOccurred || !targetOccurred || !sourceOperandOK || !targetOperandOK || !batch.Seal() {
		t.Fatal("transformed-carry batch")
	}

	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint, targetPoint := admitPoint(assembly, sourceSite), admitPoint(assembly, targetSite)
		sourceMember := admitInstance(assembly, sourcePoint, sourceOccurrence, sourceOperand, sourceInstance)
		targetMember := admitInstance(assembly, targetPoint, targetOccurrence, targetOperand, carryInstance)
		sourceGroup := admitGroup(assembly, sourcePoint, sourceMember)
		targetGroup := admitGroup(assembly, targetPoint, targetMember)
		boundary := equation.BoundaryInput(sourceSite, targetSite, coldKey(160_130+uint64(mode)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, leftToken, leftRef) && InstanceQueryRead(binding, rightToken, rightRef)
		})
		observation := admitQueryAt(assembly, targetPoint, queryInstance)
		return sourcePoint != nil && targetPoint != nil && sourceMember != nil && targetMember != nil && sourceGroup != nil && targetGroup != nil && boundary.Available() && admitBoundary(assembly, targetGroup, boundary) && queryDeclared && observation != nil
	})
	if !compiled || solver == nil {
		t.Fatal("transformed-carry assembly")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("transformed-carry query receipt")
	}
	return solver, receipt
}

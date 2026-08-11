package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func recurrenceSolverFixture(t *testing.T, ranked bool) (*Solver, bool) {
	return recurrenceSolverFixtureWithAdmission(t, ranked, func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return testTrustedTheorem[uint64](19802)
	})
}

func recurrenceSolverFixtureWithAdmission(t *testing.T, ranked bool, admissionFor func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit]) (*Solver, bool) {
	solver, _, _, assembled := recurrenceSolverFixtureWithCallbacks(t, ranked, admissionFor, nil, nil)
	return solver, assembled
}

func recurrenceSolverFixtureWithObservedTransfer(t *testing.T, ranked bool, admissionFor func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit], transferObserved func()) (*Solver, bool) {
	transfer := func(access Access[uint64, ruleUnit]) bool {
		if transferObserved != nil {
			transferObserved()
		}
		return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
	}
	solver, _, _, assembled := recurrenceSolverFixtureWithCallbacks(t, ranked, admissionFor, transfer, nil)
	return solver, assembled
}

// recurrenceSolverFixtureWithCallbacks declares the rule and query behavior
// before compilation so tests can observe execution without reaching into a
// compiled runtime. Nil callbacks use the ordinary one-cell recurrence.
func recurrenceSolverFixtureWithCallbacks(t *testing.T, ranked bool, admissionFor func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit], transfer func(Access[uint64, ruleUnit]) bool, project func(Observation, QueryRead[OrderedCells[uint64]]) uint64) (*Solver, *Query[uint64], QueryReceipt[uint64], bool) {
	t.Helper()
	if admissionFor == nil {
		admissionFor = func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
			return testTrustedTheorem[uint64](19802)
		}
	}
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(9801))
	spec.KeyEnd = 1
	if ranked {
		spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
		spec.Lattice.Narrow = func(_ uint64, desired uint64) uint64 { return desired }
		spec.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	}
	factor, ok := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	if !ok || factor == nil {
		t.Fatal("factor declaration")
	}
	write, ok := ExactWriteForm(factor)
	if !ok {
		t.Fatal("write form")
	}
	read, ok := ExactReadForm(factor)
	if !ok {
		t.Fatal("read form")
	}
	var ruleRead Read[OrderedCells[uint64]]
	var ruleWrite Write[uint64]
	rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(9802), Output: factor.Output(), Inputs: 1, Admission: admissionFor(&ruleRead),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			if transfer != nil {
				return transfer(access)
			}
			return Product(access, func(row Row) bool { return StageValue(access, row, 1) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		ruleRead, readOK = ReadFrom(rule, input, read)
		ruleWrite, writeOK = WriteTo(rule, write)
		return inputOK && readOK && writeOK
	})
	if !ok || rule == nil {
		t.Fatal("rule declaration")
	}
	var token QueryRead[OrderedCells[uint64]]
	query, ok := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(9803),
		Project: func(observation Observation) uint64 {
			if project != nil {
				return project(observation, token)
			}
			return recurrenceQueryValue(observation, token)
		},
		Result: FrozenResult[uint64]{
			Semantic: coldKey(9804), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
		},
	}, func(query *Query[uint64]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, read)
		return declared
	})
	if !ok || query == nil || !composition.Seal() {
		t.Fatal("query/composition")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(9806)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceRead(binding, ruleRead, readRef) && InstanceWrite(binding, ruleWrite, writeRef)
	})
	batch, sites, occurrences, operands, admitted := lifecycleLawBatch(
		[]SemanticKey{coldKey(9805)}, []SemanticKey{coldKey(9802)}, []*RuleInstance[uint64, ruleUnit]{instance}, []equation.InitDisposition{equation.InitPresent},
	)
	if !readRefOK || !writeRefOK || !instanceOK || !admitted {
		return nil, nil, QueryReceipt[uint64]{}, false
	}
	boundary := equation.BoundaryInput(sites[0], sites[0], coldKey(9807).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(sites[0].Scope()), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, sites[0])
		member := admitInstance(assembly, point, occurrences[0], operands[0], instance)
		group := admitGroup(assembly, point, member)
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		observation := admitQueryAt(assembly, point, queryInstance)
		return point != nil && queryDeclared && member != nil && group != nil && boundary.Available() && admitBoundary(assembly, group, boundary) && observation != nil
	})
	if !compiled || solver == nil {
		return nil, nil, QueryReceipt[uint64]{}, false
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		return nil, nil, QueryReceipt[uint64]{}, false
	}
	return solver, query, receipt, true
}

func recurrenceQueryValue(observation Observation, token QueryRead[OrderedCells[uint64]]) uint64 {
	value := uint64(0)
	if !ProjectRows(observation, func(row QueryRow) bool {
		cells, ok := QueryValue(row, token)
		if !ok || cells.Count() != 1 {
			return false
		}
		cell, present, ok := cells.At(0)
		if !ok || !present {
			return false
		}
		value = cell
		return true
	}) {
		return 0
	}
	return value
}

func TestRuleDerivationCheckerRejectsBeforePatchAdmission(t *testing.T) {
	checks := 0
	solver, ok := recurrenceSolverFixtureWithAdmission(t, true, func(read *Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return AdmitRuleByDerivation(coldKey(19803), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			if derivation.Rule() != coldKey(9802) || !derivation.Anchor().Available() || derivation.InputCount() != 1 || derivation.DispositionCount() != 1 || derivation.ReadCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, ok := derivation.DispositionAt(0)
			value, readOK := DerivationDispositionReadValue(derivation, disposition, *read)
			input, _, inputOK := value.At(0)
			staged, stagedOK := disposition.Value()
			if !ok || !readOK || value.Count() != 1 || !inputOK || !stagedOK || disposition.Kind() != RuleDispositionStaged || staged != input+1 || disposition.TargetCount() != 1 {
				return RuleEvidence{}, false
			}
			return RuleEvidence{}, false
		})
	})
	if !ok || solver == nil {
		t.Fatal("derivation fixture assembly")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete || checks != 1 {
		t.Fatalf("unchecked result published or checker was not exactly once: state=%v status=%v checks=%d", state, status, checks)
	}
}

func TestRuleDerivationCheckerPanicRejectsBeforePatchAdmission(t *testing.T) {
	checks := 0
	solver, ok := recurrenceSolverFixtureWithAdmission(t, true, func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return AdmitRuleByDerivation(coldKey(19804), func(RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			panic("checker failure")
		})
	})
	if !ok || solver == nil {
		t.Fatal("derivation fixture assembly")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete || checks != 1 {
		t.Fatalf("panicking checker published or retried: state=%v status=%v checks=%d", state, status, checks)
	}
}

func TestRuleDerivationCheckerRejectsWrongTypedOutput(t *testing.T) {
	checks := 0
	solver, ok := recurrenceSolverFixtureWithAdmission(t, true, func(read *Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return AdmitRuleByDerivation(coldKey(19805), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			disposition, dispositionOK := derivation.DispositionAt(0)
			input, readOK := DerivationDispositionReadValue(derivation, disposition, *read)
			value, _, cellOK := input.At(0)
			staged, stagedOK := disposition.Value()
			if !dispositionOK || !readOK || !cellOK || !stagedOK || staged != value {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		})
	})
	if !ok || solver == nil {
		t.Fatal("wrong-output fixture assembly")
	}
	state, status := solver.Solve(context.Background())
	if state != nil || status != SolveIncomplete || checks != 1 {
		t.Fatalf("wrong typed output was admitted: state=%v status=%v checks=%d", state, status, checks)
	}
}

func TestRuleDerivationReadExpiresAfterCheckerReturns(t *testing.T) {
	var retained RuleDerivation[uint64, ruleUnit]
	var retainedDisposition RuleDisposition[uint64]
	var retainedRead Read[OrderedCells[uint64]]
	checks := 0
	solver, ok := recurrenceSolverFixtureWithAdmission(t, true, func(read *Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		retainedRead = *read
		return AdmitRuleByDerivation(coldKey(19806), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			disposition, dispositionOK := derivation.DispositionAt(0)
			if !dispositionOK {
				return RuleEvidence{}, false
			}
			value, readOK := DerivationDispositionReadValue(derivation, disposition, retainedRead)
			if !readOK || value.Count() != 1 {
				return RuleEvidence{}, false
			}
			retained, retainedDisposition = derivation, disposition
			return derivation.Accept()
		})
	})
	if !ok || solver == nil {
		t.Fatal("stale-read fixture assembly")
	}
	_, _ = solver.Solve(context.Background())
	if checks == 0 {
		t.Fatal("checker was never invoked")
	}
	if _, live := DerivationDispositionReadValue(retained, retainedDisposition, retainedRead); live {
		t.Fatal("derivation read survived callback/frame revocation")
	}
}

// TestNoCandidateIsOneExplicitCheckerVisibleDisposition proves that an empty
// Factor successor is neither a failed Product callback nor a Default write.
// The checker sees the same row's read through the one disposition vector and
// accepts the complete empty successor without a Factor patch.
func TestNoCandidateIsOneExplicitCheckerVisibleDisposition(t *testing.T) {
	checks := 0
	solver, query, receipt, assembled := recurrenceSolverFixtureWithCallbacks(t, true, func(read *Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return AdmitRuleByDerivation(coldKey(19809), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			if derivation.DispositionCount() != 1 || derivation.ReadCount() != 1 {
				return RuleEvidence{}, false
			}
			disposition, dispositionOK := derivation.DispositionAt(0)
			cells, readOK := DerivationDispositionReadValue(derivation, disposition, *read)
			if !dispositionOK || !readOK || disposition.Kind() != RuleDispositionNoCandidate || disposition.TargetCount() != 0 || cells.Count() != 1 {
				return RuleEvidence{}, false
			}
			if _, staged := disposition.Value(); staged {
				return RuleEvidence{}, false
			}
			return derivation.Accept()
		})
	}, func(access Access[uint64, ruleUnit]) bool {
		return Product(access, func(row Row) bool { return NoCandidate(access, row) })
	}, func(observation Observation, token QueryRead[OrderedCells[uint64]]) uint64 {
		rows := 0
		if !ProjectRows(observation, func(row QueryRow) bool {
			cells, readable := QueryValue(row, token)
			_, present, cellOK := cells.At(0)
			if !readable || cells.Count() != 1 || !cellOK || present {
				return false
			}
			rows++
			return true
		}) || rows != 1 {
			return 0
		}
		return 17
	})
	if !assembled || solver == nil || query == nil {
		t.Fatal("no-candidate fixture assembly")
	}
	state, status := solver.Solve(context.Background())
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !readable || result != 17 || checks != 1 {
		t.Fatalf("no-candidate solve = state:%v status:%v result:%d readable:%t checks:%d", state, status, result, readable, checks)
	}
}

func TestUnsettledProductRowAndCallbackAbortRemainFailures(t *testing.T) {
	for name, transfer := range map[string]func(Access[uint64, ruleUnit]) bool{
		"unsettled": func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(Row) bool { return true })
		},
		"callback-abort": func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(Row) bool { return false })
		},
	} {
		t.Run(name, func(t *testing.T) {
			solver, _, _, assembled := recurrenceSolverFixtureWithCallbacks(t, true, nil, transfer, nil)
			if !assembled || solver == nil {
				t.Fatal("failure fixture assembly")
			}
			state, status := solver.Solve(context.Background())
			if state != nil || status != SolveIncomplete {
				t.Fatalf("%s result = state:%v status:%v", name, state, status)
			}
		})
	}
}

// TestTrustedTheoremAdmissionKeepsOnlyNecessarySolverWork compares the two
// public admission bases on the same compiled recurrence. Trusted admission
// adds no checker callback; derivation admission receives the complete
// operands/results exactly once per otherwise identical transfer. The warmed
// trusted solver remains allocation-free through the public Solve boundary.
func TestTrustedTheoremAdmissionKeepsOnlyNecessarySolverWork(t *testing.T) {
	trustedTransfers := 0
	trusted, trustedOK := recurrenceSolverFixtureWithObservedTransfer(t, true, func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return testTrustedTheorem[uint64](19807)
	}, func() { trustedTransfers++ })
	if !trustedOK || trusted == nil {
		t.Fatal("trusted fixture assembly")
	}

	derivedTransfers, checks := 0, 0
	derived, derivedOK := recurrenceSolverFixtureWithObservedTransfer(t, true, func(*Read[OrderedCells[uint64]]) RuleAdmission[uint64, ruleUnit] {
		return AdmitRuleByDerivation(coldKey(19808), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
			checks++
			return derivation.Accept()
		})
	}, func() { derivedTransfers++ })
	if !derivedOK || derived == nil {
		t.Fatal("derivation fixture assembly")
	}

	trustedState, trustedStatus := trusted.Solve(context.Background())
	derivedState, derivedStatus := derived.Solve(context.Background())
	if trustedStatus != SolveComplete || derivedStatus != SolveComplete || trustedState == nil || derivedState == nil {
		t.Fatalf("admission solves = trusted:%v/%v derived:%v/%v", trustedState, trustedStatus, derivedState, derivedStatus)
	}
	if trustedTransfers != derivedTransfers || checks != derivedTransfers {
		t.Fatalf("admission work = trusted transfers:%d derived transfers:%d checks:%d", trustedTransfers, derivedTransfers, checks)
	}
	allocations := testing.AllocsPerRun(100, func() {
		state, status := trusted.Solve(context.Background())
		if status != SolveComplete || state == nil {
			panic("trusted warm Solve")
		}
	})
	if allocations != 0 {
		t.Fatalf("trusted warm Solve allocations = %v, want 0", allocations)
	}
}

func TestCyclicPointRequiresRankedWidenAndConverges(t *testing.T) {
	if solver, ok := recurrenceSolverFixture(t, false); ok || solver != nil {
		t.Fatal("cyclic unranked factor passed recurrence admission")
	}
	solver, ok := recurrenceSolverFixture(t, true)
	if !ok || solver == nil {
		t.Fatal("ranked cyclic factor rejected")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("ranked cycle did not converge: %v", status)
	}
}

func TestCompletedStatesRemainValidAfterWarmSolve(t *testing.T) {
	projects := 0
	solver, query, receipt, ok := recurrenceSolverFixtureWithCallbacks(t, true, nil, nil, func(observation Observation, token QueryRead[OrderedCells[uint64]]) uint64 {
		projects++
		return recurrenceQueryValue(observation, token)
	})
	if !ok || solver == nil || query == nil {
		t.Fatal("ranked cyclic fixture")
	}
	first, firstStatus := solver.Solve(context.Background())
	second, secondStatus := solver.Solve(context.Background())
	if firstStatus != SolveComplete || secondStatus != SolveComplete || first == nil || second == nil {
		t.Fatalf("statuses = %v, %v", firstStatus, secondStatus)
	}
	firstValue, firstReadable := QueryResult(receipt, first)
	secondValue, secondReadable := QueryResult(receipt, second)
	if !firstReadable || !secondReadable || firstValue != 1 || secondValue != firstValue || projects != 1 {
		t.Fatalf("warm completed results = first:%d/%v second:%d/%v projects:%d", firstValue, firstReadable, secondValue, secondReadable, projects)
	}
}

func TestCarryOnlyRuleContributesItsWholeFactorPlane(t *testing.T) {
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(9810))
	spec.KeyEnd = 1
	spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
	spec.Lattice.Narrow = func(_ uint64, desired uint64) uint64 { return desired }
	spec.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	factor, ok := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	if !ok || factor == nil {
		t.Fatal("factor declaration")
	}
	carry, ok := Carry(factor)
	if !ok {
		t.Fatal("carry form")
	}
	write, ok := ExactWriteForm(factor)
	if !ok {
		t.Fatal("source write form")
	}
	sourceTransfers := 0
	var sourceWrite Write[uint64]
	source, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(9811), Output: factor.Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](19811),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			sourceTransfers++
			return Product(access, func(row Row) bool { return StageValue(access, row, 9) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		sourceWrite, declared = WriteTo(rule, write)
		return declared
	})
	if !ok || source == nil {
		t.Fatal("source rule declaration")
	}
	carryTransfers := 0
	rule, ok := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
		Semantic: coldKey(9815), Output: factor.Output(), Inputs: 1, Admission: testTrustedTheorem[uint64](19815),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			carryTransfers++
			return Product(access, func(Row) bool { return true })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		return inputOK && CarryFrom(rule, input, carry)
	})
	if !ok || rule == nil {
		t.Fatal("carry-only rule declaration")
	}
	read, readOK := ExactReadForm(factor)
	if !readOK {
		t.Fatal("carry-only read form")
	}
	var token QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(9813),
		Project: func(observation Observation) uint64 {
			return recurrenceQueryValue(observation, token)
		},
		Result: FrozenResult[uint64]{Semantic: coldKey(9814), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value }, Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value }},
	}, func(query *Query[uint64]) bool {
		var declared bool
		token, declared = QueryReadFrom(query, read)
		return declared
	})
	if !queryOK || query == nil {
		t.Fatal("carry-only query declaration")
	}
	if !composition.Seal() {
		t.Fatal("carry-only composition seal")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	sourceInstance, sourceInstanceOK := NewRuleInstance(source, ruleUnitForSemantic(coldKey(9817)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, sourceWrite, writeRef)
	})
	carryInstance, carryInstanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(9818)), func(*RuleBinding[uint64, ruleUnit]) bool { return true })
	batch, sites, occurrences, operands, admitted := lifecycleLawBatch(
		[]SemanticKey{coldKey(9812), coldKey(9816)}, []SemanticKey{coldKey(9811), coldKey(9815)}, []*RuleInstance[uint64, ruleUnit]{sourceInstance, carryInstance},
		[]equation.InitDisposition{equation.InitPresent, equation.InitPresent},
	)
	if !readRefOK || !writeRefOK || !sourceInstanceOK || !carryInstanceOK || !admitted {
		t.Fatal("carry-only source batch")
	}
	boundary := equation.BoundaryInput(sites[0], sites[1], coldKey(9819).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(sites[0].Scope()), equation.TrueExpr())
	var queryInstance *QueryInstance[uint64]
	solver, compiled := assemble(composition, batch, func(assembly *Assembly) bool {
		sourcePoint := admitPoint(assembly, sites[0])
		carryPoint := admitPoint(assembly, sites[1])
		sourceMember := admitInstance(assembly, sourcePoint, occurrences[0], operands[0], sourceInstance)
		carryMember := admitInstance(assembly, carryPoint, occurrences[1], operands[1], carryInstance)
		sourceGroup := admitGroup(assembly, sourcePoint, sourceMember)
		carryGroup := admitGroup(assembly, carryPoint, carryMember)
		var queryDeclared bool
		queryInstance, queryDeclared = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, token, readRef)
		})
		observation := admitQueryAt(assembly, carryPoint, queryInstance)
		return sourcePoint != nil && carryPoint != nil && sourceMember != nil && carryMember != nil &&
			sourceGroup != nil && carryGroup != nil && boundary.Available() && admitBoundary(assembly, carryGroup, boundary) && queryDeclared && observation != nil
	})
	if !compiled || solver == nil {
		t.Fatal("carry-only solver compilation")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("carry-only solve = %v", status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	value, readable := QueryResult(receipt, state)
	if !receiptOK || !readable || value != 9 || sourceTransfers == 0 || carryTransfers == 0 {
		t.Fatalf("carry-only result = value:%d readable:%v source:%d carry:%d", value, readable, sourceTransfers, carryTransfers)
	}
}

package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

type candidateRevisionFixture struct {
	solver  *Solver
	flip    *bool
	receipt QueryReceipt[uint64]
}

func newCandidateRevisionFixture(t *testing.T, recursive bool) candidateRevisionFixture {
	t.Helper()
	composition := NewComposition()
	spec := coldFactorSpec(coldKey(110_001))
	spec.KeyEnd = 1
	if recursive {
		spec.WidenRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return ^value }}
		spec.Lattice.Narrow = func(_ uint64, desired uint64) uint64 { return desired }
		spec.NarrowRank = Measure[uint64, uint64]{Width: 1, At: func(_ uint64, value uint64, _ int) uint64 { return value }}
	}
	factor, factorOK := DeclareFactor(composition, spec, func(*Factor[uint64, uint64]) bool { return true })
	read, readOK := ExactReadForm(factor)
	write, writeOK := ExactWriteForm(factor)
	if !factorOK || factor == nil || !readOK || !writeOK {
		t.Fatal("factor")
	}
	flip := false
	rules := make([]*Rule[uint64, ruleUnit], 2)
	reads := make([]Read[OrderedCells[uint64]], 2)
	writes := make([]Write[uint64], 2)
	for index := range rules {
		index := index
		rule, declared := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
			OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: coldKey(uint64(110_010 + index)), Output: factor.Output(), Inputs: boolInputCount(recursive), Admission: testTrustedTheorem[uint64](uint64(110_020 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				if index == 1 {
					return Product(access, func(row Row) bool { return NoCandidate(access, row) })
				}
				value := uint64(1)
				if index == 0 && flip {
					value = 2
				}
				return Product(access, func(row Row) bool { return StageValue(access, row, value) })
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			var ok bool
			if recursive {
				input, inputOK := rule.InputAt(0)
				reads[index], ok = ReadFrom(rule, input, read)
				if !inputOK || !ok {
					return false
				}
			}
			writes[index], ok = WriteTo(rule, write)
			return ok
		})
		if !declared || rule == nil {
			t.Fatalf("rule %d", index)
		}
		rules[index] = rule
	}
	var queryRead QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(110_030),
		Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, ok := QueryValue(row, queryRead)
				value, present, valid := cells.At(0)
				if !ok || !valid || !present {
					return false
				}
				if value > result {
					result = value
				}
				return true
			}) {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(110_031)),
	}, func(query *Query[uint64]) bool {
		var ok bool
		queryRead, ok = QueryReadFrom(query, read)
		return ok
	})
	if !queryOK || query == nil || !composition.Seal() {
		t.Fatal("query/composition")
	}
	readRef, readRefOK := factor.Ref(0)
	writeRef, writeRefOK := factor.Ref(0)
	if !readRefOK || !writeRefOK {
		t.Fatal("refs")
	}
	instances := make([]*RuleInstance[uint64, ruleUnit], len(rules))
	for index, rule := range rules {
		index := index
		instance, instanceOK := NewRuleInstance(rule, ruleUnitForSemantic(coldKey(uint64(110_040+index))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
			if recursive && !InstanceRead(binding, reads[index], readRef) {
				return false
			}
			return InstanceWrite(binding, writes[index], writeRef)
		})
		if !instanceOK {
			t.Fatalf("instance %d", index)
		}
		instances[index] = instance
	}
	batch, sites, occurrences, operands, admitted := lifecycleLawBatchForSites(
		[]SemanticKey{coldKey(110_050)}, []SemanticKey{coldKey(110_060), coldKey(110_061)}, instances, []int{0, 0}, []equation.InitDisposition{equation.InitPresent},
	)
	if !admitted {
		t.Fatal("batch")
	}
	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, batch, func(assembly *Assembly) bool {
		point := admitPoint(assembly, sites[0])
		if point == nil {
			return false
		}
		for index := range instances {
			member := admitInstance(assembly, point, occurrences[index], operands[index], instances[index])
			group := admitGroup(assembly, point, member)
			if member == nil || group == nil {
				return false
			}
			if recursive {
				boundary, boundaryOK := wtoIdentityBoundary(sites[0], sites[0], coldKey(uint64(110_070+index)))
				if !boundaryOK || !admitBoundary(assembly, group, boundary) {
					return false
				}
			}
		}
		var queryOK bool
		queryInstance, queryOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, readRef)
		})
		return queryOK && admitQueryAt(assembly, point, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("assembly")
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("query receipt")
	}
	return candidateRevisionFixture{solver: solver, flip: &flip, receipt: receipt}
}

func boolInputCount(recursive bool) int {
	if recursive {
		return 1
	}
	return 0
}

// publishCandidateRevision uses the same final query-materialization and
// retention/publication cut as Solver.Solve after the deliberately direct
// epoch wake. This keeps the laws focused on refreshPoint while still
// checking the result through the real opaque QueryReceipt path.
func publishCandidateRevision(t *testing.T, fixture candidateRevisionFixture, epoch *executorEpoch) *State {
	t.Helper()
	if epoch == nil || epoch.runtime == nil || len(epoch.runtime.queries) != 1 {
		t.Fatal("query publication shape")
	}
	result, materialized := epoch.runtime.queries[0].materialize(epoch.work, epoch.points[0].State())
	if !materialized || result == nil {
		t.Fatal("query materialization")
	}
	nextCompletion := fixture.solver.completion + 1
	state := &State{
		owner:      fixture.solver,
		completion: &completionAuthority{solver: fixture.solver, serial: nextCompletion, revision: fixture.solver.revision},
		results:    []*queryResult{result},
	}
	retained, retainedOK := epoch.work.Retain()
	if !retainedOK {
		t.Fatal("epoch retention")
	}
	epoch.work = nil
	if !fixture.solver.publishCompleted(epoch, epoch.runtime, state, nextCompletion, retained) {
		retained.Close()
		epoch.discard()
		t.Fatal("epoch publication")
	}
	epoch.discard()
	return state
}

// TestAcyclicCandidateChangeSurvivesUnchangedLaterProducer is the direct
// regression for the aggregate witness: the first producer replaces its
// candidate, while a later no-candidate producer is reevaluated to the exact
// same representation. The first change must still publish the point RHS.
func TestAcyclicCandidateChangeSurvivesUnchangedLaterProducer(t *testing.T) {
	fixture := newCandidateRevisionFixture(t, false)
	epoch, opened := newRuntimeEpoch(fixture.solver.runtime, nil, context.Background())
	if !opened || !epoch.run() {
		t.Fatal("initial epoch")
	}
	if len(epoch.producers) != 2 {
		t.Fatalf("producers=%d", len(epoch.producers))
	}
	beforePoint := epoch.versions[0]
	beforeFirst := epoch.producers[0].version
	beforeSecond := epoch.producers[1].version
	*fixture.flip = true
	for index := range epoch.producers {
		if !epoch.markDirty(index) {
			t.Fatalf("mark dirty %d", index)
		}
	}
	if !epoch.run() {
		t.Fatal("second epoch")
	}
	if epoch.versions[0] <= beforePoint {
		t.Fatalf("point publication version = %d, want > %d after first candidate changed", epoch.versions[0], beforePoint)
	}
	if epoch.producers[0].version <= beforeFirst || epoch.producers[1].version != beforeSecond {
		t.Fatalf("producer versions = %d/%d, want first > %d and second = %d", epoch.producers[0].version, epoch.producers[1].version, beforeFirst, beforeSecond)
	}
	state := publishCandidateRevision(t, fixture, epoch)
	value, readable := QueryResult(fixture.receipt, state)
	if !readable || value != 2 {
		t.Fatalf("acyclic query = value:%d readable:%t, want 2/true", value, readable)
	}
}

// TestRegionCandidateChangeAdvancesProducerVersion is the direct recurrence
// regression. The first back producer changes while the later producer is an
// exact no-candidate repeat; its producer version must invalidate the region's
// exact-input witness so the next episode cannot reuse stale exact state.
func TestRegionCandidateChangeAdvancesProducerVersion(t *testing.T) {
	fixture := newCandidateRevisionFixture(t, true)
	epoch, opened := newRuntimeEpoch(fixture.solver.runtime, nil, context.Background())
	if !opened || !epoch.run() {
		t.Fatal("initial epoch")
	}
	if len(epoch.producers) != 2 || len(epoch.runtime.regions) != 1 {
		t.Fatalf("producers=%d regions=%d", len(epoch.producers), len(epoch.runtime.regions))
	}
	before := epoch.producers[0].version
	*fixture.flip = true
	for index := range epoch.producers {
		if !epoch.markDirty(index) {
			t.Fatalf("mark dirty %d", index)
		}
	}
	if !epoch.run() {
		t.Fatal("second epoch")
	}
	if epoch.producers[0].version <= before {
		t.Fatalf("region producer version = %d, want > %d after changed back candidate", epoch.producers[0].version, before)
	}
	state := publishCandidateRevision(t, fixture, epoch)
	value, readable := QueryResult(fixture.receipt, state)
	if !readable || value != 2 {
		t.Fatalf("region query = value:%d readable:%t, want 2/true", value, readable)
	}
}

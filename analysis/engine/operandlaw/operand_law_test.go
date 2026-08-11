package operandlaw_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type payload struct{ value uint64 }

func TestTypedOperandAdmissionRespectsOpenBatchAuthority(t *testing.T) {
	fixture := newFixture(t)
	instance := newInstance(t, fixture.rule, payload{value: 17}, fixture.write, fixture.ref)
	source := engine.NewSourceAssembly(fixture.composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	site, siteOK := source.Site(semantic(300), scope, truth, true)
	occurrence, occurrenceOK := source.At(site)
	if !scopeOK || !truthOK || !siteOK || !occurrenceOK {
		t.Fatal("open source occurrence")
	}
	if occurrence.Available() {
		t.Fatal("open occurrence exposed sealed authority")
	}
	prepared, admitted := source.PrepareInstance(occurrence, instance)
	if !admitted {
		t.Fatal("owning open batch rejected typed operand")
	}
	if prepared.Available() {
		t.Fatal("open operand exposed sealed authority")
	}
	if !source.Seal() || !occurrence.Available() || !prepared.Available() {
		t.Fatal("sealed source authority unavailable")
	}

	foreign := engine.NewSourceAssembly(fixture.composition)
	foreignScope, foreignScopeOK := foreign.Scope()
	foreignTruth, foreignTruthOK := foreign.TrueExpr()
	foreignSite, foreignSiteOK := foreign.Site(semantic(301), foreignScope, foreignTruth, true)
	_, foreignOccurrenceOK := foreign.At(foreignSite)
	foreignInstance := newInstance(t, fixture.rule, payload{value: 19}, fixture.write, fixture.ref)
	if !foreignScopeOK || !foreignTruthOK || !foreignSiteOK || !foreignOccurrenceOK {
		t.Fatal("foreign open source occurrence")
	}
	if _, accepted := foreign.PrepareInstance(occurrence, foreignInstance); accepted {
		t.Fatal("foreign batch accepted typed operand occurrence")
	}
}

func TestTypedOperandsBindOncePerTopologyAndRejectForeignSource(t *testing.T) {
	fixture := newFixture(t)
	firstInstance := newInstance(t, fixture.rule, payload{value: 11}, fixture.write, fixture.ref)
	secondInstance := newInstance(t, fixture.rule, payload{value: 29}, fixture.write, fixture.ref)
	source, firstSite, firstPrepared, secondSite, secondPrepared := newBatch(t, fixture.composition, 1, firstInstance, secondInstance)

	var firstQueryInstance, secondQueryInstance *engine.QueryInstance[uint64]
	solver, ok := source.Assemble(func(assembly *engine.Assembly) bool {
		firstPoint, firstPointOK := assembly.Point(firstSite)
		secondPoint, secondPointOK := assembly.Point(secondSite)
		first, firstOK := assembly.Member(firstPoint, firstPrepared)
		second, secondOK := assembly.Member(secondPoint, secondPrepared)
		_, firstGroupOK := assembly.Group(firstPoint, first)
		_, secondGroupOK := assembly.Group(secondPoint, second)
		if !firstPointOK || !secondPointOK || !firstOK || !secondOK || !firstGroupOK || !secondGroupOK {
			return false
		}
		firstQueryInstance = newQueryInstance(t, fixture.firstQuery, fixture.ref)
		secondQueryInstance = newQueryInstance(t, fixture.secondQuery, fixture.ref)
		_, firstObservationOK := assembly.Query(firstPoint, firstQueryInstance)
		_, secondObservationOK := assembly.Query(secondPoint, secondQueryInstance)
		return firstObservationOK && secondObservationOK
	})
	if !ok || solver == nil {
		t.Fatal("canonical typed topology did not compile")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("solve status = %v", status)
	}
	firstReceipt, firstReceiptOK := firstQueryInstance.Receipt()
	secondReceipt, secondReceiptOK := secondQueryInstance.Receipt()
	if !firstReceiptOK || !secondReceiptOK {
		t.Fatal("query receipts")
	}
	if value, readable := engine.QueryResult(firstReceipt, state); !readable || value != 11 {
		t.Fatalf("first payload result = %d, readable=%v", value, readable)
	}
	if value, readable := engine.QueryResult(secondReceipt, state); !readable || value != 29 {
		t.Fatalf("second payload result = %d, readable=%v", value, readable)
	}

	t.Run("mismatched occurrence", func(t *testing.T) {
		firstInstance := newInstance(t, fixture.rule, payload{value: 1}, fixture.write, fixture.ref)
		secondInstance := newInstance(t, fixture.rule, payload{value: 2}, fixture.write, fixture.ref)
		source, firstSite, _, secondSite, secondPrepared := newBatch(t, fixture.composition, 20, firstInstance, secondInstance)
		_, accepted := source.Assemble(func(assembly *engine.Assembly) bool {
			point, pointOK := assembly.Point(firstSite)
			row, rowOK := assembly.Member(point, secondPrepared)
			return pointOK && !rowOK && !row.Available() && secondSite.Available()
		})
		if accepted {
			t.Fatal("operand was accepted at a different source Site")
		}
	})

	t.Run("foreign batch", func(t *testing.T) {
		foreignInstance := newInstance(t, fixture.rule, payload{value: 1}, fixture.write, fixture.ref)
		foreignSecond := newInstance(t, fixture.rule, payload{value: 2}, fixture.write, fixture.ref)
		foreignSource, foreignSite, foreignPrepared, _, _ := newBatch(t, fixture.composition, 40, foreignInstance, foreignSecond)
		_, accepted := source.Assemble(func(assembly *engine.Assembly) bool {
			point, pointOK := assembly.Point(foreignSite)
			_, memberOK := assembly.Member(point, foreignPrepared)
			return pointOK && memberOK
		})
		_ = foreignSource
		if accepted {
			t.Fatal("foreign source Batch compiled")
		}
	})
}

func TestAssemblyBindsCompleteProgramBoundary(t *testing.T) {
	fixture := newBoundaryFixture(t)
	seedInstance := newInstance(t, fixture.seed, payload{value: 11}, fixture.seedWrite, fixture.ref)
	edgeInstance := newInstance(t, fixture.edge, payload{value: 29}, fixture.edgeWrite, fixture.ref)
	source := engine.NewSourceAssembly(fixture.composition)
	scope, scopeOK := source.Scope()
	truth, truthOK := source.TrueExpr()
	falsity, falseOK := source.FalseExpr()
	firstSite, firstSiteOK := source.Site(semantic(70), scope, truth, true)
	secondSite, secondSiteOK := source.Site(semantic(71), scope, falsity, false)
	firstOccurrence, firstOccurrenceOK := source.At(firstSite)
	secondOccurrence, secondOccurrenceOK := source.At(secondSite)
	firstPrepared, firstPreparedOK := source.PrepareInstance(firstOccurrence, seedInstance)
	secondPrepared, secondPreparedOK := source.PrepareInstance(secondOccurrence, edgeInstance)
	reindex, reindexOK := source.IdentityReindex(scope)
	boundary, boundaryOK := source.Boundary(firstSite, secondSite, semantic(90), truth, reindex, truth)
	if source == nil || !scopeOK || !truthOK || !falseOK || !firstSiteOK || !secondSiteOK || !firstOccurrenceOK || !secondOccurrenceOK || !firstPreparedOK || !secondPreparedOK || !reindexOK || !boundaryOK || !source.Seal() {
		t.Fatal("source assembly")
	}
	var queryInstance *engine.QueryInstance[uint64]
	solver, ok := source.Assemble(func(assembly *engine.Assembly) bool {
		firstPoint, firstPointOK := assembly.Point(firstSite)
		secondPoint, secondPointOK := assembly.Point(secondSite)
		first, firstOK := assembly.Member(firstPoint, firstPrepared)
		second, secondOK := assembly.Member(secondPoint, secondPrepared)
		_, firstGroupOK := assembly.Group(firstPoint, first)
		secondGroup, secondGroupOK := assembly.Group(secondPoint, second)
		if !firstPointOK || !secondPointOK || !firstOK || !secondOK || !firstGroupOK || !secondGroupOK || !boundary.Available() || !assembly.Boundary(secondGroup, boundary) {
			return false
		}
		queryInstance = newQueryInstance(t, fixture.query, fixture.ref)
		_, observationOK := assembly.Query(secondPoint, queryInstance)
		return observationOK
	})
	if !ok || solver == nil {
		t.Fatal("boundary topology did not compile")
	}
	state, status := solver.Solve(context.Background())
	if status != engine.SolveComplete || state == nil {
		t.Fatalf("boundary solve status = %v", status)
	}
	receipt, receiptOK := queryInstance.Receipt()
	if !receiptOK {
		t.Fatal("boundary query receipt")
	}
	if value, readable := engine.QueryResult(receipt, state); !readable || value != 29 {
		t.Fatalf("boundary payload result = %d, readable=%v", value, readable)
	}
}

// TestTypedOperandPlanScaleBindsEveryMember grows one sealed topology plan
// while keeping one monomorphic Rule schema. Compile must bind every accepted
// member; the observed tail payload then executes through boundRule directly.
// The production plan sorts P payload rows once and each of M graph members
// performs only binary searches during cold revision construction. Solve and
// query retain no payload lookup at all.
func TestTypedOperandPlanScaleBindsEveryMember(t *testing.T) {
	for _, members := range []int{1, 17, 257} {
		t.Run("members="+strconv.Itoa(members), func(t *testing.T) {
			compositionRoot := engine.NewComposition()
			factor, factorOK := engine.DeclareFactor(compositionRoot, engine.FactorSpec[uint64, uint64]{
				Semantic: semantic(uint64(4_000 + members)), KeyEnd: 1, Lattice: uintLattice(), Default: 0,
				AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
			}, func(*engine.Factor[uint64, uint64]) bool { return true })
			if !factorOK || factor == nil {
				t.Fatal("factor")
			}
			read, readOK := engine.ExactReadForm(factor)
			writeForm, writeOK := engine.ExactWriteForm(factor)
			var write engine.Write[uint64]
			var writeBound bool
			rule, ruleOK := engine.DeclareRule(compositionRoot, engine.RuleSpec[uint64, payload]{
				Semantic: semantic(uint64(5_000 + members)), OperandFamily: semantic(uint64(6_000 + members)), Output: factor.Output(), Inputs: 0,
				OperandContent: payloadContent,
				Admission:      engine.AdmitRuleByTrustedTheorem[uint64, payload](semantic(uint64(7_000 + members))),
				Transfer: func(access engine.Access[uint64, payload]) bool {
					value, ok := engine.Operand(access)
					return ok && engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, value.value) })
				},
			}, func(rule *engine.Rule[uint64, payload]) bool {
				write, writeBound = engine.WriteTo(rule, writeForm)
				return writeBound
			})
			query, queryOK := declareQuery(compositionRoot, uint64(8_000+members), uint64(9_000+members), read)
			if !readOK || !writeOK || !writeBound || !ruleOK || rule == nil || !queryOK || query.query == nil || !compositionRoot.Seal() {
				t.Fatal("composition")
			}
			ref, refOK := factor.Ref(0)
			if !refOK {
				t.Fatal("factor ref")
			}

			source := engine.NewSourceAssembly(compositionRoot)
			scope, scopeOK := source.Scope()
			truth, truthOK := source.TrueExpr()
			if source == nil || !scopeOK || !truthOK {
				t.Fatal("source assembly")
			}
			sites := make([]engine.SourceSite, members)
			prepared := make([]engine.SourceInstance, members)
			instances := make([]*engine.RuleInstance[uint64, payload], members)
			for index := range sites {
				site, siteOK := source.Site(semantic(uint64(10_000+index)), scope, truth, true)
				occurrence, occurrenceOK := source.At(site)
				instance := newInstance(t, rule, payload{value: uint64(index + 1)}, write, ref)
				preparedInstance, preparedOK := source.PrepareInstance(occurrence, instance)
				if !siteOK || !occurrenceOK || !preparedOK {
					t.Fatalf("source row %d", index)
				}
				sites[index], prepared[index], instances[index] = site, preparedInstance, instance
			}
			if !source.Seal() {
				t.Fatal("source batch")
			}
			var queryInstance *engine.QueryInstance[uint64]
			solver, compiled := source.Assemble(func(assembly *engine.Assembly) bool {
				for index := range sites {
					point, pointOK := assembly.Point(sites[index])
					member, memberOK := assembly.Member(point, prepared[index])
					_, groupOK := assembly.Group(point, member)
					if !pointOK || !memberOK || !groupOK {
						return false
					}
					if index == len(sites)-1 {
						queryInstance = newQueryInstance(t, query, ref)
						if _, observationOK := assembly.Query(point, queryInstance); !observationOK {
							return false
						}
					}
				}
				return true
			})
			if !compiled || solver == nil {
				t.Fatal("compile")
			}
			state, status := solver.Solve(context.Background())
			if status != engine.SolveComplete || state == nil {
				t.Fatalf("solve status = %v", status)
			}
			receipt, receiptOK := queryInstance.Receipt()
			if !receiptOK {
				t.Fatal("scale query receipt")
			}
			if value, readable := engine.QueryResult(receipt, state); !readable || value != uint64(members) {
				t.Fatalf("tail payload = %d, readable=%v, want=%d", value, readable, members)
			}
		})
	}
}

type fixture struct {
	composition *engine.Composition
	rule        *engine.Rule[uint64, payload]
	ref         engine.Ref[uint64]
	write       engine.Write[uint64]
	firstQuery  declaredQuery
	secondQuery declaredQuery
}

type boundaryFixture struct {
	composition *engine.Composition
	seed        *engine.Rule[uint64, payload]
	edge        *engine.Rule[uint64, payload]
	ref         engine.Ref[uint64]
	seedWrite   engine.Write[uint64]
	edgeWrite   engine.Write[uint64]
	query       declaredQuery
}

func newFixture(t testing.TB) fixture {
	t.Helper()
	compositionRoot := engine.NewComposition()
	factor, declared := engine.DeclareFactor(compositionRoot, engine.FactorSpec[uint64, uint64]{
		Semantic: semantic(100), KeyEnd: 1, Lattice: uintLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("factor")
	}
	read, readOK := engine.ExactReadForm(factor)
	writeForm, writeOK := engine.ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("forms")
	}
	var write engine.Write[uint64]
	var writeBound bool
	rule, declared := engine.DeclareRule(compositionRoot, engine.RuleSpec[uint64, payload]{
		Semantic: semantic(101), OperandFamily: semantic(102), Output: factor.Output(), Inputs: 0,
		OperandContent: payloadContent,
		Admission:      engine.AdmitRuleByTrustedTheorem[uint64, payload](semantic(103)),
		Transfer: func(access engine.Access[uint64, payload]) bool {
			operand, ok := engine.Operand(access)
			return ok && engine.Product(access, func(row engine.Row) bool {
				return engine.StageValue(access, row, operand.value)
			})
		},
	}, func(rule *engine.Rule[uint64, payload]) bool {
		write, writeBound = engine.WriteTo(rule, writeForm)
		return writeBound
	})
	if !declared || !writeBound || rule == nil {
		t.Fatal("rule")
	}
	first, firstOK := declareQuery(compositionRoot, 104, 105, read)
	second, secondOK := declareQuery(compositionRoot, 106, 107, read)
	if !firstOK || !secondOK || !compositionRoot.Seal() {
		t.Fatal("query/composition")
	}
	ref, issued := factor.Ref(0)
	if !issued {
		t.Fatal("factor ref")
	}
	return fixture{composition: compositionRoot, rule: rule, ref: ref, write: write, firstQuery: first, secondQuery: second}
}

func newBoundaryFixture(t testing.TB) boundaryFixture {
	t.Helper()
	compositionRoot := engine.NewComposition()
	factor, declared := engine.DeclareFactor(compositionRoot, engine.FactorSpec[uint64, uint64]{
		Semantic: semantic(200), KeyEnd: 1, Lattice: uintLattice(), Default: 0,
		AdmitAt: func(uint64, uint64) bool { return true }, Fingerprint: func(value uint64) uint64 { return value },
	}, func(*engine.Factor[uint64, uint64]) bool { return true })
	if !declared || factor == nil {
		t.Fatal("boundary factor")
	}
	read, readOK := engine.ExactReadForm(factor)
	writeForm, writeOK := engine.ExactWriteForm(factor)
	if !readOK || !writeOK {
		t.Fatal("boundary forms")
	}
	transfer := func(access engine.Access[uint64, payload]) bool {
		operand, ok := engine.Operand(access)
		return ok && engine.Product(access, func(row engine.Row) bool { return engine.StageValue(access, row, operand.value) })
	}
	var seedWrite, edgeWrite engine.Write[uint64]
	var seedWriteBound, edgeWriteBound bool
	seed, seedOK := engine.DeclareRule(compositionRoot, engine.RuleSpec[uint64, payload]{
		Semantic: semantic(201), OperandFamily: semantic(202), Output: factor.Output(), Inputs: 0,
		OperandContent: payloadContent,
		Admission:      engine.AdmitRuleByTrustedTheorem[uint64, payload](semantic(203)), Transfer: transfer,
	}, func(rule *engine.Rule[uint64, payload]) bool {
		seedWrite, seedWriteBound = engine.WriteTo(rule, writeForm)
		return seedWriteBound
	})
	edge, edgeOK := engine.DeclareRule(compositionRoot, engine.RuleSpec[uint64, payload]{
		Semantic: semantic(204), OperandFamily: semantic(202), Output: factor.Output(), Inputs: 1,
		OperandContent: payloadContent,
		Admission:      engine.AdmitRuleByTrustedTheorem[uint64, payload](semantic(205)), Transfer: transfer,
	}, func(rule *engine.Rule[uint64, payload]) bool {
		edgeWrite, edgeWriteBound = engine.WriteTo(rule, writeForm)
		return edgeWriteBound
	})
	query, queryOK := declareQuery(compositionRoot, 206, 207, read)
	if !seedOK || !edgeOK || !seedWriteBound || !edgeWriteBound || seed == nil || edge == nil || !queryOK || !compositionRoot.Seal() {
		t.Fatal("boundary composition")
	}
	ref, issued := factor.Ref(0)
	if !issued {
		t.Fatal("boundary ref")
	}
	return boundaryFixture{composition: compositionRoot, seed: seed, edge: edge, ref: ref, seedWrite: seedWrite, edgeWrite: edgeWrite, query: query}
}

func newInstance(t testing.TB, rule *engine.Rule[uint64, payload], value payload, write engine.Write[uint64], ref engine.Ref[uint64]) *engine.RuleInstance[uint64, payload] {
	t.Helper()
	instance, ok := engine.NewRuleInstance(rule, value, func(binding *engine.RuleBinding[uint64, payload]) bool {
		return engine.InstanceWrite(binding, write, ref)
	})
	if !ok || instance == nil {
		t.Fatal("rule instance")
	}
	return instance
}

type declaredQuery struct {
	query *engine.Query[uint64]
	read  engine.QueryRead[engine.OrderedCells[uint64]]
}

func newQueryInstance(t testing.TB, declared declaredQuery, ref engine.Ref[uint64]) *engine.QueryInstance[uint64] {
	t.Helper()
	instance, ok := engine.NewQueryInstance(declared.query, func(binding *engine.QueryBinding[uint64]) bool {
		return engine.InstanceQueryRead(binding, declared.read, ref)
	})
	if !ok || instance == nil {
		t.Fatal("query instance")
	}
	return instance
}

func declareQuery(compositionRoot *engine.Composition, queryID, resultID uint64, read engine.ReadForm[uint64, engine.OrderedCells[uint64]]) (declaredQuery, bool) {
	var queryRead engine.QueryRead[engine.OrderedCells[uint64]]
	query, ok := engine.DeclareQuery(compositionRoot, engine.QuerySpec[uint64]{
		Semantic: semantic(queryID), Result: frozen(resultID),
		Project: func(observation engine.Observation) uint64 {
			var result uint64
			if !engine.ProjectRows(observation, func(row engine.QueryRow) bool {
				cells, ok := engine.QueryValue(row, queryRead)
				if !ok {
					return false
				}
				result, _, ok = cells.At(0)
				return ok
			}) {
				return 0
			}
			return result
		},
	}, func(query *engine.Query[uint64]) bool {
		var ok bool
		queryRead, ok = engine.QueryReadFrom(query, read)
		return ok
	})
	return declaredQuery{query: query, read: queryRead}, ok
}

func newBatch(t testing.TB, composition *engine.Composition, base uint64, firstInstance, secondInstance *engine.RuleInstance[uint64, payload]) (*engine.SourceAssembly, engine.SourceSite, engine.SourceInstance, engine.SourceSite, engine.SourceInstance) {
	t.Helper()
	source := engine.NewSourceAssembly(composition)
	scope, scoped := source.Scope()
	truth, truthOK := source.TrueExpr()
	if source == nil || !scoped || !truthOK {
		t.Fatal("scope")
	}
	firstSite, firstSiteOK := source.Site(semantic(base), scope, truth, true)
	secondSite, secondSiteOK := source.Site(semantic(base+1), scope, truth, true)
	firstOccurrence, firstOccurrenceOK := source.At(firstSite)
	secondOccurrence, secondOccurrenceOK := source.At(secondSite)
	firstPrepared, firstPreparedOK := source.PrepareInstance(firstOccurrence, firstInstance)
	secondPrepared, secondPreparedOK := source.PrepareInstance(secondOccurrence, secondInstance)
	if !firstSiteOK || !secondSiteOK || !firstOccurrenceOK || !secondOccurrenceOK || !firstPreparedOK || !secondPreparedOK || !source.Seal() {
		t.Fatal("source batch")
	}
	return source, firstSite, firstPrepared, secondSite, secondPrepared
}

func payloadContent(value payload) (payload, [32]byte, bool) {
	return value, digest(value.value), value.value != 0
}

func semantic(value uint64) engine.SemanticKey {
	key, ok := engine.NewSemanticKey(digest(value), 1)
	if !ok {
		panic("semantic")
	}
	return key
}

func digest(value uint64) [32]byte {
	var result [32]byte
	for index := 0; index < 8; index++ {
		result[24+index] = byte(value >> uint((7-index)*8))
	}
	return result
}

func frozen(value uint64) engine.FrozenResult[uint64] {
	return engine.FrozenResult[uint64]{
		Semantic: semantic(value), Freeze: func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
		Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
	}
}

func uintLattice() lattice.Lattice[uint64] {
	return lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) }, Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
	}
}

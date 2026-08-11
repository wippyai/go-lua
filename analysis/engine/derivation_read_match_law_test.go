package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

type derivationReadMatchBatch struct {
	batch       *equation.Batch
	sourceSite  equation.Site
	ruleSite    equation.Site
	source      equation.Occurrence
	rule        equation.Occurrence
	sourceValue equation.Operand
	ruleValue   equation.Operand
	boundary    equation.Input
}

func TestSummaryReadProofSharesCanonicalKeysAcrossAliases(t *testing.T) {
	composition := NewComposition()
	schema := &factorSchema{composition: composition}
	form := &formSchema{factor: schema, readKind: summaryReadForm}
	factor := &Factor[uint64, uint64]{composition: composition, schema: schema}
	keys := []uint64{3, 7, 11}
	bound := &boundFactor[uint64, uint64]{
		factor: factor,
		reads:  make(map[equation.Surface]boundUnit, 64),
	}
	unit := boundUnit{kind: carrier.SummaryUnit, summaryKeys: keys}
	proofs := make([]ruleSummaryReadProof, 64)
	for index := range proofs {
		surface := equation.Surface{Local: uint64(index + 1)}
		bound.reads[surface] = unit
		proof, proved := bound.summaryReadProof(surface, form)
		if !proved || len(proof.keys) != len(keys) || &proof.keys[0] != &keys[0] {
			t.Fatalf("summary alias %d did not retain the canonical key vector", index)
		}
		proofs[index] = proof
	}
	bound.releaseColdBindings()
	for index, proof := range proofs {
		if len(proof.keys) != len(keys) || &proof.keys[0] != &keys[0] {
			t.Fatalf("summary alias %d lost or copied the canonical key vector after release", index)
		}
	}
}

func newDerivationReadMatchBatch(t testing.TB, base int, sourceInstance, ruleInstance *RuleInstance[uint64, ruleUnit]) derivationReadMatchBatch {
	t.Helper()
	batch := equation.NewBatch()
	scope := equation.EmptyScope()
	sourceSite, sourceSiteOK := batch.AdmitSite(coldKey(base+1).compositionKey(), scope, equation.TrueExpr(), equation.InitPresent)
	ruleSite, ruleSiteOK := batch.AdmitSite(coldKey(base+2).compositionKey(), scope, equation.FalseExpr(), equation.InitAbsent)
	source, sourceOK := batch.Relation(sourceSite, coldKey(base+3).compositionKey())
	rule, ruleOK := batch.Relation(ruleSite, coldKey(base+4).compositionKey())
	sourceValue, sourceValueOK := admitInstanceOperand(batch, source, sourceInstance)
	ruleValue, ruleValueOK := admitInstanceOperand(batch, rule, ruleInstance)
	if batch == nil || !scope.Available() || !sourceSiteOK || !ruleSiteOK || !sourceOK || !ruleOK || !sourceValueOK || !ruleValueOK || !batch.Seal() {
		t.Fatal("derivation read match source batch")
	}
	boundary := equation.BoundaryInput(sourceSite, ruleSite, coldKey(base+7).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
	if !boundary.Available() {
		t.Fatal("derivation read match boundary")
	}
	return derivationReadMatchBatch{
		batch: batch, sourceSite: sourceSite, ruleSite: ruleSite, source: source, rule: rule,
		sourceValue: sourceValue, ruleValue: ruleValue, boundary: boundary,
	}
}

func declareDerivationReadMatchQuery(t testing.TB, composition *Composition, sink *Factor[uint64, uint64], base int) (*Query[uint64], QueryRead[OrderedCells[uint64]]) {
	t.Helper()
	readForm, readFormOK := ExactReadForm(sink)
	if !readFormOK {
		t.Fatal("derivation read match query form")
	}
	var read QueryRead[OrderedCells[uint64]]
	query, queryOK := DeclareQuery(composition, QuerySpec[uint64]{
		Semantic: coldKey(base + 1),
		Project: func(observation Observation) uint64 {
			var result uint64
			if !ProjectRows(observation, func(row QueryRow) bool {
				cells, readable := QueryValue(row, read)
				value, present, available := cells.At(0)
				if !readable || !available || !present {
					return false
				}
				result = value
				return true
			}) {
				return 0
			}
			return result
		},
		Result: frozenColdResult(coldKey(base + 2)),
	}, func(query *Query[uint64]) bool {
		var declared bool
		read, declared = QueryReadFrom(query, readForm)
		return declared
	})
	if !queryOK || query == nil {
		t.Fatal("derivation read match query")
	}
	return query, read
}

func TestDerivationReadMatchesOnlyItsExactBoundRef(t *testing.T) {
	tests := []struct {
		name  string
		probe func(source, foreign *Factor[uint64, uint64]) Ref[uint64]
		want  bool
	}{
		{
			name: "expected exact Ref accepted",
			probe: func(source, _ *Factor[uint64, uint64]) Ref[uint64] {
				ref, _ := source.Ref(0)
				return ref
			},
			want: true,
		},
		{
			name: "swapped exact Ref rejected",
			probe: func(source, _ *Factor[uint64, uint64]) Ref[uint64] {
				ref, _ := source.Ref(1)
				return ref
			},
		},
		{
			name: "foreign exact Ref rejected",
			probe: func(_, foreign *Factor[uint64, uint64]) Ref[uint64] {
				ref, _ := foreign.Ref(0)
				return ref
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := 268_000 + index*100
			composition := NewComposition()
			source := coldFactor(composition, coldKey(base+1))
			foreign := coldFactor(composition, coldKey(base+2))
			sink := coldFactor(composition, coldKey(base+3))
			if source == nil || foreign == nil || sink == nil {
				t.Fatal("derivation exact read factors")
			}
			sourceRead, sourceReadOK := ExactReadForm(source)
			sourceWrite, sourceWriteOK := ExactWriteForm(source)
			sinkWrite, sinkWriteOK := ExactWriteForm(sink)
			if !sourceReadOK || !sourceWriteOK || !sinkWriteOK {
				t.Fatal("derivation exact read forms")
			}
			var seedWrite Write[uint64]
			seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
				OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 4), Output: source.Output(), Inputs: 0,
				Admission: testTrustedTheorem[uint64](uint64(base + 5)),
				Transfer: func(access Access[uint64, ruleUnit]) bool {
					return Product(access, func(row Row) bool { return StageValue(access, row, uint64(11)) })
				},
			}, func(rule *Rule[uint64, ruleUnit]) bool {
				var declared bool
				seedWrite, declared = WriteTo(rule, sourceWrite)
				return declared
			})
			checks := 0
			var projectRead Read[OrderedCells[uint64]]
			var probe Ref[uint64]
			admission := AdmitRuleByDerivation(coldKey(base+6), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
				checks++
				if DerivationReadMatchesRef(derivation, projectRead, probe) != test.want {
					return RuleEvidence{}, false
				}
				return derivation.Accept()
			})
			var projectWrite Write[uint64]
			project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
				OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 7), Output: sink.Output(), Inputs: 1,
				Admission: admission,
				Transfer: func(access Access[uint64, ruleUnit]) bool {
					return Product(access, func(row Row) bool { return StageValue(access, row, uint64(17)) })
				},
			}, func(rule *Rule[uint64, ruleUnit]) bool {
				input, inputOK := rule.InputAt(0)
				var readOK, writeOK bool
				projectRead, readOK = ReadFrom(rule, input, sourceRead)
				projectWrite, writeOK = WriteTo(rule, sinkWrite)
				return inputOK && readOK && writeOK
			})
			query, queryRead := declareDerivationReadMatchQuery(t, composition, sink, base+8)
			if !seedOK || seed == nil || !projectOK || project == nil || !composition.Seal() {
				t.Fatal("derivation exact read declarations")
			}
			sourceRef, sourceRefOK := source.Ref(0)
			sinkRef, sinkRefOK := sink.Ref(0)
			probe = test.probe(source, foreign)
			if !sourceRefOK || !sinkRefOK || probe.sealAuthority == 0 {
				t.Fatal("derivation exact read refs")
			}
			seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(base+25)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
				return InstanceWrite(binding, seedWrite, sourceRef)
			})
			projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(base+26)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
				return InstanceRead(binding, projectRead, sourceRef) && InstanceWrite(binding, projectWrite, sinkRef)
			})
			if !seedInstanceOK || !projectInstanceOK {
				t.Fatal("derivation exact read instances")
			}
			graph := newDerivationReadMatchBatch(t, base+20, seedInstance, projectInstance)
			var queryInstance *QueryInstance[uint64]
			solver, assembled := assemble(composition, graph.batch, func(assembly *Assembly) bool {
				sourcePoint, rulePoint := admitPoint(assembly, graph.sourceSite), admitPoint(assembly, graph.ruleSite)
				sourceMember := admitInstance(assembly, sourcePoint, graph.source, graph.sourceValue, seedInstance)
				projectMember := admitInstance(assembly, rulePoint, graph.rule, graph.ruleValue, projectInstance)
				sourceGroup, projectGroup := admitGroup(assembly, sourcePoint, sourceMember), admitGroup(assembly, rulePoint, projectMember)
				var queryInstanceOK bool
				queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
					return InstanceQueryRead(binding, queryRead, sinkRef)
				})
				return sourcePoint != nil && rulePoint != nil && seedInstanceOK && projectInstanceOK && sourceMember != nil && projectMember != nil &&
					sourceGroup != nil && projectGroup != nil && admitBoundary(assembly, projectGroup, graph.boundary) && queryInstanceOK && admitQueryAt(assembly, rulePoint, queryInstance) != nil
			})
			if !assembled || solver == nil {
				t.Fatal("derivation exact read assembly")
			}
			state, status := solver.Solve(context.Background())
			receipt, receiptOK := queryInstance.Receipt()
			result, readable := QueryResult(receipt, state)
			if status != SolveComplete || state == nil || !receiptOK || !readable || result != 17 || checks != 1 {
				t.Fatalf("derivation exact read solve = state:%v status:%v result:%d readable:%t checks:%d", state, status, result, readable, checks)
			}
		})
	}
}

func TestDerivationReadMatchesSummaryRefsOwnerFence(t *testing.T) {
	const base = 268_400
	composition := NewComposition()
	var summaryForm ReadForm[uint64, uint64]
	source, sourceOK := DeclareFactor(composition, coldFactorSpec(coldKey(base+1)), func(factor *Factor[uint64, uint64]) bool {
		normalizer, normalizerOK := DeclareNormalizer(factor, coldKey(base+2), func(cells OrderedCells[uint64]) uint64 {
			return uint64(cells.Count())
		}, func(left, right uint64) bool { return left == right }, func(value uint64) uint64 { return value })
		if !normalizerOK {
			return false
		}
		var declared bool
		summaryForm, declared = SummaryReadForm(normalizer)
		return declared
	})
	sink := coldFactor(composition, coldKey(base+3))
	if !sourceOK || source == nil || sink == nil {
		t.Fatal("derivation summary factors")
	}
	sourceWrite, sourceWriteOK := ExactWriteForm(source)
	sinkWrite, sinkWriteOK := ExactWriteForm(sink)
	if !sourceWriteOK || !sinkWriteOK {
		t.Fatal("derivation summary forms")
	}
	var seedWrites [2]Write[uint64]
	seed, seedOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 4), Output: source.Output(), Inputs: 0,
		Admission: testTrustedTheorem[uint64](base + 5),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(19)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var leftOK, rightOK bool
		seedWrites[0], leftOK = WriteTo(rule, sourceWrite)
		seedWrites[1], rightOK = WriteTo(rule, sourceWrite)
		return leftOK && rightOK
	})
	checks := 0
	var summaryRead Read[uint64]
	var sourceRef Ref[uint64]
	var expected, wrongWidth, wrongFactor, open, stale, foreign *ClosedRefs[uint64]
	admission := AdmitRuleByDerivation(coldKey(base+6), func(derivation RuleDerivation[uint64, ruleUnit]) (RuleEvidence, bool) {
		checks++
		if DerivationReadMatchesRef(derivation, summaryRead, sourceRef) {
			return RuleEvidence{}, false
		}
		if !DerivationReadMatchesSummaryRefs(derivation, summaryRead, expected) ||
			DerivationReadMatchesSummaryRefs(derivation, summaryRead, wrongWidth) ||
			DerivationReadMatchesSummaryRefs(derivation, summaryRead, wrongFactor) ||
			DerivationReadMatchesSummaryRefs(derivation, summaryRead, open) ||
			DerivationReadMatchesSummaryRefs(derivation, summaryRead, stale) ||
			DerivationReadMatchesSummaryRefs(derivation, summaryRead, foreign) {
			return RuleEvidence{}, false
		}
		return derivation.Accept()
	})
	var projectWrite Write[uint64]
	project, projectOK := DeclareRule(composition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 7), Output: sink.Output(), Inputs: 1,
		Admission: admission,
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(23)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		input, inputOK := rule.InputAt(0)
		var readOK, writeOK bool
		summaryRead, readOK = ReadFrom(rule, input, summaryForm)
		projectWrite, writeOK = WriteTo(rule, sinkWrite)
		return inputOK && readOK && writeOK
	})
	query, queryRead := declareDerivationReadMatchQuery(t, composition, sink, base+8)
	if !seedOK || seed == nil || !projectOK || project == nil || !composition.Seal() {
		t.Fatal("derivation summary declarations")
	}
	leftRef, leftRefOK := source.Ref(0)
	rightRef, rightRefOK := source.Ref(1)
	sourceRef = leftRef
	sinkRef, sinkRefOK := sink.Ref(0)
	expected = source.NewClosedRefs()
	wrongWidth = source.NewClosedRefs()
	wrongFactor = sink.NewClosedRefs()
	open = source.NewClosedRefs()
	if !leftRefOK || !rightRefOK || !sinkRefOK || expected == nil || wrongWidth == nil || wrongFactor == nil || open == nil ||
		!expected.Append(leftRef) || !expected.Append(rightRef) || !expected.Close() ||
		!wrongWidth.Append(leftRef) || !wrongWidth.Close() ||
		!wrongFactor.Append(sinkRef) || !wrongFactor.Close() ||
		!open.Append(leftRef) || !open.Append(rightRef) {
		t.Fatal("derivation summary refs")
	}
	stale = &ClosedRefs[uint64]{factor: source.schema, refs: append([]Ref[uint64](nil), expected.refs...), closed: true}
	stale.refs[0].sealAuthority++
	foreignComposition := NewComposition()
	foreignFactor := coldFactor(foreignComposition, coldKey(base+30))
	foreignRead, foreignReadOK := ExactReadForm(foreignFactor)
	foreignWrite, foreignWriteOK := ExactWriteForm(foreignFactor)
	var foreignRuleWrite Write[uint64]
	_, foreignRuleOK := DeclareRule(foreignComposition, RuleSpec[uint64, ruleUnit]{
		OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent, Semantic: coldKey(base + 31), Output: foreignFactor.Output(), Inputs: 0,
		Admission: testTrustedTheorem[uint64](base + 32),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(29)) })
		},
	}, func(rule *Rule[uint64, ruleUnit]) bool {
		var declared bool
		foreignRuleWrite, declared = WriteTo(rule, foreignWrite)
		return declared
	})
	_, foreignQueryOK := DeclareQuery(foreignComposition, QuerySpec[uint64]{
		Semantic: coldKey(base + 33),
		Project:  func(Observation) uint64 { return 0 },
		Result:   frozenColdResult(coldKey(base + 34)),
	}, func(query *Query[uint64]) bool {
		_, declared := QueryReadFrom(query, foreignRead)
		return declared
	})
	if foreignFactor == nil || !foreignReadOK || !foreignWriteOK || !foreignRuleOK || !foreignQueryOK || !foreignComposition.Seal() || foreignRuleWrite.rule == nil {
		t.Fatal("derivation foreign summary factor")
	}
	foreignRef, foreignRefOK := foreignFactor.Ref(0)
	foreign = foreignFactor.NewClosedRefs()
	if !foreignRefOK || foreign == nil || !foreign.Append(foreignRef) || !foreign.Close() {
		t.Fatal("derivation foreign summary refs")
	}
	seedInstance, seedInstanceOK := NewRuleInstance(seed, ruleUnitForSemantic(coldKey(base+25)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceWrite(binding, seedWrites[0], leftRef) && InstanceWrite(binding, seedWrites[1], rightRef)
	})
	projectInstance, projectInstanceOK := NewRuleInstance(project, ruleUnitForSemantic(coldKey(base+26)), func(binding *RuleBinding[uint64, ruleUnit]) bool {
		return InstanceSummaryRead(binding, summaryRead, summaryForm, expected) && InstanceWrite(binding, projectWrite, sinkRef)
	})
	if !seedInstanceOK || !projectInstanceOK {
		t.Fatal("derivation summary instances")
	}
	graph := newDerivationReadMatchBatch(t, base+20, seedInstance, projectInstance)
	var queryInstance *QueryInstance[uint64]
	solver, assembled := assemble(composition, graph.batch, func(assembly *Assembly) bool {
		sourcePoint, rulePoint := admitPoint(assembly, graph.sourceSite), admitPoint(assembly, graph.ruleSite)
		sourceMember := admitInstance(assembly, sourcePoint, graph.source, graph.sourceValue, seedInstance)
		projectMember := admitInstance(assembly, rulePoint, graph.rule, graph.ruleValue, projectInstance)
		sourceGroup, projectGroup := admitGroup(assembly, sourcePoint, sourceMember), admitGroup(assembly, rulePoint, projectMember)
		var queryInstanceOK bool
		queryInstance, queryInstanceOK = NewQueryInstance(query, func(binding *QueryBinding[uint64]) bool {
			return InstanceQueryRead(binding, queryRead, sinkRef)
		})
		return sourcePoint != nil && rulePoint != nil && seedInstanceOK && projectInstanceOK && sourceMember != nil && projectMember != nil &&
			sourceGroup != nil && projectGroup != nil && admitBoundary(assembly, projectGroup, graph.boundary) && queryInstanceOK && admitQueryAt(assembly, rulePoint, queryInstance) != nil
	})
	if !assembled || solver == nil {
		t.Fatal("derivation summary assembly")
	}
	state, status := solver.Solve(context.Background())
	receipt, receiptOK := queryInstance.Receipt()
	result, readable := QueryResult(receipt, state)
	if status != SolveComplete || state == nil || !receiptOK || !readable || result != 23 || checks != 1 {
		t.Fatalf("derivation summary solve = state:%v status:%v result:%d readable:%t checks:%d", state, status, result, readable, checks)
	}
}

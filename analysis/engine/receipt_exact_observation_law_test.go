package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type exactObservationWriteFixture struct {
	count        int
	surface      equation.Surface
	route        uint64
	candidates   int
	dependencies int
	relations    int
}

func hotExactObservationFactorSpec() HotFactorSpec[uint64, uint64] {
	spec := hotUintFactorSpec()
	// The law observes a graph-owned coordinate other than Local 1. Keep the
	// factor bound exact to the largest hostile fixture coordinate (Local 9).
	spec.KeyEnd = 9
	return spec
}

func (fixture exactObservationWriteFixture) WriteCount() int { return fixture.count }
func (fixture exactObservationWriteFixture) WriteAt(index int) (equation.Surface, bool) {
	return fixture.surface, index == 0
}
func (fixture exactObservationWriteFixture) WriteRouteRead(index int) (uint64, bool) {
	return fixture.route, index == 0
}
func (fixture exactObservationWriteFixture) WriteCandidateCount(index int) (int, bool) {
	return fixture.candidates, index == 0
}
func (fixture exactObservationWriteFixture) WriteDependencyCount(index int) (int, bool) {
	return fixture.dependencies, index == 0
}
func (fixture exactObservationWriteFixture) WriteRelationCount(index int) (int, bool) {
	return fixture.relations, index == 0
}

func TestExactObservationDerivesCommittedExactWriteCoordinate(t *testing.T) {
	factor := compositionKeyOf(coldKey(948_010))
	other := compositionKeyOf(coldKey(948_011))
	write := equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}
	read, readOK := exactObservationReadSurface(exactObservationWriteFixture{count: 1, surface: write}, factor)
	if !readOK || read.Factor != factor || read.Form != equation.SurfaceReadExact || read.Local != write.Local || read.Mode != equation.TargetModeNone || read.Semantic.Available() || read.Normalizer.Available() {
		t.Fatal("exact observation did not derive its read from the committed exact write")
	}
	for name, fixture := range map[string]exactObservationWriteFixture{
		"same-point other member factor": {count: 1, surface: equation.Surface{Factor: other, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}},
		"multiple writes":                {count: 2, surface: write},
		"non-exact write":                {count: 1, surface: equation.Surface{Factor: factor, Form: equation.SurfaceWriteSelect, Local: 7}},
		"weak exact write":               {count: 1, surface: equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeWeak}},
		"zero coordinate":                {count: 1, surface: equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Mode: equation.TargetModeStrong}},
	} {
		if read, accepted := exactObservationReadSurface(fixture, factor); accepted || read.Available() {
			t.Fatalf("exact observation accepted %s", name)
		}
	}
}

func TestRuleExactObservationReadsCommittedMemberAndRejectsForeignState(t *testing.T) {
	fixture := newExactRuleObservationFixture(t, hotExactQuerySpec())
	if _, attached := AttachRuleExactObservation(fixture.compilation, fixture.implementation, identity.ContentID{}, fixture.member); attached {
		t.Fatal("exact observation accepted unavailable ID")
	}
	observation, attached := AttachRuleExactObservation(fixture.compilation, fixture.implementation, receiptAssemblySemanticID(92), fixture.member)
	if !attached || !observation.Available() {
		t.Fatal("exact observation attachment")
	}
	if _, attached := AttachRuleExactObservation(fixture.compilation, fixture.implementation, receiptAssemblySemanticID(92), fixture.member); attached {
		t.Fatal("exact observation accepted duplicate ID")
	}
	points, pointsOK := indexReceiptObservationPoints(fixture.member.graph.graph)
	memberPoint, memberPointOK := points[fixture.member.member.Key()]
	otherPoint, otherPointOK := points[fixture.otherMember.member.Key()]
	if !pointsOK || !memberPointOK || !otherPointOK || memberPoint.Key() != otherPoint.Key() {
		t.Fatal("exact observation fixture did not retain two members at one output point")
	}
	if _, failure := AttachRuleExactObservationWithFailure(fixture.compilation, fixture.implementation, receiptAssemblySemanticID(95), fixture.otherMember); failure != ReceiptObservationAttachFailureMapping {
		t.Fatalf("exact observation accepted same-point foreign-factor member failure=%v", failure)
	}
	foreignFixture := newExactRuleObservationFixture(t, hotExactQuerySpec())
	if _, attached := AttachRuleExactObservation(fixture.compilation, fixture.implementation, receiptAssemblySemanticID(93), foreignFixture.member); attached {
		t.Fatal("exact observation accepted foreign Rule member")
	}
	if _, attached := AttachRuleExactObservation(fixture.compilation, foreignFixture.implementation, receiptAssemblySemanticID(93), fixture.member); attached {
		t.Fatal("exact observation accepted foreign exact implementation")
	}
	solver, solverOK := fixture.compilation.Solver()
	if !solverOK || solver == nil {
		t.Fatal("exact observation solver")
	}
	state, status, report := solver.SolveWithReport(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("exact observation solve = status:%v state:%t report:%t reason:%v phase:%v point:%v group:%v member:%v rule:%v", status, state != nil, report.Available(), report.Reason(), report.Phase(), report.Point(), report.Group(), report.Member(), report.Rule())
	}
	value, readable := ReceiptObservationResult[uint64](observation, solver, state)
	if !readable || value != 1 {
		t.Fatalf("exact observation = value:%d readable:%t", value, readable)
	}
	write, writeOK := fixture.member.member.WriteAt(0)
	if !writeOK || write.Local != 7 {
		t.Fatal("exact observation fixture lost its nonzero committed coordinate")
	}
	foreignObservation, foreignAttached := AttachRuleExactObservation(foreignFixture.compilation, foreignFixture.implementation, receiptAssemblySemanticID(92), foreignFixture.member)
	foreign, foreignSolverOK := foreignFixture.compilation.Solver()
	if !foreignAttached || !foreignObservation.Available() || !foreignSolverOK || foreign == nil {
		t.Fatal("foreign exact observation fixture")
	}
	foreignState, foreignStatus := foreign.Solve(context.Background())
	if foreignStatus != SolveComplete || foreignState == nil {
		t.Fatalf("foreign exact observation solve = status:%v state:%t", foreignStatus, foreignState != nil)
	}
	if _, readable := ReceiptObservationResult[uint64](observation, foreign, state); readable {
		t.Fatal("exact observation read through a foreign solver")
	}
	if _, readable := ReceiptObservationResult[uint64](observation, solver, foreignState); readable {
		t.Fatal("exact observation read a foreign completed state")
	}
	if _, readable := ReceiptObservationResult[uint64](observation, foreign, foreignState); readable {
		t.Fatal("exact observation read a foreign completed solver state")
	}
}

func TestReceiptObservationTopologyNeedsOwnedObservationBeforeSolver(t *testing.T) {
	fixture := newExactRuleObservationFixture(t, hotExactQuerySpec())
	if solver, assembled := fixture.compilation.Solver(); assembled || solver != nil {
		t.Fatal("observation topology assembled without an owned observation root")
	}
}

func TestReceiptAssemblyCommitDoesNotImplicitlyDeferQueryFamilies(t *testing.T) {
	if _, committed := buildExactRuleObservationFixture(t, hotExactQuerySpec(), false); committed {
		t.Fatal("ordinary receipt commit deferred a declared query family")
	}
}

// hotMutableExactQuerySpec is the exact-query spec of a result type whose
// value is a mutable backing store. Its freezer copies, so materialization
// detaches the projector's scratch, and its Clone copies, so an explicit
// detachment hands the caller an owned value.
func hotMutableExactQuerySpec(project func(OrderedCells[uint64]) []uint64) HotExactQuerySpec[uint64, []uint64] {
	clone := func(value []uint64) []uint64 { return append([]uint64(nil), value...) }
	return HotExactQuerySpec[uint64, []uint64]{
		Project: project,
		Result: FrozenResult[[]uint64]{
			Semantic: coldKey(949_902), Freeze: clone, Clone: clone,
			Equal: func(left, right []uint64) bool {
				if len(left) != len(right) {
					return false
				}
				for index := range left {
					if left[index] != right[index] {
						return false
					}
				}
				return true
			},
			Fingerprint: func(value []uint64) uint64 {
				var fingerprint uint64
				for index, item := range value {
					fingerprint ^= uint64(index+1)*0x9e3779b97f4a7c15 ^ item
				}
				return fingerprint
			},
		},
	}
}

// TestRuleExactObservationFreezesOnceAndDetachesOnDemand fixes the borrow
// contract for a mutable result type. Materialization freezes the projector's
// backing store, so a later mutation of that scratch is invisible; every
// borrowed read then returns that one published value without copying it; and a
// caller that needs an owned copy asks for one explicitly and mutates it
// without reaching the published result.
func TestRuleExactObservationFreezesOnceAndDetachesOnDemand(t *testing.T) {
	scratch := []uint64{7, 11}
	fixture := newExactRuleObservationFixture(t, hotMutableExactQuerySpec(func(_ OrderedCells[uint64]) []uint64 { return scratch }))
	observation, attached := AttachRuleExactObservation(fixture.compilation, fixture.implementation, receiptAssemblySemanticID(94), fixture.member)
	if !attached || !observation.Available() {
		t.Fatal("mutable exact observation attachment")
	}
	solver, solverOK := fixture.compilation.Solver()
	if !solverOK || solver == nil {
		t.Fatal("mutable exact observation solver")
	}
	state, status := solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("mutable exact observation solve = status:%v state:%t", status, state != nil)
	}
	// The projector deliberately returns its mutable scratch backing store. A
	// materialized observation must retain the freezer's detached copy.
	scratch[0] = 99
	first, firstReadable := ReceiptObservationResult[[]uint64](observation, solver, state)
	if !firstReadable || len(first) != 2 || first[0] != 7 || first[1] != 11 {
		t.Fatalf("first frozen observation read = %#v/%t", first, firstReadable)
	}
	second, secondReadable := ReceiptObservationResult[[]uint64](observation, solver, state)
	if !secondReadable || len(second) != 2 || &second[0] != &first[0] {
		t.Fatalf("a borrowed read copied the published result = %#v/%t", second, secondReadable)
	}
	detached, detachedReadable := DetachReceiptObservationResult[[]uint64](observation, solver, state)
	if !detachedReadable || len(detached) != 2 || detached[0] != 7 || detached[1] != 11 || &detached[0] == &first[0] {
		t.Fatalf("detached observation read = %#v/%t", detached, detachedReadable)
	}
	detached[0], detached[1] = 101, 103
	third, thirdReadable := ReceiptObservationResult[[]uint64](observation, solver, state)
	if !thirdReadable || len(third) != 2 || third[0] != 7 || third[1] != 11 {
		t.Fatalf("a detached copy reached the published result = %#v/%t", third, thirdReadable)
	}
}

// newExactRuleObservationFixture intentionally has no graph query row: the
// attached observation itself supplies the exact committed-member demand root.
// This keeps the law focused on observation ownership rather than ordinary
// query execution.
type exactRuleObservationFixture[R any] struct {
	compilation    *ReceiptCompilation
	implementation *ExactQueryImplementation[uint64, R]
	member         ReceiptRuleMember
	otherMember    ReceiptRuleMember
}

func newExactRuleObservationFixture[R any](t testing.TB, querySpec HotExactQuerySpec[uint64, R]) exactRuleObservationFixture[R] {
	t.Helper()
	fixture, committed := buildExactRuleObservationFixture(t, querySpec, true)
	if !committed {
		t.Fatal("exact observation commit")
	}
	return fixture
}

func buildExactRuleObservationFixture[R any](t testing.TB, querySpec HotExactQuerySpec[uint64, R], deferredQueries bool) (exactRuleObservationFixture[R], bool) {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(948_001))
	writeForm, writeFormOK := factor.ExactWrite()
	readForm, readOK := factor.ExactRead()
	otherFactor, otherFactorOK := DeclareFactorSlot[uint64](builder, coldKey(948_011))
	otherWriteForm, otherWriteFormOK := otherFactor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_031), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(948_032)}, Output: factor.Ref(),
	})
	write, writeOK := SchemaWrite(rule, writeForm)
	otherRule, otherRuleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(948_033), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(948_034)}, Output: otherFactor.Ref(),
	})
	otherWrite, otherWriteOK := SchemaWrite(otherRule, otherWriteForm)
	query, queryOK := DeclareQuerySlot[R](builder, SchemaQuerySpec{Semantic: coldKey(948_002), Freezer: querySpec.Result.Semantic})
	queryReadOK := SchemaQueryRead(query, readForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeFormOK || !readOK || !otherFactorOK || !otherWriteFormOK || !ruleOK || !writeOK || !otherRuleOK || !otherWriteOK || !queryOK || !queryReadOK || !schemaOK || schema == nil {
		t.Fatal("exact observation schema")
	}
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_032)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	otherRuleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(948_034)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(9)) })
		},
	}
	if binding == nil || !BindFactor(binding, factor, hotExactObservationFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, ruleSpec) ||
		!BindFactor(binding, otherFactor, hotExactObservationFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, otherRule, otherWrite, otherFactor, otherRuleSpec) ||
		!BindExactQuery(binding, query, factor, querySpec) || !binding.Seal() {
		t.Fatal("exact observation binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	otherImplementation, otherImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, otherRule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, R](binding, query)
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !implementationOK || implementation == nil || !otherImplementationOK || otherImplementation == nil || !queryImplementationOK || queryImplementation == nil || !assemblyOK || assembly == nil {
		t.Fatal("exact observation assembly")
	}

	proof := implementation.receipt.proof
	otherProof := otherImplementation.receipt.proof
	if proof == nil || otherProof == nil {
		t.Fatal("exact observation rule proof")
	}
	site, siteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(949_900)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := assembly.builder.admitAt(site)
	operandValue := ruleUnitForSemantic(coldKey(949_901))
	entity, entityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
	otherOccurrence, otherOccurrenceOK := assembly.builder.admitAt(site)
	otherOperandValue := ruleUnitForSemantic(coldKey(949_903))
	otherEntity, otherEntityOK := operandEntityForContent(otherOperandValue.content)
	otherOperand, otherOperandOK := assembly.builder.admitOperand(otherOccurrence, otherEntity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || !otherOccurrenceOK || !otherEntityOK || !otherOperandOK || !assembly.SealSources() {
		t.Fatal("exact observation source")
	}
	point, pointOK := assembly.builder.issuePointRow(equation.PointSpec{Site: site})
	_, pointSemanticOK := assembly.builder.addSemanticPoint(receiptAssemblySemanticID(90), point)
	source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}}},
	})
	draft, draftOK := implementation.BeginBindingRuleRow(source)
	part, partOK := implementation.WritePart(source, 0)
	if !pointOK || !pointSemanticOK || !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
		t.Fatal("exact observation rule row")
	}
	ruleRow, ruleRowOK := assembly.builder.issueRuleRow(draft)
	_, ruleSemanticOK := assembly.builder.addSemanticRule(receiptAssemblySemanticID(91), ruleRow)
	otherSource, otherSourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: otherProof.semantic, OperandFamily: otherProof.operandFamily, Occurrence: otherOccurrence, Operand: otherOperand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: otherProof.output, Form: equation.SurfaceWriteExact, Local: 9, Mode: equation.TargetModeStrong}}},
	})
	otherDraft, otherDraftOK := otherImplementation.BeginBindingRuleRow(otherSource)
	otherPart, otherPartOK := otherImplementation.WritePart(otherSource, 0)
	if !otherSourceOK || !otherDraftOK || !otherPartOK || !otherDraft.AddWrite(otherPart) {
		t.Fatal("exact observation foreign-factor rule row")
	}
	otherRuleRow, otherRuleRowOK := assembly.builder.issueRuleRow(otherDraft)
	_, otherRuleSemanticOK := assembly.builder.addSemanticRule(receiptAssemblySemanticID(96), otherRuleRow)
	if !ruleRowOK || !ruleSemanticOK || !otherRuleRowOK || !otherRuleSemanticOK {
		t.Fatal("exact observation topology")
	}
	var graph *ReceiptGraph
	var committed bool
	if deferredQueries {
		_, graph, committed = assembly.CommitObservationTopology()
	} else {
		_, graph, committed = assembly.Commit()
	}
	if !committed || graph == nil {
		if deferredQueries {
			failure, failureAvailable := assembly.CommitFailure()
			topologyFailure, topologyAvailable := failure.Topology()
			precondition, preconditionAvailable := failure.Precondition()
			t.Fatalf("exact observation deferred commit committed=%t graph=%t failure=%t phase=%v topology=%t/%v precondition=%t/%v", committed, graph != nil, failureAvailable, failure.Phase(), topologyAvailable, topologyFailure, preconditionAvailable, precondition)
		}
		return exactRuleObservationFixture[R]{}, false
	}
	member, memberOK := graph.RuleMember(receiptAssemblySemanticID(91))
	otherMember, otherMemberOK := graph.RuleMember(receiptAssemblySemanticID(96))
	compilation, compilationOK := BeginReceiptCompilation(implementation, graph)
	if !memberOK || !otherMemberOK || !compilationOK || compilation == nil {
		t.Fatal("exact observation receipt compilation")
	}
	if _, attached := AttachReceiptRuleMember(compilation, implementation, member, operandValue); !attached {
		t.Fatal("exact observation member attachment")
	}
	if _, attached := AttachReceiptRuleMember(compilation, otherImplementation, otherMember, otherOperandValue); !attached {
		t.Fatal("exact observation same-point member attachment")
	}
	return exactRuleObservationFixture[R]{compilation: compilation, implementation: queryImplementation, member: member, otherMember: otherMember}, true
}

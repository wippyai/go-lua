package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	planAxisKey      schema.Key = "axis/plan-owner"
	planOtherAxisKey schema.Key = "axis/plan-other"

	planCandidateRelation schema.Key     = "relation/plan-candidate"
	planJoinRelation      schema.Key     = "relation/plan-join"
	planJoinKey           schema.Key     = "projection/plan-join-key"
	planJoinPredicate     schema.Key     = "projection/plan-join-predicate"
	planDestination       schema.Key     = "projection/plan-destination"
	planReducer           schema.Key     = "reducer/plan"
	planCarryTransform    schema.Key     = "transform/plan-carry"
	planOutput            schema.Key     = "output/plan"
	planKeyCarrier        member.Carrier = "carrier/plan/key"
	planFactCarrier       member.Carrier = "carrier/plan/fact"
	planCandidateCarrier  member.Carrier = "carrier/plan/candidate"

	planDenominator      schema.Key = "coordinates/axis/plan-owner"
	planOtherDenominator schema.Key = "coordinates/axis/plan-other"

	planRuleKey   schema.Key = "rule/plan"
	planAbsentKey schema.Key = "rule/plan-absent"
)

type planFixture struct {
	catalog        member.Catalog
	otherCatalog   member.Catalog
	mainSignature  axis.Signature
	otherSignature axis.Signature
	declaration    program.Program
	outputWriter   schema.Key
	// issuance stands in for the sealed issuance surface. A Program whose
	// candidate is an issued row names a relation there, and the seal resolves
	// that reference like any other.
	issuance []schema.Entry
	// expectRefusal turns the fixture's own seal verdict into data for the one
	// law that is about that verdict, and refusal records it.
	expectRefusal bool
	refusal       schema.SealFailure
}

// planNoopSurface is only the wiring needed to make the test schema complete.
// The compiler laws below exercise the real axis, rule, and denominator
// surfaces; unrelated surface laws are deliberately not part of this fixture.
type planNoopSurface struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
}

func (surface planNoopSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface planNoopSurface) Entries() []schema.Entry {
	return append([]schema.Entry(nil), surface.entries...)
}

func (planNoopSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func newPlanFixture(t *testing.T) *planFixture {
	t.Helper()
	mainAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey}
	declaration := program.Program{
		OperandRole: vocabulary.RoleKey("plan/operand"),
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}),
		Joins: []program.JoinDecl{{
			Sources:  []program.SourceRef{program.CandidateSource()},
			Relation: member.RelationRef{Axis: mainAxis, Member: planJoinRelation},
			Key:      member.ProjectionRef{Axis: mainAxis, Member: planJoinKey},
			Read: program.ReadDecl{
				PointBound: program.PointBound,
				Axis:       program.AxisRef(mainAxis),
				Form:       program.Exact,
				Contract: program.ReadContract{
					Order:        program.OrderCanonical,
					Sparse:       program.SparseExplicit,
					OnOpaque:     program.OnOpaqueRefuse,
					Multiplicity: program.MultiplicityOne,
					DenominatorRef: program.DenominatorRef{
						Surface: schema.SurfaceKindDenominator,
						Key:     planDenominator,
					},
				},
			},
		}},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: mainAxis, Member: planReducer},
			Inputs:  []program.JoinRef{0},
			Outputs: []program.OutputDecl{{
				Column: axis.OutputRef{Axis: mainAxis, Key: planOutput},
				Destination: member.ProjectionRef{
					Axis:   mainAxis,
					Member: planDestination,
				},
				Mode:      program.ModeExact,
				ValueSlot: 0,
			}},
		},
	}
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("fixture program rejected before sealing: %+v", problem)
	}

	catalog, ok := member.NewCatalog(
		[]member.Relation{
			{
				Key:               planCandidateRelation,
				Subject:           planCandidateCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}),
			},
			{
				Key:               planJoinRelation,
				Subject:           planFactCarrier,
				Inputs:            []member.Carrier{planCandidateCarrier},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}),
			},
		},
		[]member.Projection{
			{
				Key:               planJoinKey,
				Relation:          planJoinRelation,
				Role:              member.Key,
				Result:            planKeyCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}),
			},
			{
				Key:               planDestination,
				Relation:          planCandidateRelation,
				Role:              member.Destination,
				Result:            planKeyCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}),
			},
			{
				Key:               planJoinPredicate,
				Relation:          planJoinRelation,
				Role:              member.Predicate,
				Result:            planKeyCarrier,
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: mainAxis, Member: planCandidateRelation}),
			},
		},
		[]member.Reducer{{
			Key: planReducer,
			Inputs: []member.ReducerInput{{
				Axis: mainAxis, Carrier: planFactCarrier,
				Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []member.ReducerOutput{{Axis: mainAxis, Carrier: planFactCarrier}},
		}},
		nil,
	)
	if !ok {
		t.Fatal("fixture member catalog rejected")
	}
	return &planFixture{catalog: catalog, mainSignature: axis.Signature{Key: planKeyCarrier, Fact: planFactCarrier}, declaration: declaration, outputWriter: planAxisKey}
}

func (fixture *planFixture) seal(t *testing.T) *seal.Schema {
	return fixture.sealOrder(t, false)
}

// sealFailure is the same fixture read for the declaration table's own verdict
// rather than for the table. A law about an unresolvable reference cannot use
// seal, which fatals on exactly that verdict.
func (fixture *planFixture) sealFailure(t *testing.T) schema.SealFailure {
	t.Helper()
	fixture.expectRefusal = true
	fixture.sealOrder(t, false)
	return fixture.refusal
}

func (fixture *planFixture) sealOrder(t *testing.T, reverseAxis bool) *seal.Schema {
	t.Helper()
	mainUniverse, ok := identity.DeriveContentID("go-lua/plan-law/main", []byte(planAxisKey))
	if !ok {
		t.Fatal("main denominator universe identity unavailable")
	}
	otherUniverse, ok := identity.DeriveContentID("go-lua/plan-law/other", []byte(planOtherAxisKey))
	if !ok {
		t.Fatal("other denominator universe identity unavailable")
	}
	mainDenominator, ok := denominator.Coordinate(planAxisKey, mainUniverse)
	if !ok {
		t.Fatal("main denominator rejected")
	}
	otherDenominator, ok := denominator.Coordinate(planOtherAxisKey, otherUniverse)
	if !ok {
		t.Fatal("other denominator rejected")
	}

	mainAxis, ok := axis.New(axis.Spec[struct{}]{
		Key:         planAxisKey,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame: axis.Frame{Outputs: []axis.Output{{
			Key:    planOutput,
			Writer: fixture.outputWriter,
		}}},
		Catalog:   fixture.catalog,
		Signature: fixture.mainSignature,
		Semantic:  vocabulary.RoleKey("plan/axis-owner"),
	})
	if !ok {
		t.Fatal("main axis rejected")
	}
	otherAxis, ok := axis.New(axis.Spec[struct{}]{
		Key:         planOtherAxisKey,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Catalog:     fixture.otherCatalog,
		Signature:   fixture.otherSignature,
		Semantic:    vocabulary.RoleKey("plan/axis-other"),
	})
	if !ok {
		t.Fatal("other axis rejected")
	}

	programRule, ok := rule.New(rule.Spec{
		Key:      planRuleKey,
		Lane:     rule.LaneLink,
		Writes:   planAxisKey,
		Owner:    planAxisKey,
		Semantic: vocabulary.RoleKey("plan/rule"),
		// The activation family is declared where every other role a Program
		// names is: on the rule that names it.
		Roles: func() []schema.Key {
			roles := []schema.Key{vocabulary.RoleKey("plan/operand")}
			if fixture.declaration.ActivationRole.Available() {
				roles = append(roles, fixture.declaration.ActivationRole)
			}
			return roles
		}(),
		Program: fixture.declaration,
	})
	if !ok {
		t.Fatal("program rule rejected")
	}
	absentRule, ok := rule.New(rule.Spec{
		Key:      planAbsentKey,
		Lane:     rule.LaneLink,
		Writes:   planAxisKey,
		Owner:    planAxisKey,
		Semantic: vocabulary.RoleKey("plan/absent"),
	})
	if !ok {
		t.Fatal("absent rule rejected")
	}

	roles := make([]schema.Entry, 0, 4)
	for ordinal, role := range []string{
		"plan/axis-owner",
		"plan/axis-other",
		"plan/rule",
		"plan/operand",
		"plan/absent",
		"plan/activation-family",
	} {
		entry, entryOK := structure.New(structure.Spec{
			Key:      vocabulary.RoleKey(role),
			Category: structure.CategorySemanticRole,
			Ordinal:  uint16(ordinal + 1),
			Spelling: role,
			Accepted: true,
		})
		if !entryOK {
			t.Fatalf("semantic role %q rejected", role)
		}
		roles = append(roles, entry)
	}

	builder := seal.NewBuilder()
	if !builder.Register(planNoopSurface{
		kind:    schema.SurfaceKindStructure,
		entries: roles,
	}) {
		t.Fatal("structure stand-in registration failed")
	}
	axes := []*axis.Template[struct{}]{mainAxis, otherAxis}
	if reverseAxis {
		axes[0], axes[1] = axes[1], axes[0]
	}
	if !builder.Register(axis.NewSurface(axes)) {
		t.Fatal("axis surface registration failed")
	}
	if !builder.Register(planNoopSurface{kind: schema.SurfaceKindIssuance, entries: fixture.issuance}) {
		t.Fatal("issuance stand-in registration failed")
	}
	if !builder.Register(rule.NewSurface([]*rule.Template{programRule, absentRule})) {
		t.Fatal("rule surface registration failed")
	}
	for _, kind := range []schema.SurfaceKind{
		schema.SurfaceKindDiagnostic,
		schema.SurfaceKindComposite,
	} {
		if !builder.Register(planNoopSurface{kind: kind}) {
			t.Fatalf("surface %d stand-in registration failed", kind)
		}
	}
	if !builder.Register(denominator.NewSurface([]*denominator.Entry{mainDenominator, otherDenominator})) {
		t.Fatal("denominator surface registration failed")
	}
	for _, kind := range []schema.SurfaceKind{
		schema.SurfaceKindQuery,
		schema.SurfaceKindObservation,
	} {
		if !builder.Register(planNoopSurface{kind: kind}) {
			t.Fatalf("surface %d stand-in registration failed", kind)
		}
	}
	table, failure := builder.Seal()
	if fixture.expectRefusal {
		fixture.refusal = failure
		return table
	}
	if failure.Available() || table == nil {
		t.Fatalf("fixture schema rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	return table
}

func enablePlanCarry(fixture *planFixture, input program.InputRef, mode program.CarryMode) {
	carry := &program.CarryDecl{Input: input, Mode: mode}
	if mode == program.CarryTransform {
		carry.Transform = member.CarryTransformRef{
			Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey},
			Member: planCarryTransform,
		}
	}
	fixture.declaration.Carry = carry
	fixture.catalog.CarryTransforms = []member.CarryTransform{{
		Key:       planCarryTransform,
		Candidate: planCandidateCarrier,
		Input:     planFactCarrier,
		Output:    planFactCarrier,
	}}
}

func TestCompileLowersExactJoinToDenseOwnerQualifiedPlan(t *testing.T) {
	fixture := newPlanFixture(t)
	table := fixture.seal(t)
	compiled, failure := Compile(table)
	if failure.Available() {
		t.Fatalf("valid sealed program rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	if !compiled.Available() || compiled.Digest() != table.Digest() || compiled.Count() != 2 {
		t.Fatalf("compiled catalog availability/digest/count = %t/%t/%d", compiled.Available(), compiled.Digest() == table.Digest(), compiled.Count())
	}

	programPlan, ok := compiled.At(0)
	if !ok || !programPlan.Present() || programPlan.Available() != programPlan.Present() || programPlan.Rule() != 0 {
		t.Fatalf("present plan = %+v/%t", programPlan, ok)
	}
	roles, rolesOK := compileRoles(table)
	if !rolesOK {
		t.Fatal("sealed semantic roles unavailable")
	}
	ruleSemantic, ruleSemanticOK := roles.Key(vocabulary.RoleKey("plan/rule"))
	operandSemantic, operandSemanticOK := roles.Key(vocabulary.RoleKey("plan/operand"))
	if !ruleSemanticOK || !operandSemanticOK || programPlan.Semantic() != ruleSemantic || programPlan.OperandFamily() != operandSemantic {
		t.Fatalf("resolved semantics = %v/%v, want %v/%v", programPlan.Semantic(), programPlan.OperandFamily(), ruleSemantic, operandSemantic)
	}
	if got, want := programPlan.Candidate(), (RelationAddr{Axis: 0, Member: 0}); got != want {
		t.Fatalf("candidate address = %+v, want %+v", got, want)
	}
	if got, want := programPlan.InputCount(), 1; got != want {
		t.Fatalf("input count=%d, want %d", got, want)
	}
	if got, want := programPlan.Reducer(), (ReducerAddr{Axis: 0, Member: 0}); got != want {
		t.Fatalf("reducer address = %+v, want %+v", got, want)
	}
	if got, want := programPlan.Scratch(), (ScratchShape{SourceCount: 1, JoinCount: 1, FoldInputCount: 1, OutputCount: 1}); got != want {
		t.Fatalf("scratch shape = %+v, want %+v", got, want)
	}

	source, ok := programPlan.SourceAt(0)
	if !ok || source != (Source{Position: 0, Candidate: true}) {
		t.Fatalf("source = %+v/%t", source, ok)
	}
	join, ok := programPlan.JoinAt(0)
	if !ok {
		t.Fatal("compiled join missing")
	}
	if join.Sources != (Span{Start: 0, Count: 1}) ||
		join.Relation != (RelationAddr{Axis: 0, Member: 1}) ||
		join.Key != (ProjectionAddr{Axis: 0, Member: 0}) ||
		join.PredicatePresent || join.ReadAxis != 0 || join.ReadForm != program.Exact ||
		join.ReadContract != (ReadContract{
			Order:        program.OrderCanonical,
			Sparse:       program.SparseExplicit,
			OnOpaque:     program.OnOpaqueRefuse,
			Multiplicity: program.MultiplicityOne,
		}) || join.Denominator != (DenominatorAddr{Ordinal: 0, Present: true}) {
		t.Fatalf("compiled exact join = %+v", join)
	}
	foldInput, ok := programPlan.FoldInputAt(0)
	if !ok || foldInput != 0 {
		t.Fatalf("fold input = %d/%t", foldInput, ok)
	}
	output, ok := programPlan.OutputAt(0)
	if !ok {
		t.Fatal("compiled output missing")
	}
	if output != (Output{
		Address:     OutputAddr{Axis: 0, Frame: 0},
		Destination: ProjectionAddr{Axis: 0, Member: 1},
		Mode:        program.ModeExact,
		Slot:        0,
	}) {
		t.Fatalf("compiled output = %+v", output)
	}

	absent, ok := compiled.At(1)
	if !ok || absent.Present() || absent.Available() || absent.Rule() != 1 || absent.SourceCount() != 0 || absent.JoinCount() != 0 || absent.FoldInputCount() != 0 || absent.OutputCount() != 0 {
		t.Fatalf("explicit absent plan = %+v/%t", absent, ok)
	}
	if _, ok := compiled.At(2); ok {
		t.Fatal("out-of-range plan unexpectedly available")
	}
}

func TestCompilePublishesCanonicalAxisSemanticDirectory(t *testing.T) {
	fixture := newPlanFixture(t)
	table := fixture.seal(t)
	compiled, failure := Compile(table)
	if failure.Available() || !compiled.Available() {
		t.Fatalf("valid schema rejected: catalog=%#v failure=%+v", compiled, failure)
	}
	axisView, axisOK := table.Surface(schema.SurfaceKindAxis)
	structureView, structureOK := table.Surface(schema.SurfaceKindStructure)
	if !axisOK || !structureOK {
		t.Fatal("sealed fixture omitted axis or structure view")
	}
	entries := make([]*structure.Entry, 0, structureView.Count())
	for position := 0; position < structureView.Count(); position++ {
		row, rowOK := structureView.At(position)
		entry, entryOK := row.(*structure.Entry)
		if !rowOK || !entryOK || entry == nil {
			t.Fatalf("structure row %d is not canonical", position)
		}
		entries = append(entries, entry)
	}
	roles, rolesOK := vocabulary.NewRoles(entries)
	if !rolesOK {
		t.Fatal("sealed fixture semantic-role vocabulary did not resolve")
	}
	if compiled.AxisCount() != axisView.Count() {
		t.Fatalf("axis directory count=%d, want sealed axis count %d", compiled.AxisCount(), axisView.Count())
	}
	for position := 0; position < axisView.Count(); position++ {
		row, rowOK := axisView.At(position)
		template, templateOK := row.(*axis.Template[struct{}])
		if !rowOK || !templateOK || template == nil {
			t.Fatalf("axis row %d is not the fixture template", position)
		}
		semantic, semanticOK := roles.Key(template.Semantic())
		if !semanticOK {
			t.Fatalf("axis row %d semantic role did not resolve", position)
		}
		got, gotOK := compiled.AxisAt(position)
		if !gotOK || got.Key != template.Key() || got.Semantic != semantic {
			t.Fatalf("directory row %d=%#v/%t, want key=%q semantic=%#v", position, got, gotOK, template.Key(), semantic)
		}
	}
	if _, ok := compiled.AxisAt(-1); ok {
		t.Fatal("negative axis directory access unexpectedly succeeded")
	}
	if _, ok := compiled.AxisAt(compiled.AxisCount()); ok {
		t.Fatal("out-of-range axis directory access unexpectedly succeeded")
	}
}

func TestCompileAxisDirectoryTracksSealedReorderWithoutOrdinalGuessing(t *testing.T) {
	canonicalFixture := newPlanFixture(t)
	canonicalTable := canonicalFixture.sealOrder(t, false)
	canonical, canonicalFailure := Compile(canonicalTable)
	if canonicalFailure.Available() || !canonical.Available() {
		t.Fatalf("canonical schema rejected: catalog=%#v failure=%+v", canonical, canonicalFailure)
	}
	reorderedFixture := newPlanFixture(t)
	reorderedTable := reorderedFixture.sealOrder(t, true)
	reordered, reorderedFailure := Compile(reorderedTable)
	if reorderedFailure.Available() || !reordered.Available() {
		t.Fatalf("reordered schema rejected: catalog=%#v failure=%+v", reordered, reorderedFailure)
	}
	if canonical.Digest() == reordered.Digest() {
		t.Fatal("axis reorder did not change sealed schema digest")
	}
	canonicalFirst, canonicalFirstOK := canonical.AxisAt(0)
	canonicalSecond, canonicalSecondOK := canonical.AxisAt(1)
	reorderedFirst, reorderedFirstOK := reordered.AxisAt(0)
	reorderedSecond, reorderedSecondOK := reordered.AxisAt(1)
	if !canonicalFirstOK || !canonicalSecondOK || !reorderedFirstOK || !reorderedSecondOK {
		t.Fatal("axis directory rows missing after reorder")
	}
	if canonicalFirst != reorderedSecond || canonicalSecond != reorderedFirst {
		t.Fatalf("reordered directory pairs changed: canonical=[%#v %#v] reordered=[%#v %#v]", canonicalFirst, canonicalSecond, reorderedFirst, reorderedSecond)
	}
	plan, planOK := reordered.At(0)
	if !planOK || !plan.Present() {
		t.Fatal("reordered program plan missing")
	}
	if plan.Candidate().Axis != 1 || plan.Reducer().Axis != 1 {
		t.Fatalf("reordered plan retained guessed ordinals: candidate=%#v reducer=%#v", plan.Candidate(), plan.Reducer())
	}
	join, joinOK := plan.JoinAt(0)
	if !joinOK || join.Relation.Axis != 1 || join.Key.Axis != 1 || join.ReadAxis != 1 {
		t.Fatalf("reordered join addresses=%#v", join)
	}
	output, outputOK := plan.OutputAt(0)
	if !outputOK || output.Address.Axis != 1 || output.Destination.Axis != 1 {
		t.Fatalf("reordered output addresses=%#v", output)
	}
}

func TestAxisDirectoryAccessIsDetachedValueCopy(t *testing.T) {
	fixture := newPlanFixture(t)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() || !compiled.Available() {
		t.Fatalf("valid schema rejected: catalog=%#v failure=%+v", compiled, failure)
	}
	row, rowOK := compiled.AxisAt(0)
	if !rowOK {
		t.Fatal("axis directory row missing")
	}
	original := row
	row.Key = "axis/foreign"
	row.Semantic = identity.SemanticKey{}
	again, againOK := compiled.AxisAt(0)
	if !againOK || again != original {
		t.Fatalf("mutating AxisAt result changed catalog: original=%#v after=%#v", original, again)
	}
}

func TestCompileRejectsAxisWithoutResolvedSemantic(t *testing.T) {
	role, roleOK := structure.New(structure.Spec{
		Key:      vocabulary.RoleKey("plan/known-role"),
		Category: structure.CategorySemanticRole,
		Ordinal:  1,
		Spelling: "plan/known-role",
		Accepted: true,
	})
	if !roleOK {
		t.Fatal("known semantic role rejected")
	}
	template, templateOK := axis.New(axis.Spec[struct{}]{
		Key:         "axis/unresolved-semantic",
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Semantic:    vocabulary.RoleKey("plan/missing-role"),
	})
	if !templateOK {
		t.Fatal("unresolved-semantic axis rejected before sealing")
	}
	builder := seal.NewBuilder()
	if !builder.Register(planNoopSurface{
		kind:    schema.SurfaceKindStructure,
		entries: []schema.Entry{role},
	}) {
		t.Fatal("structure fixture registration failed")
	}
	// Keep the axis surface deliberately no-op: this law is about the plan
	// compiler's independent refusal when an otherwise sealed view cannot
	// resolve Template.Semantic through the structural authority.
	if !builder.Register(planNoopSurface{
		kind:    schema.SurfaceKindAxis,
		entries: []schema.Entry{template},
	}) {
		t.Fatal("axis fixture registration failed")
	}
	for _, kind := range []schema.SurfaceKind{
		schema.SurfaceKindIssuance,
		schema.SurfaceKindRule,
		schema.SurfaceKindDiagnostic,
		schema.SurfaceKindComposite,
	} {
		if !builder.Register(planNoopSurface{kind: kind}) {
			t.Fatalf("surface %d fixture registration failed", kind)
		}
	}
	if !builder.Register(denominator.NewSurface(nil)) {
		t.Fatal("denominator fixture registration failed")
	}
	for _, kind := range []schema.SurfaceKind{
		schema.SurfaceKindQuery,
		schema.SurfaceKindObservation,
	} {
		if !builder.Register(planNoopSurface{kind: kind}) {
			t.Fatalf("surface %d fixture registration failed", kind)
		}
	}
	table, sealFailure := builder.Seal()
	if sealFailure.Available() || table == nil || !table.Available() {
		t.Fatalf("compiler-refusal fixture did not seal: table=%p failure=%+v", table, sealFailure)
	}
	compiled, failure := Compile(table)
	if compiled.Available() || compiled.AxisCount() != 0 || compiled.Digest().Available() {
		t.Fatalf("unresolved semantic exposed partial catalog: catalog=%#v", compiled)
	}
	if !failure.Available() || failure.Schema != table.Digest() || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved semantic failure=%+v, want digest-fenced incomplete failure", failure)
	}
}

func TestCompileAcceptsRawSetShapedNonzeroCarryInput(t *testing.T) {
	fixture := newPlanFixture(t)
	enablePlanCarry(fixture, 1, program.CarryIdentity)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("nonzero carry input rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, ok := compiled.At(0)
	if !ok || planned.InputCount() != 2 {
		t.Fatalf("compiled input count=%d/%t, want 2/true", planned.InputCount(), ok)
	}
	carry, carryOK := planned.Carry()
	if !carryOK || carry.Input != 1 || carry.Mode != program.CarryIdentity || carry.TransformPresent {
		t.Fatalf("compiled identity carry=%#v/%t", carry, carryOK)
	}
}

func TestCompileAcceptsCarryOnlyOutputPortWithoutARead(t *testing.T) {
	fixture := newPlanFixture(t)
	fixture.declaration.Joins = nil
	fixture.declaration.Fold.Inputs = nil
	fixture.catalog.Reducers[0].Inputs = nil
	enablePlanCarry(fixture, 0, program.CarryIdentity)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("carry-only output port rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, ok := compiled.At(0)
	carry, carryOK := planned.Carry()
	if !ok || planned.InputCount() != 1 || planned.JoinCount() != 0 || !carryOK || carry.Input != 0 || carry.Mode != program.CarryIdentity {
		t.Fatalf("carry-only plan=%#v input=%d/%t carry=%#v/%t", planned, planned.InputCount(), ok, carry, carryOK)
	}
}

func TestProgramRejectsCarryInputHolesAcrossReadsAndCarry(t *testing.T) {
	fixture := newPlanFixture(t)
	enablePlanCarry(fixture, 2, program.CarryIdentity)
	if problem, valid := fixture.declaration.Check(); valid || problem.Kind != program.ProblemInput {
		t.Fatalf("carry input hole valid=%v problem=%+v", valid, problem)
	}
}

func TestCompileAcceptsTypedCarryTransformAndRejectsForeignOrMismatched(t *testing.T) {
	fixture := newPlanFixture(t)
	enablePlanCarry(fixture, 0, program.CarryTransform)
	compiled, failure := Compile(fixture.seal(t))
	if failure.Available() {
		t.Fatalf("typed carry transform rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	planned, ok := compiled.At(0)
	carry, carryOK := planned.Carry()
	if !ok || !carryOK || carry.Mode != program.CarryTransform || !carry.TransformPresent || carry.Transform != (CarryTransformAddr{Axis: 0, Member: 0}) {
		t.Fatalf("compiled typed carry=%#v/%t", carry, carryOK)
	}

	foreign := newPlanFixture(t)
	enablePlanCarry(foreign, 0, program.CarryTransform)
	foreign.declaration.Carry.Transform.Axis.Key = planOtherAxisKey
	_, failure = Compile(foreign.seal(t))
	if !failure.Available() {
		t.Fatal("foreign carry transform owner accepted")
	}

	mismatched := newPlanFixture(t)
	enablePlanCarry(mismatched, 0, program.CarryTransform)
	mismatched.catalog.CarryTransforms[0].Candidate = planFactCarrier
	_, failure = Compile(mismatched.seal(t))
	if !failure.Available() {
		t.Fatal("carry transform candidate carrier mismatch accepted")
	}

	factMismatch := newPlanFixture(t)
	enablePlanCarry(factMismatch, 0, program.CarryTransform)
	factMismatch.catalog.CarryTransforms[0].Input = planKeyCarrier
	factMismatch.catalog.CarryTransforms[0].Output = planKeyCarrier
	_, failure = Compile(factMismatch.seal(t))
	if !failure.Available() {
		t.Fatal("carry transform fact carrier mismatch accepted")
	}

	missing := newPlanFixture(t)
	missing.declaration.Carry = &program.CarryDecl{
		Input: 0, Mode: program.CarryTransform,
		Transform: member.CarryTransformRef{
			Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey},
			Member: "transform/plan-missing",
		},
	}
	_, failure = Compile(missing.seal(t))
	if !failure.Available() {
		t.Fatal("missing carry transform member accepted")
	}
}

func TestCompileRetainsExactTransformIdentityForAllFourRoles(t *testing.T) {
	roles := []schema.Key{
		"transform/value/allocation",
		"transform/value/callresult-freshresult",
		"transform/heap/allocation-empty",
		"transform/heap/allocation-closed",
	}
	for _, role := range roles {
		t.Run(string(role), func(t *testing.T) {
			fixture := newPlanFixture(t)
			fixture.declaration.Carry = &program.CarryDecl{
				Input: 0, Mode: program.CarryTransform,
				Transform: member.CarryTransformRef{
					Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: planAxisKey},
					Member: role,
				},
			}
			fixture.catalog.CarryTransforms = []member.CarryTransform{{
				Key: role, Candidate: planCandidateCarrier,
				Input: planFactCarrier, Output: planFactCarrier,
			}}
			compiled, failure := Compile(fixture.seal(t))
			if failure.Available() {
				t.Fatalf("transform role rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
			}
			planned, ok := compiled.At(0)
			if !ok {
				t.Fatal("compiled plan missing")
			}
			carry, carryOK := planned.Carry()
			if !carryOK || carry.Mode != program.CarryTransform || !carry.TransformPresent || carry.TransformKey != role || carry.TransformAxis != planAxisKey {
				t.Fatalf("transform identity=%#v/%t", carry, carryOK)
			}
		})
	}
}

func TestCompileRejectsNearestMalformedProgramDeclarations(t *testing.T) {
	tests := []struct {
		name        string
		law         schema.LawID
		disposition schema.Disposition
		mutate      func(*planFixture)
	}{
		{
			name:        "relation-input-signature-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Relations[1].Inputs = append(fixture.catalog.Relations[1].Inputs, planCandidateCarrier)
			},
		},
		{
			name:        "relation-role-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Projections[0].Role = member.Predicate
			},
		},
		{
			name:        "read-key-carrier-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Projections[0].Result = "carrier/plan/foreign-key"
			},
		},
		{
			name:        "reducer-axis-signature-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Reducers[0].Inputs[0].Axis = schema.EntryReference{
					Surface: schema.SurfaceKindAxis, Key: planOtherAxisKey,
				}
			},
		},
		{
			name:        "reducer-fact-carrier-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Reducers[0].Inputs[0].Carrier = "carrier/plan/foreign-fact"
			},
		},
		{
			name:        "reducer-read-contract-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Reducers[0].Inputs[0].Multiplicity = member.MultiplicityOptional
			},
		},
		{
			name:        "reducer-output-carrier-mismatch",
			law:         rule.LawProgramOutput,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Reducers[0].Outputs[0].Carrier = "carrier/plan/foreign-output"
			},
		},
		{
			name:        "destination-key-carrier-mismatch",
			law:         rule.LawProgramOutput,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.catalog.Projections[1].Result = "carrier/plan/foreign-destination"
			},
		},
		{
			name:        "output-column-mismatch",
			law:         rule.LawProgramOutput,
			disposition: schema.DispositionIncomplete,
			mutate: func(fixture *planFixture) {
				fixture.declaration.Fold.Outputs[0].Column.Key = "output/plan-missing"
			},
		},
		{
			name:        "output-writer-mismatch",
			law:         rule.LawProgramOutput,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.outputWriter = planOtherAxisKey
			},
		},
		{
			name:        "denominator-owner-mismatch",
			law:         rule.LawProgramShape,
			disposition: schema.DispositionMalformed,
			mutate: func(fixture *planFixture) {
				fixture.declaration.Joins[0].Read.Contract.DenominatorRef = program.DenominatorRef{
					Surface: schema.SurfaceKindDenominator,
					Key:     planOtherDenominator,
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPlanFixture(t)
			test.mutate(fixture)
			table := fixture.seal(t)
			compiled, failure := Compile(table)
			if !failure.Available() {
				t.Fatal("malformed declaration compiled without a failure")
			}
			if failure.Contributor != schema.SurfaceKindRule || failure.Law != test.law || failure.Disposition != test.disposition {
				t.Fatalf("failure = contributor %d law %d disposition %s, want rule/%d/%s", failure.Contributor, failure.Law, failure.Disposition, test.law, test.disposition)
			}
			if failure.Schema != table.Digest() {
				t.Fatal("compiler failure was not fenced by the sealed schema digest")
			}
			if compiled.Available() || compiled.Digest().Available() || compiled.Count() != 0 {
				t.Fatalf("failed compilation was not fail-closed: available=%t digest=%t count=%d", compiled.Available(), compiled.Digest().Available(), compiled.Count())
			}
		})
	}
}

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	memberrelation "github.com/wippyai/go-lua/analysis/schema/axis/member/relation"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	generatedRuleLawUnrelatedAxisKey  schema.Key = "axis/generated-rule-law-unrelated"
	generatedRuleLawUnrelatedOutput   schema.Key = "output/generated-rule-law-unrelated"
	generatedRuleLawUnrelatedAxisRole            = "generated-rule-law/unrelated-axis"
)

func generatedRuleLawUnrelatedRole(t testing.TB) schema.Key {
	t.Helper()
	axisRole := vocabulary.RoleKey(generatedRuleLawAxisRoleSpelling)
	axisSemantic, axisOK := vocabulary.Key(string(axisRole))
	if !axisOK {
		t.Fatal("generated Rule law axis semantic")
	}
	// Schema factors are canonically ordered by their resolved semantic key.
	// Pick a stable unrelated role on the later side of the axis role so the
	// builder's axis order and the sealed Factor order agree in this fixture.
	for _, spelling := range []string{
		generatedRuleLawUnrelatedAxisRole,
		"generated-rule-law/unrelated-axis-z",
		"generated-rule-law/unrelated-axis-zz",
		"generated-rule-law/unrelated-axis-later",
	} {
		role := vocabulary.RoleKey(spelling)
		semantic, semanticOK := vocabulary.Key(string(role))
		if semanticOK && identity.CompareSemanticKey(semantic, axisSemantic) > 0 {
			return role
		}
	}
	t.Fatal("generated Rule law unrelated semantic ordering")
	return ""
}

// generatedBindingLawOwner is a deliberately small owner double.  It models
// the only construction authority the generated arm is allowed to consume:
// a mount/occurrence candidate and two owner-issued projection locals.  It
// keeps no domain value and refuses every coordinate outside its table.
type generatedBindingLawOwner struct {
	relation        uint32
	candidate       uint32
	projections     map[[2]uint32]uint32
	acceptCandidate bool
	// members is the nested ordered member set one parent candidate carries,
	// keyed by (relation, parent). memberProjections is the local each member
	// row projects to, keyed by (relation, projection, member).
	members           map[[2]uint32][]uint32
	memberProjections map[[3]uint32]uint32
}

var _ memberrelation.Owner = (*generatedBindingLawOwner)(nil)

func (owner *generatedBindingLawOwner) candidateFor(relationOrdinal uint32, mount, occurrence identity.ContentID) (uint32, bool) {
	if owner == nil || !owner.acceptCandidate || relationOrdinal != owner.relation || !mount.Available() || !occurrence.Available() {
		return 0, false
	}
	return owner.candidate, true
}

func (owner *generatedBindingLawOwner) CandidateCount(relationOrdinal uint32, mount, occurrence identity.ContentID) (int, bool) {
	if _, ok := owner.candidateFor(relationOrdinal, mount, occurrence); !ok {
		return 0, false
	}
	return 1, true
}

func (owner *generatedBindingLawOwner) CandidateAt(relationOrdinal uint32, mount, occurrence identity.ContentID, index int) (uint32, bool) {
	if index != 0 {
		return 0, false
	}
	return owner.candidateFor(relationOrdinal, mount, occurrence)
}

func (owner *generatedBindingLawOwner) MemberCount(relationOrdinal, parentCandidateOrdinal uint32) (int, bool) {
	if owner == nil {
		return 0, false
	}
	members, ok := owner.members[[2]uint32{relationOrdinal, parentCandidateOrdinal}]
	if !ok {
		return 0, false
	}
	return len(members), true
}

func (owner *generatedBindingLawOwner) MemberAt(relationOrdinal, parentCandidateOrdinal uint32, ordinal int) (uint32, bool) {
	if owner == nil || ordinal < 0 {
		return 0, false
	}
	members, ok := owner.members[[2]uint32{relationOrdinal, parentCandidateOrdinal}]
	if !ok || ordinal >= len(members) {
		return 0, false
	}
	return members[ordinal], true
}

func (owner *generatedBindingLawOwner) Project(relationOrdinal, projectionOrdinal, candidateOrdinal uint32) (uint32, bool) {
	if owner == nil {
		return 0, false
	}
	if local, ok := owner.memberProjections[[3]uint32{relationOrdinal, projectionOrdinal, candidateOrdinal}]; ok {
		return local, true
	}
	if candidateOrdinal != owner.candidate {
		return 0, false
	}
	local, ok := owner.projections[[2]uint32{relationOrdinal, projectionOrdinal}]
	return local, ok
}

type generatedBindingLawFixture struct {
	fixture    generatedRuleLawFixture
	schema     *Schema
	binding    *SchemaBinding
	factors    []*FactorSlot[uint64]
	slot       *GeneratedRuleSlot
	cap        RuleSlotCapability
	owner      *generatedBindingLawOwner
	mount      identity.ContentID
	occurrence identity.ContentID
}

// generatedBindingLawSummaryForm is one axis's declared summary read form
// semantic. It is derived from the axis ordinal so each Factor publishes its
// own form rather than sharing one identity.
func generatedBindingLawSummaryForm(t testing.TB, axis int) identity.SemanticKey {
	t.Helper()
	return coldKey(962_000 + axis)
}

func generatedBindingLawIDs(t testing.TB) (identity.ContentID, identity.ContentID) {
	t.Helper()
	mount, mountOK := identity.DeriveContentID("go-lua/generated-binding-law/mount", []byte("mount"))
	occurrence, occurrenceOK := identity.DeriveContentID("go-lua/generated-binding-law/occurrence", []byte("occurrence"))
	if !mountOK || !occurrenceOK {
		t.Fatal("generated binding law identities")
	}
	return mount, occurrence
}

func openGeneratedBindingLaw(t testing.TB, fixture generatedRuleLawFixture) generatedBindingLawFixture {
	t.Helper()
	return openGeneratedBindingLaneLaw(t, fixture, false, 0)
}

// openGeneratedBindingSpareLaw seals the same binding with additional bound
// Factors the Plan names no axis for. A spare Factor is what a claim fenced by
// the rule's own output axis has to be refused against: without one, every
// Factor in the binding is the right answer by accident.
func openGeneratedBindingSpareLaw(t testing.TB, fixture generatedRuleLawFixture, spare int) generatedBindingLawFixture {
	t.Helper()
	return openGeneratedBindingLaneLaw(t, fixture, false, spare)
}

// openGeneratedBindingLaneLaw seals one generated Rule on the lane under test.
// The two lanes differ only in which capability the composition issues, so the
// fixture keeps one construction and names the lane.
func openGeneratedBindingLaneLaw(t testing.TB, fixture generatedRuleLawFixture, link bool, spare int) generatedBindingLawFixture {
	t.Helper()
	builder := NewSchema()
	factors := make([]*FactorSlot[uint64], fixture.catalog.AxisCount()+spare)
	summaryForms := make([]SchemaReadForm[uint64], len(factors))
	for index := range factors {
		semantic := coldKey(964_000 + index)
		if index < fixture.catalog.AxisCount() {
			axisRow, axisOK := fixture.catalog.AxisAt(index)
			if !axisOK {
				t.Fatalf("generated binding law axis %d", index)
			}
			semantic = axisRow.Semantic
		}
		factor, factorOK := DeclareFactorSlot[uint64](builder, semantic)
		if !factorOK {
			t.Fatalf("generated binding law factor %d", index)
		}
		// Every axis of this fixture publishes a summary read form, the way an
		// axis that answers a vector read does. The form is the Factor's
		// statement of how a whole denominator is delivered, so a rule that
		// declares a vector join has one to be delivered over.
		form, formOK := factor.SummaryRead(generatedBindingLawSummaryForm(t, index))
		if !formOK {
			t.Fatalf("generated binding law summary form %d", index)
		}
		factors[index] = factor
		summaryForms[index] = form
	}
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	schema, schemaOK := builder.Seal()
	if !slotOK || slot == nil || !schemaOK || schema == nil {
		t.Fatal("generated binding law schema")
	}
	binding := NewSchemaBinding(schema)
	if binding == nil {
		t.Fatal("generated binding law binding")
	}
	for index, factor := range factors {
		if !BindFactor(binding, factor, hotUintFactorSpec()) {
			t.Fatalf("generated binding law bind factor %d", index)
		}
		if !BindIdentitySummaryReadForFactor[uint64](binding, factor, summaryForms[index]) {
			t.Fatalf("generated binding law bind summary form %d", index)
		}
	}
	if !BindGeneratedRule(binding, slot) {
		t.Fatal("generated binding law bind Rule")
	}
	capability, capabilityOK := RegisterMountedGeneratedSlot(binding, slot)
	if link {
		capability, capabilityOK = RegisterLinkGeneratedSlot(binding, slot)
	}
	if !capabilityOK || capability.Mounted() == link || capability.Link() != link {
		t.Fatalf("generated binding law capability link=%t mounted=%t issued=%t", capability.Link(), capability.Mounted(), capabilityOK)
	}
	mount, occurrence := generatedBindingLawIDs(t)
	return generatedBindingLawFixture{
		fixture: fixture, schema: schema, binding: binding, factors: factors,
		slot: slot, cap: capability, mount: mount, occurrence: occurrence,
	}
}

func generatedBindingLawOwnerForDescriptor(t testing.TB, fixture generatedBindingLawFixture) *generatedBindingLawOwner {
	t.Helper()
	ordinal, ordinalOK := fixture.slot.Ordinal()
	if !ordinalOK {
		t.Fatal("generated binding law Rule ordinal")
	}
	cell, cellOK := fixture.binding.state.rules[ordinal].(*generatedRuleBindingCell)
	if !cellOK || cell == nil {
		t.Fatal("generated binding law generated cell")
	}
	descriptor := cell.generated.program
	owner := &generatedBindingLawOwner{
		relation:        descriptor.CandidateRelation().Member,
		candidate:       1,
		acceptCandidate: true,
		projections:     make(map[[2]uint32]uint32),
	}
	owner.projections[[2]uint32{descriptor.JoinRelation().Member, descriptor.KeyProjection().Member}] = 0
	owner.projections[[2]uint32{descriptor.CandidateRelation().Member, descriptor.DestinationProjection().Member}] = 1
	return owner
}

func bindGeneratedLawOwner(t testing.TB, fixture *generatedBindingLawFixture, axis uint32) {
	t.Helper()
	if fixture == nil || uint64(axis) >= uint64(len(fixture.factors)) || fixture.owner == nil {
		t.Fatal("generated binding law owner inputs")
	}
	if !BindRelationOwner(fixture.binding, fixture.factors[axis], fixture.owner) {
		t.Fatalf("generated binding law owner axis %d", axis)
	}
}

func generatedLawCell(t testing.TB, fixture generatedBindingLawFixture) *generatedRuleBindingCell {
	t.Helper()
	ordinal, ordinalOK := fixture.slot.Ordinal()
	if !ordinalOK {
		t.Fatal("generated binding law cell ordinal")
	}
	cell, ok := fixture.binding.state.rules[ordinal].(*generatedRuleBindingCell)
	if !ok || cell == nil {
		t.Fatal("generated binding law cell")
	}
	return cell
}

// newGeneratedRuleLawMultiAxisFixture is the existing generated Rule law
// fixture with one additional, engine-published axis.  The Plan still names
// only axis zero; the second axis exists solely to prove owner completeness is
// descriptor-exact rather than an all-Factor requirement.
func newGeneratedRuleLawMultiAxisFixture(t testing.TB) generatedRuleLawFixture {
	t.Helper()
	axisReference := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: generatedRuleLawAxisKey}
	denominatorReference := schema.EntryReference{Surface: schema.SurfaceKindDenominator, Key: generatedRuleLawDenominator}
	read := program.ReadDecl{
		PointBound: program.PointBound,
		Input:      0, Axis: program.AxisRef(axisReference), Form: program.Exact,
		Contract: program.ReadContract{
			Order: program.OrderCanonical, Sparse: program.SparseExplicit,
			OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
			DenominatorRef: program.DenominatorRef(denominatorReference),
		},
	}
	join := program.JoinDecl{
		Sources:  []program.SourceRef{program.CandidateSource()},
		Relation: member.RelationRef{Axis: axisReference, Member: generatedRuleLawJoin},
		Key:      member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawKey}, Read: read,
	}
	declaration := program.Program{
		OperandRole: generatedRuleLawOperandRole,
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: axisReference, Member: generatedRuleLawCandidate}),
		Joins:       []program.JoinDecl{join},
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: axisReference, Member: generatedRuleLawReducer},
			Inputs:  []program.JoinRef{0},
			Outputs: []program.OutputDecl{{
				Column:      axis.OutputRef{Axis: axisReference, Key: generatedRuleLawOutput},
				Destination: member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawDestination},
				Mode:        program.ModeExact, ValueSlot: 0,
			}},
		},
		Carry: &program.CarryDecl{Input: 1, Mode: program.CarryIdentity},
	}
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("multi-axis generated Rule Program rejected: %+v", problem)
	}
	memberCatalog, catalogOK := member.NewCatalog(
		[]member.Relation{
			{Key: generatedRuleLawCandidate, Subject: generatedRuleLawCandidateFact, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
			{Key: generatedRuleLawJoin, Subject: generatedRuleLawJoinFact, Inputs: []member.Carrier{generatedRuleLawCandidateFact}, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
		},
		[]member.Projection{
			{Key: generatedRuleLawKey, Relation: generatedRuleLawJoin, Role: member.Key, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))},
			{Key: generatedRuleLawDestination, Relation: generatedRuleLawCandidate, Role: member.Destination, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
		},
		[]member.Reducer{{
			Key:     generatedRuleLawReducer,
			Inputs:  []member.ReducerInput{{Axis: axisReference, Carrier: generatedRuleLawJoinFact, Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne}},
			Outputs: []member.ReducerOutput{{Axis: axisReference, Carrier: generatedRuleLawJoinFact}},
		}}, nil,
	)
	if !catalogOK {
		t.Fatal("multi-axis generated Rule member catalog")
	}
	axisTemplate, axisOK := axis.New(axis.Spec[struct{}]{
		Key: generatedRuleLawAxisKey, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse,
		Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:   axis.Frame{Outputs: []axis.Output{{Key: generatedRuleLawOutput, Writer: generatedRuleLawAxisKey}}},
		Catalog: memberCatalog, Signature: axis.Signature{Key: generatedRuleLawKeyCarrier, Fact: generatedRuleLawJoinFact},
		Semantic: generatedRuleLawAxisRole,
	})
	if !axisOK {
		t.Fatal("multi-axis generated Rule axis")
	}
	unrelatedRole := generatedRuleLawUnrelatedRole(t)
	unrelatedTemplate, unrelatedOK := axis.New(axis.Spec[struct{}]{
		Key: generatedRuleLawUnrelatedAxisKey, Storage: axis.StorageEngine, Cardinality: axis.CardinalitySparse,
		Lifetime: axis.LifetimeProcess, Mutability: axis.MutabilityFrozen, Concurrency: axis.ConcurrencyShared,
		Frame:    axis.Frame{Outputs: []axis.Output{{Key: generatedRuleLawUnrelatedOutput, Writer: generatedRuleLawUnrelatedAxisKey}}},
		Semantic: unrelatedRole,
	})
	if !unrelatedOK {
		t.Fatal("multi-axis unrelated axis")
	}
	programRule, ruleOK := rule.New(rule.Spec{
		Key: generatedRuleLawProgram, Lane: rule.LaneLink, Writes: generatedRuleLawAxisKey, Owner: generatedRuleLawAxisKey,
		Semantic: generatedRuleLawRuleRole, Roles: []schema.Key{generatedRuleLawOperandRole}, Program: declaration,
	})
	if !ruleOK {
		t.Fatal("multi-axis generated Rule rule")
	}
	absentRule, absentOK := rule.New(rule.Spec{
		Key: generatedRuleLawAbsent, Lane: rule.LaneLink, Writes: generatedRuleLawAxisKey, Owner: generatedRuleLawAxisKey,
		Semantic: generatedRuleLawAbsentRole,
	})
	if !absentOK {
		t.Fatal("multi-axis absent Rule")
	}
	roles := make([]schema.Entry, 0, 5)
	for ordinal, role := range []schema.Key{
		generatedRuleLawAxisRole, generatedRuleLawRuleRole, generatedRuleLawOperandRole,
		generatedRuleLawAbsentRole, unrelatedRole,
	} {
		entry, entryOK := structure.New(structure.Spec{
			Key: role, Category: structure.CategorySemanticRole, Ordinal: uint16(ordinal + 1), Spelling: string(role), Accepted: true,
		})
		if !entryOK {
			t.Fatalf("multi-axis role %q", role)
		}
		roles = append(roles, entry)
	}
	universe, universeOK := identity.DeriveContentID("go-lua/generated-rule-law/axis", []byte(generatedRuleLawAxisKey))
	if !universeOK {
		t.Fatal("multi-axis denominator universe")
	}
	coordinate, coordinateOK := denominator.Coordinate(generatedRuleLawAxisKey, universe)
	if !coordinateOK {
		t.Fatal("multi-axis denominator")
	}
	builder := seal.NewBuilder()
	register := func(surface seal.Surface) {
		if !builder.Register(surface) {
			t.Fatalf("multi-axis surface %d", surface.Kind())
		}
	}
	register(generatedRuleLawSurface{kind: schema.SurfaceKindStructure, entries: roles})
	register(axis.NewSurface([]*axis.Template[struct{}]{axisTemplate, unrelatedTemplate}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindIssuance})
	register(rule.NewSurface([]*rule.Template{programRule, absentRule}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindDiagnostic})
	register(generatedRuleLawSurface{kind: schema.SurfaceKindComposite})
	register(denominator.NewSurface([]*denominator.Entry{coordinate}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindQuery})
	register(generatedRuleLawSurface{kind: schema.SurfaceKindObservation})
	table, sealFailure := builder.Seal()
	if sealFailure.Available() || table == nil {
		t.Fatalf("multi-axis schema seal: %+v", sealFailure)
	}
	catalog, compileFailure := ruleplan.Compile(table)
	if compileFailure.Available() || !catalog.Available() {
		t.Fatalf("multi-axis plan compile: %+v", compileFailure)
	}
	return generatedRuleLawFixture{table: table, catalog: catalog}
}

func TestGeneratedRuleBindsZeroJoinSource(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawSource, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() || !fixture.binding.Sealed() {
		t.Fatal("zero-join source generated Rule refused to seal")
	}
	cell := generatedLawCell(t, fixture)
	if !cell.schemaRuleComplete() || cell.generated == nil || !cell.generated.available() ||
		cell.generated.program.ReadCount() != 0 || cell.generated.read.Available() {
		t.Fatal("zero-join source generated cell is incomplete after bind")
	}
}

// TestGeneratedRelationOwnerIsOneShotAndClosed proves the owner handoff is a
// single Factor-local capability.  A duplicate, a nil owner, and a slot from
// another SchemaBinding all refuse before any construction lookup can run.
func TestGeneratedRelationOwnerIsOneShotAndClosed(t *testing.T) {
	t.Run("duplicate", func(t *testing.T) {
		schema, factor := factorOnlySlotSchema(t, coldKey(991_001))
		binding := NewSchemaBinding(schema)
		owner := &generatedBindingLawOwner{relation: 0, candidate: 0, acceptCandidate: true, projections: map[[2]uint32]uint32{}}
		if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRelationOwner(binding, factor, owner) {
			t.Fatal("first relation owner handoff")
		}
		if BindRelationOwner(binding, factor, owner) || !binding.Poisoned() {
			t.Fatal("duplicate relation owner handoff crossed the one-shot fence")
		}
	})

	t.Run("nil-owner", func(t *testing.T) {
		schema, factor := factorOnlySlotSchema(t, coldKey(991_002))
		binding := NewSchemaBinding(schema)
		if !BindFactor(binding, factor, hotUintFactorSpec()) || BindRelationOwner(binding, factor, nil) || !binding.Poisoned() {
			t.Fatal("nil relation owner was admitted")
		}
	})

	t.Run("foreign-slot", func(t *testing.T) {
		localSchema, localFactor := factorOnlySlotSchema(t, coldKey(991_003))
		foreignSchema, foreignFactor := factorOnlySlotSchema(t, coldKey(991_004))
		binding := NewSchemaBinding(localSchema)
		owner := &generatedBindingLawOwner{relation: 0, candidate: 0, acceptCandidate: true, projections: map[[2]uint32]uint32{}}
		if !BindFactor(binding, localFactor, hotUintFactorSpec()) || BindRelationOwner(binding, foreignFactor, owner) || !binding.Poisoned() || foreignSchema == localSchema {
			t.Fatal("foreign Factor slot crossed the relation owner fence")
		}
	})
}

// TestGeneratedRuleCapabilitySealRequiresReferencedOwners proves the generated
// arm can publish only after its candidate and join owners are present.  A
// separate two-axis schema below proves a non-referenced axis stays optional.
func TestGeneratedRuleCapabilitySealRequiresReferencedOwners(t *testing.T) {
	missing := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	missing.owner = generatedBindingLawOwnerForDescriptor(t, missing)
	if missing.binding.Seal() || !missing.binding.Poisoned() {
		t.Fatal("generated Rule sealed without its referenced relation owner")
	}

	bound := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	bound.owner = generatedBindingLawOwnerForDescriptor(t, bound)
	bindGeneratedLawOwner(t, &bound, 0)
	if !bound.binding.Seal() || !bound.binding.Sealed() {
		t.Fatal("generated Rule with its referenced owner refused to seal")
	}
	if capability, ok := MountedGeneratedCapabilityForSlot(bound.binding, bound.slot); !ok || capability != bound.cap || !capability.Mounted() {
		t.Fatal("mounted generated capability was not retained")
	}
}

// TestGeneratedRuleSealDoesNotRequireUnrelatedAxisOwner proves owner
// completeness is exact over the Plan descriptor's referenced axes, not over
// every Factor in the binding.
func TestGeneratedRuleSealDoesNotRequireUnrelatedAxisOwner(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawMultiAxisFixture(t))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if len(fixture.factors) < 2 {
		t.Fatal("multi-axis generated fixture has no unrelated axis")
	}
	if fixture.binding.state.factors[1].schemaFactorRelationOwner() != nil {
		t.Fatal("unrelated axis unexpectedly received an owner")
	}
	if !fixture.binding.Seal() {
		t.Fatal("generated Rule required an unrelated axis owner")
	}
}

// TestGeneratedIssuanceOwnerProjectionAndFactorSurfaces proves the deepest
// available production-style issuance seam without manufacturing an equation
// Site.  The owner resolves candidate/source/destination locals and the
// sealed Factor cells mint the exact/strong surfaces that issuance carries.
func TestGeneratedIssuanceOwnerProjectionAndFactorSurfaces(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("generated issuance binding seal")
	}
	cell := generatedLawCell(t, fixture)
	descriptor := cell.generated.program
	candidateOwner, candidateOK := relationOwnerForGeneratedAxis(fixture.binding.state, descriptor.CandidateRelation().Axis)
	joinOwner, joinOK := relationOwnerForGeneratedAxis(fixture.binding.state, descriptor.JoinRelation().Axis)
	if !candidateOK || !joinOK || candidateOwner == nil || joinOwner == nil {
		t.Fatal("generated issuance relation owners")
	}
	candidate, candidateOK := candidateOwner.CandidateAt(descriptor.CandidateRelation().Member, fixture.mount, fixture.occurrence, 0)
	sourceLocal, sourceOK := joinOwner.Project(descriptor.JoinRelation().Member, descriptor.KeyProjection().Member, candidate)
	destinationLocal, destinationOK := candidateOwner.Project(descriptor.CandidateRelation().Member, descriptor.DestinationProjection().Member, candidate)
	if !candidateOK || !sourceOK || !destinationOK || candidate != 1 || sourceLocal != 0 || destinationLocal != 1 {
		t.Fatalf("generated issuance owner coordinates candidate=%d/%t source=%d/%t destination=%d/%t", candidate, candidateOK, sourceLocal, sourceOK, destinationLocal, destinationOK)
	}
	factor := fixture.binding.state.factors[descriptor.ReadFactor()]
	output := fixture.binding.state.factors[descriptor.OutputFactor()]
	read, readOK := factor.schemaFactorExactRead(fixture.binding.state, fixture.binding.state.authority, uint64(sourceLocal))
	write, writeOK := output.schemaFactorExactWrite(fixture.binding.state, fixture.binding.state.authority, uint64(destinationLocal))
	if !readOK || !writeOK || read.value.Form != equation.SurfaceReadExact || read.value.Mode != equation.TargetModeNone || read.value.Local != 1 || write.value.Form != equation.SurfaceWriteExact || write.value.Mode != equation.TargetModeStrong || write.value.Local != 2 || read.value.Factor != write.value.Factor {
		t.Fatalf("generated issuance surfaces read=%+v/%t write=%+v/%t", read.value, readOK, write.value, writeOK)
	}
}

// TestGeneratedIssuanceNearestNegatives keeps the owner and Factor refusal
// boundaries adjacent to the positive projection law.
func TestGeneratedIssuanceNearestNegatives(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("generated negative binding seal")
	}
	cell := generatedLawCell(t, fixture)
	descriptor := cell.generated.program
	owner := fixture.owner
	if _, ok := owner.CandidateAt(descriptor.CandidateRelation().Member, identity.ContentID{}, fixture.occurrence, 0); ok {
		t.Fatal("absent mount admitted a generated candidate")
	}
	if _, ok := owner.CandidateAt(descriptor.CandidateRelation().Member, fixture.mount, identity.ContentID{}, 0); ok {
		t.Fatal("absent occurrence admitted a generated candidate")
	}
	if _, ok := owner.CandidateAt(descriptor.CandidateRelation().Member+1, fixture.mount, fixture.occurrence, 0); ok {
		t.Fatal("wrong relation member admitted a generated candidate")
	}
	if _, ok := owner.Project(descriptor.JoinRelation().Member, descriptor.KeyProjection().Member+1, 1); ok {
		t.Fatal("wrong projection member admitted a generated local")
	}
	if _, ok := owner.Project(descriptor.JoinRelation().Member, descriptor.KeyProjection().Member, 0); ok {
		t.Fatal("wrong candidate ordinal admitted a generated local")
	}
	owner.acceptCandidate = false
	if _, ok := owner.CandidateAt(descriptor.CandidateRelation().Member, fixture.mount, fixture.occurrence, 0); ok {
		t.Fatal("absent candidate admitted a generated issuance")
	}
	owner.acceptCandidate = true
	if _, ok := relationOwnerForGeneratedAxis(fixture.binding.state, descriptor.CandidateRelation().Axis+1); ok {
		t.Fatal("wrong generated axis resolved a relation owner")
	}
	factor := fixture.binding.state.factors[descriptor.ReadFactor()]
	read, readOK := factor.schemaFactorExactRead(fixture.binding.state, fixture.binding.state.authority, 0)
	if !readOK {
		t.Fatal("generated negative exact read")
	}
	foreignSchema, foreignSlot := factorOnlySlotSchema(t, coldKey(991_905))
	foreignBinding := NewSchemaBinding(foreignSchema)
	if foreignBinding == nil || !BindFactor(foreignBinding, foreignSlot, hotUintFactorSpec()) || !foreignBinding.Seal() {
		t.Fatal("generated foreign Factor binding")
	}
	foreignFactor := foreignBinding.state.factors[0]
	if foreignFactor == nil {
		t.Fatal("generated foreign Factor row")
	}
	if _, ok := exactReadLocal(foreignFactor, read.value); ok {
		t.Fatal("foreign Factor accepted generated read local")
	}
	if _, ok := exactReadLocal(factor, equation.Surface{Factor: read.value.Factor, Form: equation.SurfaceReadExact, Local: 0}); ok {
		t.Fatal("zero generated read local admitted")
	}
	if _, ok := exactReadLocal(factor, equation.Surface{Factor: read.value.Factor, Form: equation.SurfaceReadExact, Local: 3}); ok {
		t.Fatal("out-of-range generated read local admitted")
	}
}

// TestGeneratedMemberRowUsesOnlyTheGeneratedArm proves the construction row
// union receives the generated member in its generated field and cannot also
// expose a legacy runtimeMember arm.
func TestGeneratedMemberRowUsesOnlyTheGeneratedArm(t *testing.T) {
	factor := newGeneratedFactorAdapterFixture(t)
	member, _ := generatedMemberRuntime(t, factor, 7, 3)
	row := memberRow{generated: member}
	geometry, ok := row.geometry()
	if !ok || geometry != member || row.legacy != nil || !row.valid() {
		t.Fatal("generated member entered the wrong member row arm")
	}
	if _, legacy := any(member).(runtimeMember); legacy {
		t.Fatal("generated member implements the legacy runtimeMember arm")
	}
}

// TestGeneratedRuntimeRowsDoNotRetainRelationOwners proves the compact
// generated member and runtime program carry no relation-owner capability.
// The owner remains on the immutable sealed Factor binding for reuse, but it
// is never copied into either runtime row.
func TestGeneratedRuntimeRowsDoNotRetainRelationOwners(t *testing.T) {
	fixture := openGeneratedBindingLaw(t, newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole))
	fixture.owner = generatedBindingLawOwnerForDescriptor(t, fixture)
	bindGeneratedLawOwner(t, &fixture, 0)
	if !fixture.binding.Seal() {
		t.Fatal("generated release binding seal")
	}
	cell := generatedLawCell(t, fixture)
	if owner, ok := relationOwnerForGeneratedAxis(fixture.binding.state, cell.generated.program.CandidateRelation().Axis); !ok || owner == nil {
		t.Fatal("generated owner was not available before release")
	}
	if cell.generated == nil || !cell.generated.available() {
		t.Fatal("generated descriptor unavailable after sealing")
	}
	adapter := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, adapter, 7, 3)
	if member == nil || epoch == nil || epoch.runtime == nil || epoch.runtime.program == nil {
		t.Fatal("generated runtime rows unavailable")
	}
	if _, ownerReachable := any(member).(memberrelation.Owner); ownerReachable {
		t.Fatal("generated member retained a relation owner capability")
	}
	if _, ownerReachable := any(epoch.runtime.program).(memberrelation.Owner); ownerReachable {
		t.Fatal("runtime program retained a relation owner capability")
	}
	for _, runtimeFactor := range epoch.runtime.program.factorOwners {
		if _, ownerReachable := any(runtimeFactor).(memberrelation.Owner); ownerReachable {
			t.Fatal("runtime Factor retained a relation owner capability")
		}
	}
}

func (owner *generatedBindingLawOwner) KeyVectorCount(relationOrdinal, candidateOrdinal uint32) (int, bool) {
	return 0, false
}

func (owner *generatedBindingLawOwner) KeyVectorAt(relationOrdinal, candidateOrdinal uint32, ordinal int) (uint32, bool) {
	return 0, false
}

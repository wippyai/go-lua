package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	generatedRuleLawAxisKey                schema.Key     = "axis/generated-rule-law"
	generatedRuleLawCandidate              schema.Key     = "relation/generated-rule-law-candidate"
	generatedRuleLawJoin                   schema.Key     = "relation/generated-rule-law-join"
	generatedRuleLawRouteJoin              schema.Key     = "relation/generated-rule-law-route-join"
	generatedRuleLawBranchJoin             schema.Key     = "relation/generated-rule-law-branch-join"
	generatedRuleLawBranchKey              schema.Key     = "projection/generated-rule-law-branch-key"
	generatedRuleLawApplication            schema.Key     = "projection/generated-rule-law-application"
	generatedRuleLawTarget                 schema.Key     = "projection/generated-rule-law-target"
	generatedRuleLawEndpoint               schema.Key     = "projection/generated-rule-law-endpoint"
	generatedRuleLawBranchMount            schema.Key     = "projection/generated-rule-law-branch-mount"
	generatedRuleLawBranchBody             schema.Key     = "projection/generated-rule-law-branch-body"
	generatedRuleLawKey                    schema.Key     = "projection/generated-rule-law-key"
	generatedRuleLawRouteKey               schema.Key     = "projection/generated-rule-law-route-key"
	generatedRuleLawPredicate              schema.Key     = "projection/generated-rule-law-predicate"
	generatedRuleLawRoutePredicate         schema.Key     = "projection/generated-rule-law-route-predicate"
	generatedRuleLawDestination            schema.Key     = "projection/generated-rule-law-destination"
	generatedRuleLawRouteDestination       schema.Key     = "projection/generated-rule-law-route-destination"
	generatedRuleLawReducer                schema.Key     = "reducer/generated-rule-law"
	generatedRuleLawOutput                 schema.Key     = "output/generated-rule-law"
	generatedRuleLawDenominator            schema.Key     = "coordinates/axis/generated-rule-law"
	generatedRuleLawTransform              schema.Key     = "transform/generated-rule-law"
	generatedRuleLawProgram                schema.Key     = "rule/generated-rule-law"
	generatedRuleLawAbsent                 schema.Key     = "rule/generated-rule-law-absent"
	generatedRuleLawAxisRoleSpelling                      = "generated-rule-law/axis"
	generatedRuleLawRuleRoleSpelling                      = "generated-rule-law/rule"
	generatedRuleLawOperandRoleSpelling                   = "generated-rule-law/operand"
	generatedRuleLawActivationRoleSpelling                = "generated-rule-law/activation-family"
	generatedRuleLawAbsentRoleSpelling                    = "generated-rule-law/absent"
	generatedRuleLawCandidateFact          member.Carrier = "carrier/generated-rule-law-candidate"
	generatedRuleLawJoinFact               member.Carrier = "carrier/generated-rule-law-join"
	generatedRuleLawKeyCarrier             member.Carrier = "carrier/generated-rule-law-key"
	generatedRuleLawBranchOrdinal          member.Carrier = "carrier/generated-rule-law-branch-ordinal"
	generatedRuleLawIdentityCarrier        member.Carrier = "carrier/generated-rule-law-identity"
)

var (
	generatedRuleLawAxisRole       = vocabulary.RoleKey(generatedRuleLawAxisRoleSpelling)
	generatedRuleLawRuleRole       = vocabulary.RoleKey(generatedRuleLawRuleRoleSpelling)
	generatedRuleLawOperandRole    = vocabulary.RoleKey(generatedRuleLawOperandRoleSpelling)
	generatedRuleLawActivationRole = vocabulary.RoleKey(generatedRuleLawActivationRoleSpelling)
	generatedRuleLawAbsentRole     = vocabulary.RoleKey(generatedRuleLawAbsentRoleSpelling)
)

type generatedRuleLawVariant uint8

const (
	generatedRuleLawExact generatedRuleLawVariant = iota + 1
	generatedRuleLawSelected
	generatedRuleLawTransformedCarry
	generatedRuleLawRouteOutput
	generatedRuleLawValueTransfer
	generatedRuleLawSource
	generatedRuleLawSummary
	generatedRuleLawComplete
	generatedRuleLawStructural
	// generatedRuleLawActivation is the well-formed A form: one exact read at
	// the trigger coordinate, one vector read over the branch set hanging off
	// that same trigger row, a structural publication, and the transport
	// vector one candidate route instantiates when it crosses its transition.
	//
	// The branch read is a parent-declaring Summary and not a selection. The
	// construct topology mounts one activation member per branch before any
	// solve, so the branches have to be enumerable at issuance; a selection's
	// members exist only per invocation and nothing could have mounted them.
	generatedRuleLawActivation
)

type generatedRuleLawSurface struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
}

func (surface generatedRuleLawSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface generatedRuleLawSurface) Entries() []schema.Entry {
	return append([]schema.Entry(nil), surface.entries...)
}

func (generatedRuleLawSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

type generatedRuleLawFixture struct {
	table   *seal.Schema
	catalog ruleplan.Catalog
}

func generatedRuleLawProvider(relation schema.Key) member.RelationRef {
	return member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: generatedRuleLawAxisKey}, Member: relation}
}

func newGeneratedRuleLawFixture(t testing.TB, variant generatedRuleLawVariant, ruleRole schema.Key) generatedRuleLawFixture {
	t.Helper()
	axisReference := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: generatedRuleLawAxisKey}
	denominatorReference := schema.EntryReference{Surface: schema.SurfaceKindDenominator, Key: generatedRuleLawDenominator}

	read := program.ReadDecl{
		PointBound: program.PointBound,
		Input:      0,
		Axis:       program.AxisRef(axisReference),
		Form:       program.Exact,
		Contract: program.ReadContract{
			Order:          program.OrderCanonical,
			Sparse:         program.SparseExplicit,
			OnOpaque:       program.OnOpaqueRefuse,
			Multiplicity:   program.MultiplicityOne,
			DenominatorRef: program.DenominatorRef(denominatorReference),
		},
	}
	join := program.JoinDecl{
		Sources:  []program.SourceRef{program.CandidateSource()},
		Relation: member.RelationRef{Axis: axisReference, Member: generatedRuleLawJoin},
		Key:      member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawKey},
		Read:     read,
	}
	joins := []program.JoinDecl{join}
	output := program.OutputDecl{
		Column: axis.OutputRef{Axis: axisReference, Key: generatedRuleLawOutput},
		Destination: member.ProjectionRef{
			Axis:   axisReference,
			Member: generatedRuleLawDestination,
		},
		Mode:      program.ModeExact,
		ValueSlot: 0,
	}
	if variant == generatedRuleLawActivation {
		branchRead := read
		branchRead.Input = 1
		branchRead.Form = program.Summary
		branchRead.Contract.Multiplicity = program.MultiplicityMany
		// The branch set hangs off the CANDIDATE row, not off the trigger's
		// fact: which routes a trigger has is a property of the trigger, and
		// the fact only decides which of them activate.
		joins = append(joins, program.JoinDecl{
			Sources:  []program.SourceRef{program.CandidateSource()},
			Relation: member.RelationRef{Axis: axisReference, Member: generatedRuleLawBranchJoin},
			Key:      member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawBranchKey},
			Parent:   member.RelationRef{Axis: axisReference, Member: generatedRuleLawCandidate},
			Read:     branchRead,
		})
	}
	if variant == generatedRuleLawSelected || variant == generatedRuleLawRouteOutput {
		routeRead := read
		routeRead.Input = 1
		routeRead.Form = program.Selected
		routeJoin := program.JoinDecl{
			Sources:   []program.SourceRef{program.PriorSource(0)},
			Relation:  member.RelationRef{Axis: axisReference, Member: generatedRuleLawRouteJoin},
			Key:       member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawRouteKey},
			Read:      routeRead,
			Predicate: member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawRoutePredicate},
		}
		joins = append(joins, routeJoin)
		if variant == generatedRuleLawRouteOutput {
			output.Mode = program.ModeRoute
			output.RouteJoin = 1
			output.RouteJoinPresent = true
			output.Destination.Member = generatedRuleLawRouteDestination
		}
	}
	if variant == generatedRuleLawSummary {
		join.Read.Form = program.Summary
		join.Predicate = member.ProjectionRef{Axis: axisReference, Member: generatedRuleLawPredicate}
	}
	if variant == generatedRuleLawComplete {
		join.Read.Form = program.Complete
	}
	if variant == generatedRuleLawStructural || variant == generatedRuleLawActivation {
		output.Mode = program.ModeStructural
	}
	if variant == generatedRuleLawSummary || variant == generatedRuleLawComplete {
		joins[0] = join
	}
	carryInput := program.InputRef(1)
	if variant == generatedRuleLawValueTransfer {
		carryInput = 0
	}
	declaration := program.Program{
		OperandRole: generatedRuleLawOperandRole,
		Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: axisReference, Member: generatedRuleLawCandidate}),
		Joins:       joins,
		Fold: program.FoldDecl{
			Reducer: member.ReducerRef{Axis: axisReference, Member: generatedRuleLawReducer},
			Inputs:  []program.JoinRef{0},
			Outputs: []program.OutputDecl{output},
		},
		Carry: &program.CarryDecl{Input: carryInput, Mode: program.CarryIdentity},
	}
	if variant == generatedRuleLawSelected || variant == generatedRuleLawRouteOutput || variant == generatedRuleLawActivation {
		declaration.Fold.Inputs = []program.JoinRef{0, 1}
	}
	if variant == generatedRuleLawActivation {
		// A structural publication stages no fact, so it has no prior fact to
		// carry. Its vector and the family its branches are grouped under are
		// the one declaration the cold structural row is built from.
		declaration.Carry = nil
		declaration.Transport = []program.TransportDecl{{Axis: program.AxisRef(axisReference)}}
		declaration.ActivationRole = generatedRuleLawActivationRole
		branchProjection := func(key schema.Key) member.ProjectionRef {
			return member.ProjectionRef{Axis: axisReference, Member: key}
		}
		declaration.Activation = &program.ActivationDecl{
			Branch:      1,
			Application: branchProjection(generatedRuleLawApplication),
			Target:      branchProjection(generatedRuleLawTarget),
			Endpoint:    branchProjection(generatedRuleLawEndpoint),
			Mount:       branchProjection(generatedRuleLawBranchMount),
			Body:        branchProjection(generatedRuleLawBranchBody),
		}
	}
	if variant == generatedRuleLawSource {
		declaration = program.Program{
			OperandRole: generatedRuleLawOperandRole,
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: axisReference, Member: generatedRuleLawCandidate}),
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: axisReference, Member: generatedRuleLawReducer},
				Outputs: []program.OutputDecl{output},
			},
		}
	}
	if variant == generatedRuleLawTransformedCarry {
		declaration.Carry = &program.CarryDecl{
			Input: 1,
			Mode:  program.CarryTransform,
			Transform: member.CarryTransformRef{
				Axis:   axisReference,
				Member: generatedRuleLawTransform,
			},
		}
	}
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("generated Rule law fixture Program rejected: %+v", problem)
	}

	transforms := []member.CarryTransform(nil)
	if variant == generatedRuleLawTransformedCarry {
		transforms = []member.CarryTransform{{
			Key:       generatedRuleLawTransform,
			Candidate: generatedRuleLawCandidateFact,
			Input:     generatedRuleLawJoinFact,
			Output:    generatedRuleLawJoinFact,
		}}
	}
	projections := []member.Projection{
		{Key: generatedRuleLawKey, Relation: generatedRuleLawJoin, Role: member.Key, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))},
		{Key: generatedRuleLawDestination, Relation: generatedRuleLawCandidate, Role: member.Destination, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
	}
	if variant == generatedRuleLawSummary {
		projections = append(projections, member.Projection{Key: generatedRuleLawPredicate, Relation: generatedRuleLawJoin, Role: member.Predicate, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))})
	}
	relations := []member.Relation{
		{Key: generatedRuleLawCandidate, Subject: generatedRuleLawCandidateFact, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
		{Key: generatedRuleLawJoin, Subject: generatedRuleLawJoinFact, Inputs: []member.Carrier{generatedRuleLawCandidateFact}, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
	}
	if variant == generatedRuleLawSelected || variant == generatedRuleLawRouteOutput {
		relations = append(relations, member.Relation{Key: generatedRuleLawRouteJoin, Subject: generatedRuleLawJoinFact, Inputs: []member.Carrier{generatedRuleLawJoinFact}, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))})
	}
	if variant == generatedRuleLawActivation {
		// The branch set is addressed by (trigger candidate, ordinal): the
		// owner already published these rows under the trigger's own row, and
		// the parent it hangs off is the rule's candidate relation.
		relations = append(relations, member.Relation{
			Key: generatedRuleLawBranchJoin, Subject: generatedRuleLawJoinFact,
			Inputs:            []member.Carrier{generatedRuleLawCandidateFact},
			CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate)),
			Parent:            generatedRuleLawProvider(generatedRuleLawCandidate),
			Ordinal:           generatedRuleLawBranchOrdinal,
		})
	}
	inputs := []member.ReducerInput{{
		Axis: axisReference, Carrier: generatedRuleLawJoinFact,
		Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
	}}
	if variant == generatedRuleLawSummary {
		inputs[0].Form = member.ReadFormSummary
		inputs[0].Tag = generatedRuleLawKeyCarrier
	}
	if variant == generatedRuleLawComplete {
		inputs[0].Form = member.ReadFormComplete
	}
	if variant == generatedRuleLawSelected || variant == generatedRuleLawRouteOutput {
		inputs = append(inputs, member.ReducerInput{Axis: axisReference, Carrier: generatedRuleLawJoinFact, Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne, Tag: generatedRuleLawKeyCarrier})
	}
	if variant == generatedRuleLawActivation {
		// The branch delivery is tagged by the ordinal its parent addresses it
		// at, which is the only address a nested member set's rows have.
		inputs = append(inputs, member.ReducerInput{Axis: axisReference, Carrier: generatedRuleLawJoinFact, Form: member.ReadFormSummary, Multiplicity: member.MultiplicityMany, Tag: generatedRuleLawBranchOrdinal})
	}
	reducers := []member.Reducer{{
		Key:     generatedRuleLawReducer,
		Inputs:  inputs,
		Outputs: []member.ReducerOutput{{Axis: axisReference, Carrier: generatedRuleLawJoinFact}},
	}}
	factCarrier := generatedRuleLawJoinFact
	if variant == generatedRuleLawSource {
		projections = []member.Projection{
			{Key: generatedRuleLawDestination, Relation: generatedRuleLawCandidate, Role: member.Destination, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
		}
		relations = []member.Relation{
			{Key: generatedRuleLawCandidate, Subject: generatedRuleLawCandidateFact, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))},
		}
		reducers = []member.Reducer{{
			Key:     generatedRuleLawReducer,
			Inputs:  []member.ReducerInput{},
			Outputs: []member.ReducerOutput{{Axis: axisReference, Carrier: generatedRuleLawCandidateFact}},
		}}
		factCarrier = generatedRuleLawCandidateFact
	}
	if variant == generatedRuleLawActivation {
		candidateProvider := member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawCandidate))
		branchIdentity := func(key schema.Key, relation schema.Key) member.Projection {
			return member.Projection{Key: key, Relation: relation, Role: member.Identity, Result: generatedRuleLawIdentityCarrier, CandidateProvider: candidateProvider}
		}
		projections = append(projections,
			member.Projection{Key: generatedRuleLawBranchKey, Relation: generatedRuleLawBranchJoin, Role: member.Key, Result: generatedRuleLawKeyCarrier, CandidateProvider: candidateProvider},
			// The application is the trigger's own; the other four are the
			// branch row's, which is what the construct plane mounts.
			branchIdentity(generatedRuleLawApplication, generatedRuleLawCandidate),
			branchIdentity(generatedRuleLawTarget, generatedRuleLawBranchJoin),
			branchIdentity(generatedRuleLawEndpoint, generatedRuleLawBranchJoin),
			branchIdentity(generatedRuleLawBranchMount, generatedRuleLawBranchJoin),
			branchIdentity(generatedRuleLawBranchBody, generatedRuleLawBranchJoin),
		)
	}
	if variant == generatedRuleLawSelected || variant == generatedRuleLawRouteOutput {
		projections = append(projections,
			member.Projection{Key: generatedRuleLawRouteKey, Relation: generatedRuleLawRouteJoin, Role: member.Key, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))},
			member.Projection{Key: generatedRuleLawRoutePredicate, Relation: generatedRuleLawRouteJoin, Role: member.Predicate, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))},
		)
		if variant == generatedRuleLawRouteOutput {
			projections = append(projections, member.Projection{Key: generatedRuleLawRouteDestination, Relation: generatedRuleLawRouteJoin, Role: member.Destination, Result: generatedRuleLawKeyCarrier, CandidateProvider: member.AxisRelationCandidate(generatedRuleLawProvider(generatedRuleLawJoin))})
		}
	}
	memberCatalog, catalogOK := member.NewCatalog(relations, projections, reducers, transforms)
	if !catalogOK {
		t.Fatal("generated Rule law member catalog rejected")
	}
	axisTemplate, axisOK := axis.New(axis.Spec[struct{}]{
		Key:         generatedRuleLawAxisKey,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeProcess,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame: axis.Frame{Outputs: []axis.Output{{
			Key: generatedRuleLawOutput, Writer: generatedRuleLawAxisKey,
		}}},
		Catalog:   memberCatalog,
		Signature: axis.Signature{Key: generatedRuleLawKeyCarrier, Fact: factCarrier},
		Semantic:  generatedRuleLawAxisRole,
	})
	if !axisOK {
		t.Fatal("generated Rule law axis rejected")
	}
	programRule, ruleOK := rule.New(rule.Spec{
		Key:      generatedRuleLawProgram,
		Lane:     rule.LaneLink,
		Writes:   generatedRuleLawAxisKey,
		Owner:    generatedRuleLawAxisKey,
		Semantic: ruleRole,
		Roles: func() []schema.Key {
			if variant == generatedRuleLawActivation {
				return []schema.Key{generatedRuleLawOperandRole, generatedRuleLawActivationRole}
			}
			return []schema.Key{generatedRuleLawOperandRole}
		}(),
		Program: declaration,
	})
	if !ruleOK {
		t.Fatal("generated Rule law Program rule rejected")
	}
	absentRule, absentOK := rule.New(rule.Spec{
		Key:      generatedRuleLawAbsent,
		Lane:     rule.LaneLink,
		Writes:   generatedRuleLawAxisKey,
		Owner:    generatedRuleLawAxisKey,
		Semantic: generatedRuleLawAbsentRole,
	})
	if !absentOK {
		t.Fatal("generated Rule law absent rule rejected")
	}

	roles := make([]schema.Entry, 0, 5)
	for ordinal, role := range []schema.Key{
		generatedRuleLawAxisRole, ruleRole, generatedRuleLawOperandRole, generatedRuleLawAbsentRole,
		generatedRuleLawActivationRole,
	} {
		entry, entryOK := structure.New(structure.Spec{
			Key: role, Category: structure.CategorySemanticRole, Ordinal: uint16(ordinal + 1), Spelling: string(role), Accepted: true,
		})
		if !entryOK {
			t.Fatalf("generated Rule law semantic role %q rejected", role)
		}
		roles = append(roles, entry)
	}
	universe, universeOK := identity.DeriveContentID("go-lua/generated-rule-law/axis", []byte(generatedRuleLawAxisKey))
	if !universeOK {
		t.Fatal("generated Rule law denominator universe identity unavailable")
	}
	coordinate, coordinateOK := denominator.Coordinate(generatedRuleLawAxisKey, universe)
	if !coordinateOK {
		t.Fatal("generated Rule law denominator rejected")
	}

	builder := seal.NewBuilder()
	register := func(surface seal.Surface) {
		if !builder.Register(surface) {
			t.Fatalf("generated Rule law surface %d registration failed", surface.Kind())
		}
	}
	register(generatedRuleLawSurface{kind: schema.SurfaceKindStructure, entries: roles})
	register(axis.NewSurface([]*axis.Template[struct{}]{axisTemplate}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindIssuance})
	register(rule.NewSurface([]*rule.Template{programRule, absentRule}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindDiagnostic})
	register(generatedRuleLawSurface{kind: schema.SurfaceKindComposite})
	register(denominator.NewSurface([]*denominator.Entry{coordinate}))
	register(generatedRuleLawSurface{kind: schema.SurfaceKindQuery})
	register(generatedRuleLawSurface{kind: schema.SurfaceKindObservation})
	table, sealFailure := builder.Seal()
	if sealFailure.Available() || table == nil {
		t.Fatalf("generated Rule law schema rejected: %+v", sealFailure)
	}
	catalog, compileFailure := ruleplan.Compile(table)
	if compileFailure.Available() || !catalog.Available() {
		t.Fatalf("generated Rule law plan rejected: %+v", compileFailure)
	}
	return generatedRuleLawFixture{table: table, catalog: catalog}
}

func generatedRuleLawBuilder(t testing.TB, catalog ruleplan.Catalog, mismatch bool) *SchemaBuilder {
	t.Helper()
	builder := NewSchema()
	for index := 0; index < catalog.AxisCount(); index++ {
		axisRow, axisOK := catalog.AxisAt(index)
		if !axisOK {
			t.Fatalf("generated Rule law axis %d missing", index)
		}
		semantic := axisRow.Semantic
		if mismatch {
			semantic = coldKey(991_700 + index)
		}
		factor, factorOK := DeclareFactorSlot[struct{}](builder, semantic)
		if !factorOK {
			t.Fatalf("generated Rule law factor %d declaration failed", index)
		}
		// A Factor publishes the summary read form a vector join is delivered
		// over. The form is the Factor's statement, so a fixture whose rules
		// may declare one has to make it.
		if _, formOK := factor.SummaryRead(coldKey(991_800 + index)); !formOK {
			t.Fatalf("generated Rule law summary form %d declaration failed", index)
		}
	}
	return builder
}

func TestGeneratedRuleSlotDerivesColdRowFromCatalogPlan(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole)
	compiled, planOK := fixture.catalog.At(0)
	if !planOK || !compiled.Present() {
		t.Fatal("generated Rule law Plan missing")
	}
	builder := generatedRuleLawBuilder(t, fixture.catalog, false)
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("exact generated Rule declaration refused")
	}
	if len(builder.candidate.Rules) != 1 || len(builder.candidate.Rules[0].Reads) != 1 || len(builder.candidate.Rules[0].Carries) != 1 || len(builder.candidate.Rules[0].Writes) != 1 {
		t.Fatalf("generated cold row cardinality = %+v", builder.candidate.Rules)
	}
	row := builder.candidate.Rules[0]
	output, outputOK := compiled.OutputAt(0)
	join, joinOK := compiled.JoinAt(0)
	carry, carryOK := compiled.Carry()
	axisRow, axisOK := fixture.catalog.AxisAt(0)
	if !outputOK || !joinOK || !carryOK || !axisOK {
		t.Fatal("generated Rule law Plan geometry missing")
	}
	wantFactor := compositionKeyOf(axisRow.Semantic)
	if row.Key != compositionKeyOf(compiled.Semantic()) || row.OperandFamily != compositionKeyOf(compiled.OperandFamily()) || row.Output != wantFactor || row.OutputKind != coldcomposition.FactorOutput || row.Inputs != uint64(compiled.InputCount()) {
		t.Fatalf("generated cold identity/header = %+v", row)
	}
	read := row.Reads[0]
	if read.Kind != coldcomposition.ReadExact || read.Input != uint64(join.Input) || read.Factor != wantFactor || len(read.Dependencies) != 0 || read.Semantic.Available() || read.Normalizer.Available() {
		t.Fatalf("generated cold read = %+v", read)
	}
	if got := row.Carries[0]; got.Input != uint64(carry.Input) || got.Factor != wantFactor || got.Transform.Available() {
		t.Fatalf("generated cold carry = %+v", got)
	}
	if got := row.Writes[0]; got.Kind != coldcomposition.WriteExact || got.Factor != wantFactor || got.Route != 0 {
		t.Fatalf("generated cold write = %+v", got)
	}
	if builder.rules[0].generated == nil || builder.rules[0].generated.planDigest != fixture.catalog.Digest() || builder.rules[0].generated.rule != compiled.Rule() || builder.rules[0].generated.inputs != uint32(compiled.InputCount()) || builder.rules[0].generated.output != output.Address || builder.rules[0].generated.carryInput != carry.Input {
		t.Fatal("generated cell did not retain sealed Plan digest/geometry")
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK || sealed == nil || slot.Schema() != sealed {
		t.Fatal("generated slot did not bind to sealed Schema")
	}
	if ordinal, ordinalOK := slot.Ordinal(); !ordinalOK || ordinal >= sealed.ruleCount() {
		t.Fatalf("generated slot ordinal = %d/%t", ordinal, ordinalOK)
	}
	shape, shapeOK := sealed.ruleShapeAt(0)
	if !shapeOK || shape.OutputKind != coldcomposition.FactorOutput || shape.Inputs != uint64(compiled.InputCount()) || shape.ReadCount != 1 || shape.CarryCount != 1 || shape.WriteCount != 1 {
		t.Fatalf("sealed generated Rule shape = %+v/%t", shape, shapeOK)
	}
}

func TestGeneratedRuleSlotMarksZeroJoinSourceAvailable(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawSource, generatedRuleLawRuleRole)
	compiled, planOK := fixture.catalog.At(0)
	if !planOK || !compiled.Present() || compiled.JoinCount() != 0 || compiled.InputCount() != 0 || compiled.OutputCount() != 1 {
		t.Fatalf("zero-join source Plan = %+v/%t", compiled, planOK)
	}
	builder := generatedRuleLawBuilder(t, fixture.catalog, false)
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("zero-join source declaration refused")
	}
	if len(builder.candidate.Rules) != 1 || len(builder.candidate.Rules[0].Reads) != 0 || len(builder.candidate.Rules[0].Carries) != 0 || len(builder.candidate.Rules[0].Writes) != 1 {
		t.Fatalf("zero-join source cold row cardinality = %+v", builder.candidate.Rules)
	}
	cell := builder.rules[0].generated
	if cell == nil || !cell.available() || cell.program.ReadCount() != 0 || cell.read.Available() {
		t.Fatal("zero-join source cell unavailable")
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK || sealed == nil {
		t.Fatal("zero-join source schema refused to seal")
	}
	shape, shapeOK := sealed.ruleShapeAt(0)
	if !shapeOK || shape.ReadCount != 0 || shape.CarryCount != 0 || shape.WriteCount != 1 || shape.Inputs != 0 {
		t.Fatalf("sealed zero-join source shape = %+v/%t", shape, shapeOK)
	}
}

func TestGeneratedRuleSlotMapsOnlyReferencedSparsePlanAxes(t *testing.T) {
	fixture := newGeneratedRuleLawMultiAxisFixture(t)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() {
		t.Fatal("sparse generated Rule Plan missing")
	}
	axes := []uint32{
		compiled.Candidate().Axis,
		compiled.Reducer().Axis,
	}
	join, joinOK := compiled.JoinAt(0)
	output, outputOK := compiled.OutputAt(0)
	if !joinOK || !outputOK {
		t.Fatal("sparse generated Rule Plan geometry missing")
	}
	axes = append(axes, join.Relation.Axis, join.Key.Axis, join.ReadAxis, output.Address.Axis, output.Destination.Axis)
	for _, axis := range axes {
		if axis != compiled.Candidate().Axis {
			t.Fatalf("fixture unexpectedly references multiple Plan axes: %v", axes)
		}
	}
	axisRow, axisOK := fixture.catalog.AxisAt(int(compiled.Candidate().Axis))
	if !axisOK {
		t.Fatal("sparse generated Rule axis missing")
	}

	// The second catalog axis is intentionally left without a Factor. Only the
	// Plan-referenced semantic is admitted into the cold schema.
	builder := NewSchema()
	if _, factorOK := DeclareFactorSlot[struct{}](builder, axisRow.Semantic); !factorOK {
		t.Fatal("sparse generated Rule Factor declaration refused")
	}
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("sparse generated Rule declaration refused")
	}
	descriptor := builder.rules[0].generated.program
	if descriptor.ReadFactor() != 0 || descriptor.OutputFactor() != 0 || descriptor.ReadAxis() != 0 || descriptor.OutputAxis() != 0 ||
		descriptor.CandidateRelation().Axis != 0 || descriptor.JoinRelation().Axis != 0 || descriptor.KeyProjection().Axis != 0 ||
		descriptor.Reducer().Axis != 0 || descriptor.OutputAddress().Axis != 0 || descriptor.DestinationProjection().Axis != 0 {
		t.Fatalf("sparse generated Rule retained Plan axes: %+v", descriptor)
	}
	if sealed, sealedOK := builder.Seal(); !sealedOK || sealed == nil {
		t.Fatal("sparse generated Rule schema refused to seal")
	}
}

func TestGeneratedRuleSlotReorderedFactorsMapBySemantic(t *testing.T) {
	fixture := newGeneratedRuleLawMultiAxisFixture(t)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() {
		t.Fatal("reordered generated Rule Plan missing")
	}
	axis, axisOK := fixture.catalog.AxisAt(int(compiled.Candidate().Axis))
	other, otherOK := fixture.catalog.AxisAt(1 - int(compiled.Candidate().Axis))
	if !axisOK || !otherOK {
		t.Fatal("reordered generated Rule axes missing")
	}
	builder := NewSchema()
	// Declare in the reverse Plan order. The canonical cold Factor order is
	// semantic, so this must still bind the generated Plan axis exactly.
	if _, factorOK := DeclareFactorSlot[struct{}](builder, other.Semantic); !factorOK {
		t.Fatal("reordered unrelated Factor declaration refused")
	}
	if _, factorOK := DeclareFactorSlot[struct{}](builder, axis.Semantic); !factorOK {
		t.Fatal("reordered referenced Factor declaration refused")
	}
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("reordered generated Rule declaration refused")
	}
	descriptor := builder.rules[0].generated.program
	if descriptor.ReadFactor() != descriptor.OutputFactor() || descriptor.ReadAxis() != descriptor.OutputAxis() ||
		descriptor.CandidateRelation().Axis != descriptor.ReadAxis() || descriptor.JoinRelation().Axis != descriptor.ReadAxis() ||
		descriptor.KeyProjection().Axis != descriptor.ReadAxis() || descriptor.Reducer().Axis != descriptor.ReadAxis() ||
		descriptor.OutputAddress().Axis != descriptor.ReadAxis() || descriptor.DestinationProjection().Axis != descriptor.ReadAxis() {
		t.Fatalf("reordered generated Rule semantic mapping lost: %+v", descriptor)
	}
	if sealed, sealedOK := builder.Seal(); !sealedOK || sealed == nil {
		t.Fatal("reordered generated Rule schema refused to seal")
	}
}

func TestGeneratedRuleSlotRejectsReferencedNonFactorAxis(t *testing.T) {
	fixture := newGeneratedRuleLawMultiAxisFixture(t)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() {
		t.Fatal("non-Factor generated Rule Plan missing")
	}
	otherIndex := 1 - int(compiled.Candidate().Axis)
	other, otherOK := fixture.catalog.AxisAt(otherIndex)
	if !otherOK {
		t.Fatal("non-Factor generated Rule axis missing")
	}
	builder := NewSchema()
	if _, factorOK := DeclareFactorSlot[struct{}](builder, other.Semantic); !factorOK {
		t.Fatal("unreferenced Factor declaration refused")
	}
	if slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0); slotOK || slot != nil || builder.phase != schemaBuilderPoisoned {
		t.Fatalf("referenced non-Factor axis was admitted: slot=%v ok=%t phase=%d", slot, slotOK, builder.phase)
	}
}

func TestGeneratedRuleSlotRejectsAbsentAndOutOfRangeRule(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole)
	for _, ordinal := range []uint32{1, 2} {
		builder := generatedRuleLawBuilder(t, fixture.catalog, false)
		if slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, ordinal); slotOK || slot != nil || builder.phase != schemaBuilderPoisoned {
			t.Fatalf("rule ordinal %d was admitted: slot=%v ok=%t phase=%d", ordinal, slot, slotOK, builder.phase)
		}
	}
}

func TestGeneratedRuleSlotRejectsUnavailableCatalogDigest(t *testing.T) {
	builder := NewSchema()
	if slot, slotOK := DeclareGeneratedRuleSlot(builder, ruleplan.Catalog{}, 0); slotOK || slot != nil || builder.phase != schemaBuilderPoisoned {
		t.Fatalf("unavailable Catalog was admitted: slot=%v ok=%t phase=%d", slot, slotOK, builder.phase)
	}
}

func TestGeneratedRuleSlotRejectsFactorDirectoryMismatch(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole)
	builder := generatedRuleLawBuilder(t, fixture.catalog, true)
	if slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0); slotOK || slot != nil || builder.phase != schemaBuilderPoisoned {
		t.Fatalf("foreign factor semantic was admitted: slot=%v ok=%t phase=%d", slot, slotOK, builder.phase)
	}
}

// TestGeneratedRuleSlotRejectsUnsupportedPlanShapes is the refusal table of the
// structural arm. A structural publication is admitted - it is the A form's own
// declaration - but only when it says what it publishes: the vector one
// candidate route instantiates and the family its branches are admitted under
// are one declaration, and a row that declares neither transports nothing
// across nothing.
func TestGeneratedRuleSlotRejectsUnsupportedPlanShapes(t *testing.T) {
	variants := []struct {
		name    string
		variant generatedRuleLawVariant
	}{
		{name: "structural-output-instantiating-nothing", variant: generatedRuleLawStructural},
	}
	for _, test := range variants {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGeneratedRuleLawFixture(t, test.variant, generatedRuleLawRuleRole)
			builder := generatedRuleLawBuilder(t, fixture.catalog, false)
			if slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0); slotOK || slot != nil || builder.phase != schemaBuilderPoisoned {
				t.Fatalf("unsupported Plan shape was admitted: slot=%v ok=%t phase=%d", slot, slotOK, builder.phase)
			}
		})
	}
}

// TestGeneratedRuleSlotAdmitsTheSealedVectorReads states that the two vector
// reads are declarations the slot seals, not shapes it refuses. A summary
// vector selected by its owner-issued predicate and a closed complete vector
// each reach the descriptor whole; the predicate is what separates them, so
// neither is admitted under the other's normal form.
func TestGeneratedRuleSlotAdmitsTheSealedVectorReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		variant generatedRuleLawVariant
		form    program.ReadForm
	}{
		{name: "summary", variant: generatedRuleLawSummary, form: program.Summary},
		{name: "complete", variant: generatedRuleLawComplete, form: program.Complete},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGeneratedRuleLawFixture(t, test.variant, generatedRuleLawRuleRole)
			builder := generatedRuleLawBuilder(t, fixture.catalog, false)
			slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
			if !slotOK || slot == nil || builder.phase == schemaBuilderPoisoned {
				t.Fatalf("sealed %s read refused: slot=%v ok=%t phase=%d", test.name, slot, slotOK, builder.phase)
			}
			sealed, sealedOK := builder.Seal()
			if !sealedOK {
				t.Fatalf("seal %s schema", test.name)
			}
			descriptor, descriptorOK := sealed.generatedProgramAt(0)
			if !descriptorOK || !descriptor.Available() {
				t.Fatalf("sealed descriptor = %+v/%t", descriptor, descriptorOK)
			}
			form, formOK := descriptor.ReadFormAt(0)
			if !formOK || form != test.form {
				t.Fatalf("descriptor read form = %d/%t, want %d", form, formOK, test.form)
			}
			denominator, denominatorOK := descriptor.ReadDenominatorAt(0)
			if !denominatorOK || !denominator.Present {
				t.Fatalf("vector read reached the descriptor without its closed denominator: %+v/%t", denominator, denominatorOK)
			}
			_, predicatePresent, predicateOK := descriptor.ReadPredicateAt(0)
			if !predicateOK || predicatePresent != (test.form == program.Summary) {
				t.Fatalf("%s read predicate present = %t/%t", test.name, predicatePresent, predicateOK)
			}
		})
	}
}

// TestGeneratedRuleSlotSealsTheDeclaredCarryDisposition states that the slot
// seals the carry the Plan compiled, not a disposition of its own. A
// transformed carry reaches the descriptor as a transform with the owner-issued
// member address the Plan resolved, normalized into the runtime Factor
// directory like every other address; sealing it as an identity carry would
// silently hand a rule with a domain transition to the identity fold.
func TestGeneratedRuleSlotSealsTheDeclaredCarryDisposition(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawTransformedCarry, generatedRuleLawRuleRole)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() {
		t.Fatalf("transformed-carry Plan = %+v/%t", compiled, compiledOK)
	}
	planCarry, planCarryOK := compiled.Carry()
	if !planCarryOK || planCarry.Mode != program.CarryTransform || !planCarry.TransformPresent {
		t.Fatalf("compiled Plan carry = %+v/%t", planCarry, planCarryOK)
	}
	builder := generatedRuleLawBuilder(t, fixture.catalog, false)
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil || builder.phase == schemaBuilderPoisoned {
		t.Fatalf("transformed carry refused: slot=%v ok=%t phase=%d", slot, slotOK, builder.phase)
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK {
		t.Fatal("seal transformed-carry schema")
	}
	descriptor, descriptorOK := sealed.generatedProgramAt(0)
	if !descriptorOK || !descriptor.Available() {
		t.Fatalf("sealed descriptor = %+v/%t", descriptor, descriptorOK)
	}
	if descriptor.CarryIdentity() {
		t.Fatal("the slot sealed a transformed carry as an identity carry")
	}
	if mode, modeOK := descriptor.CarryMode(); !modeOK || mode != program.CarryTransform {
		t.Fatalf("sealed carry mode = %v/%t", mode, modeOK)
	}
	address, present := descriptor.CarryTransform()
	if !present || address.Member != planCarry.Transform.Member {
		t.Fatalf("sealed transform address = %+v/%t, want member %d", address, present, planCarry.Transform.Member)
	}
	if descriptor.CarryInput() != int(planCarry.Input) {
		t.Fatalf("sealed carry input = %d, want %d", descriptor.CarryInput(), planCarry.Input)
	}
}

func TestGeneratedRuleSlotSealsOrderedSelectedRoute(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawRouteOutput, generatedRuleLawRuleRole)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() || compiled.JoinCount() != 2 {
		t.Fatalf("selected-route Plan = %+v/%t", compiled, compiledOK)
	}
	builder := generatedRuleLawBuilder(t, fixture.catalog, false)
	slot, slotOK := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("selected-route generated Rule declaration refused")
	}
	if len(builder.candidate.Rules) != 1 || len(builder.candidate.Rules[0].Reads) != 2 || builder.candidate.Rules[0].Writes[0].Kind != coldcomposition.WriteRoute || builder.candidate.Rules[0].Writes[0].Route != 2 {
		t.Fatalf("selected-route cold projection = %+v", builder.candidate.Rules[0])
	}
	descriptor := builder.rules[0].generated.program
	first, firstOK := descriptor.ReadAt(0)
	second, secondOK := descriptor.ReadAt(1)
	output, outputOK := descriptor.OutputAt(0)
	if !firstOK || !secondOK || !outputOK || first.Form != program.Exact || second.Form != program.Selected || !second.PredicatePresent || !second.Denominator.Present || output.Mode != program.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 1 {
		t.Fatalf("selected-route descriptor lost ordered metadata: first=%+v second=%+v output=%+v", first, second, output)
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK || sealed == nil || slot.Schema() != sealed {
		t.Fatal("selected-route generated Rule schema refused to seal")
	}
	sealedDescriptor, descriptorOK := sealed.generatedProgramAt(0)
	mode, modeOK := sealedDescriptor.OutputMode()
	if !descriptorOK || !modeOK || sealedDescriptor.ReadCount() != 2 || mode != program.ModeRoute {
		t.Fatalf("sealed selected-route descriptor = %+v/%t", sealedDescriptor, descriptorOK)
	}
}

func TestGeneratedRuleSlotUsesCatalogResolvedIdentities(t *testing.T) {
	left := newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole)
	right := newGeneratedRuleLawFixture(t, generatedRuleLawExact, schema.Key("generated-rule-law/other-rule"))
	if left.catalog.Digest() == right.catalog.Digest() {
		t.Fatal("identity-shifted Plan catalogs unexpectedly share a digest")
	}
	builder := generatedRuleLawBuilder(t, left.catalog, false)
	slot, slotOK := DeclareGeneratedRuleSlot(builder, right.catalog, 0)
	if !slotOK || slot == nil {
		t.Fatal("Catalog-resolved identity declaration refused")
	}
	compiled, planOK := right.catalog.At(0)
	if !planOK || builder.candidate.Rules[0].Key != compositionKeyOf(compiled.Semantic()) || builder.candidate.Rules[0].OperandFamily != compositionKeyOf(compiled.OperandFamily()) || builder.rules[0].generated.planDigest != right.catalog.Digest() {
		t.Fatal("generated declaration did not use Catalog-resolved identity/digest")
	}
}

func TestGeneratedRuleDescriptorRetainsExactPlanMemberAddresses(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawValueTransfer, generatedRuleLawRuleRole)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() {
		t.Fatal("exact generated Rule Plan missing")
	}
	join, joinOK := compiled.JoinAt(0)
	output, outputOK := compiled.OutputAt(0)
	carry, carryOK := compiled.Carry()
	if !joinOK || !outputOK || !carryOK {
		t.Fatal("exact generated Rule Plan geometry missing")
	}
	readScratch := compiled.Scratch()
	builder := generatedRuleLawBuilder(t, fixture.catalog, false)
	if _, declared := DeclareGeneratedRuleSlot(builder, fixture.catalog, 0); !declared {
		t.Fatal("exact generated Rule declaration refused")
	}
	sealed, sealedOK := builder.Seal()
	if !sealedOK || sealed == nil {
		t.Fatal("exact generated Rule schema did not seal")
	}
	descriptor, descriptorOK := sealed.generatedProgramAt(0)
	if !descriptorOK || !descriptor.Available() {
		t.Fatal("sealed generated descriptor missing")
	}
	if descriptor.CandidateRelation() != compiled.Candidate() ||
		descriptor.JoinRelation() != join.Relation ||
		descriptor.KeyProjection() != join.Key ||
		descriptor.Reducer() != compiled.Reducer() ||
		descriptor.OutputAddress() != output.Address ||
		descriptor.DestinationProjection() != output.Destination {
		t.Fatalf("sealed member addresses lost: candidate=%+v join=%+v key=%+v reducer=%+v output=%+v destination=%+v", descriptor.CandidateRelation(), descriptor.JoinRelation(), descriptor.KeyProjection(), descriptor.Reducer(), descriptor.OutputAddress(), descriptor.DestinationProjection())
	}
	if !descriptor.CarryIdentity() || descriptor.CarryInput() != int(carry.Input) ||
		descriptor.ReadAxis() != join.ReadAxis || descriptor.OutputAxis() != output.Address.Axis ||
		descriptor.RowCapacity() != int(readScratch.JoinCount) || descriptor.CellCapacity() != int(readScratch.OutputCount) {
		t.Fatalf("sealed scalar geometry lost: carry=%t/%d axes=%d/%d scratch=%d/%d", descriptor.CarryIdentity(), descriptor.CarryInput(), descriptor.ReadAxis(), descriptor.OutputAxis(), descriptor.RowCapacity(), descriptor.CellCapacity())
	}
}

func TestGeneratedRuleDescriptorRefusesForeignOrOutOfRangeMemberAddresses(t *testing.T) {
	fixture := newGeneratedRuleLawFixture(t, generatedRuleLawExact, generatedRuleLawRuleRole)
	compiled, compiledOK := fixture.catalog.At(0)
	if !compiledOK || !compiled.Present() {
		t.Fatal("exact generated Rule Plan missing")
	}
	join, joinOK := compiled.JoinAt(0)
	output, outputOK := compiled.OutputAt(0)
	carry, carryOK := compiled.Carry()
	if !joinOK || !outputOK || !carryOK {
		t.Fatal("exact generated Rule Plan geometry missing")
	}
	scratch := compiled.Scratch()
	valid := generated.CompiledRuleSpec{
		AxisCount: fixture.catalog.AxisCount(), InputCount: compiled.InputCount(),
		Candidate: compiled.Candidate(), Reducer: compiled.Reducer(),
		Reads: []generated.ReadPlan{{
			Input: join.Input, Factor: join.ReadAxis, Axis: join.ReadAxis,
			Relation: join.Relation, Key: join.Key,
			Addressing: join.Addressing, AddressingPresent: join.AddressingPresent,
			Form: join.ReadForm, Contract: join.ReadContract, Denominator: join.Denominator,
			PointBound:  join.PointBound,
			RowCapacity: uint16(scratch.JoinCount), CellCapacity: uint16(scratch.OutputCount),
		}},
		Outputs: []generated.OutputPlan{{
			Factor: output.Address.Axis, Axis: output.Address.Axis, Address: output.Address,
			Destination: output.Destination, Mode: output.Mode, Slot: output.Slot,
			RouteJoin: output.RouteJoin, RouteJoinPresent: output.RouteJoinPresent,
			Exact: output.Mode == program.ModeExact, Strong: output.Mode == program.ModeExact,
		}},
		Carry: &generated.CarryPlan{Input: carry.Input, Factor: output.Address.Axis, Mode: program.CarryIdentity, Identity: true},
	}
	if descriptor, descriptorOK := generated.NewPlanCompiledRule(valid); !descriptorOK || !descriptor.Available() {
		t.Fatal("valid Plan descriptor refused")
	}
	negative := []struct {
		name string
		edit func(*generated.CompiledRuleSpec)
	}{
		{name: "foreign candidate member", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Candidate.Member = ^uint32(0)
		}},
		{name: "foreign join member", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[0].Relation.Member = ^uint32(0)
		}},
		{name: "foreign key member", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[0].Key.Member = ^uint32(0)
		}},
		{name: "foreign reducer member", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Reducer.Member = ^uint32(0)
		}},
		{name: "foreign destination member", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Outputs[0].Destination.Member = ^uint32(0)
		}},
		{name: "out of range candidate axis", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Candidate.Axis = uint32(fixture.catalog.AxisCount())
		}},
		{name: "out of range output frame", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Outputs[0].Address.Frame = 1
		}},
		{name: "foreign join axis", edit: func(spec *generated.CompiledRuleSpec) {
			spec.Reads[0].Relation.Axis = 1
		}},
	}
	for _, test := range negative {
		t.Run(test.name, func(t *testing.T) {
			spec := valid
			test.edit(&spec)
			if descriptor, descriptorOK := generated.NewPlanCompiledRule(spec); descriptorOK || descriptor.Available() {
				t.Fatalf("malformed member address admitted: descriptor=%+v ok=%t", descriptor, descriptorOK)
			}
		})
	}
}

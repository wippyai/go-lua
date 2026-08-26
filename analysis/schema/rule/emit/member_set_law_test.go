package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// The declaration below restates the returnescape family's own shape as its
// own local specimen: a candidate on axis "value", a self-provided nested
// member set on that same axis, and a routed publication on axis "placement"
// whose relation derivation consumes the member set as a delivered vector.
// That two-axis routed shape is the one member-vector delivery is proven
// against in production, so these laws are built over it rather than over an
// invented shape the roster's own admission gate might refuse for reasons
// unrelated to member-vector delivery.
//
// The routed relation's Derivation.Build is the live, registered
// "placement/return-escape/routes" migration row (definition.ScheduledDeaths):
// a fresh Derivation cannot be declared here, because the roster's own
// composition (Source.Compose, via Definition.Complete) refuses an authored
// derivation the migration ledger does not know about before the emitter ever
// sees the declaration. The axis, relation key and Build symbol are therefore
// the returnescape family's own; every carrier, candidate directory and
// specimen-owned symbol beside them is local to this file.

func memberSetValueAxisRef() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
}

func memberSetPlacementAxisRef() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement"}
}

// memberSetValueDefinition is the candidate axis: the return-boundary
// candidate directory, the nested self-provided member set that hangs off it,
// and a second, ordinary candidate directory (AltCandidates) used only by the
// "foreign candidate" refusal law to give the rule a real candidate that is
// not the member set's own declared parent.
func memberSetValueDefinition() definition.Definition {
	value := memberSetValueAxisRef()
	return definition.Definition{
		Name:       "MemberSetValue",
		Axis:       "value",
		ImportPath: specimenPackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "ValueKey",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "ValueSchema", 0),
		}},
		Signature: definition.Signature{Key: "ValueKey", Fact: "ValueFact"},
		Carriers: []definition.Carrier{
			{Name: "ValueKey", Key: "carrier/value/key", Type: specimenType("ValueKey")},
			{Name: "ValueFact", Key: "carrier/value/fact", Type: specimenType("ValueFact")},
			{Name: "ReturnBoundaryCarrier", Key: "carrier/value/boundary", Type: specimenType("ReturnBoundary")},
			{Name: "ReturnBoundaryMemberCarrier", Key: "carrier/value/member", Type: specimenType("ReturnBoundaryMember")},
			{Name: "ReturnBoundaryMemberOrdinalCarrier", Key: "carrier/value/member-ordinal", Type: definition.GoType{Name: "int"}},
		},
		Enumerations: []definition.Enumeration{{
			// The POSITIONS a boundary names, not the values at them. It is
			// what an indexed delivery is indexed by.
			Name: "MemberOrdinals", Over: "ReturnBoundaryCarrier", Item: "ReturnBoundaryMemberOrdinalCarrier",
			Count: specimenMethod("MemberOrdinalCount", "ReturnBoundary", -1),
			At:    specimenMethod("MemberOrdinalAt", "ReturnBoundary", 0),
		}},
		Relations: []definition.Relation{
			{
				Name: "ReturnBoundaryCandidates", Key: "value/return-boundary/candidates", Subject: "ReturnBoundaryCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: value, Member: "value/return-boundary/candidates"}),
				CandidateResolver: specimenMethod("ReturnBoundary", "ValueSchema", 0),
				CandidateOrdinal:  specimenMethod("ReturnBoundaryOrdinal", "ValueSchema", 0),
				CandidateAt:       specimenMethod("ReturnBoundaryAt", "ValueSchema", 0),
			},
			{
				// A self-provided nested member set: its own directory
				// densifies its rows, and it nests under the candidate
				// directory above by restating that relation as its Parent.
				Name: "ReturnBoundaryMembers", Key: "value/return-boundary/members", Subject: "ReturnBoundaryMemberCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: value, Member: "value/return-boundary/members"}),
				CandidateResolver: specimenMethod("ReturnBoundaryMemberForOccurrence", "ValueSchema", 0),
				CandidateOrdinal:  specimenMethod("ReturnBoundaryMemberOrdinal", "ValueSchema", 0),
				CandidateAt:       specimenMethod("ReturnBoundaryMemberAt", "ValueSchema", 0),
				MemberParent:      member.RelationRef{Axis: value, Member: "value/return-boundary/candidates"},
				MemberOrdinal:     "ReturnBoundaryMemberOrdinalCarrier",
				MemberCount:       specimenMethod("MemberCount", "ReturnBoundary", 0),
				MemberAt:          specimenMethod("MemberAt", "ReturnBoundary", 0),
			},
			{
				// An ordinary candidate directory that is not the member
				// set's parent. Only the "foreign candidate" refusal law
				// below names this as the rule's Candidate.
				Name: "AltCandidates", Key: "value/alt/candidates", Subject: "ReturnBoundaryCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: value, Member: "value/alt/candidates"}),
				CandidateResolver: specimenMethod("AltCandidate", "ValueSchema", 0),
				CandidateOrdinal:  specimenMethod("AltCandidateOrdinal", "ValueSchema", 0),
				CandidateAt:       specimenMethod("AltCandidateAt", "ValueSchema", 0),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "ReturnBoundaryMemberKey", Key: "value/return-boundary/member-key", Relation: "ReturnBoundaryMembers",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: value, Member: "value/return-boundary/members"}),
				Role:              member.Key, Result: "ValueKey",
				Accessor: specimenMethod("Coordinate", "ReturnBoundaryMember", -1),
			},
		},
	}
}

// memberSetPlacementDefinition is the write axis: the routed derivation
// relation the returnescape family emits, and a second self-provided
// candidate/member pair (SelfCandidates/SelfMembers) used only by the
// "written axis" refusal law - a member set the write axis itself hosts has
// no foreign handle to be sealed through.
func memberSetPlacementDefinition() definition.Definition {
	placement := memberSetPlacementAxisRef()
	value := memberSetValueAxisRef()
	provider := member.RelationRef{Axis: value, Member: "value/return-boundary/candidates"}
	return definition.Definition{
		Name:       "MemberSetPlacement",
		Axis:       "placement",
		ImportPath: specimenPackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "PlacementKey",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "PlacementSchema", 0),
		}},
		Signature: definition.Signature{Key: "PlacementKey", Fact: "PlacementFact"},
		Carriers: []definition.Carrier{
			{Name: "PlacementKey", Key: "carrier/placement/key", Type: specimenType("PlacementKey")},
			{Name: "PlacementFact", Key: "carrier/placement/fact", Type: specimenType("PlacementFact")},
			{Name: "RouteTagCarrier", Key: "carrier/placement/route-tag", Type: definition.GoType{Name: "uint64"}},
			{Name: "RouteCarrier", Key: "carrier/placement/route", Type: specimenType("Route")},
			{Name: "ReturnBoundaryCarrier", Key: "carrier/placement/boundary", Type: specimenType("ReturnBoundary")},
			{Name: "ValueFactCarrier", Key: "carrier/placement/value-fact", Type: specimenType("ValueFact")},
			{Name: "SelfOrdinalCarrier", Key: "carrier/placement/self-ordinal", Type: definition.GoType{Name: "int"}},
		},
		Relations: []definition.Relation{
			{
				Name: "Routes", Key: "placement/return-escape/routes", Subject: "RouteCarrier",
				// Candidate then the delivered member vector, in the
				// derivation's own declared input order.
				Inputs: []definition.RelationInput{
					{Carrier: "ReturnBoundaryCarrier"},
					{Carrier: "ValueFactCarrier", Many: true, Form: member.ReadFormSummary},
				},
				CandidateProvider: member.AxisRelationCandidate(provider),
				// Stated in the DECLARED form over the delivery of its own
				// many-valued input, which is the shape the relation this
				// specimen mirrors takes: the vector is what the invocation is
				// handed, its cells are admitted by the owner of the values
				// they carry, and one admitted value resolves to one route.
				Derivation: memberSetDeliveryDerivation(),
			},
			{
				Name: "SelfCandidates", Key: "placement/self/candidates", Subject: "RouteCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: placement, Member: "placement/self/candidates"}),
				CandidateResolver: specimenMethod("SelfCandidate", "PlacementSchema", 0),
				CandidateOrdinal:  specimenMethod("SelfCandidateOrdinal", "PlacementSchema", 0),
				CandidateAt:       specimenMethod("SelfCandidateAt", "PlacementSchema", 0),
			},
			{
				Name: "SelfMembers", Key: "placement/self/members", Subject: "RouteCarrier",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: placement, Member: "placement/self/members"}),
				CandidateResolver: specimenMethod("SelfMemberForOccurrence", "PlacementSchema", 0),
				CandidateOrdinal:  specimenMethod("SelfMemberOrdinal", "PlacementSchema", 0),
				CandidateAt:       specimenMethod("SelfMemberDirectoryAt", "PlacementSchema", 0),
				MemberParent:      member.RelationRef{Axis: placement, Member: "placement/self/candidates"},
				MemberOrdinal:     "SelfOrdinalCarrier",
				MemberCount:       specimenMethod("SelfMemberCount", "Route", 0),
				MemberAt:          specimenMethod("SelfMemberAt", "Route", 0),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "RouteKey", Key: "placement/return-escape/route-key", Relation: "Routes",
				Role: member.Key, Result: "PlacementKey",
				Accessor:          specimenMethod("Key", "Route", -1),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				// The join key and the routed output destination are
				// different semantic roles even when they use the same
				// owner-issued coordinate. Keeping both declarations makes
				// the output's destination carrier explicit to the emitter.
				Name: "RouteDestination", Key: "placement/return-escape/route-destination", Relation: "Routes",
				Role: member.Destination, Result: "PlacementKey",
				Accessor:          specimenMethod("Destination", "Route", -1),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				Name: "RouteTag", Key: "placement/return-escape/route-tag", Relation: "Routes",
				Role: member.Predicate, Result: "RouteTagCarrier",
				Accessor:          specimenMethod("Predicate", "Route", -1),
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				Name: "SelfMemberKey", Key: "placement/self/member-key", Relation: "SelfMembers",
				Role: member.Key, Result: "PlacementKey",
				Accessor:          specimenMethod("Coordinate", "Route", -1),
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: placement, Member: "placement/self/members"}),
			},
		},
	}
}

func memberSetPlacementContribution() definition.Contribution {
	placement := memberSetPlacementAxisRef()
	return definition.Contribution{
		Axis: "placement",
		Rule: memberSetRuleKey,
		Reducers: []definition.Reducer{{
			Name: "RouteReducer", Key: "placement/return-escape/reducer",
			Inputs: []definition.ReducerInput{{
				Axis: placement, Carrier: "PlacementFact",
				Form: member.ReadFormSelected, Multiplicity: member.MultiplicityOne,
				Tag: "RouteTagCarrier",
			}},
			Outputs:        []definition.ReducerOutput{{Axis: placement, Carrier: "PlacementFact"}},
			Implementation: definition.GoSymbol{PackagePath: specimenPackage, Name: "RouteFold", ResultIndex: 0},
		}},
	}
}

func memberSetRoster(t testing.TB) definition.Roster {
	t.Helper()
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "membersetvalue", Name: "membersetvalue", Base: memberSetValueDefinition()},
		definition.Source{
			Package: "membersetplacement", Name: "membersetplacement",
			Base:          memberSetPlacementDefinition(),
			Contributions: []definition.Contribution{memberSetPlacementContribution()},
		},
	)
	if !rosterOK {
		t.Fatal("member-set roster is not admissible")
	}
	return roster
}

const memberSetRuleKey schema.Key = "specimen-member-route"

func memberSetSpec() rule.Spec {
	value := memberSetValueAxisRef()
	placement := memberSetPlacementAxisRef()
	return rule.Spec{
		Key:      memberSetRuleKey,
		Writes:   "placement",
		Owner:    "placement",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/member-route", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/member-route",
		Roles:    []schema.Key{"semantic/operand/member-route"},
		Program: program.Program{
			OperandRole: "semantic/operand/member-route",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: value, Member: "value/return-boundary/candidates"}),
			Joins: []program.JoinDecl{
				{
					// A self-provided nested member set: its census
					// densifies through its own directory, so the join
					// restates the relation's own Parent instead of naming a
					// Predicate.
					Sources:  []program.SourceRef{program.CandidateSource()},
					Relation: member.RelationRef{Axis: value, Member: "value/return-boundary/members"},
					Key:      member.ProjectionRef{Axis: value, Member: "value/return-boundary/member-key"},
					Parent:   member.RelationRef{Axis: value, Member: "value/return-boundary/candidates"},
					Read: program.ReadDecl{
						PointBound: program.PointBound, Input: 0,
						Axis: program.AxisRef(value), Form: program.Summary,
						Contract: program.ReadContract{
							Order: program.OrderCanonical, Sparse: program.SparseDefault,
							OnOpaque: program.OnOpaquePropagateAuthenticated, Multiplicity: program.MultiplicityMany,
						},
					},
				},
				{
					// The routed join: candidate then the delivered member
					// vector (join 0), the derivation's own declared input
					// order.
					Sources:   []program.SourceRef{program.CandidateSource(), program.PriorSource(0)},
					Relation:  member.RelationRef{Axis: placement, Member: "placement/return-escape/routes"},
					Key:       member.ProjectionRef{Axis: placement, Member: "placement/return-escape/route-key"},
					Predicate: member.ProjectionRef{Axis: placement, Member: "placement/return-escape/route-tag"},
					Read: program.ReadDecl{
						PointBound: program.PointBound, Input: 0,
						Axis: program.AxisRef(placement), Form: program.Selected,
						Contract: program.ReadContract{
							Order: program.OrderCanonical, Sparse: program.SparseDefault,
							OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
						},
					},
				},
			},
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: placement, Member: "placement/return-escape/reducer"},
				Inputs:  []program.JoinRef{1},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: placement, Key: "placement/facts"},
					Destination: member.ProjectionRef{Axis: placement, Member: "placement/return-escape/route-destination"},
					Mode:        program.ModeRoute, ValueSlot: 0,
					RouteJoin: 1, RouteJoinPresent: true,
				}},
			},
		},
	}
}

func memberSetTarget() Target {
	return Target{PackagePath: "example/rule/memberroute", PackageName: "memberroute", Spec: memberSetSpec()}
}

func renderMemberSetTarget(t testing.TB, target Target) string {
	t.Helper()
	source, err := Render(target, memberSetRoster(t))
	if err != nil {
		t.Fatalf("member-set declaration did not emit: %v", err)
	}
	return string(source)
}

// TestMemberSetJoinDeliversOneVectorFromOrdinalSealedReads is the acceptance
// law for member-vector delivery. A routed output publishes through a
// selected join whose relation derivation consumes a Summary join over a
// nested member set, the join restating the relation's own Parent. The
// emitted family's installer seals one exact read per member AT THE
// COORDINATE THE PLAN ROW CARRIES - the engine enumerated the set at the row
// this read is addressed by and lowered every coordinate here - the worker
// views the filled cells as one execution.MemberVector, and the derivation's
// Build call is handed that vector rather than the underlying cell slice.
//
// The installer must not reach the owner's MemberCount/MemberAt at all. Those
// are the sealed directory's own enumeration, the engine is what runs them,
// and an installer running them again is a second enumeration of one set that
// is only expressible when the set happens to hang off its own candidate.
func TestMemberSetJoinDeliversOneVectorFromOrdinalSealedReads(t *testing.T) {
	source := renderMemberSetTarget(t, memberSetTarget())

	installer, installerFound := typeBody(source, "familyInstaller")
	if !installerFound {
		t.Fatalf("the emitted source declares no installer type:\n%s", source)
	}
	_ = installer

	if !strings.Contains(source, "planRow.MemberCount(0)") {
		t.Fatalf("the installer does not take the member set's width from the plan row the engine lowered it onto:\n%s", source)
	}
	if !strings.Contains(source, "planRow.MemberAt(0, index)") {
		t.Fatalf("the installer does not read each member coordinate off the plan row:\n%s", source)
	}
	if strings.Contains(source, "candidate.MemberCount()") || strings.Contains(source, "candidate.MemberAt(index)") {
		t.Fatalf("the installer enumerates the member set a second time, off its own candidate:\n%s", source)
	}
	if !strings.Contains(source, "execution.ForeignMemberExactRead[") {
		t.Fatalf("the installer seals no member read through the foreign handle's ForeignMemberExactRead:\n%s", source)
	}
	if !strings.Contains(source, "execution.NewMemberVector(read0Cells)") {
		t.Fatalf("the worker does not view the filled cell buffer through execution.NewMemberVector:\n%s", source)
	}

	call, found := callArguments(source, "deriveDerived1Rows")
	if !found {
		t.Fatalf("the emitted family makes no call to the construction its declaration derives:\n%s", source)
	}
	sawVector := false
	for _, argument := range call {
		if argument == "read0Cells" {
			t.Fatalf("the derivation is handed the raw cell slice %q rather than the sealed vector:\n%s", argument, source)
		}
		if argument == "read0Vector" {
			sawVector = true
		}
	}
	if !sawVector {
		t.Fatalf("the derivation call %v does not carry the sealed member vector:\n%s", call, source)
	}
	if !strings.Contains(source, "Reduce(routeCoordinate ") {
		t.Fatalf("the routed reducer does not receive the declared destination carrier:\n%s", source)
	}
	if !strings.Contains(source, "routeCoordinate specimen.PlacementKey") || strings.Contains(source, "routes []uint32") {
		t.Fatalf("the emitted route carrier was collapsed into the engine dense type:\n%s", source)
	}
	if !strings.Contains(source, "destinationDense") || !strings.Contains(source, "RouteMember(uint32(dense), uint32(destinationDense)") || !strings.Contains(source, "routes[index] = ") || !strings.Contains(source, "cells, members, routes,") {
		t.Fatalf("the route carrier is not paired with the observed member vector:\n%s", source)
	}
}

// TestAMemberSetJoinIsRefusedByName proves, one subtest per clause, that each
// shape deriveMemberSet has no form for is refused by name - the rule, the
// clause, and why - exactly as TestAnUnexpressibleDeclarationIsRefusedByName
// proves it for the rest of the declaration.
//
// deriveMemberSet states a fifth clause - "a member set with no census" - for
// a nested relation declaring no MemberCount/MemberAt. It has no probe here:
// definition.Relation.memberSetComplete, invoked from Definition.Complete via
// Source.Compose - which this package's own composeRoster runs before any
// join is derived - already refuses admitting a relation that declares
// MemberParent without also declaring MemberCount, MemberAt and
// MemberOrdinal together. No roster this test file can build ever reaches
// deriveMemberSet holding such a relation: the refusal happens one step
// earlier, named "a member source that does not compose" - a different,
// correctly-named refusal for a different fact, not this one.
// TestAMemberSetJoinMayNestUnderAForeignCandidate is the clause that stopped
// being a refusal.
//
// The installer used to enumerate the member set off the rule's OWN candidate
// row, so a set nesting under another directory's row had no expression: there
// was no candidate of the right type to enumerate with. The engine enumerates
// it now - at the row the read is addressed by, which a correspondence lets it
// resolve in a foreign directory - and lowers every member's coordinate onto
// the plan row, so the emitted seal reads coordinates and never asks whose
// candidate they hang off. That is the whole reason a rule may now fold a
// member set of a call it did not issue.
func TestAMemberSetJoinMayNestUnderAForeignCandidate(t *testing.T) {
	spec := memberSetSpec()
	spec.Program.Candidate = member.AxisRelationCandidate(member.RelationRef{
		Axis: memberSetValueAxisRef(), Member: "value/alt/candidates",
	})
	target := memberSetTarget()
	target.Spec = spec
	source, err := Render(target, memberSetRoster(t))
	if err != nil {
		t.Fatalf("a member set nesting under a foreign candidate was refused: %v", err)
	}
	emitted := string(source)
	if !strings.Contains(emitted, "planRow.MemberCount(0)") || !strings.Contains(emitted, "planRow.MemberAt(0, index)") {
		t.Fatalf("the emitted seal does not read the lowered member coordinates:\n%s", emitted)
	}
	if strings.Contains(emitted, "candidate.MemberCount()") || strings.Contains(emitted, "candidate.MemberAt(") {
		t.Fatalf("the emitted seal still enumerates the set from its own candidate:\n%s", emitted)
	}
}

func TestAMemberSetJoinIsRefusedByName(t *testing.T) {
	for _, probe := range []struct {
		name   string
		mutate func(*rule.Spec)
		clause string
	}{
		{
			name: "a non-Summary read over a nested member set",
			mutate: func(spec *rule.Spec) {
				spec.Program.Joins[0].Read.Form = program.Exact
			},
			clause: "read over a nested member set",
		},
		{
			name: "a Parent restatement naming a relation the resolved relation does not declare as its parent",
			mutate: func(spec *rule.Spec) {
				spec.Program.Joins[0].Parent = member.RelationRef{
					Axis: memberSetValueAxisRef(), Member: "value/return-boundary/members",
				}
			},
			clause: "parent restatement disagrees with its relation",
		},
		{
			name: "a member set on the axis the rule writes",
			mutate: func(spec *rule.Spec) {
				placement := memberSetPlacementAxisRef()
				spec.Program.Candidate = member.AxisRelationCandidate(member.RelationRef{Axis: placement, Member: "placement/self/candidates"})
				spec.Program.Joins[0].Relation = member.RelationRef{Axis: placement, Member: "placement/self/members"}
				spec.Program.Joins[0].Key = member.ProjectionRef{Axis: placement, Member: "placement/self/member-key"}
				spec.Program.Joins[0].Parent = member.RelationRef{Axis: placement, Member: "placement/self/candidates"}
				spec.Program.Joins[0].Read.Axis = program.AxisRef(placement)
			},
			clause: "member set of the written axis",
		},
	} {
		t.Run(probe.name, func(t *testing.T) {
			spec := memberSetSpec()
			probe.mutate(&spec)
			target := memberSetTarget()
			target.Spec = spec
			source, err := Render(target, memberSetRoster(t))
			if err == nil {
				t.Fatalf("an unexpressible member-set declaration emitted a family:\n%s", source)
			}
			refusal, named := err.(Unexpressible)
			if !named {
				t.Fatalf("refusal is not named as unexpressible: %v", err)
			}
			if refusal.Rule != spec.Key {
				t.Fatalf("refusal names rule %q, the declaration is %q", string(refusal.Rule), string(spec.Key))
			}
			if !strings.Contains(refusal.Clause, probe.clause) {
				t.Fatalf("refusal clause is %q, want it to name %q", refusal.Clause, probe.clause)
			}
			if refusal.Detail == "" {
				t.Fatal("refusal names a clause with no reason")
			}
		})
	}
}

// memberSetDeliveryRoster admits the member-set specimen with its route
// relation stated in the DECLARED form over the delivery of its own
// many-valued input.
// memberSetDeliveryDerivation is the specimen route relation's declared
// derivation: the whole delivery of its own many-valued input, admitted one
// cell at a time by the owner of the values it carries.
func memberSetDeliveryDerivation() definition.RelationDerivation {
	return definition.RelationDerivation{
		StaticAxes: []schema.EntryReference{memberSetPlacementAxisRef(), memberSetValueAxisRef()},
		Source: []definition.EnumerationRef{{
			Axis:     memberSetValueAxisRef(),
			Delivery: 2,
			Admit: definition.GoSymbol{
				PackagePath: specimenPackage, Name: "AdmitCell",
				Receiver: definition.GoType{PackagePath: specimenPackage, Name: "ValueSchema"}, ResultIndex: 0,
			},
		}},
		Resolve:     definition.GoSymbol{PackagePath: specimenPackage, Name: "ResolveRoute", ResultIndex: 0},
		InlineWidth: 4,
	}
}

func memberSetDeliveryRoster(t testing.TB, amend func(*definition.RelationDerivation)) definition.Roster {
	t.Helper()
	base := memberSetPlacementDefinition()
	derivation := memberSetDeliveryDerivation()
	amend(&derivation)
	for index := range base.Relations {
		if base.Relations[index].Name == "Routes" {
			base.Relations[index].Derivation = derivation
		}
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "membersetvalue", Name: "membersetvalue", Base: memberSetValueDefinition()},
		definition.Source{
			Package: "membersetplacement", Name: "membersetplacement",
			Base:          base,
			Contributions: []definition.Contribution{memberSetPlacementContribution()},
		},
	)
	if !rosterOK {
		t.Fatal("the amended member-set roster is not admissible")
	}
	return roster
}

// TestADerivedMemberSetMayBeReadOutOfItsOwnInputsDelivery states the source
// level that is not an axis's enumeration.
//
// A many-valued input arrives as a whole delivery whose census and accessor
// belong to the execution vocabulary - the emitter chose that view when it
// instantiated it - so the declaration names the INPUT and nothing else about
// how it is walked. What it does name is the owner's judgment over one cell,
// because a cell is not a value: it carries whether that coordinate holds one
// and whether the index names a cell at all, and only the owner may say which
// of those it admits.
func TestADerivedMemberSetMayBeReadOutOfItsOwnInputsDelivery(t *testing.T) {
	source, err := Render(memberSetTarget(), memberSetDeliveryRoster(t, func(*definition.RelationDerivation) {}))
	if err != nil {
		t.Fatalf("a derivation over its own input's delivery did not emit: %v", err)
	}
	build, found := functionBody(string(source), "deriveDerived1Rows")
	if !found {
		t.Fatalf("the emitted construction has no Build:\n%s", source)
	}
	if !strings.Contains(build, "count0 := given1.Count()") {
		t.Fatalf("the delivery is not censused by the view it arrives as:\n%s", build)
	}
	if !strings.Contains(build, "cell0, cell0Present, cell0Available := given1.At(cursor0)") {
		t.Fatalf("the delivery's cells are not read whole:\n%s", build)
	}
	if !strings.Contains(build, "item0, item0OK := valueSchema.AdmitCell(cell0, cell0Present, cell0Available)") {
		t.Fatalf("a cell is not admitted by its owner's judgment:\n%s", build)
	}
}

// TestADeliverySourceIsRefusedWhereItCannotBeWalked fences the level at both
// ends. An input that delivers one value has no cells, and a delivery composed
// under another level would be read out of an item - but an input is what the
// invocation is HANDED, never something read out of one.
func TestADeliverySourceIsRefusedWhereItCannotBeWalked(t *testing.T) {
	single := memberSetDeliveryRoster(t, func(derivation *definition.RelationDerivation) {
		derivation.Source[0].Delivery = 1
	})
	if source, err := Render(memberSetTarget(), single); err == nil {
		t.Fatalf("an input delivering one value was read as a delivery:\n%s", source)
	}
	nested := memberSetDeliveryRoster(t, func(derivation *definition.RelationDerivation) {
		derivation.Source = append([]definition.EnumerationRef{
			{Axis: memberSetValueAxisRef(), Name: "Members"},
		}, derivation.Source...)
	})
	if source, err := Render(memberSetTarget(), nested); err == nil {
		t.Fatalf("a delivery was composed under another level:\n%s", source)
	}
}

// TestADeliveryMayBeIndexedByTheLevelBeforeIt is the second half of a nested
// derivation over a vector: the outer level names POSITIONS and the inner one
// reads the delivery at them.
//
// Walking the delivery instead would answer over every cell the read carries,
// and selecting from that answer afterwards would be a second addressing of
// rows the owner already addressed by position. So an indexed level reads one
// cell, in the cadence the level before it opened, and opens none of its own.
func TestADeliveryMayBeIndexedByTheLevelBeforeIt(t *testing.T) {
	source, err := Render(memberSetTarget(), memberSetDeliveryRoster(t, func(derivation *definition.RelationDerivation) {
		derivation.Source = []definition.EnumerationRef{
			{Axis: memberSetValueAxisRef(), Name: "MemberOrdinals"},
			indexedDeliveryLevel(),
		}
	}))
	if err != nil {
		t.Fatalf("a derivation indexing its own delivery did not emit: %v", err)
	}
	build, found := functionBody(string(source), "deriveDerived1Rows")
	if !found {
		t.Fatalf("the emitted construction has no Build:\n%s", source)
	}
	if !strings.Contains(build, "count0 := given0.MemberOrdinalCount()") {
		t.Fatalf("the outer level does not enumerate the positions:\n%s", build)
	}
	if !strings.Contains(build, "cell1, cell1Present, cell1Available := given1.At(int(item0))") {
		t.Fatalf("the delivery is not read at the ordinal the level before it yields:\n%s", build)
	}
	// One cadence, not two: the indexed level opens no loop of its own.
	if occurrences := strings.Count(build, "for cursor"); occurrences != 1 {
		t.Fatalf("the construction opens %d cadences over a one-level walk:\n%s", occurrences, build)
	}
}

// TestAnIndexedDeliveryIsRefusedWhereItHasNoOrdinal fences the level at both
// ends. Nothing precedes the outer level, so an indexed one there has no
// ordinal to read at; and a position into a vector is an integer, so a level
// yielding anything else names no cell.
func TestAnIndexedDeliveryIsRefusedWhereItHasNoOrdinal(t *testing.T) {
	outermost := memberSetDeliveryRoster(t, func(derivation *definition.RelationDerivation) {
		derivation.Source = []definition.EnumerationRef{indexedDeliveryLevel()}
	})
	if source, err := Render(memberSetTarget(), outermost); err == nil {
		t.Fatalf("an indexed delivery was read with nothing before it:\n%s", source)
	}
	notAnOrdinal := memberSetDeliveryRoster(t, func(derivation *definition.RelationDerivation) {
		derivation.Source = []definition.EnumerationRef{
			{Axis: memberSetValueAxisRef(), Name: "Members"},
			indexedDeliveryLevel(),
		}
	})
	if source, err := Render(memberSetTarget(), notAnOrdinal); err == nil {
		t.Fatalf("a delivery was indexed at something that is not an ordinal:\n%s", source)
	}
}

// indexedDeliveryLevel is the specimen's inner level: input 1's delivery, read
// at the ordinal the level before it yields.
func indexedDeliveryLevel() definition.EnumerationRef {
	level := memberSetDeliveryDerivation().Source[0]
	level.Indexed = true
	return level
}

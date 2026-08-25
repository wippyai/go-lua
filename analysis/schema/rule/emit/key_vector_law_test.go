package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// A whole-vector read needs a denominator, and there are two places one can
// come from. A nested member set hangs off a parent row of the read's own axis
// and is enumerated there. A constructor's operand vector has no such
// directory: the row that knows which coordinates it consumes belongs to
// another axis, and the axis those coordinates belong to issued them one at a
// time and groups them nowhere. That row publishes the span itself.
//
// These laws are built over the member-set specimen with one relation added:
// the boundary directory publishes the ordered coordinates of its own row, and
// a second value-axis relation is read over exactly those.

const keyVectorRelation schema.Key = "value/return-boundary/coordinates"

// keyVectorRoster is the member-set roster whose boundary directory publishes
// a key vector, plus the relation read over it.
func keyVectorRoster(t testing.TB) definition.Roster {
	t.Helper()
	value := memberSetValueDefinition()
	provider := member.AxisRelationCandidate(member.RelationRef{
		Axis: memberSetValueAxisRef(), Member: "value/return-boundary/candidates",
	})
	for index := range value.Relations {
		if value.Relations[index].Key != "value/return-boundary/candidates" {
			continue
		}
		value.Relations[index].KeyVectorCount = specimenMethod("CoordinateCount", "ReturnBoundary", 0)
		value.Relations[index].KeyVectorAt = specimenMethod("CoordinateAt", "ReturnBoundary", 0)
	}
	value.Relations = append(value.Relations, definition.Relation{
		Name: "ReturnBoundaryCoordinates", Key: keyVectorRelation, Subject: "ValueFact",
		Inputs:            []definition.RelationInput{{Carrier: "ReturnBoundaryCarrier"}},
		CandidateProvider: provider,
	})
	value.Projections = append(value.Projections, definition.Projection{
		Name: "ReturnBoundaryCoordinateKey", Key: "value/return-boundary/coordinate-key", Relation: "ReturnBoundaryCoordinates",
		CandidateProvider: provider, Role: member.Key, Result: "ValueKey",
		Accessor: specimenMethod("Coordinate", "ValueFact", -1),
	})
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "membersetvalue", Name: "membersetvalue", Base: value},
		definition.Source{
			Package: "membersetplacement", Name: "membersetplacement",
			Base:          memberSetPlacementDefinition(),
			Contributions: []definition.Contribution{memberSetPlacementContribution()},
		},
	)
	if !rosterOK {
		t.Fatal("key-vector roster is not admissible")
	}
	return roster
}

// keyVectorSpec reads join 0 over the span the candidate publishes instead of
// over the nested member set. Everything else about the declaration is the
// member-set specimen's.
func keyVectorSpec() rule.Spec {
	spec := memberSetSpec()
	value := memberSetValueAxisRef()
	spec.Program.Joins[0].Relation = member.RelationRef{Axis: value, Member: keyVectorRelation}
	spec.Program.Joins[0].Key = member.ProjectionRef{Axis: value, Member: "value/return-boundary/coordinate-key"}
	spec.Program.Joins[0].Parent = member.RelationRef{}
	spec.Program.Joins[0].KeyVector = member.RelationRef{Axis: value, Member: "value/return-boundary/candidates"}
	return spec
}

func renderKeyVector(t testing.TB, spec rule.Spec) (string, error) {
	t.Helper()
	target := memberSetTarget()
	target.Spec = spec
	source, err := Render(target, keyVectorRoster(t))
	return string(source), err
}

// TestAPublishedKeyVectorIsDeliveredAsOneVectorOverItsOwnCoordinates is the
// acceptance law. The span comes from the candidate row, the engine lowers the
// coordinates it published onto the plan row, and the emitted family seals one
// exact read per coordinate through the read axis's foreign handle and views
// the filled cells as the one vector its reader is declared to receive.
//
// It is the same delivery a nested member set gets, and deliberately so: by
// the time cells are read the two addressings have answered the same fact -
// the ordered coordinates this read is taken over - and a second delivery
// shape for the second addressing would be a second thing to keep correct.
func TestAPublishedKeyVectorIsDeliveredAsOneVectorOverItsOwnCoordinates(t *testing.T) {
	source, err := renderKeyVector(t, keyVectorSpec())
	if err != nil {
		t.Fatalf("a read over a published key vector did not emit: %v", err)
	}
	if !strings.Contains(source, "planRow.MemberCount(0)") || !strings.Contains(source, "planRow.MemberAt(0, index)") {
		t.Fatalf("the installer does not take the span the engine lowered onto the plan row:\n%s", source)
	}
	if !strings.Contains(source, "execution.ForeignMemberExactRead[") {
		t.Fatalf("the installer seals no coordinate through the read axis's foreign handle:\n%s", source)
	}
	if !strings.Contains(source, "execution.NewMemberVector(read0Cells)") {
		t.Fatalf("the worker does not view the filled cells as one vector:\n%s", source)
	}
	call, found := callArguments(source, "DeriveReturnRoutes")
	if !found {
		t.Fatalf("the emitted family makes no call to the declared derivation:\n%s", source)
	}
	delivered := false
	for _, argument := range call {
		if argument == "read0Cells" {
			t.Fatalf("the derivation is handed the raw cell slice rather than the sealed vector:\n%s", source)
		}
		if argument == "read0Vector" {
			delivered = true
		}
	}
	if !delivered {
		t.Fatalf("the derivation call %v does not carry the sealed vector:\n%s", call, source)
	}
}

// TestAWholeVectorReadIsRefusedWithoutASpan states the other half: a Summary
// read that names neither addressing has no denominator at all, and the
// emitter says so rather than opening a cursor over a partition nothing
// bounded.
func TestAWholeVectorReadIsRefusedWithoutASpan(t *testing.T) {
	spec := keyVectorSpec()
	spec.Program.Joins[0].KeyVector = member.RelationRef{}
	source, err := renderKeyVector(t, spec)
	if err == nil {
		t.Fatalf("a whole-vector read with no span emitted a family:\n%s", source)
	}
	refusal, named := err.(Unexpressible)
	if !named || !strings.Contains(refusal.Clause, "summary read over a Factor cursor") {
		t.Fatalf("refusal clause is %q", refusalClause(err))
	}
}

// TestAKeyVectorIsIssuedByTheDirectoryItsReadIsJoinedFrom is the ownership
// fence. The span belongs to the candidate row the read is joined from; a read
// that names another directory is borrowing a denominator it has no claim to,
// and the two would agree only by coincidence of width.
func TestAKeyVectorIsIssuedByTheDirectoryItsReadIsJoinedFrom(t *testing.T) {
	spec := keyVectorSpec()
	spec.Program.Joins[0].KeyVector = member.RelationRef{
		Axis: memberSetValueAxisRef(), Member: "value/alt/candidates",
	}
	source, err := renderKeyVector(t, spec)
	if err == nil {
		t.Fatalf("a read took its span from a directory it is not joined from:\n%s", source)
	}
	refusal, named := err.(Unexpressible)
	if !named || !strings.Contains(refusal.Clause, "key vector is not published by the directory it is joined from") {
		t.Fatalf("refusal clause is %q", refusalClause(err))
	}
}

// TestAKeyVectorOfTheWrittenAxisIsRefused closes the same fence on the other
// side. A published span is sealed one exact read per coordinate through the
// read axis's FOREIGN handle, and a rule's own Factor publishes no such
// handle, so a rule cannot read a span of the axis it writes this way.
func TestAKeyVectorOfTheWrittenAxisIsRefused(t *testing.T) {
	spec := keyVectorSpec()
	placement := memberSetPlacementAxisRef()
	spec.Program.Candidate = member.AxisRelationCandidate(member.RelationRef{Axis: placement, Member: "placement/self/candidates"})
	spec.Program.Joins[0].Relation = member.RelationRef{Axis: placement, Member: "placement/self/members"}
	spec.Program.Joins[0].Key = member.ProjectionRef{Axis: placement, Member: "placement/self/member-key"}
	spec.Program.Joins[0].KeyVector = member.RelationRef{Axis: placement, Member: "placement/self/candidates"}
	spec.Program.Joins[0].Read.Axis = program.AxisRef(placement)
	source, err := renderKeyVector(t, spec)
	if err == nil {
		t.Fatalf("a rule read a published span of the axis it writes:\n%s", source)
	}
	if _, named := err.(Unexpressible); !named {
		t.Fatalf("refusal is not named as unexpressible: %v", err)
	}
}

func refusalClause(err error) string {
	if refusal, named := err.(Unexpressible); named {
		return refusal.Clause
	}
	return err.Error()
}

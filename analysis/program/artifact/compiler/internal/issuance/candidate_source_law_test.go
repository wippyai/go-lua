package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// livenessPublication seals count subject-liveness rows with their executable
// occurrence views and finish geometry, in emission order.
func livenessPublication(t *testing.T, count int) (programissuance.Rows, schemaissuance.Table) {
	t.Helper()
	table := scheduleTable(t)
	builder := programissuance.NewBuilder()
	publication := &programpublication.Publication{}
	for index := 0; index < count; index++ {
		call, subject := lawID(byte(60+index)), lawID(byte(80+index))
		id, idOK := lifecycle.SubjectLivenessSpanIdentity(lifecycle.SubjectLivenessCell, subject, uint32(index), uint32(index))
		if !idOK {
			t.Fatal("liveness span identity unavailable")
		}
		row, rowOK := lifecycle.NewSubjectLivenessSpan(id, subject, lifecycle.SubjectLivenessCell, uint32(index), uint32(index), lifecycle.SubjectLivenessLive)
		occurrence, occurrenceOK := programschema.NewOccurrence(
			programschema.OccurrenceSubjectLiveness, id, identity.ContentID{}, 0,
			uint32(index), 1, uint32(index), 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
		)
		point, pointOK := programschema.NewOccurrencePoint(lawID(byte(90 + index)))
		input, inputOK := programschema.NewOccurrenceInput(call)
		if !rowOK || !occurrenceOK || !pointOK || !inputOK {
			t.Fatalf("liveness fixture row %d unavailable", index)
		}
		if !builder.AddGeometry(programschema.OccurrenceSubjectLiveness, id, nil, []identity.ContentID{lawID(byte(90 + index))}) {
			t.Fatalf("liveness finish geometry %d refused", index)
		}
		publication.Lifecycle.SubjectSpans = append(publication.Lifecycle.SubjectSpans, row)
		publication.Occurrences = append(publication.Occurrences, occurrence)
		publication.OccurrencePoints = append(publication.OccurrencePoints, point)
		publication.OccurrenceInputs = append(publication.OccurrenceInputs, input)
	}
	rows, sealed := builder.Seal(table, publication)
	if !sealed {
		t.Fatal("liveness publication refused sealing")
	}
	return rows, table
}

func livenessSubscription() schemaissuance.SubscriptionSpec {
	return schemaissuance.SubscriptionSpec{
		Family:      "occurrence/subject-liveness",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormLocalFinish,
		Rule:        "law/rule/liveness",
		Writes:      "law/axis/liveness",
	}
}

// TestIssuanceResolvesTheCandidateRowOncePerAdmittedOccurrence is the join this
// cut moves into issuance. Every request a subject-liveness subscription emits
// carries the dense ordinal of its own liveness row, resolved while issuance
// still owns both the occurrence and the row space, so no later stage looks it
// up by identity.
func TestIssuanceResolvesTheCandidateRowOncePerAdmittedOccurrence(t *testing.T) {
	rows, table := livenessPublication(t, 3)
	spec := livenessSubscription()
	spec.Source = programissuance.RelationOccurrenceSubjectLiveness
	plan, planOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{spec})
	if !planOK {
		t.Fatal("subject-liveness plan refused its declared candidate source")
	}
	requests, evaluated := Evaluate(plan, rows)
	if !evaluated || len(requests) != 3 {
		t.Fatalf("liveness requests = %d evaluated=%t, want 3", len(requests), evaluated)
	}
	for index, request := range requests {
		source, resolved := request.Source()
		if !resolved {
			t.Fatalf("request %d carries no candidate row", index)
		}
		if source.Space != programissuance.RowSubjectLivenessSpan || source.Index != int(request.Occurrence()) {
			t.Fatalf("request %d candidate row = %+v, want liveness ordinal %d", index, source, request.Occurrence())
		}
	}
}

// TestIssuanceLeavesRequestsSourcelessWithoutADeclaration keeps the addition
// off every rule that did not ask for it: a subscription with no declared
// source emits exactly the requests it emitted before, carrying no row.
func TestIssuanceLeavesRequestsSourcelessWithoutADeclaration(t *testing.T) {
	rows, table := livenessPublication(t, 2)
	plan, planOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{livenessSubscription()})
	if !planOK {
		t.Fatal("sourceless subject-liveness plan refused")
	}
	requests, evaluated := Evaluate(plan, rows)
	if !evaluated || len(requests) != 2 {
		t.Fatalf("sourceless requests = %d evaluated=%t, want 2", len(requests), evaluated)
	}
	for index, request := range requests {
		if _, resolved := request.Source(); resolved {
			t.Fatalf("request %d acquired a candidate row it never declared", index)
		}
	}
}

// TestIssuanceRefusesAnUnreachableCandidateRow states the totality the runtime
// depends on. A rule issued on an occurrence whose source relation reaches no
// row has no candidate ordinal, and issuance refuses rather than publishing a
// placement the runtime would have to resolve or skip.
func TestIssuanceRefusesAnUnreachableCandidateRow(t *testing.T) {
	table := scheduleTable(t)
	builder := programissuance.NewBuilder()
	id := lawID(55)
	occurrence, occurrenceOK := programschema.NewOccurrence(
		programschema.OccurrenceSubjectLiveness, id, identity.ContentID{}, 0,
		0, 1, 0, 1, keyspace.FamilyInvalid, keyspace.LiteralValue{}, false,
	)
	point, pointOK := programschema.NewOccurrencePoint(lawID(56))
	input, inputOK := programschema.NewOccurrenceInput(lawID(57))
	if !occurrenceOK || !pointOK || !inputOK {
		t.Fatal("orphan liveness occurrence fixture unavailable")
	}
	if !builder.AddGeometry(programschema.OccurrenceSubjectLiveness, id, nil, []identity.ContentID{lawID(56)}) {
		t.Fatal("orphan liveness geometry refused")
	}
	rows, sealed := builder.Seal(table, &programpublication.Publication{
		Occurrences:      []programschema.Occurrence{occurrence},
		OccurrencePoints: []programschema.OccurrencePoint{point},
		OccurrenceInputs: []programschema.OccurrenceInput{input},
	})
	if !sealed {
		t.Fatal("orphan liveness publication refused sealing")
	}
	// The same occurrence issues normally without a declared source, so the
	// refusal below is attributable to the unreachable candidate row and not
	// to anything else about this fixture.
	sourceless, sourcelessOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{livenessSubscription()})
	baseline, baselineEvaluated := Evaluate(sourceless, rows)
	if !sourcelessOK || !baselineEvaluated || len(baseline) != 1 {
		t.Fatalf("sourceless baseline requests = %d evaluated=%t, want 1", len(baseline), baselineEvaluated)
	}
	spec := livenessSubscription()
	spec.Source = programissuance.RelationOccurrenceSubjectLiveness
	plan, planOK := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{spec})
	if !planOK {
		t.Fatal("subject-liveness plan refused its declared candidate source")
	}
	if _, evaluated := Evaluate(plan, rows); evaluated {
		t.Fatal("an occurrence reaching no candidate row issued a placement")
	}
}

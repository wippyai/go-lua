package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/subjectflow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/storage"
	"github.com/wippyai/go-lua/analysis/program/valuesource"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

func TestSubjectLivenessStateProjectionDoesNotFabricateUnknown(t *testing.T) {
	state, ok := artifactSubjectLivenessState(subjectflow.LivenessState(99))
	if ok || state.Valid() {
		t.Fatalf("invalid Flow liveness projected as %v/%t, want refusal", state, ok)
	}
}

// TestSubjectLivenessProjectionFoldsSharedProgramCoordinates pins the
// projection denominator restated for the live-range plane: distinct Flow
// subjects may resolve to the same Program-owned value coordinate, but an
// Artifact publishes one span per coordinate and run, and exactly one
// occurrence per span. The occurrence is anchored at the span's LO boundary,
// because that is where the obligation is issued; the answer is constant
// across the run by construction, so the later boundaries carry none.
func TestSubjectLivenessProjectionFoldsSharedProgramCoordinates(t *testing.T) {
	fixtures := []struct {
		name string
		text string
	}{
		{name: "cast multiple in statement", text: `
local data: any = {a = "1", b = 2, c = true}
local s, n, b = string(data.a), integer(data.b), boolean(data.c)
		`},
		{name: "arithmetic metamethod operand withheld", text: `
type Vec = { x: number }

local VecMT = {}
VecMT.__add = function(l: Vec, r: Vec): Vec
    return { x = math.max(l.x, r.x) }
end

local function combine(a: Vec, b: Vec): Vec
    return a + b
end

return combine(setmetatable({ x = 1 }, VecMT), setmetatable({ x = 2 }, VecMT))
		`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			sealed, view := compileStorageLifetimeLawProgram(t, fixture.text)
			count, published := view.SubjectLivenessSpanCount()
			if !published || count == 0 {
				t.Fatalf("subject-liveness span denominator = %d/%t", count, published)
			}
			boundaryCount, boundariesPublished := view.SubjectYieldBoundaryCount()
			if !boundariesPublished || boundaryCount == 0 {
				t.Fatalf("subject-yield-boundary denominator = %d/%t", boundaryCount, boundariesPublished)
			}
			seen := make(map[[32]byte]struct{}, count)
			for index := 0; index < count; index++ {
				row, ok := view.SubjectLivenessSpanAt(index)
				if !ok || !row.Available() {
					t.Fatalf("subject-liveness span %d unavailable", index)
				}
				id := [32]byte(row.ID())
				if _, duplicate := seen[id]; duplicate {
					t.Fatalf("Program published duplicate subject-liveness span at row %d", index)
				}
				seen[id] = struct{}{}
				anchor, anchored := view.SubjectYieldBoundaryAt(int(row.Lo()))
				if !anchored || anchor.Ordinal() != row.Lo() {
					t.Fatalf("span %d has no boundary at its lo ordinal %d", index, row.Lo())
				}
				ordinal, occurrenceOK := sealed.OccurrenceOrdinalForID(programschema.OccurrenceSubjectLiveness, row.ID())
				point, pointOK := sealed.OccurrencePointID(ordinal, 0)
				call, callOK := sealed.OccurrenceInputID(ordinal, 0)
				if !occurrenceOK || !pointOK || point != anchor.YieldFromPathID() || !callOK || call != anchor.CallID() {
					t.Fatalf("span occurrence %d = occurrence:%t point:%x/%t call:%x/%t, want point:%x call:%x", index, occurrenceOK, point, pointOK, call, callOK, anchor.YieldFromPathID(), anchor.CallID())
				}
				if _, extraPoint := sealed.OccurrencePointID(ordinal, 1); extraPoint {
					t.Fatalf("span occurrence %d published a second execution point", index)
				}
				if _, extraInput := sealed.OccurrenceInputID(ordinal, 1); extraInput {
					t.Fatalf("span occurrence %d published a second input", index)
				}
			}
			if occurrenceCount, ok := sealed.OccurrenceKindCount(programschema.OccurrenceSubjectLiveness); !ok || occurrenceCount != count {
				t.Fatalf("subject-liveness occurrence denominator = %d/%t, want span denominator %d", occurrenceCount, ok, count)
			}
		})
	}
}

// TestSubjectLivenessSpanAnswersEveryBoundaryItCovers is the other half of the
// re-encoding: a span is only lossless if every boundary inside it reads the
// span's state. The pair plane stored that answer; the range plane derives it,
// and this law is what says the derivation is total and agrees.
func TestSubjectLivenessSpanAnswersEveryBoundaryItCovers(t *testing.T) {
	_, view := compileStorageLifetimeLawProgram(t, `
local data: any = {a = "1", b = 2, c = true}
local s, n, b = string(data.a), integer(data.b), boolean(data.c)
	`)
	count, published := view.SubjectLivenessSpanCount()
	if !published || count == 0 {
		t.Fatalf("subject-liveness span denominator = %d/%t", count, published)
	}
	covered := 0
	for index := 0; index < count; index++ {
		span, ok := view.SubjectLivenessSpanAt(index)
		if !ok || !span.Available() {
			t.Fatalf("span %d unavailable", index)
		}
		for ordinal := span.Lo(); ordinal <= span.Hi(); ordinal++ {
			boundary, located := view.SubjectYieldBoundaryAt(int(ordinal))
			if !located || boundary.Ordinal() != ordinal {
				t.Fatalf("span %d covers ordinal %d, which names no boundary", index, ordinal)
			}
			state, answered := view.SubjectLivenessAtBoundary(boundary.YieldRouteID(), span.SubjectKind(), span.SubjectID())
			if !answered {
				t.Fatalf("span %d covers ordinal %d but the range read returned no answer", index, ordinal)
			}
			if state != span.State() {
				t.Fatalf("span %d covers ordinal %d reading %v, want the span's own %v", index, ordinal, state, span.State())
			}
			covered++
		}
	}
	if covered == 0 {
		t.Fatal("no span covered a boundary, so the law proved nothing")
	}
}

func subjectLivenessOwnerLawProgram(t testing.TB, text string) *program.Program {
	t.Helper()
	input, err := lower.Lower(lower.Source{Name: "subject-liveness-owner-law.lua", Text: []byte(text)})
	if err != nil || input == nil || !input.Available() {
		t.Fatalf("lower subject-liveness owner law: %v", err)
	}
	return input
}

func TestSubjectLivenessUsesCanonicalValueOwners(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local function make(a, ...)
    local negated = -a
    local selected = a or 1
    local computed = a + 1
    return negated, selected, computed, ...
end
local result = make(1, 2)
return result
`)
	state := &compiler{input: input, key: testCompileKey(t, input)}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %s", failure.Error())
	}
	programID := input.ContentID()
	flow := input.Flow()

	reads := flow.Authored().Storage().Reads()
	readChecked := 0
	for index := 0; index < reads.Count(); index++ {
		term, termOK := reads.At(index)
		want, issued, wantOK := storage.ReadIdentityAt(input, index)
		if !termOK || !wantOK || issued != term {
			continue
		}
		got, gotOK := state.valueLivenessID(programID, term)
		if !gotOK || got != want {
			t.Fatalf("Read[%d] liveness identity = %v/%v, want StorageReadIdentity %v/true", index, got, gotOK, want)
		}
		readChecked++
	}
	if readChecked == 0 {
		t.Fatal("fixture emitted no executable StorageRead owner to check")
	}

	values := flow.Authored().Values()
	tailChecked := 0
	for index := 0; index < values.Count(); index++ {
		valuesTerm, termOK := values.At(index)
		_, tail, rowOK := values.Get(valuesTerm)
		if !termOK || !rowOK || tail == 0 || keyspace.TermFamily(tail) != keyspace.FamilyVararg {
			continue
		}
		want, wantOK := flow.ValuesTailID(valuesTerm)
		got, gotOK := state.valueLivenessID(programID, tail)
		if !wantOK || !gotOK || got != want {
			t.Fatalf("Values tail %v liveness identity = %v/%v, want ValuesTailID %v/true", tail, got, gotOK, want)
		}
		tailChecked++
	}
	if tailChecked == 0 {
		t.Fatal("fixture emitted no Vararg Values tail owner to check")
	}

	operators := flow.Authored().Operators()
	unaryChecked := 0
	for index := 0; index < operators.Unaries().Count(); index++ {
		term, termOK := operators.Unaries().At(index)
		if !termOK {
			continue
		}
		span, spanOK := input.Span(term)
		if !spanOK {
			continue
		}
		got, gotOK := state.valueLivenessID(programID, term)
		if !gotOK || got != span.ContextID() {
			t.Fatalf("Unary[%d] liveness identity = %v/%v, want mounted Span %v/true", index, got, gotOK, span.ContextID())
		}
		unaryChecked++
	}
	if unaryChecked == 0 {
		t.Fatal("fixture emitted no executable Unary owner to check")
	}

	binaryChecked := 0
	for index := 0; index < operators.Binaries().Count(); index++ {
		term, termOK := operators.Binaries().At(index)
		primitive, primitiveOK := flow.BinaryPrimitives().Primitive(term)
		_, operationOK := primitive.Operation()
		span, spanOK := input.Span(term)
		if !termOK || !primitiveOK || !operationOK || !spanOK {
			continue
		}
		if got, gotOK := state.valueLivenessID(programID, term); !gotOK || got != span.ContextID() {
			t.Fatalf("Binary[%d] liveness identity = %v/%v, want mounted occurrence Span %v/true", index, got, gotOK, span.ContextID())
		}
		binaryChecked++
	}
	if binaryChecked == 0 {
		t.Fatal("fixture emitted no Binary occurrence owner to check")
	}
}

func TestSubjectLivenessAllocationUsesCanonicalArtifactTemplate(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local function run()
    local captured = { value = 1 }
    coroutine.yield()
    return captured.value
end
return run
`)
	state := &compiler{input: input, key: testCompileKey(t, input)}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %s", failure.Error())
	}
	if failure := state.copyAllocationRowsFailure(); failure.Available() {
		t.Fatalf("copy allocations: %s", failure.Error())
	}
	table, tableOK := input.Flow().Authored().Tables().At(0)
	row, rowOK := state.allocations.ForTerm(table)
	want, templateOK := row.Template()
	source, sourceOK := input.Flow().AllocationID(table)
	got, gotOK := state.valueLivenessID(input.ContentID(), table)
	if !tableOK || !rowOK || !templateOK || !sourceOK || !gotOK || got != want {
		t.Fatalf("table liveness identity = %v/%v, want canonical artifact template %v/true", got, gotOK, want)
	}
	if source == want {
		t.Fatal("fixture did not distinguish Flow allocation occurrence from artifact allocation template")
	}
}

func TestSubjectLivenessCallUsesCanonicalResultSlotOwner(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local function identity(value)
    return value
end
local result = -identity(1)
return result
`)
	state := &compiler{
		input: input, key: testCompileKey(t, input),
		issuanceRows: programissuance.NewBuilder(),
	}
	if failure := state.indexPointAttachmentsFailure(); failure.Available() {
		t.Fatalf("index point attachments: %s", failure.Error())
	}
	if failure := state.copyCalls(); failure.Available() {
		t.Fatalf("copy calls: %s", failure.Error())
	}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %s", failure.Error())
	}
	if failure := state.copyCallRowsFailure(); failure.Available() {
		t.Fatalf("copy call rows: %s", failure.Error())
	}
	call, callOK := input.Flow().Authored().Calls().At(0)
	identities, identitiesOK := input.CallIdentityAt(0)
	path, pathOK := input.Flow().SemanticTermPath(call)
	if !callOK || !identitiesOK || !pathOK {
		t.Fatal("fixture did not issue an authenticated Call")
	}
	if got, gotOK := state.valueLivenessID(input.ContentID(), call); gotOK || got.Available() {
		t.Fatalf("Call occurrence was accepted as a Value coordinate: %v/%v", got, gotOK)
	}
	coordinates, coordinatesOK := state.subjectLivenessCoordinates(input.ContentID(), subjectflow.Subject{Kind: subjectflow.SubjectValue, ID: path, Term: call})
	if !coordinatesOK || len(coordinates) != 1 {
		t.Fatalf("Call result-coordinate projection = %d/%v, want one finite result slot", len(coordinates), coordinatesOK)
	}
	coordinate := coordinates[0]
	if coordinate.kind != lifecycle.SubjectLivenessValue || !coordinate.id.Available() || coordinate.id == identities.Call {
		t.Fatalf("Call result coordinate = %+v, want canonical non-occurrence ValueID", coordinate)
	}
	if len(state.publication.CallResultSlots) != 1 {
		t.Fatalf("Call result slot count = %d, want one", len(state.publication.CallResultSlots))
	}
	want, wantOK := state.publication.CallResultSlots[0].ValueID()
	if !wantOK || coordinate.id != want {
		t.Fatalf("Call liveness coordinate = %v, want sealed CallResultSlot.ValueID %v", coordinate.id, want)
	}
}

func TestSubjectLivenessOpenValuesPublishesOnlyFiniteCoordinates(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local function identity(value)
    return value
end
return identity(1)
`)
	state := &compiler{input: input, key: testCompileKey(t, input)}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %s", failure.Error())
	}
	values := input.Flow().Authored().Values()
	for index := 0; index < values.Count(); index++ {
		term, termOK := values.At(index)
		_, tail, rowOK := values.Get(term)
		if !termOK || !rowOK || keyspace.TermFamily(tail) != keyspace.FamilyCall {
			continue
		}
		path, pathOK := input.Flow().SemanticTermPath(term)
		coordinates, coordinatesOK := state.subjectLivenessCoordinates(input.ContentID(), subjectflow.Subject{Kind: subjectflow.SubjectValues, ID: path, Term: term})
		if !pathOK || !coordinatesOK || len(coordinates) == 0 {
			t.Fatalf("open Values finite projection = %d/%v", len(coordinates), coordinatesOK)
		}
		for _, coordinate := range coordinates {
			if coordinate.kind != lifecycle.SubjectLivenessValue || !coordinate.id.Available() {
				t.Fatalf("open Values fabricated a non-finite coordinate: %+v", coordinate)
			}
		}
		return
	}
	t.Fatal("fixture emitted no open Call tail")
}

func TestSubjectLivenessTypeValueUsesValueSourceOwner(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local value = 1
return string(value)
`)
	state := &compiler{input: input, key: testCompileKey(t, input)}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %s", failure.Error())
	}
	typeValues := input.Flow().Authored().TypeValues()
	if typeValues.Count() == 0 {
		t.Skip("lowerer did not emit a runtime TypeValue in this build")
	}
	for index := 0; index < typeValues.Count(); index++ {
		term, termOK := typeValues.At(index)
		want, _, issued, wantOK := valuesource.IdentityAt(input, keyspace.FamilyTypeValue, index)
		if !termOK || !wantOK || issued != term {
			continue
		}
		got, gotOK := state.valueLivenessID(input.ContentID(), term)
		if !gotOK || got != want {
			t.Fatalf("TypeValue[%d] liveness identity = %v/%v, want ValueSource owner %v/true", index, got, gotOK, want)
		}
		flowPath, flowPathOK := input.Flow().SemanticTermPath(term)
		if flowPathOK && flowPath == got {
			t.Fatalf("TypeValue[%d] still uses the generic Flow semantic path", index)
		}
	}
}

func TestSubjectLivenessExcludesStructuralLenses(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local root = {}
local dynamic = "dynamic"
local fixed = root.fixed
local selected = root[dynamic]
root.other = fixed
root[dynamic] = selected
return fixed, selected
`)
	projection := input.Flow().SubjectFlow()
	if projection == nil || !projection.Available() {
		t.Fatal("fixture published no SubjectFlow")
	}
	for index := 0; index < projection.LivenessCount(); index++ {
		row, ok := projection.LivenessAt(index)
		family := keyspace.TermFamily(row.Subject.Term)
		if !ok {
			t.Fatalf("liveness row %d unavailable", index)
		}
		if family == keyspace.FamilyLensExact || family == keyspace.FamilyLensKey {
			t.Fatalf("structural Lens leaked into Value liveness at row %d", index)
		}
	}
}

func TestSubjectLivenessIndexReadUsesEvaluationSpan(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `
local root = { value = 1 }
local selected = root.value
return selected
`)
	state := &compiler{input: input, key: testCompileKey(t, input)}
	reads := input.Flow().AccessGeometry().IndexAccesses().Reads()
	if reads.Count() == 0 {
		t.Fatal("fixture emitted no index Read")
	}
	row, rowOK := state.indexReadAt(0)
	span, spanOK := input.Span(row.term)
	got, gotOK := state.valueLivenessID(input.ContentID(), row.term)
	if !rowOK || !spanOK || !gotOK || got != span.ContextID() || row.resultID != span.ContextID() {
		t.Fatalf("index Read liveness identity = %v/%v, want evaluation Span %v/true", got, gotOK, span.ContextID())
	}
}

func TestSubjectLivenessRejectsMissingOwnerIdentity(t *testing.T) {
	input := subjectLivenessOwnerLawProgram(t, `return 1 .. 2`)
	state := &compiler{input: input, key: testCompileKey(t, input)}
	if failure := state.copyValuesFailure(); failure.Available() {
		t.Fatalf("copy values: %s", failure.Error())
	}
	term := keyspace.MakeTerm(keyspace.FamilyBinary, 1)
	if id, ok := state.valueLivenessID(input.ContentID(), term); ok || id.Available() {
		t.Fatalf("missing binary owner was compensated as %v/%v", id, ok)
	}
	foreign := input.ContentID()
	foreign[0] ^= 0xff
	if id, ok := state.valueLivenessID(foreign, term); ok || id.Available() {
		t.Fatalf("foreign Program identity was accepted as %v/%v", id, ok)
	}
}

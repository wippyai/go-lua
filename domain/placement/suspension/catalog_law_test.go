package suspension

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func suspensionCatalogLawID(t testing.TB, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("placement-suspension-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

func suspensionCatalogLawProgram(t testing.TB, tailPresent bool) (programschema.Program, programschema.Values, []identity.ContentID) {
	t.Helper()
	schemaID := suspensionCatalogLawID(t, "schema")
	catalogID, catalogOK := programcatalog.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("derive Program catalog")
	}
	aggregateID := suspensionCatalogLawID(t, "values")
	memberOneID := suspensionCatalogLawID(t, "member-one")
	memberTwoID := suspensionCatalogLawID(t, "member-two")
	tailID := suspensionCatalogLawID(t, "tail")
	tailSpanID := suspensionCatalogLawID(t, "tail-span")
	cellID := suspensionCatalogLawID(t, "cell")
	tail := programschema.ValuesTail{}
	if tailPresent {
		var tailOK bool
		tail, tailOK = programschema.NewValuesTail(tailID, tailSpanID, programschema.ValuesTailCall, true)
		if !tailOK {
			t.Fatal("open Values tail")
		}
	} else {
		var tailOK bool
		tail, tailOK = programschema.NewValuesTail(identity.ContentID{}, identity.ContentID{}, programschema.ValuesTailInvalid, false)
		if !tailOK {
			t.Fatal("closed Values tail")
		}
	}
	values, valuesOK := programschema.NewValues(
		aggregateID, suspensionCatalogLawID(t, "body"), suspensionCatalogLawID(t, "root-span"), 0, 2, tail,
	)
	memberOne, memberOneOK := programschema.NewValuesMember(memberOneID)
	memberTwo, memberTwoOK := programschema.NewValuesMember(memberTwoID)
	cellLifetime, cellLifetimeOK := lifecycle.NewStorageCellLifetime(cellID, lifecycle.StorageLifetimeUnknown)
	if !valuesOK || !memberOneOK || !memberTwoOK || !cellLifetimeOK {
		t.Fatal("Values family row")
	}
	frozen, sealed := (publication.Publication{
		Values:        []programschema.Values{values},
		ValuesMembers: []programschema.ValuesMember{memberOne, memberTwo},
		Lifecycle:     lifecycle.Publication{StorageCellLifetimes: []lifecycle.StorageCellLifetime{cellLifetime}},
	}).Seal(catalogID, identity.StoreID(41))
	if !sealed {
		t.Fatal("seal Values publication")
	}
	program := programschema.Program{
		Frozen: frozen, ArtifactID: suspensionCatalogLawID(t, "artifact"),
		ProgramID: suspensionCatalogLawID(t, "program"), SchemaID: schemaID,
	}
	if !program.Available() {
		t.Fatal("Values law Program")
	}
	want := []identity.ContentID{aggregateID, memberOneID, memberTwoID}
	if tailPresent {
		want = append(want, tailID)
	}
	return program, values, want
}

func suspensionCatalogLawView(t testing.TB, program programschema.Program) lifecycle.View {
	t.Helper()
	state, stateOK := program.ColdState()
	view, viewOK := lifecycle.NewView(state)
	if !stateOK || !viewOK {
		t.Fatal("open lifecycle view")
	}
	return view
}

func suspensionCatalogLawSubject(t testing.TB, route identity.ContentID, kind lifecycle.SubjectLivenessKind, subject identity.ContentID) spanSubject {
	t.Helper()
	_ = route
	return spanSubject{kind: kind, subject: subject, state: lifecycle.SubjectLivenessLive, call: suspensionCatalogLawID(t, "call")}
}

func suspensionCatalogLawSubjectValueIDs(t testing.TB, program programschema.Program, view lifecycle.View, row spanSubject) ([]identity.ContentID, bool) {
	t.Helper()
	index, indexOK := buildValueAggregateIndex(program)
	if !indexOK {
		return nil, false
	}
	return subjectValueIDsIndexed(program, view, row, index)
}

func TestSubjectValuesProjectionRetainsClosedMembersBeforeOpenTailRefusal(t *testing.T) {
	program, _, want := suspensionCatalogLawProgram(t, true)
	view := suspensionCatalogLawView(t, program)
	row := suspensionCatalogLawSubject(t, suspensionCatalogLawID(t, "yield"), lifecycle.SubjectLivenessValues, want[0])
	got, ok := suspensionCatalogLawSubjectValueIDs(t, program, view, row)
	fixed := want[:len(want)-1]
	index, indexOK := buildValueAggregateIndex(program)
	entry := index.entries[want[0]]
	if !ok || !indexOK || !entry.open || len(got) != len(fixed) {
		t.Fatalf("Values bridge = %v/%t open=%v, want fixed %v/true", got, ok, entry.open, fixed)
	}
	for index := range fixed {
		if got[index] != fixed[index] {
			t.Fatalf("Values bridge[%d] = %v, want %v", index, got[index], fixed[index])
		}
	}
}

func TestSubjectValuesProjectionOmitsClosedTailAndKeepsScalarCellValue(t *testing.T) {
	program, _, want := suspensionCatalogLawProgram(t, false)
	view := suspensionCatalogLawView(t, program)
	row := suspensionCatalogLawSubject(t, suspensionCatalogLawID(t, "yield-values"), lifecycle.SubjectLivenessValues, want[0])
	got, ok := suspensionCatalogLawSubjectValueIDs(t, program, view, row)
	if !ok || len(got) != len(want) {
		t.Fatalf("closed Values bridge = %v/%t, want %v/true", got, ok, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("closed Values bridge[%d] = %v, want %v", index, got[index], want[index])
		}
	}
	for _, item := range []struct {
		name string
		kind lifecycle.SubjectLivenessKind
	}{
		{name: "cell", kind: lifecycle.SubjectLivenessCell},
		{name: "value", kind: lifecycle.SubjectLivenessValue},
	} {
		subject := suspensionCatalogLawID(t, item.name)
		if item.kind == lifecycle.SubjectLivenessCell {
			subject = suspensionCatalogLawID(t, "cell")
		}
		row := suspensionCatalogLawSubject(t, suspensionCatalogLawID(t, item.name+"-yield"), item.kind, subject)
		got, ok := suspensionCatalogLawSubjectValueIDs(t, program, view, row)
		if !ok || len(got) != 1 || got[0] != subject {
			t.Fatalf("scalar %s bridge = %v/%t, want [%v]/true", item.name, got, ok, subject)
		}
	}
}

func TestRootWithoutAProgramValuePreimageStaysExplicitlyOpen(t *testing.T) {
	program, _, _ := suspensionCatalogLawProgram(t, false)
	view := suspensionCatalogLawView(t, program)
	row := suspensionCatalogLawSubject(t, suspensionCatalogLawID(t, "yield-root"), lifecycle.SubjectLivenessRoot, suspensionCatalogLawID(t, "body"))
	if ids, ok := suspensionCatalogLawSubjectValueIDs(t, program, view, row); ok || len(ids) != 0 {
		t.Fatalf("Root bridge fabricated Value coordinates = %v/%t", ids, ok)
	}
}

func TestCellBridgeRequiresTheProgramStorageLifetimeFence(t *testing.T) {
	program, _, _ := suspensionCatalogLawProgram(t, false)
	view := suspensionCatalogLawView(t, program)
	row := suspensionCatalogLawSubject(t, suspensionCatalogLawID(t, "yield-forged-cell"), lifecycle.SubjectLivenessCell, suspensionCatalogLawID(t, "not-a-cell"))
	if ids, ok := suspensionCatalogLawSubjectValueIDs(t, program, view, row); ok || len(ids) != 0 {
		t.Fatalf("Cell bridge accepted an unfenced semantic ID = %v/%t", ids, ok)
	}
}

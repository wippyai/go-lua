package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	"github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
)

// structuralInitializerFixtures are annotated local declarations whose
// initializer is a structural constructor. The declaration carries a declared
// type and the value it binds is established by an allocation, so each one is
// a conformance subject the census has to carry.
var structuralInitializerFixtures = []struct {
	name string
	line uint32
}{
	{"types/wrong-field-type", 2},
	{"types/missing-field", 2},
}

// A value an allocation establishes is measured like any other. The
// declaration's own line is where the subject reports, so the census carries a
// conformance site there rather than dropping the declaration for want of a
// producing rule role.
func TestStructuralInitializerDeclarationIssuesAConformanceSite(t *testing.T) {
	for _, fixture := range structuralInitializerFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, fixture.name)
			axes, axesOK := composite.ProducedValueAxes(testCase.state.compilation)
			if !axesOK || len(axes) == 0 {
				t.Fatal("declared produced-value axes")
			}
			census, ok := mounted.SealObservationSites(testCase.linked.Boundary(), testCase.mounts, axes)
			if !ok || !census.Available() {
				t.Fatalf("seal mounted observation sites: ok=%v available=%v", ok, census.Available())
			}
			sites := 0
			for index := 0; index < census.Count(); index++ {
				site, siteOK := census.At(index)
				if !siteOK {
					t.Fatalf("census row %d", index)
				}
				if site.Kind != structure.DiagnosticObservationTypeConformance || site.Location.StartLine != fixture.line {
					continue
				}
				sites++
				if site.ProducerCount() == 0 {
					t.Fatal("conformance site carries no producer geometry")
				}
			}
			if sites == 0 {
				t.Fatalf("no conformance site at line %d", fixture.line)
			}
		})
	}
}

// One occurrence carries every rule placed on it. Only the placements writing
// a produced-value axis establish the value an observation measures, so the
// census admits those and no others: an allocation's heap placements observe
// the heap, not the value the constructor denotes.
func TestConformanceProducersAreTheValueWritingPlacementsAlone(t *testing.T) {
	for _, fixture := range structuralInitializerFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, fixture.name)
			axes, axesOK := composite.ProducedValueAxes(testCase.state.compilation)
			if !axesOK || len(axes) == 0 {
				t.Fatal("declared produced-value axes")
			}
			admitted := make(map[string]struct{}, len(axes))
			for _, axis := range axes {
				admitted[string(axis)] = struct{}{}
			}
			census, ok := mounted.SealObservationSites(testCase.linked.Boundary(), testCase.mounts, axes)
			if !ok || !census.Available() {
				t.Fatalf("seal mounted observation sites: ok=%v available=%v", ok, census.Available())
			}
			placements := make(map[string]string)
			for _, mount := range testCase.mounts {
				program := mount.Snapshot.Program()
				count, published := program.RuleOccurrenceCount()
				if !published {
					t.Fatal("rule occurrence column")
				}
				for index := 0; index < count; index++ {
					row, held := program.RuleOccurrenceAt(index)
					if !held {
						t.Fatalf("rule occurrence %d", index)
					}
					placements[string(row.Key())] = string(row.Writes())
				}
			}
			measured := 0
			for index := 0; index < census.Count(); index++ {
				site, siteOK := census.At(index)
				if !siteOK {
					t.Fatalf("census row %d", index)
				}
				if site.Kind != structure.DiagnosticObservationTypeConformance {
					continue
				}
				for position := 0; position < site.ProducerCount(); position++ {
					producer, producerOK := site.ProducerAt(position)
					if !producerOK {
						t.Fatalf("producer %d", position)
					}
					writes, known := placements[string(producer.Key)]
					if !known {
						t.Fatalf("producer rule %q is not a published placement", producer.Key)
					}
					if _, value := admitted[writes]; !value {
						t.Fatalf("producer rule %q writes %q, which no produced-value observation reads", producer.Key, writes)
					}
					measured++
				}
			}
			if measured == 0 {
				t.Fatal("no conformance producer was measured")
			}
		})
	}
}

// Every published placement names the axis its rule writes. The row is the
// only place a consumer can separate the rules sharing one occurrence, so a
// placement that names none is not a row.
func TestEveryPublishedPlacementNamesTheAxisItWrites(t *testing.T) {
	for _, fixture := range structuralInitializerFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, fixture.name)
			for _, mount := range testCase.mounts {
				program := mount.Snapshot.Program()
				count, published := program.RuleOccurrenceCount()
				if !published {
					t.Fatal("rule occurrence column")
				}
				for index := 0; index < count; index++ {
					row, held := program.RuleOccurrenceAt(index)
					if !held || !row.Writes().Available() {
						t.Fatalf("placement %d publishes no written axis", index)
					}
				}
			}
		})
	}
	point, pointOK := identity.DeriveContentID("analysis/structural-initializer-law", []byte("point"))
	input, inputOK := identity.DeriveContentID("analysis/structural-initializer-law", []byte("input"))
	if !pointOK || !inputOK {
		t.Fatal("derive placement coordinates")
	}
	if _, sealed := programschema.NewRuleOccurrence("value-source", "", 0, point, input, programschema.RuleStageLocal, programschema.RuleInputFinish, identity.ContentID{}); sealed {
		t.Fatal("a placement sealed without the axis it writes")
	}
	if _, sealed := programschema.NewRuleOccurrence("value-source", "value", 0, point, input, programschema.RuleStageLocal, programschema.RuleInputFinish, identity.ContentID{}); !sealed {
		t.Fatal("a placement naming the axis it writes is refused")
	}
}

// structuralMemberSite is one issued conformance row read back out of a
// mounted artifact: the site it names, where it reports, and the declared node
// it is measured against.
type structuralMemberSite struct {
	site     programdiagnostic.DiagnosticObservationSite
	location programsource.Span
	declared identity.ContentID
	measured identity.ContentID
	position uint32
}

// structuralMemberSites reads every conformance row one compiled fixture
// publishes. The rows are the issuance population; no verdict is read here.
func structuralMemberSites(t *testing.T, testCase mountedPopulationCase) []structuralMemberSite {
	t.Helper()
	rows := make([]structuralMemberSite, 0)
	for _, mount := range testCase.mounts {
		program := mount.Snapshot.Program()
		cold, coldOK := program.ColdState()
		view, viewOK := programdiagnostic.NewView(cold)
		count, published := view.DiagnosticObservationCount()
		if !coldOK || !viewOK || !published {
			t.Fatal("diagnostic observation column")
		}
		for index := 0; index < count; index++ {
			observation, held := view.DiagnosticObservationAt(index)
			if !held {
				t.Fatalf("diagnostic observation %d", index)
			}
			if observation.Kind() != structure.DiagnosticObservationTypeConformance {
				continue
			}
			location, locationOK := observation.Location()
			position, positionOK := observation.Position()
			if !locationOK || !positionOK {
				t.Fatalf("conformance observation %d carries no location", index)
			}
			rows = append(rows, structuralMemberSite{
				site: observation.Site(), location: location,
				declared: observation.DeclaredStaticTypeID(),
				measured: observation.MeasuredValueID(),
				position: position,
			})
		}
	}
	return rows
}

// recordFieldChild names the declared node one record field resolves to. It is
// read out of the published static graph, so a member site's declaration is
// compared against the same column a consumer reads.
func recordFieldChild(t *testing.T, testCase mountedPopulationCase, field string) identity.ContentID {
	t.Helper()
	child := identity.ContentID{}
	found := 0
	for _, mount := range testCase.mounts {
		program := mount.Snapshot.Program()
		cold, coldOK := program.ColdState()
		view, viewOK := staticnode.NewView(cold)
		if !coldOK || !viewOK {
			t.Fatal("static node cold view")
		}
		count, published := view.StaticTypeNodeRecordFieldCount()
		if !published {
			t.Fatal("static record field column")
		}
		for index := 0; index < count; index++ {
			row, held := view.StaticTypeNodeRecordFieldAt(index)
			if !held {
				t.Fatalf("static record field %d", index)
			}
			if row.Text() != field {
				continue
			}
			child = row.ChildID()
			found++
		}
	}
	if found != 1 || !child.Available() {
		t.Fatalf("declared field %q resolves to %d nodes", field, found)
	}
	return child
}

// A constructor member is measured where it is written, against the node the
// declaration gives that member. The offending member of a wrong-field-type
// initializer therefore reports at its own value, and its declaration is the
// field's child node -- not the record's, which the whole value already carries.
func TestConstructorMemberIsMeasuredAtItsOwnSpanAgainstTheFieldNode(t *testing.T) {
	testCase := compileMountedPopulationCase(t, "types/wrong-field-type")
	declared := recordFieldChild(t, testCase, "x")
	members := 0
	for _, row := range structuralMemberSites(t, testCase) {
		if row.site != programdiagnostic.DiagnosticObservationSiteMember {
			continue
		}
		if row.location.StartLine != 2 || row.location.StartCol != 23 {
			continue
		}
		members++
		if row.declared != declared {
			t.Fatalf("member site at 2:23 is measured against %x, want the field node %x", row.declared, declared)
		}
	}
	if members != 1 {
		t.Fatalf("wrong-field-type issues %d member sites at 2:23, want 1", members)
	}
}

// A required declared field the constructor's key set does not establish is
// named once, at the constructor, against the missing field's node. An optional
// field is not required, so its absence names nothing.
func TestAbsentRequiredMemberIsNamedOnceAndOptionalAbsenceIsNot(t *testing.T) {
	missing := compileMountedPopulationCase(t, "types/missing-field")
	declared := recordFieldChild(t, missing, "y")
	absent := make([]structuralMemberSite, 0)
	for _, row := range structuralMemberSites(t, missing) {
		if row.site == programdiagnostic.DiagnosticObservationSiteMemberAbsent {
			absent = append(absent, row)
		}
	}
	if len(absent) != 1 {
		t.Fatalf("missing-field issues %d absent-member sites, want 1", len(absent))
	}
	if absent[0].declared != declared {
		t.Fatalf("absent-member site names %x, want the declared field y %x", absent[0].declared, declared)
	}
	if absent[0].location.StartLine != 2 {
		t.Fatalf("absent-member site reports at line %d, want the constructor's line 2", absent[0].location.StartLine)
	}

	optional := compileMountedPopulationCase(t, "types/record-optional-field")
	for _, row := range structuralMemberSites(t, optional) {
		if row.site == programdiagnostic.DiagnosticObservationSiteMemberAbsent {
			t.Fatalf("an optional declared field is named absent at line %d", row.location.StartLine)
		}
	}
}

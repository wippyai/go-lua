package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	axes, axesOK := composite.ProducedValueAxes()
	if !axesOK || len(axes) == 0 {
		t.Fatal("declared produced-value axes")
	}
	for _, fixture := range structuralInitializerFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, fixture.name)
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
	axes, axesOK := composite.ProducedValueAxes()
	if !axesOK || len(axes) == 0 {
		t.Fatal("declared produced-value axes")
	}
	admitted := make(map[string]struct{}, len(axes))
	for _, axis := range axes {
		admitted[string(axis)] = struct{}{}
	}
	for _, fixture := range structuralInitializerFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, fixture.name)
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

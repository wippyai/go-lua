package analysis

import (
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// mountedPopulationFixtures is the selected proof set for the Link-owned
// populations. It is deliberately a handful rather than the corpus: the claims
// are structural, so what a fixture has to supply is shape -- a callable
// interior, a recurrence with several points, a branch with producers, and a
// multi-module Link with more than one mount -- not volume.
var mountedPopulationFixtures = []string{
	"bench/fibonacci",
	"flow/break-in-while",
	"functions/pcall-error-handling",
	"modules/imported-handler-map-lookup-after-discriminant",
	"modules/imported-map-of-time-record-store",
	"narrowing/congruence-equality-persists",
}

// mountedPopulationCase is one compiled fixture: the sealed Link, the plan it
// compiled to, and the mount rows the populations are derived from.
type mountedPopulationCase struct {
	linked *link.Link
	plan   *Plan
	state  *compiledState
	mounts []programmount.MountedArtifact
}

// compileMountedPopulationCase compiles one named corpus fixture to the seam
// the populations are derived from.
func compileMountedPopulationCase(t *testing.T, name string) mountedPopulationCase {
	t.Helper()
	linked, err := testfixture.SealCorpusProject(fixtureContract(t), fixtureProject(t, name))
	if err != nil {
		t.Fatalf("seal fixture %q: %v", name, err)
	}
	plan, status, diagnostics := CompileWithDiagnostics(linked)
	if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
		if plan != nil {
			plan.Close()
		}
		t.Fatalf("compile fixture %q = %v diagnostics=%+v", name, status, diagnostics)
	}
	// The compiled query plan is instantiated with the runtime topology, so
	// this law reaches the same committed-program seam as a solve.
	if _, _, _, ok := plan.state.instantiateRuntimeTopology(); !ok {
		plan.Close()
		t.Fatalf("instantiate runtime topology for fixture %q", name)
	}
	rows := append([]programmount.MountedArtifact(nil), plan.state.artifacts.mounts...)
	return mountedPopulationCase{linked: linked, plan: plan, state: plan.state, mounts: rows}
}

// reversed returns the same mount rows in the opposite order. Every population
// claims to be a function of sealed content alone, so deriving one from a
// permuted input must produce the identical column.
func (testCase mountedPopulationCase) reversed() []programmount.MountedArtifact {
	rows := make([]programmount.MountedArtifact, len(testCase.mounts))
	for index, mount := range testCase.mounts {
		rows[len(rows)-1-index] = mount
	}
	return rows
}

// TestMountedObservationCensusCapturesTheCompiledObservationSites is the census
// receipt. Every site the current diagnostic projection observes is a census
// row with the same kind, span, and -- for a branch -- the same
// anchor-to-execution geometry, and the census carries no site that projection
// does not observe.
func TestMountedObservationCensusCapturesTheCompiledObservationSites(t *testing.T) {
	branches := 0
	for _, name := range mountedPopulationFixtures {
		t.Run(name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, name)
			defer testCase.plan.Close()
			producerAxes, axesOK := composite.ProducedValueAxes(testCase.state.compilation)
			if !axesOK {
				t.Fatal("declared produced-value axes")
			}
			census, ok := mounted.SealObservationSites(testCase.linked.Boundary(), testCase.mounts, producerAxes)
			if !ok || !census.Available() {
				t.Fatalf("seal mounted observation sites: ok=%v available=%v", ok, census.Available())
			}
			observations, observationsOK := testCase.state.artifacts.observationCensus()
			if !observationsOK {
				t.Fatal("compile diagnostic observations")
			}
			if census.Count() != len(observations) {
				t.Fatalf("census holds %d sites, the compiled projection observes %d", census.Count(), len(observations))
			}
			type siteKey struct {
				mount identity.ContentID
				local identity.ContentID
			}
			held := make(map[siteKey]mounted.ObservationSite, census.Count())
			for index := 0; index < census.Count(); index++ {
				site, siteOK := census.At(index)
				if !siteOK {
					t.Fatalf("census row %d is not addressable", index)
				}
				held[siteKey{mount: site.Mount, local: site.Local}] = site
				if index != 0 {
					previous, _ := census.At(index - 1)
					if previous.Mount == site.Mount && previous.Local == site.Local {
						t.Fatalf("census row %d repeats a site key", index)
					}
				}
			}
			for _, observation := range observations {
				site, present := held[siteKey{mount: observation.Mount, local: observation.Local}]
				if !present {
					t.Fatalf("observed site %s of mount %s is missing from the census", observation.Local, observation.Mount)
				}
				if site.Kind != observation.Kind || site.Location != observation.Location {
					t.Fatalf("census site %s = kind %v span %+v, observed kind %v span %+v", observation.Local, site.Kind, site.Location, observation.Kind, observation.Location)
				}
				points, producers, produced := mountedPopulationGeometry(observation)
				if !produced {
					if site.ProducerCount() != 0 {
						t.Fatalf("static census site %s carries %d producers", observation.Local, site.ProducerCount())
					}
					continue
				}
				if observation.Kind == structure.DiagnosticObservationBranchCondition {
					branches++
				}
				observedValue, measured := observation.MeasuredValueID()
				if !measured {
					t.Fatalf("census site %s measures no ValueID", observation.Local)
				}
				if site.ValueID != observedValue {
					t.Fatalf("census site %s value %s, observed value %s", observation.Local, site.ValueID, observedValue)
				}
				if site.ProducerCount() != len(producers) {
					t.Fatalf("census site %s carries %d producers, observed %d", observation.Local, site.ProducerCount(), len(producers))
				}
				geometry := make(map[anadiag.Producer]struct{}, len(producers))
				for _, producer := range producers {
					geometry[producer] = struct{}{}
				}
				anchors := make(map[identity.ContentID]struct{}, site.ProducerCount())
				for index := 0; index < site.ProducerCount(); index++ {
					producer, producerOK := site.ProducerAt(index)
					if !producerOK {
						t.Fatalf("census producer %d is not addressable", index)
					}
					key := anadiag.Producer{Key: producer.Key, Occurrence: producer.Occurrence, Point: producer.Point, Anchor: producer.Anchor}
					if _, observed := geometry[key]; !observed {
						t.Fatalf("census site %s carries unobserved producer geometry %+v", observation.Local, key)
					}
					anchors[producer.Anchor] = struct{}{}
				}
				if len(anchors) != len(points) {
					t.Fatalf("census site %s anchors %d evidence points, the population carries %d", observation.Local, len(anchors), len(points))
				}
				for _, point := range points {
					if _, anchored := anchors[point]; !anchored {
						t.Fatalf("census site %s leaves evidence point %s unanchored", observation.Local, point)
					}
				}
			}
			permuted, permutedOK := mounted.SealObservationSites(testCase.linked.Boundary(), testCase.reversed(), producerAxes)
			if !permutedOK || permuted.Count() != census.Count() {
				t.Fatalf("permuted census = %d sites, want %d", permuted.Count(), census.Count())
			}
			for index := 0; index < census.Count(); index++ {
				direct, _ := census.At(index)
				other, _ := permuted.At(index)
				if direct.Mount != other.Mount || direct.Local != other.Local || direct.Kind != other.Kind || direct.ValueID != other.ValueID {
					t.Fatalf("census row %d depends on mount order", index)
				}
			}
		})
	}
	if branches == 0 {
		t.Fatal("selected fixtures observe no branch site; the anchor geometry is unproven")
	}
}

// TestObservationPublicationsDeriveFromSealedGeometry is the detach address
// floor: Snapshot observation keys are the publication identity of the sealed
// census consumed from compiledState's precomputed Geometry owner.
func TestObservationPublicationsDeriveFromSealedGeometry(t *testing.T) {
	for _, name := range mountedPopulationFixtures {
		t.Run(name, func(t *testing.T) {
			testCase := compileMountedPopulationCase(t, name)
			defer testCase.plan.Close()
			geometry := testCase.state.geometry
			if !geometry.Valid() {
				t.Fatal("result geometry")
			}
			// Both produced-value populations publish through the same
			// boundary, each under the observation family its own population
			// issues, so the law is stated over both rather than over the one
			// that happened to be wired first. The address is the producing
			// occurrence's: one statement carries one base evidence point and
			// as many producers as it has measured values, so an anchor may
			// repeat while an address may not.
			for _, population := range [][]anadiag.Observation{geometry.BranchObservations, geometry.ConformanceObservations} {
				got, ok := anadiag.Publications(testCase.state.compilation, testCase.state.contextDirectory, population)
				if !ok {
					t.Fatal("observation publications")
				}
				seen := make(map[identity.ContentID]identity.ContentID, len(got))
				for _, row := range got {
					if !row.Mount.Available() || !row.Point.Available() || !row.Key.Available() {
						t.Fatal("publication row missing identities")
					}
					if _, duplicate := seen[row.Key]; duplicate {
						t.Fatalf("publication repeats observation address %s", row.Key)
					}
					seen[row.Key] = row.Point
				}
				for _, observation := range population {
					points, producers, _ := mountedPopulationGeometry(observation)
					anchors := make(map[identity.ContentID]struct{}, len(points))
					for _, point := range points {
						anchors[point] = struct{}{}
					}
					for _, producer := range producers {
						for contextIndex := 0; contextIndex < testCase.state.contextDirectory.ContextCount(); contextIndex++ {
							context, contextOK := testCase.state.contextDirectory.ContextAt(contextIndex)
							if !contextOK || context.ModuleKey() != observation.Mount {
								continue
							}
							want, wantOK := anadiag.ValueObservationAddress(testCase.state.compilation, observation.Kind, observation.Mount, producer.Point, context)
							anchor, published := seen[want]
							if !wantOK || !published {
								t.Fatalf("producing occurrence %s has no publication for context %s", producer.Point, context.ID())
							}
							if _, based := anchors[anchor]; !based {
								t.Fatalf("publication of %s is indexed by %s, which is no evidence point of its row", producer.Point, anchor)
							}
						}
					}
				}
			}
		})
	}
}

// mountedPopulationGeometry is the produced-value read of one observed row:
// the base evidence points and the producers that must anchor to them. A
// static population answers false and carries neither.
func mountedPopulationGeometry(observation anadiag.Observation) ([]identity.ContentID, []anadiag.Producer, bool) {
	switch observation.Kind {
	case structure.DiagnosticObservationBranchCondition:
		return observation.Branch.Points, observation.Branch.Producers, true
	case structure.DiagnosticObservationTypeConformance:
		return observation.Conformance.Evidence, observation.Conformance.Producers, true
	default:
		return nil, nil, false
	}
}

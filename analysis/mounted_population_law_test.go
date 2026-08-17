package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/testfixture"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/link/mounted"
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
	name   string
	linked *link.Link
	state  *compiledState
	mounts []mounted.Mount
}

func mountedPopulationCases(t *testing.T) []mountedPopulationCase {
	t.Helper()
	corpus, err := testfixture.LoadCorpus(architectureBatteryRepositoryRoot(t))
	if err != nil {
		t.Fatalf("load frozen corpus: %v", err)
	}
	contract := corpusHarnessContract(t)
	cases := make([]mountedPopulationCase, 0, len(mountedPopulationFixtures))
	for _, name := range mountedPopulationFixtures {
		project, err := corpus.Project(name)
		if err != nil {
			t.Fatalf("fixture project %q: %v", name, err)
		}
		linked, err := testfixture.SealCorpusProject(contract, project)
		if err != nil {
			t.Fatalf("seal fixture %q: %v", name, err)
		}
		plan, status, diagnostics := CompileWithDiagnostics(linked)
		if status != CompileComplete || plan == nil || plan.state == nil || plan.state.artifacts == nil {
			t.Fatalf("compile fixture %q = %v diagnostics=%+v", name, status, diagnostics)
		}
		t.Cleanup(func() { plan.Close() })
		// The compiled query plan is instantiated with the runtime topology, so
		// the replacement receipt has to reach the same seam a solve does.
		if _, ok := plan.state.instantiateRuntimeTopology(); !ok {
			t.Fatalf("instantiate runtime topology for fixture %q", name)
		}
		rows := make([]mounted.Mount, 0, len(plan.state.artifacts.mounts))
		for _, mount := range plan.state.artifacts.mounts {
			rows = append(rows, mounted.Mount{ModuleKey: mount.moduleKey, Artifact: mount.artifact})
		}
		cases = append(cases, mountedPopulationCase{name: name, linked: linked, state: plan.state, mounts: rows})
	}
	return cases
}

// reversed returns the same mount rows in the opposite order. Every population
// claims to be a function of sealed content alone, so deriving one from a
// permuted input must produce the identical column.
func (testCase mountedPopulationCase) reversed() []mounted.Mount {
	rows := make([]mounted.Mount, len(testCase.mounts))
	for index, mount := range testCase.mounts {
		rows[len(rows)-1-index] = mount
	}
	return rows
}

// TestMountedExecutionPointDenominatorIsTotalOverPlacedArtifacts proves the
// subject population: every point row of every placed artifact is a member, in
// canonical key order, and a callable body's interior is a member whether a
// call reaches it or not.
func TestMountedExecutionPointDenominatorIsTotalOverPlacedArtifacts(t *testing.T) {
	for _, testCase := range mountedPopulationCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			points, ok := mounted.SealExecutionPoints(testCase.mounts)
			if !ok || !points.Available() {
				t.Fatalf("seal mounted execution points: ok=%v available=%v", ok, points.Available())
			}
			expected := 0
			for _, mount := range testCase.mounts {
				expected += mount.Artifact.PointCount()
				for index := 0; index < mount.Artifact.PointCount(); index++ {
					point, pointOK := mount.Artifact.PointAt(index)
					if !pointOK {
						t.Fatalf("artifact point %d is not addressable", index)
					}
					key := mounted.ExecutionPoint{Mount: mount.ModuleKey, Point: point.ID()}
					if !points.Contains(key) {
						t.Fatalf("point %s of mount %s is outside the denominator", point.ID(), mount.ModuleKey)
					}
				}
			}
			if points.Count() != expected {
				t.Fatalf("denominator = %d members, placed artifacts carry %d points", points.Count(), expected)
			}
			for index := 1; index < points.Count(); index++ {
				previous, previousOK := points.At(index - 1)
				current, currentOK := points.At(index)
				if !previousOK || !currentOK || mounted.CompareExecutionPoint(previous, current) >= 0 {
					t.Fatalf("denominator row %d breaks canonical key order", index)
				}
			}
			permuted, permutedOK := mounted.SealExecutionPoints(testCase.reversed())
			if !permutedOK || permuted.Count() != points.Count() {
				t.Fatalf("permuted denominator = %d members, want %d", permuted.Count(), points.Count())
			}
			for index := 0; index < points.Count(); index++ {
				direct, _ := points.At(index)
				other, _ := permuted.At(index)
				if direct != other {
					t.Fatalf("denominator row %d depends on mount order", index)
				}
			}
		})
	}
}

// TestMountedCallableInteriorIsDenominatorMemberAndNeverASeed proves the
// runtime cut the root set states: a callable body's points are subjects, and
// none of them is seeded by an execution root.
func TestMountedCallableInteriorIsDenominatorMemberAndNeverASeed(t *testing.T) {
	interiors := 0
	for _, testCase := range mountedPopulationCases(t) {
		points, pointsOK := mounted.SealExecutionPoints(testCase.mounts)
		roots, rootsOK := mounted.SealExecutionRoots(testCase.mounts)
		if !pointsOK || !rootsOK {
			t.Fatalf("%s: seal populations: points=%v roots=%v", testCase.name, pointsOK, rootsOK)
		}
		seeds := roots.Seeds()
		for _, mount := range testCase.mounts {
			callable := make(map[identity.ContentID]struct{})
			for index := 0; index < mount.Artifact.BodyCount(); index++ {
				body, bodyOK := mount.Artifact.BodyAt(index)
				if !bodyOK {
					t.Fatalf("%s: artifact body %d is not addressable", testCase.name, index)
				}
				if body.Callable() {
					callable[body.ID()] = struct{}{}
				}
			}
			for index := 0; index < mount.Artifact.OccurrenceCount(); index++ {
				occurrence, occurrenceOK := mount.Artifact.OccurrenceAt(index)
				if !occurrenceOK {
					t.Fatalf("%s: occurrence %d is not addressable", testCase.name, index)
				}
				body, bodyOK := occurrence.BodyID()
				if !bodyOK {
					continue
				}
				if _, interior := callable[body]; !interior {
					continue
				}
				for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
					point, pointOK := occurrence.PointAt(pointIndex)
					if !pointOK {
						t.Fatalf("%s: occurrence point %d is not addressable", testCase.name, pointIndex)
					}
					key := mounted.ExecutionPoint{Mount: mount.ModuleKey, Point: point}
					if !points.Contains(key) {
						t.Fatalf("%s: callable interior point %s is outside the denominator", testCase.name, point)
					}
					if seeds.Contains(key) {
						t.Fatalf("%s: callable interior point %s is seeded as an execution root", testCase.name, point)
					}
					interiors++
				}
			}
		}
	}
	if interiors == 0 {
		t.Fatal("selected fixtures carry no callable interior point; the runtime cut is unproven")
	}
}

// TestMountedExecutionRootsSeedExactlyTheCompiledQueryPlan is the replacement
// receipt. The independent root set is derived without consulting a query
// family; the points it seeds are exactly the points the compiled query plan
// attaches to, fixture by fixture.
func TestMountedExecutionRootsSeedExactlyTheCompiledQueryPlan(t *testing.T) {
	for _, testCase := range mountedPopulationCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			roots, ok := mounted.SealExecutionRoots(testCase.mounts)
			if !ok || !roots.Available() {
				t.Fatalf("seal mounted execution roots: ok=%v available=%v", ok, roots.Available())
			}
			plan := testCase.state.queryPlan
			if plan == nil || len(plan.rows) == 0 {
				t.Fatal("compiled plan carries no query rows")
			}
			planned := make(map[mounted.ExecutionPoint]struct{}, len(plan.rows))
			for _, row := range plan.rows {
				planned[mounted.ExecutionPoint{Mount: row.mount, Point: row.point}] = struct{}{}
			}
			seeds := roots.Seeds()
			if seeds.Count() != len(planned) {
				t.Fatalf("execution roots seed %d points, the compiled query plan attaches at %d", seeds.Count(), len(planned))
			}
			for index := 0; index < seeds.Count(); index++ {
				seed, seedOK := seeds.At(index)
				if !seedOK {
					t.Fatalf("seed %d is not addressable", index)
				}
				if _, attached := planned[seed]; !attached {
					t.Fatalf("seed %s of mount %s is not a compiled query point", seed.Point, seed.Mount)
				}
			}
			bodies := make(map[identity.ContentID]programartifact.BodyRow)
			for _, mount := range testCase.mounts {
				for index := 0; index < mount.Artifact.BodyCount(); index++ {
					body, bodyOK := mount.Artifact.BodyAt(index)
					if !bodyOK {
						t.Fatalf("artifact body %d is not addressable", index)
					}
					bodies[body.ID()] = body
				}
			}
			for index := 0; index < roots.Count(); index++ {
				root, rootOK := roots.At(index)
				if !rootOK {
					t.Fatalf("root %d is not addressable", index)
				}
				body, held := bodies[root.Body]
				if !held || body.Callable() {
					t.Fatalf("root %d names body %s, which is %v/callable=%v", index, root.Body, held, body.Callable())
				}
				if index != 0 {
					previous, _ := roots.At(index - 1)
					if mounted.CompareExecutionRoot(previous, root) >= 0 {
						t.Fatalf("root row %d breaks canonical key order", index)
					}
				}
			}
			permuted, permutedOK := mounted.SealExecutionRoots(testCase.reversed())
			if !permutedOK || permuted.Count() != roots.Count() || permuted.Seeds().Count() != seeds.Count() {
				t.Fatalf("permuted roots = %d/%d, want %d/%d", permuted.Count(), permuted.Seeds().Count(), roots.Count(), seeds.Count())
			}
			for index := 0; index < roots.Count(); index++ {
				direct, _ := roots.At(index)
				other, _ := permuted.At(index)
				if direct != other {
					t.Fatalf("root row %d depends on mount order", index)
				}
			}
		})
	}
}

// TestMountedObservationCensusCapturesTheCompiledObservationSites is the census
// receipt. Every site the current diagnostic projection observes is a census
// row with the same kind, span, and -- for a branch -- the same
// anchor-to-execution geometry, and the census carries no site that projection
// does not observe.
func TestMountedObservationCensusCapturesTheCompiledObservationSites(t *testing.T) {
	branches := 0
	for _, testCase := range mountedPopulationCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			census, ok := mounted.SealObservationSites(testCase.linked.Boundary(), testCase.mounts)
			if !ok || !census.Available() {
				t.Fatalf("seal mounted observation sites: ok=%v available=%v", ok, census.Available())
			}
			coordinates, coordinatesOK := compileValueCoordinates(testCase.linked)
			if !coordinatesOK {
				t.Fatal("compile value coordinates")
			}
			observations, observationsOK := compileDiagnosticObservations(testCase.linked, testCase.state.artifacts, coordinates)
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
				site, present := held[siteKey{mount: observation.mount, local: observation.local}]
				if !present {
					t.Fatalf("observed site %s of mount %s is missing from the census", observation.local, observation.mount)
				}
				if site.Kind != observation.kind || site.Location != observation.location {
					t.Fatalf("census site %s = kind %v span %+v, observed kind %v span %+v", observation.local, site.Kind, site.Location, observation.kind, observation.location)
				}
				if observation.kind != programartifact.DiagnosticObservationBranchCondition {
					if site.ProducerCount() != 0 {
						t.Fatalf("static census site %s carries %d producers", observation.local, site.ProducerCount())
					}
					continue
				}
				branches++
				if site.ProducerCount() != len(observation.producers) {
					t.Fatalf("census site %s carries %d producers, observed %d", observation.local, site.ProducerCount(), len(observation.producers))
				}
				geometry := make(map[compiledObservationProducer]struct{}, len(observation.producers))
				for _, producer := range observation.producers {
					geometry[producer] = struct{}{}
				}
				anchors := make(map[identity.ContentID]struct{}, site.ProducerCount())
				for index := 0; index < site.ProducerCount(); index++ {
					producer, producerOK := site.ProducerAt(index)
					if !producerOK {
						t.Fatalf("census producer %d is not addressable", index)
					}
					key := compiledObservationProducer{role: producer.Role, occurrence: producer.Occurrence, point: producer.Point, anchor: producer.Anchor}
					if _, observed := geometry[key]; !observed {
						t.Fatalf("census site %s carries unobserved producer geometry %+v", observation.local, key)
					}
					anchors[producer.Anchor] = struct{}{}
				}
				if len(anchors) != len(observation.points) {
					t.Fatalf("census site %s anchors %d evidence points, the branch carries %d", observation.local, len(anchors), len(observation.points))
				}
				for _, point := range observation.points {
					if _, anchored := anchors[point]; !anchored {
						t.Fatalf("census site %s leaves evidence point %s unanchored", observation.local, point)
					}
				}
			}
			permuted, permutedOK := mounted.SealObservationSites(testCase.linked.Boundary(), testCase.reversed())
			if !permutedOK || permuted.Count() != census.Count() {
				t.Fatalf("permuted census = %d sites, want %d", permuted.Count(), census.Count())
			}
			for index := 0; index < census.Count(); index++ {
				direct, _ := census.At(index)
				other, _ := permuted.At(index)
				if direct.Mount != other.Mount || direct.Local != other.Local || direct.Kind != other.Kind {
					t.Fatalf("census row %d depends on mount order", index)
				}
			}
		})
	}
	if branches == 0 {
		t.Fatal("selected fixtures observe no branch site; the anchor geometry is unproven")
	}
}

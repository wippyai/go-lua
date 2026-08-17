package axis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// enginePublishedSpec is one axis whose column the engine fills. It declares a
// coordinate space, a writer principal and a published column, and no hot half
// at all: no cold fragment, no factor binding, and no algebra of one, because
// the pass that fills its column is not a factor lane.
func enginePublishedSpec(key, semantic schema.Key) Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64] {
	return Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]{
		Key:         key,
		Storage:     StorageEngine,
		Cardinality: CardinalitySparse,
		Lifetime:    LifetimeProgram,
		Mutability:  MutabilityFrozen,
		Concurrency: ConcurrencyShared,
		Frame:       Frame{Outputs: []Output{{Key: key + "/facts", Writer: key}}},
		Semantic:    semantic,
	}
}

// TestEnginePublishedAxisSealsWithoutAHotHalf states that a non-factor
// principal is declarable. An execution-reachability pass publishes its column
// itself: there is no factor cell to bind and no rule lane to write it, so the
// surface admits an axis that declares the space, the writer and the column and
// stops there. Requiring a hot half of it would make the only way to declare a
// published fact population an empty factor binding.
func TestEnginePublishedAxisSealsWithoutAHotHalf(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
		mustTemplate(t, enginePublishedSpec("reachability", heapRole)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("engine-published axis rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	engine := templates[1]
	if engine.Storage() != StorageEngine || engine.Storage().Bound() {
		t.Fatal("the engine-published axis reports a bound storage")
	}
	if engine.MountDeclared() {
		t.Fatal("the engine-published axis declares a mount")
	}
	if _, ok := engine.Declare(Declaration{Builder: nil}); ok {
		t.Fatal("the engine-published axis recorded a cold shape")
	}
}

// TestEnginePublishedAxisRejectsAHotHalf states the other half of the same
// rule. An axis that declares an engine-published column and also a factor
// binding says two different things about who fills that column, so it is
// rejected at construction rather than sealed as whichever the reader prefers.
func TestEnginePublishedAxisRejectsAHotHalf(t *testing.T) {
	spec := enginePublishedSpec("reachability", heapRole)
	spec.Declare = func(Declaration) (*scratchFragment, bool) { return &scratchFragment{}, true }
	if _, ok := New(spec); ok {
		t.Fatal("an engine-published axis declaring a cold fragment was admitted")
	}
}

// TestBoundAxisRejectsAMissingHotHalf states the same rule from the factor
// side: a Link-bound factor axis without its binding declares a coordinate
// space no lane can write.
func TestBoundAxisRejectsAMissingHotHalf(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Bind = nil
	if _, ok := New(spec); ok {
		t.Fatal("a factor axis without its binding was admitted")
	}
}

// TestOnePublishedColumnHasOneWriter is the write-capability law. A published
// column is what a consumer reads and what the engine mints a write capability
// against, so two axes claiming one column name would leave a reader holding
// rows without knowing whose they are. The table refuses the pair rather than
// letting the first writer to reach the column decide.
func TestOnePublishedColumnHasOneWriter(t *testing.T) {
	first := scratchSpec("value", valueRole)
	first.Frame = Frame{Outputs: []Output{{Key: "shared/facts", Writer: "value"}}}
	second := scratchSpec("heap", heapRole)
	second.Frame = Frame{Outputs: []Output{{Key: "shared/facts", Writer: "heap"}}}
	templates := []*Template[scratchInputs]{mustTemplate(t, first), mustTemplate(t, second)}
	failure := sealTemplates(t, templates)
	if failure.Law != LawOutputUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("two writers of one column sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestOutputWriterIsADeclaredAxis states that a writer is a principal this
// table knows. An axis is a writer principal, so a writer that resolves to no
// declared axis is a capability the engine would mint for a writer no law is
// ever stated about.
func TestOutputWriterIsADeclaredAxis(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Frame = Frame{Outputs: []Output{{Key: "value/facts", Writer: "reachability"}}}
	failure := sealTemplates(t, []*Template[scratchInputs]{mustTemplate(t, spec)})
	if failure.Law != LawOutputWriterResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("an undeclared writer sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestOutputDeclaresBothHalves states that a column and its writer are declared
// together. A column with no writer is one nothing may fill, and a writer over
// no column is a capability over nothing.
func TestOutputDeclaresBothHalves(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	template.outputs = []Output{{Key: "value/facts"}}
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawOutputDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("a column without a writer sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestPublishedFrameIsContent states that the frame is declared data. Which
// columns a catalog publishes, and which principal is admitted to write each of
// them, is what a consumer addresses and what a write capability is minted
// against, so two catalogs that differ in either are different catalogs and the
// table digest says so.
func TestPublishedFrameIsContent(t *testing.T) {
	published := scratchSpec("value", valueRole)
	published.Frame = Frame{Outputs: []Output{{Key: "value/facts", Writer: "value"}}}
	unpublished := scratchSpec("value", valueRole)
	rewritten := scratchSpec("value", valueRole)
	rewritten.Frame = Frame{Outputs: []Output{{Key: "value/summary", Writer: "value"}}}

	digests := make(map[identity.ContentID]schema.Key, 3)
	for _, spec := range []Spec[scratchInputs, *scratchFragment, *scratchAxis, uint64]{published, unpublished, rewritten} {
		sealed, failure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, spec)})
		if failure.Available() {
			t.Fatalf("catalog rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
		}
		digest := sealed.Digest()
		if prior, collision := digests[digest]; collision {
			t.Fatalf("catalogs publishing %q and %q share one digest", prior, spec.Frame.Outputs)
		}
		digests[digest] = spec.Key
	}
}

// TestCoverageFollowsCardinality states the single derivation between the
// declared key-space shape and what a published column concludes about a key it
// holds no row for. A publisher reads this instead of deciding the question a
// second time from its own reading of the metadata, which is what keeps the
// analyzer's storage vocabulary spelled once.
func TestCoverageFollowsCardinality(t *testing.T) {
	if CoverageFor(CardinalityDense) != CoverageTotal {
		t.Fatal("a dense axis publishes a column that cannot prove an absence")
	}
	if CoverageFor(CardinalitySparse) != CoveragePartial {
		t.Fatal("a sparse axis publishes a column that claims to prove an absence")
	}
	if CoverageFor(CardinalityInvalid).Available() {
		t.Fatal("an undeclared cardinality projects a usable coverage")
	}
}

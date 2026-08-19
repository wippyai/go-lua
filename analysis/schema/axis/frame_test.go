package axis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// TestOutputDeclaresBothHalves states that a column and its writer are declared
// together. A column with no writer is one nothing may fill, and a writer over
// no column is a capability over nothing.
// Secondary subject: axis.go states LawOutputDeclared at surface.Seal.
func TestOutputDeclaresBothHalves(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	template.outputs = []Output{{Key: "value/facts"}}
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawOutputDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("a column without a writer sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestOnePublishedColumnHasOneWriter is the write-capability law. A published
// column is what a consumer reads and what the engine mints a write capability
// against, so two axes claiming one column name would leave a reader holding
// rows without knowing whose they are. The table refuses the pair rather than
// letting the first writer to reach the column decide.
// Secondary subject: axis.go states LawOutputUnique at surface.Seal.
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
// Secondary subject: axis.go states LawOutputWriterResolves at surface.Seal.
func TestOutputWriterIsADeclaredAxis(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Frame = Frame{Outputs: []Output{{Key: "value/facts", Writer: "reachability"}}}
	failure := sealTemplates(t, []*Template[scratchInputs]{mustTemplate(t, spec)})
	if failure.Law != LawOutputWriterResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("an undeclared writer sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestPublishedFrameIsContent states that the frame is declared data. Which
// columns a catalog publishes, and which principal is admitted to write each of
// them, is what a consumer addresses and what a write capability is minted
// against, so two catalogs that differ in either are different catalogs and the
// table digest says so.
// Secondary subject: axis.go writes the frame into Template.EntryContent.
func TestPublishedFrameIsContent(t *testing.T) {
	published := scratchSpec("value", valueRole)
	published.Frame = Frame{Outputs: []Output{{Key: "value/facts", Writer: "value"}}}
	unpublished := scratchSpec("value", valueRole)
	rewritten := scratchSpec("value", valueRole)
	rewritten.Frame = Frame{Outputs: []Output{{Key: "value/summary", Writer: "value"}}}

	digests := make(map[identity.ContentID]schema.Key, 3)
	for _, spec := range []Spec[scratchInputs]{published, unpublished, rewritten} {
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
// Secondary subject: axis.go declares the Cardinality this coverage derives from.
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

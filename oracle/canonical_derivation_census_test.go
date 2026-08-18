package oracle

import (
	"testing"

	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// TestCanonicalDerivationCensusIsShared is the corpus-scale statement of the
// derive-once records. The corpus is 912 analyses over one shared standard
// library, so the vocabulary of canonical forms and the closed subtype universe
// they seal are overwhelmingly the same from fixture to fixture. This walk
// measures that: it reports the distinct forms and universes the corpus
// actually contains against the number of times they were asked for, and fails
// only if a record answers nothing - a record that never hits is a record whose
// key does not describe its workload.
//
// The counts are process-wide and cumulative, which is what makes them the
// right diagnostic here: the corpus is exactly one process.
func TestCanonicalDerivationCensusIsShared(t *testing.T) {
	counts, outcomes := censusRatchetCensus(t, corpusHarnessProjects(t))
	if len(outcomes) != corpusHarnessProjectCount {
		t.Fatalf("census walked %d fixtures, want %d", len(outcomes), corpusHarnessProjectCount)
	}
	t.Logf("census invalid=%d complete=%d incomplete=%d unsupported=%d", counts[0], counts[1], counts[2], counts[3])

	wire, graph := typ.CanonicalFormCensus()
	t.Logf("canonical wire forms: distinct=%d sightings=%d misses=%d", wire.Forms, wire.Sightings, wire.Misses)
	t.Logf("canonical graph forms: distinct=%d sightings=%d misses=%d", graph.Forms, graph.Sightings, graph.Misses)
	seals, materializations, judged := typeauthority.RuntimeRelationCensus()
	t.Logf("subtype relation: seals=%d materialized=%d judged-pairs=%d", seals, materializations, judged)

	for name, record := range map[string]typ.CanonicalFormRecordCensus{"wire": wire, "graph": graph} {
		if record.Sightings == 0 {
			t.Fatalf("the %s form record was never consulted by the corpus", name)
		}
		if record.Forms == 0 {
			t.Fatalf("the %s form record admitted no form over the whole corpus", name)
		}
		if record.Misses >= record.Sightings {
			t.Fatalf("the %s form record answered none of its %d sightings; its key does not describe the workload", name, record.Sightings)
		}
	}
	if seals == 0 {
		t.Fatal("the corpus sealed no subtype relation")
	}
	if materializations >= seals {
		t.Fatalf("the corpus materialized %d relations for %d seals; no closed universe was shared", materializations, seals)
	}
}

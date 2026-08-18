package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
)

// Observation construction enumerates the sealed observation table. Producer
// family names stay on their owning registrations.

func TestObservationConstructionUsesSealedProducers(t *testing.T) {
	issued := composite.ObservationIssuance()
	if len(issued) == 0 {
		t.Fatal("sealed table declares no observation rows")
	}
	producer, producerOK := composite.ObservationProducerForPopulationKind(structure.DiagnosticObservationBranchCondition.Key())
	if !producerOK || !producer.Available() {
		t.Fatal("sealed table names no producer for the branch-condition population")
	}
	seen := false
	for _, observation := range issued {
		if !observation.Key.Available() || !observation.Producer.Available() {
			t.Fatalf("issued observation %q is not a complete sealed identity", observation.Key)
		}
		if observation.Population.Kind.Key == structure.DiagnosticObservationBranchCondition.Key() {
			if observation.Producer != producer {
				t.Fatalf("branch-condition producer %q is not issuance producer %q", observation.Producer, producer)
			}
			seen = true
		}
	}
	if !seen {
		t.Fatal("sealed issuance holds no branch-condition observation")
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	root := filepath.Dir(thisFile)
	for _, name := range []string{"artifact_diagnostic_plan.go", filepath.Join("..", "domain", "composite", "publication", "branch_value_observation.go")} {
		src, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(src), `"value-summary"`) {
			t.Fatalf("%s restates producer %q; construction walks composite.ObservationIssuance", name, "value-summary")
		}
	}
}

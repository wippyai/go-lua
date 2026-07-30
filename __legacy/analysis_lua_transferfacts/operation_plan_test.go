package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

func TestLowerDetailedAttachesDenseOperationPlan(t *testing.T) {
	graph := cfg.New()
	lowered := LowerDetailed(graph, Config{Registry: standard.Registry(), WIR: wir.NewBody("plan")})
	if lowered.Plan == nil {
		t.Fatal("lowering did not attach an operation plan")
	}
	if got, want := lowered.Plan.PointCount(), graph.Size(); got != want {
		t.Fatalf("plan rows=%d want %d", got, want)
	}
	if lowered.Facts.HasCallSites() != lowered.Plan.Facts().HasCallSites() {
		t.Fatal("lowered facts do not alias the plan-owned snapshot")
	}
}

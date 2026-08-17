package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
)

func TestDeriveComponentsRequiresASealedSourceControlGraph(t *testing.T) {
	if parts, err := deriveComponents(nil); err == nil || len(parts.of) != 0 {
		t.Fatalf("deriveComponents accepted nil graph: parts=%v err=%v", parts, err)
	}
	if parts, err := deriveComponents(&sourcecontrol.Result{}); err == nil || len(parts.sizes) != 0 {
		t.Fatalf("deriveComponents accepted unavailable graph: parts=%v err=%v", parts, err)
	}
}

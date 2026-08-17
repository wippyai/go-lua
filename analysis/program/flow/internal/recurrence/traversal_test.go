package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestBuildEventTraceRequiresStructuralOwners(t *testing.T) {
	if trace, err := buildEventTrace(source.View{}, authored.View{}, nil, nil, nil, components{}); err == nil || len(trace.events) != 0 {
		t.Fatalf("buildEventTrace accepted unavailable owners: trace=%v err=%v", trace, err)
	}
}

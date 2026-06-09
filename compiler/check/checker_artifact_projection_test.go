package check

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
)

type artifactProjectionProbe struct {
	name   string
	called int
}

func (p *artifactProjectionProbe) Name() string { return p.name }

func (p *artifactProjectionProbe) Project(_ *cfg.Graph, scopes map[cfg.Point]*scope.State) any {
	p.called++
	return len(scopes)
}

func TestChecker_WithArtifactProjection(t *testing.T) {
	projection := &artifactProjectionProbe{name: "probe"}
	c := NewChecker(db.New(), Deps{Types: core.NewEngine()}, WithArtifactProjection(projection))
	sess := c.Check("local x = 1", "test.lua")
	if sess == nil {
		t.Fatal("Check returned nil")
	}
	if projection.called == 0 {
		t.Fatal("artifact projection was not called")
	}
	found := false
	for _, result := range sess.Results {
		if result == nil || result.Extras == nil {
			continue
		}
		if v, ok := result.Extras[projection.name]; ok {
			if _, ok := v.(int); !ok {
				t.Fatalf("artifact projection extra has wrong type: %T", v)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected artifact projection result in FuncResult.Extras")
	}
}

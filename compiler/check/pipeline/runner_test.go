package pipeline

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNewRunner(t *testing.T) {
	globals := map[string]typ.Type{"print": typ.Any}
	r := NewRunner(RunnerConfig{
		GlobalTypes:   globals,
		MaxScopeDepth: 10,
	})
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
	if r.globalTypes["print"] != typ.Any {
		t.Error("globals not set")
	}
	if r.maxScopeDepth != 10 {
		t.Error("maxScopeDepth not set")
	}
}

func TestRunner_Run_NilStore(t *testing.T) {
	r := NewRunner(RunnerConfig{})
	result := r.Run(nil, api.FuncKey{})
	if result != nil {
		t.Error("expected nil when context is nil")
	}
}

func TestRunnerConfig_Fields(t *testing.T) {
	cfg := RunnerConfig{
		MaxScopeDepth: 5,
	}
	if cfg.MaxScopeDepth != 5 {
		t.Error("field not set")
	}
}

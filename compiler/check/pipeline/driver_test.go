package pipeline

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestNew(t *testing.T) {
	d := New(Config{
		MaxScopeDepth: 10,
		GlobalTypes:   map[string]typ.Type{"print": typ.Any},
	})
	if d == nil {
		t.Fatal("expected non-nil driver")
	}
	if d.cfg.MaxScopeDepth != 10 {
		t.Error("MaxScopeDepth not set")
	}
	if got, ok := d.globalTypes.Type("print"); !ok || !typ.TypeEquals(got, typ.Any) {
		t.Errorf("globalTypes(print) = %v/%v, want any/true", got, ok)
	}
}

func TestDriver_Run_NilSession(t *testing.T) {
	d := New(Config{})
	d.Run(nil, nil)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		MaxScopeDepth: 8,
		EmitScopeDiag: true,
		GlobalTypes:   map[string]typ.Type{"foo": typ.String},
	}
	if cfg.MaxScopeDepth != 8 {
		t.Error("MaxScopeDepth not set")
	}
	if !cfg.EmitScopeDiag {
		t.Error("EmitScopeDiag not set")
	}
	if cfg.GlobalTypes["foo"] != typ.String {
		t.Error("GlobalTypes not set")
	}
}

package regression

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// zzScopeFixture reads a types/<name>/main.lua fixture source.
func zzScopeFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "fixtures", "types", name, "main.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// TestZZScopeProbe captures diagnostics for the lexical-scope target fixtures. It
// is a diagnostic probe, not a gate.
func TestZZScopeProbe(t *testing.T) {
	for _, name := range []string{"not-visible-outside-block", "used-before-definition", "shadowing"} {
		name := name
		t.Run(name, func(t *testing.T) {
			src := zzScopeFixture(t, name)
			result := testutil.Check(src, testutil.WithStdlib())
			t.Logf("=== %s ===", name)
			for _, d := range result.Diagnostics {
				t.Logf("FLOW %d:%d %s | %s", d.Position.Line, d.Position.Column, d.Code.Name(), d.Message)
			}
		})
	}
}

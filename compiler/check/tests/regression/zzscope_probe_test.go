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

// TestZZScopeProbe captures the legacy vs canonical diagnostics for the three
// lexical-scope target fixtures. It is a diagnostic probe, not a parity gate.
func TestZZScopeProbe(t *testing.T) {
	for _, name := range []string{"not-visible-outside-block", "used-before-definition", "shadowing"} {
		name := name
		t.Run(name, func(t *testing.T) {
			src := zzScopeFixture(t, name)
			diff := testutil.Differential(src, "main.lua", testutil.WithStdlib())
			t.Logf("=== %s ===", name)
			for _, e := range diff.LegacyAll {
				t.Logf("LEGACY    %d:%d %s | %s", e.Diagnostic.Position.Line, e.Diagnostic.Position.Column, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
			for _, e := range diff.CanonicalAll {
				t.Logf("CANONICAL %d:%d %s | %s", e.Diagnostic.Position.Line, e.Diagnostic.Position.Column, e.Diagnostic.Code.Name(), e.Diagnostic.Message)
			}
		})
	}
}

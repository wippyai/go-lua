//go:build zzsolveprobe

package oracle

import (
	"os"
	"testing"
)

// TestZZProbeCompileDiagnostic prints the full compile error for one named
// fixture so a refusal's stage/rule signature is attributable. Set
// ZZPROBE_FIXTURE.
func TestZZProbeCompileDiagnostic(t *testing.T) {
	name := os.Getenv("ZZPROBE_FIXTURE")
	if name == "" {
		t.Skip("set ZZPROBE_FIXTURE to a canonical fixture name")
	}
	run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, name), corpusHarnessDiagnosticMode())
	t.Logf("ZZDIAG fixture=%s class=%s", name, class)
	t.Logf("ZZDIAG signature: %s", zzProbeFailureSignature(run, class))
	if err != nil {
		t.Logf("ZZDIAG error: %v", err)
	}
}

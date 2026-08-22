//go:build zzfamilyprobe

package oracle

import (
	"os"
	"strings"
	"testing"
)

// TestZZFamilyProbe prints every finding one fixture's acceptance run actually
// produced, so a family lane can read the live report instead of inferring it
// from the acceptance verdict. Set ZZFAMILY_FIXTURE to a canonical fixture
// name; ZZFAMILY_CODES optionally restricts the printed rows to a
// comma-separated code list.
func TestZZFamilyProbe(t *testing.T) {
	name := os.Getenv("ZZFAMILY_FIXTURE")
	if name == "" {
		t.Skip("set ZZFAMILY_FIXTURE to a canonical fixture name")
	}
	filter := make(map[string]bool)
	for _, code := range strings.Split(os.Getenv("ZZFAMILY_CODES"), ",") {
		if code = strings.TrimSpace(code); code != "" {
			filter[code] = true
		}
	}
	run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, name), corpusSemanticAcceptanceMode())
	t.Logf("ZZFAMILY fixture=%s class=%s err=%v", name, class, err)
	t.Logf("ZZFAMILY policy=%v unsupported=%v", run.policy.Enabled, run.policyUnsupported)
	if run.report == nil {
		t.Logf("ZZFAMILY report=nil")
		return
	}
	t.Logf("ZZFAMILY available=%v collectionFailure=%d findings=%d", run.report.Available(), run.report.CollectionFailure(), run.report.FindingCount())
	for index := 0; index < run.report.FindingCount(); index++ {
		finding, findingOK := run.report.FindingAt(index)
		if !findingOK {
			t.Logf("ZZFAMILY finding %d unavailable", index)
			continue
		}
		code := finding.Code().String()
		if len(filter) != 0 && !filter[code] {
			continue
		}
		location, _ := finding.Location()
		line, column := location.Start()
		t.Logf("ZZFAMILY finding %d code=%s severity=%d at %s:%d:%d message=%q",
			index, code, finding.Severity(), location.File(), line, column, finding.Message())
	}
}

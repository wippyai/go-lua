package lua

import (
	"os"
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic/diffreport"
)

// TestWriteDiagnosticDiffReport is an opt-in tooling entrypoint for comparing
// diagnostic JSONL snapshots without adding a cmd tree to this repository.
func TestWriteDiagnosticDiffReport(t *testing.T) {
	baselinePath := os.Getenv("DIFFREPORT_BASELINE")
	currentPath := os.Getenv("DIFFREPORT_CURRENT")
	if baselinePath == "" && currentPath == "" {
		t.Skip("set DIFFREPORT_BASELINE and DIFFREPORT_CURRENT to write a diagnostic diff report")
	}
	if baselinePath == "" || currentPath == "" {
		t.Fatal("DIFFREPORT_BASELINE and DIFFREPORT_CURRENT must both be set")
	}

	baseline, err := diffreport.ReadJSONLFile(baselinePath)
	if err != nil {
		t.Fatalf("reading baseline %s: %v", baselinePath, err)
	}
	current, err := diffreport.ReadJSONLFile(currentPath)
	if err != nil {
		t.Fatalf("reading current %s: %v", currentPath, err)
	}
	report := diffreport.Compare(baseline, current)

	outPath := os.Getenv("DIFFREPORT_OUT")
	if outPath == "" {
		if err := diffreport.WriteReport(os.Stdout, report, os.Getenv("DIFFREPORT_FORMAT")); err != nil {
			t.Fatalf("writing report: %v", err)
		}
	} else {
		file, err := os.Create(outPath)
		if err != nil {
			t.Fatalf("creating report %s: %v", outPath, err)
		}
		if err := diffreport.WriteReport(file, report, os.Getenv("DIFFREPORT_FORMAT")); err != nil {
			_ = file.Close()
			t.Fatalf("writing report %s: %v", outPath, err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("closing report %s: %v", outPath, err)
		}
	}

	if envEnabled("DIFFREPORT_FAIL_ON_NEW") && len(report.New) > 0 {
		t.Fatalf("diagnostic diff contains %d new record(s)", len(report.New))
	}
}

func envEnabled(name string) bool {
	switch os.Getenv(name) {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return false
	}
}

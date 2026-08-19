package diagnostic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReportImplementationLivesInThisPackage is the owner fence: rendering
// and report construction are declared here. Analysis root names no alias.
func TestReportImplementationLivesInThisPackage(t *testing.T) {
	here := filepath.Join("report.go")
	body, err := os.ReadFile(here)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "type DiagnosticReport struct") {
		t.Fatal("DiagnosticReport implementation is not in analysis/diagnostic")
	}
	status, err := os.ReadFile(filepath.Join("status.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), "type AnalyzeDiagnostics struct") {
		t.Fatal("AnalyzeDiagnostics implementation is not in analysis/diagnostic")
	}
	collect, err := os.ReadFile(filepath.Join("collect.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(collect), "func CollectGuardPolarity(") || !strings.Contains(string(collect), "func CollectBranch(") {
		t.Fatal("branch collection implementation is not in analysis/diagnostic")
	}
	query, err := os.ReadFile(filepath.Join("collect_query.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(query), "func CollectConformance(") {
		t.Fatal("type-conformance collection implementation is not in analysis/diagnostic")
	}
	static, err := os.ReadFile(filepath.Join("collect_static.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(static), "func CollectStatic(") {
		t.Fatal("static collection implementation is not in analysis/diagnostic")
	}
	observation, err := os.ReadFile(filepath.Join("observation.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(observation), "func ProjectSites(") || !strings.Contains(string(observation), "type Observation struct") {
		t.Fatal("sealed observation census is not in analysis/diagnostic")
	}
	attach, err := os.ReadFile(filepath.Join("attach.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(attach), "func ValueObservations(") || !strings.Contains(string(attach), "func Publications(") {
		t.Fatal("observation attach and publication addressing are not in analysis/diagnostic")
	}
	for _, name := range []string{"diagnostic_report.go", "diagnostics.go"} {
		if _, err := os.Stat(filepath.Join("..", name)); err == nil {
			t.Fatalf("analysis root still holds alias file %s", name)
		}
	}
}

// remainingRootDiagnosticImplementation is shrink-only. The cut lands when
// the list is empty: analysis root retains no diagnostic implementation.
var remainingRootDiagnosticImplementation = []string{}

func TestRemainingRootDiagnosticImplementationIsShrinkOnly(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range remainingRootDiagnosticImplementation {
		if seen[name] {
			t.Fatalf("remaining list must stay unique: %s", name)
		}
		seen[name] = true
		if _, err := os.Stat(filepath.Join("..", name)); err != nil {
			t.Fatalf("stale remaining pin %s; the list is shrink-only, so remove it", name)
		}
	}
}

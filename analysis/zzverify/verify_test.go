package zzverify

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis"
	"github.com/wippyai/go-lua/analysis/internal/testfixture"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
)

func TestZZCorpusCensus(t *testing.T) {
	prefix := os.Getenv("ZZ_PREFIX")
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	projects, err := testfixture.FrozenCorpusProjects()
	if err != nil {
		t.Fatal(err)
	}
	statuses := make([]string, len(projects))
	names := make([]string, len(projects))
	var wait sync.WaitGroup
	limit := make(chan struct{}, 8)
	for index, project := range projects {
		if prefix != "" && !strings.HasPrefix(project.Name(), prefix) {
			continue
		}
		wait.Add(1)
		go func(index int, project testfixture.CorpusProject) {
			defer wait.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			names[index] = project.Name()
			statuses[index] = censusStatus(contract, project)
		}(index, project)
	}
	wait.Wait()
	counts := map[string]int{}
	for index, name := range names {
		if name == "" {
			continue
		}
		counts[statuses[index]]++
		if statuses[index] != "complete" {
			t.Logf("ZZCENSUS %s %s", name, statuses[index])
		}
	}
	t.Logf("ZZCENSUS-TOTALS %v", counts)
}

func censusStatus(contract *target.Contract, project testfixture.CorpusProject) (status string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			status = fmt.Sprintf("panic: %v", recovered)
		}
	}()
	linked, err := testfixture.SealCorpusProject(contract, project)
	if err != nil {
		return "link: " + err.Error()
	}
	plan, compileStatus, diagnostics := analysis.CompileWithDiagnostics(linked)
	if compileStatus != analysis.CompileComplete || plan == nil {
		return fmt.Sprintf("compile:%v phase=%s binding=%s", compileStatus, diagnostics.Phase, diagnostics.Binding)
	}
	defer plan.Close()
	if os.Getenv("ZZ_COMPILE_ONLY") != "" {
		return "complete"
	}
	result, analyzeStatus := plan.Solve(context.Background())
	if analyzeStatus != analysis.AnalyzeComplete || result == nil {
		return fmt.Sprintf("solve:%v", analyzeStatus)
	}
	return "complete"
}

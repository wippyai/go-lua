package analysis

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Temporary diagnosis probe. Delete before finishing.

func diagProbeEnvInt(name string, fallback int) int {
	if raw := os.Getenv(name); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return value
		}
	}
	return fallback
}

func diagProbeSelect(t *testing.T) []corpusHarnessProject {
	projects := corpusHarnessProjects(t)
	stride := diagProbeEnvInt("DIAGPROBE_STRIDE", 1)
	if stride < 1 {
		stride = 1
	}
	if prefix := os.Getenv("DIAGPROBE_PREFIX"); prefix != "" {
		filtered := projects[:0:0]
		for _, project := range projects {
			if strings.HasPrefix(project.name, prefix) {
				filtered = append(filtered, project)
			}
		}
		projects = filtered
	}
	if names := os.Getenv("DIAGPROBE_NAMES"); names != "" {
		wanted := make(map[string]bool)
		for _, name := range strings.Split(names, ",") {
			wanted[strings.TrimSpace(name)] = true
		}
		filtered := projects[:0:0]
		for _, project := range projects {
			if wanted[project.name] {
				filtered = append(filtered, project)
			}
		}
		return filtered
	}
	if stride == 1 {
		return projects
	}
	filtered := make([]corpusHarnessProject, 0, len(projects)/stride+1)
	for index := 0; index < len(projects); index += stride {
		filtered = append(filtered, projects[index])
	}
	return filtered
}

type diagProbeRow struct {
	name    string
	files   int
	status  string
	class   string
	seal    time.Duration
	compile time.Duration
	solve   time.Duration
}

// TestDiagProbeCensusTiming walks the census mode (public Analyze) with
// per-fixture timing and writes a TSV.
func TestDiagProbeCensusTiming(t *testing.T) {
	projects := diagProbeSelect(t)
	workers := diagProbeEnvInt("DIAGPROBE_WORKERS", 32)
	if workers < 1 {
		workers = 1
	}
	if workers > len(projects) {
		workers = len(projects)
	}
	mode := corpusHarnessCensusMode()
	rows := make([]diagProbeRow, len(projects))
	var next atomic.Int64
	var group sync.WaitGroup
	started := time.Now()
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(projects) {
					return
				}
				project := projects[index]
				if os.Getenv("DIAGPROBE_PROGRESS") != "" {
					fmt.Fprintf(os.Stderr, "START %s\n", project.name)
				}
				fixtureStarted := time.Now()
				run, class, _ := corpusHarnessExecute(t, project, mode)
				if os.Getenv("DIAGPROBE_PROGRESS") != "" {
					fmt.Fprintf(os.Stderr, "DONE  %s %s %s\n", project.name, time.Since(fixtureStarted).Round(time.Millisecond), corpusHarnessStatusName(run.status))
				}
				rows[index] = diagProbeRow{
					name:    project.name,
					files:   project.source.FileCount(),
					status:  corpusHarnessStatusName(run.status),
					class:   class,
					seal:    run.cost.seal,
					compile: run.cost.compile,
					solve:   run.cost.solve,
				}
			}
		}()
	}
	group.Wait()
	wallClock := time.Since(started)

	var sum time.Duration
	counts := map[string]int{}
	durations := make([]time.Duration, 0, len(rows))
	for _, row := range rows {
		total := row.seal + row.compile + row.solve
		sum += total
		counts[row.status]++
		durations = append(durations, total)
	}
	sort.Slice(durations, func(a, b int) bool { return durations[a] < durations[b] })
	percentile := func(p float64) time.Duration {
		if len(durations) == 0 {
			return 0
		}
		index := int(p * float64(len(durations)-1))
		return durations[index]
	}

	out := os.Getenv("DIAGPROBE_OUT")
	if out != "" {
		var builder strings.Builder
		builder.WriteString("name\tfiles\tstatus\tclass\tseal_ms\tcompile_ms\tsolve_ms\ttotal_ms\n")
		for _, row := range rows {
			total := row.seal + row.compile + row.solve
			fmt.Fprintf(&builder, "%s\t%d\t%s\t%s\t%.2f\t%.2f\t%.2f\t%.2f\n",
				row.name, row.files, row.status, row.class,
				float64(row.seal.Microseconds())/1000, float64(row.compile.Microseconds())/1000,
				float64(row.solve.Microseconds())/1000, float64(total.Microseconds())/1000)
		}
		if err := os.WriteFile(out, []byte(builder.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	ranked := append([]diagProbeRow(nil), rows...)
	sort.Slice(ranked, func(a, b int) bool {
		return (ranked[a].seal + ranked[a].compile + ranked[a].solve) > (ranked[b].seal + ranked[b].compile + ranked[b].solve)
	})
	limit := 30
	if limit > len(ranked) {
		limit = len(ranked)
	}
	var top strings.Builder
	for _, row := range ranked[:limit] {
		fmt.Fprintf(&top, "\n  %-52s files=%-3d %-11s seal=%-8s compile=%-9s solve=%s",
			row.name, row.files, row.status,
			row.seal.Round(time.Millisecond), row.compile.Round(time.Millisecond), row.solve.Round(time.Millisecond))
	}
	t.Logf("DIAGPROBE fixtures=%d workers=%d wall-clock=%s analysis-wall=%s counts=%v\nmedian=%s p90=%s p99=%s max=%s\nslowest:%s",
		len(rows), workers, wallClock.Round(time.Millisecond), sum.Round(time.Millisecond), counts,
		percentile(0.5).Round(time.Millisecond), percentile(0.9).Round(time.Millisecond),
		percentile(0.99).Round(time.Millisecond), percentile(1.0).Round(time.Millisecond), top.String())
}

// TestDiagProbeIncompleteReasons runs the diagnostic solve over the selected
// fixtures and reports the engine failure certificate of each non-complete one.
func TestDiagProbeIncompleteReasons(t *testing.T) {
	projects := diagProbeSelect(t)
	workers := diagProbeEnvInt("DIAGPROBE_WORKERS", 32)
	if workers < 1 {
		workers = 1
	}
	if workers > len(projects) {
		workers = len(projects)
	}
	mode := corpusHarnessReceiptMode()
	type reasonRow struct {
		name    string
		status  string
		class   string
		detail  string
		elapsed time.Duration
	}
	rows := make([]reasonRow, len(projects))
	var next atomic.Int64
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				index := int(next.Add(1) - 1)
				if index >= len(projects) {
					return
				}
				project := projects[index]
				run, class, _ := corpusHarnessExecute(t, project, mode)
				detail := fmt.Sprintf("phase=%v reason=%v rule=%v stage=%v work=%d epochs=%d passes=%d evals=%d evalfail=%d folds=%d restarts=%d activations=%d maxqueue=%d pubs=%d fr=%v fp=%v",
					run.solveDiagnostics.Phase, run.solveDiagnostics.Reason, run.solveDiagnostics.Rule, run.solveDiagnostics.ReceiptStage,
					run.solveDiagnostics.Engine.Work, run.solveDiagnostics.Engine.Epochs, run.solveDiagnostics.Engine.EpochPasses,
					run.solveDiagnostics.Engine.Evaluates, run.solveDiagnostics.Engine.EvaluateFailures,
					run.solveDiagnostics.Engine.Folds, run.solveDiagnostics.Engine.Restarts, run.solveDiagnostics.Engine.Activations,
					run.solveDiagnostics.Engine.MaxQueue, run.solveDiagnostics.Engine.Publications,
					run.solveDiagnostics.Engine.Failure.Reason(), run.solveDiagnostics.Engine.Failure.Phase())
				rows[index] = reasonRow{name: project.name, status: corpusHarnessStatusName(run.status), class: class, detail: detail, elapsed: run.cost.total()}
			}
		}()
	}
	group.Wait()

	grouped := map[string][]string{}
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.status]++
		if row.status == "complete" {
			continue
		}
		grouped[row.detail] = append(grouped[row.detail], fmt.Sprintf("%s(%s,%s)", row.name, row.status, row.elapsed.Round(time.Millisecond)))
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool { return len(grouped[keys[a]]) > len(grouped[keys[b]]) })
	var report strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&report, "\n[%d] %s\n    %s", len(grouped[key]), key, strings.Join(grouped[key], " "))
	}
	if out := os.Getenv("DIAGPROBE_REASON_OUT"); out != "" {
		var builder strings.Builder
		builder.WriteString("name\tstatus\tclass\telapsed_ms\tdetail\n")
		for _, row := range rows {
			fmt.Fprintf(&builder, "%s\t%s\t%s\t%.2f\t%s\n", row.name, row.status, row.class, float64(row.elapsed.Microseconds())/1000, row.detail)
		}
		if err := os.WriteFile(out, []byte(builder.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("DIAGPROBE-REASONS fixtures=%d counts=%v%s", len(rows), counts, report.String())
}

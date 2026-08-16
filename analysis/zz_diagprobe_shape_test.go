package analysis

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// Temporary diagnosis probe. Delete before finishing.

// TestDiagProbeArtifactShape reports, per selected fixture, the compiled
// artifact's rule-occurrence census per role and its attached point count.
func TestDiagProbeArtifactShape(t *testing.T) {
	projects := diagProbeSelect(t)
	mode := corpusHarnessCompileMode()
	var report strings.Builder
	for _, project := range projects {
		run, class, err := corpusHarnessExecute(t, project, mode)
		if err != nil {
			fmt.Fprintf(&report, "\n%s COMPILE-FAIL %s %v", project.name, class, err)
			continue
		}
		if run.plan == nil || run.plan.state == nil || run.plan.state.artifacts == nil {
			fmt.Fprintf(&report, "\n%s no artifacts", project.name)
			continue
		}
		type roleRow struct {
			role   string
			rows   int
			points int
		}
		var rows []roleRow
		totalRows, totalPoints := 0, 0
		for _, mount := range run.plan.state.artifacts.mounts {
			for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
				role, ok := programartifact.MountedRuleRoleAt(index)
				if !ok {
					continue
				}
				count := mount.artifact.RuleOccurrenceCount(role)
				if count == 0 {
					continue
				}
				points := 0
				for occurrence := 0; occurrence < count; occurrence++ {
					row, rowOK := mount.artifact.RuleOccurrenceAt(role, occurrence)
					if !rowOK {
						continue
					}
					points += row.PointCount()
				}
				rows = append(rows, roleRow{role: fmt.Sprintf("%v", role), rows: count, points: points})
				totalRows += count
				totalPoints += points
			}
		}
		sort.Slice(rows, func(a, b int) bool { return rows[a].points > rows[b].points })
		fmt.Fprintf(&report, "\n%s mounts=%d rules=%d points=%d", project.name, len(run.plan.state.artifacts.mounts), totalRows, totalPoints)
		for _, row := range rows {
			fmt.Fprintf(&report, "\n    %-34s rows=%-6d points=%d", row.role, row.rows, row.points)
		}
	}
	if out := os.Getenv("DIAGPROBE_SHAPE_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(report.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("DIAGPROBE-SHAPE%s", report.String())
}

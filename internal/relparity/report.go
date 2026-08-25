package relparity

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Report is the machine-readable outcome of one parity run.
//
// DivergenceCount is the total; Divergences holds the retained prefix of that
// total in rank order, so a report over a diverged corpus stays a file a lane
// can read while still stating how much diverged.
type Report struct {
	Probe             Probe             `json:"probe"`
	Baseline          Side              `json:"baseline"`
	Replacement       Side              `json:"replacement"`
	WorkingDirectory  string            `json:"working_directory"`
	Shard             int               `json:"shard"`
	Shards            int               `json:"shards"`
	Fixtures          []string          `json:"fixtures"`
	FixtureListDigest string            `json:"fixture_list_digest"`
	Observations      int               `json:"observations"`
	RowsCompared      int               `json:"rows_compared"`
	RowsByDimension   map[Dimension]int `json:"rows_by_dimension"`
	DivergenceCount   int               `json:"divergence_count"`
	DimensionCounts   map[Dimension]int `json:"divergences_by_dimension"`
	Divergences       []Divergence      `json:"divergences"`
	First             *Divergence       `json:"first_divergence"`
	Exhausted         []string          `json:"exhausted_bounds"`
}

// Identical reports whether the two sides agreed everywhere the run reached.
func (report Report) Identical() bool { return report.DivergenceCount == 0 }

// JSON renders the report as the diff artifact a lane consumes.
func (report Report) JSON() ([]byte, error) {
	text, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("relparity: render report: %w", err)
	}
	return append(text, '\n'), nil
}

// Summary is the human line the run prints beside the artifact.
func (report Report) Summary() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "baseline    %s (%s)\n", report.Baseline.Ref, shortCommit(report.Baseline.Commit))
	fmt.Fprintf(&builder, "replacement %s (%s)\n", report.Replacement.Ref, shortCommit(report.Replacement.Commit))
	fmt.Fprintf(&builder, "shard %d/%d, %d fixtures, %d observations, %d rows compared\n",
		report.Shard, report.Shards, len(report.Fixtures), report.Observations, report.RowsCompared)
	fmt.Fprintf(&builder, "rows by dimension: %s\n", renderCounts(report.RowsByDimension))
	for _, note := range report.Exhausted {
		fmt.Fprintf(&builder, "EXHAUSTED BOUND %s\n", note)
	}
	if report.Identical() {
		builder.WriteString("PARITY: no divergence\n")
		return builder.String()
	}
	fmt.Fprintf(&builder, "PARITY: %d divergences (%s)\n",
		report.DivergenceCount, renderCounts(report.DimensionCounts))
	if report.First != nil {
		fmt.Fprintf(&builder, "first divergent row:\n%s\n", report.First)
	}
	return builder.String()
}

func shortCommit(commit string) string {
	if len(commit) > 10 {
		return commit[:10]
	}
	return commit
}

func renderCounts(counts map[Dimension]int) string {
	if len(counts) == 0 {
		return "none"
	}
	names := make([]string, 0, len(counts))
	for dimension := range counts {
		names = append(names, string(dimension))
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, counts[Dimension(name)]))
	}
	return strings.Join(parts, " ")
}

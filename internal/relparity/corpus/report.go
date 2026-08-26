package corpus

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Report is the machine-readable catalogue of one corpus walk.
//
// DivergenceCount is the total and Classes is the histogram over it, so a
// truncated catalogue still states exactly how much diverged and in what
// proportion.
type Report struct {
	Protocol          string          `json:"protocol"`
	Probe             string          `json:"probe"`
	WorkingDirectory  string          `json:"working_directory"`
	Shard             int             `json:"shard"`
	Shards            int             `json:"shards"`
	Workers           int             `json:"workers"`
	FixtureTimeout    string          `json:"fixture_timeout"`
	ProcessTimeout    string          `json:"process_timeout"`
	Fixtures          []string        `json:"fixtures"`
	FixtureListDigest string          `json:"fixture_list_digest"`
	FixtureCount      int             `json:"fixture_count"`
	FixturesAtParity  int             `json:"fixtures_at_parity"`
	FixturesUnreached int             `json:"fixtures_unreached"`
	FixturesDiverged  int             `json:"fixtures_diverged"`
	Observations      int             `json:"observations"`
	RowsCompared      int             `json:"rows_compared"`
	DivergenceCount   int             `json:"divergence_count"`
	Classes           map[Class]int   `json:"divergences_by_class"`
	FixtureClasses    map[Class]int   `json:"fixtures_by_leading_class"`
	Divergences       []Divergence    `json:"divergences"`
	Truncated         bool            `json:"truncated"`
	Elapsed           string          `json:"elapsed"`
	SlowestFixtures   []FixtureTiming `json:"slowest_fixtures"`
}

// FixtureTiming is how long one fixture's observation took, kept for the few
// slowest so a bound that fires has a measured neighbourhood around it.
type FixtureTiming struct {
	Fixture string  `json:"fixture"`
	Seconds float64 `json:"seconds"`
}

// Identical reports whether the two engines agreed everywhere the walk reached.
func (report Report) Identical() bool { return report.DivergenceCount == 0 }

// JSON renders the catalogue as the artifact a lane consumes.
func (report Report) JSON() ([]byte, error) {
	text, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("corpus: render report: %w", err)
	}
	return append(text, '\n'), nil
}

// ParseReport reads a rendered catalogue back. The round trip is what makes
// the report an artifact other lanes can act on rather than a printout.
func ParseReport(text []byte) (Report, error) {
	var report Report
	if err := json.Unmarshal(text, &report); err != nil {
		return Report{}, fmt.Errorf("corpus: read report: %w", err)
	}
	return report, nil
}

// Summary is the human line the walk prints beside the artifact.
func (report Report) Summary() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "probe %s\n", report.Probe)
	fmt.Fprintf(&builder, "shard %d/%d, %d fixtures, %d workers, solve bound %s, process watchdog %s, elapsed %s\n",
		report.Shard, report.Shards, report.FixtureCount, report.Workers,
		report.FixtureTimeout, report.ProcessTimeout, report.Elapsed)
	fmt.Fprintf(&builder, "fixtures: %d at parity, %d diverged, %d never reached either engine\n",
		report.FixturesAtParity, report.FixturesDiverged, report.FixturesUnreached)
	fmt.Fprintf(&builder, "fixtures by leading class: %s\n", renderClasses(report.FixtureClasses))
	if report.Identical() {
		builder.WriteString("PARITY: no divergence\n")
		return builder.String()
	}
	fmt.Fprintf(&builder, "PARITY: %d divergences (%s)\n",
		report.DivergenceCount, renderClasses(report.Classes))
	if len(report.Divergences) > 0 {
		fmt.Fprintf(&builder, "first divergence:\n%s\n", report.Divergences[0])
	}
	return builder.String()
}

func renderClasses(counts map[Class]int) string {
	if len(counts) == 0 {
		return "none"
	}
	names := make([]string, 0, len(counts))
	for class := range counts {
		names = append(names, string(class))
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%d", name, counts[Class(name)]))
	}
	return strings.Join(parts, " ")
}

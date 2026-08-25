package relparity

import (
	"context"
	"fmt"
	"sort"
)

// Plan is one parity run: two built sides, one fixture tree, one shard of one
// fixture list.
type Plan struct {
	Probe            Probe
	Baseline         Side
	Replacement      Side
	WorkingDirectory string
	Fixtures         []string
	Shard            int
	Shards           int
	// RetainedDivergences bounds how many divergences the report carries in
	// full. Every divergence is still counted and the first one is always
	// retained; zero selects DefaultRetainedDivergences.
	RetainedDivergences int
}

// DefaultRetainedDivergences bounds a diverged report to a size a lane reads.
const DefaultRetainedDivergences = 200

// Run compares the two sides over the plan's fixtures and returns the report.
//
// Fixtures are walked in canonical order and verbs in the probe's declared
// order, so the report's first divergence is the first one in that order and
// not the first one observed.
func Run(ctx context.Context, plan Plan) Report {
	report := Report{
		Probe:             plan.Probe,
		Baseline:          plan.Baseline,
		Replacement:       plan.Replacement,
		WorkingDirectory:  plan.WorkingDirectory,
		Shard:             plan.Shard,
		Shards:            plan.Shards,
		Fixtures:          plan.Fixtures,
		FixtureListDigest: FixtureListDigest(plan.Fixtures),
		DimensionCounts:   map[Dimension]int{},
		RowsByDimension:   map[Dimension]int{},
	}
	retained := plan.RetainedDivergences
	if retained <= 0 {
		retained = DefaultRetainedDivergences
	}
	fixtures := append([]string(nil), plan.Fixtures...)
	sort.Strings(fixtures)

	rank := 0
	for _, fixture := range fixtures {
		for _, verb := range plan.Probe.Verbs {
			baseline := Observe(ctx, plan.Baseline, plan.Probe, plan.WorkingDirectory, fixture, verb)
			replacement := Observe(ctx, plan.Replacement, plan.Probe, plan.WorkingDirectory, fixture, verb)
			report.Observations++
			if baseline.TimedOut || replacement.TimedOut {
				report.Exhausted = append(report.Exhausted,
					fmt.Sprintf("%s %s: baseline timed-out=%t replacement timed-out=%t (bound %s)",
						fixture, verb, baseline.TimedOut, replacement.TimedOut, plan.Probe.Timeout))
			}
			baselineRows := ParseDump(fixture, verb, baseline.Stdout)
			report.RowsCompared += len(baselineRows)
			for _, row := range baselineRows {
				report.RowsByDimension[row.Dimension]++
			}
			for _, divergence := range CompareObservations(baseline, replacement) {
				divergence.Rank = rank
				rank++
				report.DivergenceCount++
				report.DimensionCounts[divergence.Dimension]++
				if len(report.Divergences) < retained {
					report.Divergences = append(report.Divergences, divergence)
				}
			}
		}
	}
	if len(report.Divergences) > 0 {
		first := report.Divergences[0]
		report.First = &first
	}
	return report
}

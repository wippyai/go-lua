package relparity

import "fmt"

// DivergenceKind names how two sides failed to agree at one address.
type DivergenceKind string

const (
	// KindValue is one address held by both sides under different values.
	KindValue DivergenceKind = "value"
	// KindAbsentReplacement is an address the baseline published and the
	// replacement did not.
	KindAbsentReplacement DivergenceKind = "absent-in-replacement"
	// KindAbsentBaseline is an address the replacement published and the
	// baseline did not.
	KindAbsentBaseline DivergenceKind = "absent-in-baseline"
	// KindProcess is a disagreement about the run itself: exit status,
	// refusal text, or a side that exhausted its bound.
	KindProcess DivergenceKind = "process"
)

// Divergence is one named disagreement. Rank orders the whole report
// deterministically so "the first divergent row" is a fact about the
// comparison rather than about the order the runs happened to finish in.
type Divergence struct {
	Fixture     string         `json:"fixture"`
	Verb        string         `json:"verb"`
	Key         string         `json:"key"`
	Occurrence  int            `json:"occurrence"`
	Dimension   Dimension      `json:"dimension"`
	Kind        DivergenceKind `json:"kind"`
	Baseline    string         `json:"baseline"`
	Replacement string         `json:"replacement"`
	Rank        int            `json:"rank"`
}

// String renders one divergence as the line a lane acts on.
func (divergence Divergence) String() string {
	return fmt.Sprintf("%s %s [%s/%s] %s#%d\n  baseline:    %s\n  replacement: %s",
		divergence.Fixture, divergence.Verb, divergence.Dimension, divergence.Kind,
		divergence.Key, divergence.Occurrence, divergence.Baseline, divergence.Replacement)
}

// CompareRows compares two row sets addressed by accessor and occurrence.
//
// Ordering: every address the baseline published is examined in baseline order
// first, then every address only the replacement published, in replacement
// order. The result is therefore total over both sides and independent of map
// iteration.
func CompareRows(baseline, replacement []Row) []Divergence {
	replacementByAddress := make(map[string]Row, len(replacement))
	for _, row := range replacement {
		replacementByAddress[row.Address()] = row
	}
	baselineByAddress := make(map[string]Row, len(baseline))
	for _, row := range baseline {
		baselineByAddress[row.Address()] = row
	}

	var divergences []Divergence
	for _, row := range baseline {
		other, held := replacementByAddress[row.Address()]
		switch {
		case !held:
			divergences = append(divergences, Divergence{
				Fixture: row.Fixture, Verb: row.Verb, Key: row.Key, Occurrence: row.Occurrence,
				Dimension: row.Dimension, Kind: KindAbsentReplacement, Baseline: row.Value,
			})
		case other.Value != row.Value:
			divergences = append(divergences, Divergence{
				Fixture: row.Fixture, Verb: row.Verb, Key: row.Key, Occurrence: row.Occurrence,
				Dimension: row.Dimension, Kind: KindValue, Baseline: row.Value, Replacement: other.Value,
			})
		}
	}
	for _, row := range replacement {
		if _, held := baselineByAddress[row.Address()]; held {
			continue
		}
		divergences = append(divergences, Divergence{
			Fixture: row.Fixture, Verb: row.Verb, Key: row.Key, Occurrence: row.Occurrence,
			Dimension: row.Dimension, Kind: KindAbsentBaseline, Replacement: row.Value,
		})
	}
	return divergences
}

// CompareObservations compares one (fixture, verb) end to end: how the two
// processes ended, and then the rows they published.
//
// A side that exhausted its bound, or ended with a status the other did not,
// is a process divergence and is reported before any row. The harness never
// treats an exhausted bound as agreement; the run that did not finish is the
// finding.
func CompareObservations(baseline, replacement Observation) []Divergence {
	var divergences []Divergence
	appendProcess := func(key, left, right string) {
		divergences = append(divergences, Divergence{
			Fixture: baseline.Fixture, Verb: baseline.Verb, Key: key,
			Dimension: DimensionProcess, Kind: KindProcess, Baseline: left, Replacement: right,
		})
	}
	if baseline.TimedOut != replacement.TimedOut {
		appendProcess("process.timed-out",
			fmt.Sprintf("%t", baseline.TimedOut), fmt.Sprintf("%t", replacement.TimedOut))
	}
	if baseline.ExitCode != replacement.ExitCode {
		appendProcess("process.exit-code",
			fmt.Sprintf("%d", baseline.ExitCode), fmt.Sprintf("%d", replacement.ExitCode))
	}
	if baseline.Stderr != replacement.Stderr {
		appendProcess("process.stderr", baseline.Stderr, replacement.Stderr)
	}
	divergences = append(divergences,
		CompareRows(
			ParseDump(baseline.Fixture, baseline.Verb, baseline.Stdout),
			ParseDump(replacement.Fixture, replacement.Verb, replacement.Stdout),
		)...)
	return divergences
}

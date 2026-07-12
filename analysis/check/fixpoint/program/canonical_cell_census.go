package program

import (
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// CanonicalCellResets classifies why the persistent-call POC would have to
// discard a function workspace instead of extending/reusing it. Stage 0 only
// observes current solves; it never selects a solve path.
type CanonicalCellResets struct {
	CalleeRevision int
	EntryShrink    int
	Context        int
	Routing        int
}

// CanonicalCellStats reports the behavior-neutral persistent-cell simulation
// for one exact lexical-body/semantic-entry cell.
type CanonicalCellStats struct {
	BodyID                       uint64
	Function                     summary.SummaryKey
	Context                      bool
	ActualBodySolves             int
	WorkspaceBuilds              int
	EligibleMonotoneExtensions   int
	Resets                       CanonicalCellResets
	TheoreticalBodySolvesAvoided int
}

// CanonicalCellReport is one program unit's deterministic Stage-0 census.
// RegisteredStateLanes comes directly from the production State catalog;
// ContextKeyDimensions comes directly from summary.EntryKey's fields.
type CanonicalCellReport struct {
	LexicalFunctions             int
	SemanticContextCells         int
	RegisteredStateLanes         []state.LaneID
	ContextKeyDimensions         []string
	ActualBodySolves             int
	WorkspaceBuilds              int
	EligibleMonotoneExtensions   int
	Resets                       CanonicalCellResets
	TheoreticalBodySolvesAvoided int
	Cells                        []CanonicalCellStats
}

type canonicalCellKey struct {
	bodyID   uint64
	function summary.SummaryKey
}

type canonicalSolveObservation struct {
	valid       bool
	registry    *axis.Registry
	entry       state.State
	lanes       []state.LaneID
	inputDigest uint64
	resolution  uint64
}

type canonicalCellAccumulator struct {
	stats CanonicalCellStats

	hasObservation bool
	lastEntry      state.State
	lastLanes      []state.LaneID
	lastInput      uint64
	lastResolution uint64
}

func (a *solveAttribution) withCanonicalObservation(config body.Config, inputDigest, resolution uint64) *solveAttribution {
	if a == nil {
		return nil
	}
	copy := *a
	lanes := state.CloneLanes(config.StateLanes)
	if lanes == nil {
		lanes = state.DefaultLanes()
	}
	copy.canonical = canonicalSolveObservation{
		valid:       true,
		registry:    config.Registry,
		entry:       config.EntryState.Snapshot(),
		lanes:       lanes,
		inputDigest: inputDigest,
		resolution:  resolution,
	}
	return &copy
}

func (s *Stats) recordCanonicalSolve(a *solveAttribution) {
	if s == nil || a == nil || a.key.phase != SolvePhaseSummary {
		return
	}
	if s.canonicalCells == nil {
		s.canonicalCells = make(map[canonicalCellKey]*canonicalCellAccumulator)
	}
	key := canonicalCellKey{bodyID: a.key.bodyID, function: a.key.function}
	cell := s.canonicalCells[key]
	if cell == nil {
		cell = &canonicalCellAccumulator{stats: CanonicalCellStats{
			BodyID:   key.bodyID,
			Function: key.function,
			Context:  a.key.context,
		}}
		s.canonicalCells[key] = cell
		cell.stats.WorkspaceBuilds++
		// A semantic entry is a context-partition build rather than reuse of the
		// lexical default cell. Deriving this from the key is order-independent.
		if a.key.context || a.key.function.Entry != (summary.EntryKey{}) {
			cell.stats.Resets.Context++
		}
	}
	cell.stats.ActualBodySolves++
	if cell.stats.ActualBodySolves == 1 {
		cell.observe(a.canonical)
		return
	}

	reset := ""
	if a.canonical.valid && cell.hasObservation {
		switch {
		case a.canonical.resolution != cell.lastResolution:
			reset = "routing"
		case !sameLanes(a.canonical.lanes, cell.lastLanes):
			reset = "context"
		default:
			if a.canonical.registry == nil {
				reset = "context"
				break
			}
			domain := state.DomainWithLanes(a.canonical.registry, a.canonical.lanes)
			oldLEQNew := domain.LessOrEq(cell.lastEntry, a.canonical.entry)
			newLEQOld := domain.LessOrEq(a.canonical.entry, cell.lastEntry)
			switch {
			case !oldLEQNew && newLEQOld:
				reset = "entry-shrink"
			case !oldLEQNew && !newLEQOld:
				reset = "context"
			case cell.lastInput != 0 && a.canonical.inputDigest != 0 && cell.lastInput != a.canonical.inputDigest && oldLEQNew && newLEQOld:
				reset = "context"
			}
		}
	}
	if reset == "" && a.dependencyChange {
		reset = "callee-revision"
	}
	if reset == "" {
		cell.stats.EligibleMonotoneExtensions++
		cell.stats.TheoreticalBodySolvesAvoided++
	} else {
		cell.stats.WorkspaceBuilds++
		switch reset {
		case "callee-revision":
			cell.stats.Resets.CalleeRevision++
		case "entry-shrink":
			cell.stats.Resets.EntryShrink++
		case "context":
			cell.stats.Resets.Context++
		case "routing":
			cell.stats.Resets.Routing++
		}
	}
	cell.observe(a.canonical)
}

func (c *canonicalCellAccumulator) observe(observation canonicalSolveObservation) {
	if !observation.valid {
		return
	}
	c.hasObservation = true
	c.lastEntry = observation.entry
	c.lastLanes = state.CloneLanes(observation.lanes)
	c.lastInput = observation.inputDigest
	c.lastResolution = observation.resolution
}

// CanonicalCellCensus returns a stable snapshot. It is safe to serialize in
// per-unit timing output and does not expose mutable simulator state.
func (s *Stats) CanonicalCellCensus() CanonicalCellReport {
	report := CanonicalCellReport{
		RegisteredStateLanes: state.DefaultLanes(),
		ContextKeyDimensions: summaryEntryKeyDimensions(),
	}
	if s == nil || len(s.canonicalCells) == 0 {
		return report
	}
	refs := make(map[summaryRefIdentity]struct{})
	for _, accumulated := range s.canonicalCells {
		cell := accumulated.stats
		report.Cells = append(report.Cells, cell)
		refs[summaryRefIdentity{ref: cell.Function.Ref}] = struct{}{}
		if cell.Context || cell.Function.Entry != (summary.EntryKey{}) {
			report.SemanticContextCells++
		}
		report.ActualBodySolves += cell.ActualBodySolves
		report.WorkspaceBuilds += cell.WorkspaceBuilds
		report.EligibleMonotoneExtensions += cell.EligibleMonotoneExtensions
		report.TheoreticalBodySolvesAvoided += cell.TheoreticalBodySolvesAvoided
		report.Resets.CalleeRevision += cell.Resets.CalleeRevision
		report.Resets.EntryShrink += cell.Resets.EntryShrink
		report.Resets.Context += cell.Resets.Context
		report.Resets.Routing += cell.Resets.Routing
	}
	report.LexicalFunctions = len(refs)
	if s.MaxFunctionCount > report.LexicalFunctions {
		report.LexicalFunctions = s.MaxFunctionCount
	}
	if s.MaxSemanticCallContextCount > report.SemanticContextCells {
		report.SemanticContextCells = s.MaxSemanticCallContextCount
	}
	sort.Slice(report.Cells, func(i, j int) bool {
		if report.Cells[i].Function != report.Cells[j].Function {
			return report.Cells[i].Function.Less(report.Cells[j].Function)
		}
		return report.Cells[i].BodyID < report.Cells[j].BodyID
	})
	return report
}

type summaryRefIdentity struct{ ref ref.FuncRef }

func summaryEntryKeyDimensions() []string {
	t := reflect.TypeFor[summary.EntryKey]()
	out := make([]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		out[i] = t.Field(i).Name
	}
	return out
}

func sameLanes(a, b []state.LaneID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

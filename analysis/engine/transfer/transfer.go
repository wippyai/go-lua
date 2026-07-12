// Package transfer wires CFG topology to the generic fixed-point solver.
package transfer

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/solve/concreteflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// InitialState supplies an explicit starting state for a point.
//
// Points for which InitialState returns false start at bottom, except for the
// configured entry point, which starts at Config.EntryState.
type InitialState func(point cfg.Point) (state.State, bool)

// NodeTransfer maps a point's input state to the state sent to outgoing edges.
type NodeTransfer func(ctx NodeContext, in state.State) state.State

// EdgeTransfer maps node output across a single CFG edge.
type EdgeTransfer func(ctx EdgeContext, out state.State) state.State

// Stats holds caller-owned observational counters for transfer runs.
type Stats struct {
	Solver solve.Stats
}

// Schedule selects the ascending fixed-point schedule. FIFO remains the
// default. WTODual executes WTO for comparison but publishes FIFO.
type Schedule uint8

const (
	ScheduleFIFO Schedule = iota
	ScheduleWTO
	ScheduleWTODual
)

// WTOComparison is an observational report for an opt-in or dual WTO run.
// LaneDifferences is populated only for differing point states.
type WTOComparison struct {
	Fallback         bool
	FIFOTransfers    int
	WTOTransfers     int
	StateDifferences int
	FIFOBelowWTO     int
	WTOBelowFIFO     int
	Incomparable     int
	LaneDifferences  map[state.LaneID]int
}

// StateRead is one solve-local dependency observed while evaluating a node
// transfer. Version changes whenever the solver replaces that point's state,
// including a replacement made by narrowing.
type StateRead struct {
	Point   cfg.Point
	Version uint64
}

// NodeObservation is an ephemeral transfer artifact. It is produced only for
// points selected by Config.ObserveNode and is intended to be validated and
// projected before the solved result escapes. It is not a generic point-state
// query index.
type NodeObservation struct {
	Point        cfg.Point
	InputVersion uint64
	Output       state.State
	Reads        []StateRead
}

// NodeContext is the generic context passed to node transfer hooks.
type NodeContext struct {
	// Context is the solve context. Transfer callbacks that perform their own
	// graph-sized traversal must observe it at their own bounded cadence.
	Context  context.Context
	Session  *cancellation.Session
	Graph    cfg.Graph
	Registry *axis.Registry
	Point    cfg.Point
	Node     *cfg.Node
	Read     func(cfg.Point) state.State
}

// EdgeContext is the generic context passed to edge transfer hooks.
type EdgeContext struct {
	Context  context.Context
	Session  *cancellation.Session
	Graph    cfg.Graph
	Registry *axis.Registry
	Edge     cfg.Edge
	HasCond  bool
	Read     func(cfg.Point) state.State
}

// Config describes one forward dataflow run.
type Config struct {
	// Context cooperatively stops the worklist between transfer iterations. A
	// nil context preserves the legacy uncancelable Run/TryRun behavior.
	Context context.Context
	// Session is the solve-scoped cancellation signal. If nil, TryRun recovers
	// or attaches one to Context.
	Session *cancellation.Session

	Graph    cfg.Graph
	Registry *axis.Registry
	Schedule Schedule
	WTOPlan  *solve.WTOPlan[cfg.Point]
	// ConcreteFlow is an optional immutable dense executor compiled with the
	// prepared body's WTO and operation plan. It is used only by ordinary WTO
	// solves; every unsupported or dynamically uncovered run falls back to the
	// canonical generic WTO/FIFO path.
	ConcreteFlow *concreteflow.Plan
	// CanonicalConcreteTransactions certifies that NodeTransfer and EdgeTransfer
	// are the complete factapply point transactions described by ConcreteFlow's
	// operation plan. Custom transfer callers must leave it false.
	CanonicalConcreteTransactions bool
	// FuseConcreteIdentity permits certified empty, unique-predecessor rows to
	// bypass the otherwise-identity canonical point transaction.
	FuseConcreteIdentity bool
	// CompareWTO receives a deterministic aggregate after a dual run or a WTO
	// fallback. It is observational and must not mutate solver inputs.
	CompareWTO func(WTOComparison)

	// StateLanes selects the State product-lattice lanes used by this solve.
	// Nil uses the default lane set; a non-nil slice is the exact enabled set.
	StateLanes []state.LaneID
	// StateOptions are per-solve lattice options such as widening thresholds.
	StateOptions state.DomainOptions
	// PreparedDomain is an immutable domain compiled with exactly StateOptions
	// for the default lane set. Prepared bodies use it to avoid rebuilding the
	// 17-lane product on every solve. It is ignored for explicit StateLanes.
	PreparedDomain *lattice.Lattice[state.State]

	// Entry is the point seeded with EntryState. Nil uses Graph.Entry().
	Entry      *cfg.Point
	EntryState state.State

	// Initial supplies explicit starting states for any point. When it returns
	// true for the entry point, it takes precedence over EntryState.
	Initial InitialState

	// NodeTransfer and EdgeTransfer default to identity.
	NodeTransfer NodeTransfer
	EdgeTransfer EdgeTransfer

	// WidenAt and WidenDelay are forwarded directly to the solver.
	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int

	// Stats, when non-nil, receives observational counters for this run.
	Stats *Stats

	// ObserveNode selects points whose latest node-transfer output should be
	// captured. RecordNodeObservation is called in deterministic worklist order
	// and FinalizeNodeObservations receives the final state revisions after both
	// worklist convergence and narrowing. These hooks are solve-local; they do
	// not retain arbitrary point state in Result.
	ObserveNode              func(cfg.Point) bool
	RecordNodeObservation    func(NodeObservation)
	FinalizeNodeObservations func(finalVersion func(cfg.Point) uint64)
	ResetNodeObservations    func()

	// BeforePoint and AfterPoint bracket one CFG point transfer.  They are used
	// by resumable callers to attribute external reads to the active point.
	// They do not participate in the transfer equation.
	BeforePoint func(cfg.Point)
	AfterPoint  func(cfg.Point)

	// Resume, when non-nil, retains the ascending checkpoint for this CFG.
	// A nil ResumePoints slice establishes the initial checkpoint; a non-nil
	// slice forces precisely those points and propagates through normal emits.
	Resume       *Session
	ResumePoints []cfg.Point
}

// Result maps each reachable CFG point in Graph.RPO() to its input state.
type Result map[cfg.Point]state.State

// Session is a run-local pre-narrowing CFG checkpoint.  It is safe to reuse
// only when all static transfer inputs (graph, domain, initials and widening
// policy) are identical; dynamic node/edge closures are replaced per resume.
type Session struct {
	solver *solve.Session[cfg.Point, state.State]
}

func NewSession() *Session { return &Session{} }

func (s *Session) Checkpoint() Result {
	if s == nil || s.solver == nil {
		return nil
	}
	return Result(s.solver.CheckpointCells())
}

// Run executes a one-off forward transfer run.
func Run(config Config) Result {
	result, err := TryRun(config)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// TryRun executes a one-off forward transfer run, returning configuration
// errors instead of panicking.
func TryRun(config Config) (Result, error) {
	uncancelable := config.Context == nil && config.Session == nil
	if config.Session == nil {
		config.Context, config.Session = cancellation.Attach(config.Context)
	} else {
		config.Context = cancellation.WithSession(config.Context, config.Session)
	}
	if err := config.Session.Token().Err(); err != nil {
		return nil, errors.Join(solve.ErrCanceled, err)
	}
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	registry := config.Registry
	var domain lattice.Lattice[state.State]
	if config.PreparedDomain != nil && config.StateLanes == nil {
		domain = *config.PreparedDomain
	} else {
		var err error
		domain, err = state.TryDomainWithOptionalLanesAndOptions(registry, config.StateLanes, config.StateOptions)
		if err != nil {
			return nil, err
		}
	}
	// Dense observation capture is transactional: a canceled or dynamically
	// uncovered scratch run must not publish artifacts from discarded revisions.
	var denseObservations []NodeObservation
	denseObservationMode := config.Schedule == ScheduleWTO && config.ConcreteFlow != nil && config.RecordNodeObservation != nil
	originalRecordNodeObservation := config.RecordNodeObservation
	if denseObservationMode {
		config.RecordNodeObservation = func(observation NodeObservation) {
			if denseObservationMode {
				denseObservations = append(denseObservations, observation)
				return
			}
			originalRecordNodeObservation(observation)
		}
	}
	plan := newEquationPlan(config, domain, equationPlanHooks{})
	sys := plan.system
	cells := plan.cells
	observing := plan.observing
	if config.Resume != nil {
		if config.Resume.solver == nil {
			config.Resume.solver = solve.NewSession(sys)
			if err := config.Resume.solver.Ascend(config.Context); err != nil {
				return nil, err
			}
		} else {
			config.Resume.solver.ReplaceTransfer(sys.Transfer, sys.TransferVersioned, sys.Stats)
			if err := config.Resume.solver.Resume(config.Context, config.ResumePoints); err != nil {
				return nil, err
			}
		}
		result, versions, err := config.Resume.solver.Publish(config.Context)
		if err != nil {
			return nil, err
		}
		if config.FinalizeNodeObservations != nil {
			config.FinalizeNodeObservations(func(point cfg.Point) uint64 { return versions[point] })
		}
		return Result(result), nil
	}

	runFIFO := func(system solve.EquationSystem[cfg.Point, state.State]) (map[cfg.Point]state.State, map[cfg.Point]uint64, error) {
		if uncancelable {
			if !observing {
				return solve.Solve(system), nil, nil
			}
			result, versions := solve.SolveWithVersions(system)
			return result, versions, nil
		}
		if !observing {
			result, err := solve.SolveContext(config.Context, system)
			return result, nil, err
		}
		return solve.SolveContextWithVersions(config.Context, system)
	}
	finalize := func(versions map[cfg.Point]uint64) {
		if config.FinalizeNodeObservations != nil {
			config.FinalizeNodeObservations(func(point cfg.Point) uint64 { return versions[point] })
		}
	}
	reportComparison := func(fifo, wto map[cfg.Point]state.State, fifoTransfers, wtoTransfers int, fallback bool) {
		if config.CompareWTO == nil {
			return
		}
		report := WTOComparison{Fallback: fallback, FIFOTransfers: fifoTransfers, WTOTransfers: wtoTransfers}
		if !fallback {
			for _, point := range cells {
				left, right := fifo[point], wto[point]
				if domain.Equal(left, right) {
					continue
				}
				report.StateDifferences++
				leftBelow := domain.LessOrEq(left, right)
				rightBelow := domain.LessOrEq(right, left)
				switch {
				case leftBelow && !rightBelow:
					report.FIFOBelowWTO++
				case rightBelow && !leftBelow:
					report.WTOBelowFIFO++
				default:
					report.Incomparable++
				}
				lanes := config.StateLanes
				if lanes == nil {
					lanes = state.DefaultLanes()
				}
				for _, lane := range state.NewLaneSet(lanes...).IDs() {
					laneDomain := state.DomainWithLanes(registry, []state.LaneID{lane})
					if !laneDomain.Equal(left, right) {
						if report.LaneDifferences == nil {
							report.LaneDifferences = make(map[state.LaneID]int)
						}
						report.LaneDifferences[lane]++
					}
				}
			}
		}
		config.CompareWTO(report)
	}

	if config.Schedule == ScheduleWTO || config.Schedule == ScheduleWTODual {
		if config.Schedule == ScheduleWTO && config.ConcreteFlow != nil {
			denseStats := &solve.Stats{}
			denseSystem := plan.withStats(denseStats)
			fuse := func(point cfg.Point) bool {
				return config.FuseConcreteIdentity && config.BeforePoint == nil && config.AfterPoint == nil &&
					(!observing || !config.ObserveNode(point))
			}
			dense, denseErr := concreteflow.Run(concreteflow.RunConfig{
				Context: config.Context, FuseIdentity: fuse, IncludeVersions: config.FinalizeNodeObservations != nil,
				Transfer: plan.denseTransfer, TransferVersioned: plan.denseTransferVersioned,
			}, denseSystem, config.ConcreteFlow)
			if denseErr == nil {
				if sys.Stats != nil {
					sys.Stats.TransferCalls += denseStats.TransferCalls
				}
				for _, observation := range denseObservations {
					originalRecordNodeObservation(observation)
				}
				finalize(dense.Versions)
				if config.CompareWTO != nil {
					config.CompareWTO(WTOComparison{WTOTransfers: denseStats.TransferCalls})
				}
				return Result(dense.Points), nil
			}
			if !errors.Is(denseErr, solve.ErrWTOPlanUncovered) {
				return nil, denseErr
			}
			denseObservationMode = false
			denseObservations = nil
			// Scratch observations can refer to revisions that are about to be
			// discarded. The fallback owns a clean observation generation.
			if config.ResetNodeObservations != nil && originalRecordNodeObservation == nil {
				config.ResetNodeObservations()
			}
		}
		wtoStats := &solve.Stats{}
		wtoSystem := plan.withStats(wtoStats)
		var wto map[cfg.Point]state.State
		var wtoVersions map[cfg.Point]uint64
		var wtoErr error
		if uncancelable {
			wto, wtoVersions, wtoErr = solve.SolveWTOWithVersions(wtoSystem, plan.wto)
		} else {
			wto, wtoVersions, wtoErr = solve.SolveWTOContextWithVersions(config.Context, wtoSystem, plan.wto)
		}
		fallback := errors.Is(wtoErr, solve.ErrWTOPlanUncovered)
		if wtoErr != nil && !fallback {
			return nil, wtoErr
		}
		if config.Schedule == ScheduleWTO && !fallback {
			if sys.Stats != nil {
				sys.Stats.TransferCalls += wtoStats.TransferCalls
			}
			finalize(wtoVersions)
			if config.CompareWTO != nil {
				config.CompareWTO(WTOComparison{WTOTransfers: wtoStats.TransferCalls})
			}
			return Result(wto), nil
		}
		// A failed scratch WTO attempt and dual mode both publish a clean FIFO
		// observation capture, never records from the discarded schedule.
		if config.ResetNodeObservations != nil {
			config.ResetNodeObservations()
		}
		beforeFIFO := 0
		if sys.Stats != nil {
			beforeFIFO = sys.Stats.TransferCalls
		}
		fifo, fifoVersions, fifoErr := runFIFO(sys)
		if fifoErr != nil {
			return nil, fifoErr
		}
		fifoTransfers := 0
		if sys.Stats != nil {
			fifoTransfers = sys.Stats.TransferCalls - beforeFIFO
		}
		reportComparison(fifo, wto, fifoTransfers, wtoStats.TransferCalls, fallback)
		finalize(fifoVersions)
		return Result(fifo), nil
	}

	result, versions, err := runFIFO(sys)
	if err != nil {
		return nil, err
	}
	finalize(versions)
	return Result(result), nil
}

func solverStats(stats *Stats) *solve.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Solver
}

func validateConfig(config Config) error {
	if config.Graph == nil {
		return errors.New("transfer: Config.Graph is nil")
	}
	if config.Registry == nil {
		return errors.New("transfer: Config.Registry is nil")
	}
	if config.Schedule > ScheduleWTODual {
		return errors.New("transfer: Config.Schedule is invalid")
	}
	if config.Entry != nil {
		for _, point := range config.Graph.RPO() {
			if point == *config.Entry {
				return nil
			}
		}
		return errors.New("transfer: Config.Entry is not in graph.RPO()")
	}
	return nil
}

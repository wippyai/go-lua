// Package query runs pure fixed-point summary equations.
package query

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// Body computes one function summary from the active fixed-point context. The
// driver consumes the returned summary and may canonicalize its mutable lanes.
type Body func(Context) (summary.Summary, error)

// Context is passed to a Body during fixed-point evaluation.
type Context struct {
	Key       summary.SummaryKey
	Summaries summary.Reader
}

// Function binds a summary key to its equation body.
type Function struct {
	Key  summary.SummaryKey
	Body Body
}

// Stats holds caller-owned observational counters for summary query runs.
type Stats struct {
	BodyInvocations int
	Solver          solve.Stats
}

// Config configures a summary fixed-point run.
type Config struct {
	// Context cooperatively stops summary worklist iteration. Nil preserves the
	// legacy uncancelable driver behavior.
	Context    context.Context
	Registry   *axis.Registry
	Functions  []Function
	Seed       summary.Reader
	WidenAt    func(summary.SummaryKey) bool
	WidenDelay func(summary.SummaryKey) int
	Stats      *Stats
}

// Driver is a reusable validated summary fixed-point driver.
type Driver struct {
	ctx        context.Context
	reg        *axis.Registry
	functions  []Function
	known      map[summary.SummaryKey]struct{}
	seed       summary.Reader
	widenAt    func(summary.SummaryKey) bool
	widenDelay func(summary.SummaryKey) int
	Stats      *Stats
}

var ErrRegistryRequired = errors.New("query: registry is required")
var ErrDuplicateFunction = errors.New("query: duplicate function")
var ErrNilBody = errors.New("query: nil body")

// New validates config and returns a driver.
func New(config Config) (*Driver, error) {
	if config.Registry == nil {
		return nil, ErrRegistryRequired
	}

	known := make(map[summary.SummaryKey]struct{}, len(config.Functions))
	functions := make([]Function, len(config.Functions))
	for i, fn := range config.Functions {
		if _, ok := known[fn.Key]; ok {
			return nil, ErrDuplicateFunction
		}
		if fn.Body == nil {
			return nil, ErrNilBody
		}
		known[fn.Key] = struct{}{}
		functions[i] = fn
	}

	return &Driver{
		ctx:        config.Context,
		reg:        config.Registry,
		functions:  functions,
		known:      known,
		seed:       config.Seed,
		widenAt:    config.WidenAt,
		widenDelay: config.WidenDelay,
		Stats:      config.Stats,
	}, nil
}

// Run constructs a driver from config, runs it, and returns its snapshot.
func Run(config Config) (summary.Snapshot, error) {
	d, err := New(config)
	if err != nil {
		return summary.Snapshot{}, err
	}
	return d.Run()
}

// Run computes the fixed point and returns an exact-key summary snapshot.
func (d *Driver) Run() (summary.Snapshot, error) {
	if err := driverContextErr(d.ctx); err != nil {
		return summary.Snapshot{}, err
	}
	var firstErr error

	cells := make([]summary.SummaryKey, len(d.functions))
	byKey := make(map[summary.SummaryKey]Body, len(d.functions))
	for i, fn := range d.functions {
		if i%256 == 0 {
			if err := driverContextErr(d.ctx); err != nil {
				return summary.Snapshot{}, err
			}
		}
		cells[i] = fn.Key
		byKey[fn.Key] = fn.Body
	}

	system := solve.EquationSystem[summary.SummaryKey, summary.Summary]{
		Lattice: summary.NormalizedDomain(d.reg),
		Cells:   cells,
		Initial: func(key summary.SummaryKey) summary.Summary {
			if d.seed == nil {
				return summary.Summary{}
			}
			seeded, ok := d.seed.Read(key)
			if !ok {
				return summary.Summary{}
			}
			return summary.Normalize(d.reg, seeded)
		},
		Transfer: func(key summary.SummaryKey, read func(summary.SummaryKey) summary.Summary, emit func(summary.SummaryKey, summary.Summary)) {
			if firstErr != nil {
				return
			}
			body := byKey[key]
			if d.Stats != nil {
				d.Stats.BodyInvocations++
			}
			got, err := body(Context{
				Key:       key,
				Summaries: activeReader{reg: d.reg, known: d.known, seed: d.seed, read: read},
			})
			if err != nil {
				firstErr = err
				return
			}
			emit(key, summary.NormalizeOwned(d.reg, got))
		},
		WidenAt:    d.widenAt,
		WidenDelay: d.widenDelay,
		Stats:      solverStats(d.Stats),
	}
	var result map[summary.SummaryKey]summary.Summary
	if d.ctx == nil {
		result = solve.Solve(system)
	} else {
		var err error
		result, err = solve.SolveContext(d.ctx, system)
		if err != nil {
			return summary.Snapshot{}, err
		}
	}
	if firstErr != nil {
		return summary.Snapshot{}, firstErr
	}

	entries := make([]summary.EntrySummary, 0, len(d.functions))
	for i, fn := range d.functions {
		if i%256 == 0 {
			if err := driverContextErr(d.ctx); err != nil {
				return summary.Snapshot{}, err
			}
		}
		entries = append(entries, summary.EntrySummary{
			Key:     fn.Key,
			Summary: result[fn.Key],
		})
	}
	return summary.NewSnapshotOwnedNormalized(d.reg, entries...), nil
}

func driverContextErr(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return errors.Join(solve.ErrCanceled, ctx.Err())
}

func solverStats(stats *Stats) *solve.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Solver
}

type activeReader struct {
	reg   *axis.Registry
	known map[summary.SummaryKey]struct{}
	seed  summary.Reader
	read  func(summary.SummaryKey) summary.Summary
}

func (r activeReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	got := r.read(key)
	if _, ok := r.known[key]; ok {
		return summary.Normalize(r.reg, got), true
	}
	if r.seed == nil {
		return summary.Summary{}, false
	}
	seeded, ok := r.seed.Read(key)
	if !ok {
		return summary.Summary{}, false
	}
	return summary.Normalize(r.reg, seeded), true
}

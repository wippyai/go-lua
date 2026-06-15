// Package query runs pure fixed-point summary equations.
package query

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// Body computes one function summary from the active fixed-point context.
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
	Registry   *axis.Registry
	Functions  []Function
	Seed       summary.Reader
	WidenAt    func(summary.SummaryKey) bool
	WidenDelay func(summary.SummaryKey) int
	Stats      *Stats
}

// Driver is a reusable validated summary fixed-point driver.
type Driver struct {
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
	var firstErr error

	cells := make([]summary.SummaryKey, len(d.functions))
	byKey := make(map[summary.SummaryKey]Body, len(d.functions))
	for i, fn := range d.functions {
		cells[i] = fn.Key
		byKey[fn.Key] = fn.Body
	}

	result := solve.Solve[summary.SummaryKey, summary.Summary](solve.EquationSystem[summary.SummaryKey, summary.Summary]{
		Lattice: summaryLattice(d.reg),
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
			emit(key, summary.Normalize(d.reg, got))
		},
		WidenAt:    d.widenAt,
		WidenDelay: d.widenDelay,
		Stats:      solverStats(d.Stats),
	})
	if firstErr != nil {
		return summary.Snapshot{}, firstErr
	}

	entries := make([]summary.EntrySummary, 0, len(d.functions))
	for _, fn := range d.functions {
		entries = append(entries, summary.EntrySummary{
			Key:     fn.Key,
			Summary: result[fn.Key],
		})
	}
	return summary.NewSnapshot(d.reg, entries...), nil
}

func solverStats(stats *Stats) *solve.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Solver
}

func summaryLattice(reg *axis.Registry) lattice.Lattice[summary.Summary] {
	return lattice.Lattice[summary.Summary]{
		Bottom: func() summary.Summary { return summary.Summary{} },
		Equal: func(a, b summary.Summary) bool {
			return summary.Equal(reg, a, b)
		},
		Join: func(a, b summary.Summary) summary.Summary {
			return summary.Join(reg, a, b)
		},
		Widen: func(prev, next summary.Summary) summary.Summary {
			return summary.Widen(reg, prev, next)
		},
	}
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

// Package genericlift is an isolated proof of the fail-closed registration
// seam required by a future symbolic transfer interpreter. It does not lift or
// execute production transfers.
package genericlift

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

// Support is an all-or-nothing transformer capability verdict.
type Support uint8

const (
	Unsupported Support = iota
	Exact
)

// Operation is one lowered semantic operation. Production has no such shared
// operation stream today; the type makes that prerequisite explicit instead of
// pretending a concrete State snapshot is a symbolic program.
type Operation struct {
	Kind  string
	Lanes []state.LaneID
}

// BuildContext is the immutable input to every lane adapter.
type BuildContext struct {
	Operations []Operation
}

// Bindings names a transformer instantiation without exposing caller State.
// A production form would carry typed parameter/path/heap binding services.
type Bindings struct {
	CallID string
}

// Patch is transaction-owned POC output. Lane programs cannot publish it
// directly; Transformer.Instantiate commits only after every lane succeeds.
type Patch struct {
	values map[state.LaneID]string
}

func (p Patch) Value(lane state.LaneID) (string, bool) {
	value, ok := p.values[lane]
	return value, ok
}

func (p *Patch) Set(lane state.LaneID, value string) {
	if p.values == nil {
		p.values = make(map[state.LaneID]string)
	}
	p.values[lane] = value
}

func (p Patch) clone() Patch {
	out := Patch{values: make(map[state.LaneID]string, len(p.values))}
	for lane, value := range p.values {
		out.values[lane] = value
	}
	return out
}

// LaneProgram owns its typed symbolic payload behind its implementation. The
// common registry never transports an untyped payload between adapters.
type LaneProgram interface {
	Lane() state.LaneID
	Instantiate(Bindings, *Patch) Support
}

// LaneLifter is the smallest lane-owned transformer adapter. Build must reject
// any operation whose semantics it cannot represent exactly.
type LaneLifter interface {
	Lane() state.LaneID
	Build(BuildContext) (LaneProgram, Support)
}

type role struct {
	lane   state.LaneID
	lifter LaneLifter
}

// Registry is ordered by the State lane catalog, never adapter registration
// order. Every missing adapter is materialized as unsupported.
type Registry struct {
	roles []role
}

// DefaultRegistry derives coverage from the production State catalog. Adding a
// production lane therefore fails closed until its one adapter is supplied.
func DefaultRegistry(adapters ...LaneLifter) (*Registry, error) {
	return NewRegistry(state.DefaultLaneCatalog().LaneSet().IDs(), adapters...)
}

// NewRegistry is exported for catalog evolution and isolated tests.
func NewRegistry(lanes []state.LaneID, adapters ...LaneLifter) (*Registry, error) {
	known := make(map[state.LaneID]struct{}, len(lanes))
	for _, lane := range lanes {
		if lane == "" {
			return nil, fmt.Errorf("genericlift: empty catalog lane")
		}
		if _, duplicate := known[lane]; duplicate {
			return nil, fmt.Errorf("genericlift: duplicate catalog lane %q", lane)
		}
		known[lane] = struct{}{}
	}
	byLane := make(map[state.LaneID]LaneLifter, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			return nil, fmt.Errorf("genericlift: nil adapter")
		}
		lane := adapter.Lane()
		if _, ok := known[lane]; !ok {
			return nil, fmt.Errorf("genericlift: orphan adapter %q", lane)
		}
		if _, duplicate := byLane[lane]; duplicate {
			return nil, fmt.Errorf("genericlift: duplicate adapter %q", lane)
		}
		byLane[lane] = adapter
	}
	registry := &Registry{roles: make([]role, len(lanes))}
	for i, lane := range lanes {
		adapter := byLane[lane]
		if adapter == nil {
			adapter = unsupportedLifter{lane: lane}
		}
		registry.roles[i] = role{lane: lane, lifter: adapter}
	}
	return registry, nil
}

func (r *Registry) Lanes() []state.LaneID {
	if r == nil {
		return nil
	}
	out := make([]state.LaneID, len(r.roles))
	for i, role := range r.roles {
		out[i] = role.lane
	}
	return out
}

// Build constructs every used lane or returns one atomic fallback transformer.
// An empty used set means every catalog lane, the conservative default.
func (r *Registry) Build(ctx BuildContext, used ...state.LaneID) Transformer {
	if r == nil {
		return contextualTransformer("missing registry")
	}
	wanted := make(map[state.LaneID]struct{}, len(used))
	for _, lane := range used {
		wanted[lane] = struct{}{}
	}
	all := len(used) == 0
	if !all {
		// Operation-declared dependencies are authoritative. A caller-provided
		// used set may add conservative lanes, but can never hide one.
		for _, operation := range ctx.Operations {
			for _, lane := range operation.Lanes {
				wanted[lane] = struct{}{}
			}
		}
	}
	out := Transformer{valid: true}
	for _, role := range r.roles {
		if !all {
			if _, ok := wanted[role.lane]; !ok {
				continue
			}
			delete(wanted, role.lane)
		}
		program, support := role.lifter.Build(ctx)
		if support != Exact || program == nil || program.Lane() != role.lane {
			out.fallback = append(out.fallback, role.lane)
			continue
		}
		out.programs = append(out.programs, program)
	}
	unknown := make([]state.LaneID, 0, len(wanted))
	for lane := range wanted {
		unknown = append(unknown, lane)
	}
	sort.Slice(unknown, func(i, j int) bool { return unknown[i] < unknown[j] })
	out.fallback = append(out.fallback, unknown...)
	return out
}

type unsupportedLifter struct{ lane state.LaneID }

func (l unsupportedLifter) Lane() state.LaneID { return l.lane }
func (unsupportedLifter) Build(BuildContext) (LaneProgram, Support) {
	return nil, Unsupported
}

// Transformer is exact only when every requested lane built successfully.
type Transformer struct {
	programs []LaneProgram
	fallback []state.LaneID
	valid    bool
	reason   string
}

func contextualTransformer(reason string) Transformer {
	return Transformer{valid: true, reason: reason}
}

func (t Transformer) Contextual() bool { return !t.valid || t.reason != "" || len(t.fallback) != 0 }

func (t Transformer) FallbackLanes() []state.LaneID {
	return append([]state.LaneID(nil), t.fallback...)
}

// Instantiate commits no partial lane output when any late adapter check fails.
func (t Transformer) Instantiate(bindings Bindings, base Patch) (Patch, Support) {
	if t.Contextual() {
		return base, Unsupported
	}
	scratch := base.clone()
	for _, program := range t.programs {
		if program.Instantiate(bindings, &scratch) != Exact {
			return base, Unsupported
		}
	}
	return scratch, Exact
}

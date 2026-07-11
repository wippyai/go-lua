package axiscompose

// Support is the result of one lane's boundary capability.
type Support uint8

const (
	// Exact means projection/instantiation is supported for this payload.
	Exact Support = iota
	// Contextual means the complete contextual analysis must be used.
	Contextual
)

// Binding gives the POC a named symbolic boundary and a concrete call binding.
// Toy lanes use Symbol as an integrity check; production would carry paths.
type Binding struct {
	Symbol string
}

// ProjectCtx describes one callee-boundary projection.
type ProjectCtx struct {
	Used    Mask
	Binding Binding
}

// InstantiateCtx describes one caller-boundary instantiation.
type InstantiateCtx struct {
	Binding Binding
}

// Boundary is one typed lane's project/instantiate capability.
type Boundary[T any] struct {
	Project     func(ProjectCtx, T) (any, Support)
	Instantiate func(InstantiateCtx, any) (T, Support)
}

type erasedBoundary struct {
	project     func(ProjectCtx, any) (any, Support)
	instantiate func(InstantiateCtx, any) (any, Support)
}

func eraseBoundary[T any](b *Boundary[T]) *erasedBoundary {
	if b == nil || b.Project == nil || b.Instantiate == nil {
		return nil
	}
	return &erasedBoundary{
		project: func(ctx ProjectCtx, value any) (any, Support) {
			return b.Project(ctx, value.(T))
		},
		instantiate: func(ctx InstantiateCtx, payload any) (any, Support) {
			return b.Instantiate(ctx, payload)
		},
	}
}

// LanePayload is one descriptor-owned projected fact.
type LanePayload struct {
	ID      AxisID
	Payload any
}

// Projection is an all-lane boundary plan. A non-empty Fallback forbids
// publishing any partial instantiation.
type Projection struct {
	Schema   *Schema
	Lanes    []LanePayload
	Fallback Mask
}

// AllUsed returns a sound default when no transfer-derived dependency mask is
// available.
func AllUsed(schema *Schema) Mask {
	out := newMask(schema)
	for i := 0; i < schema.Len(); i++ {
		out.set(i)
	}
	return out
}

// UsedAxes constructs a certified used mask by axis ID.
func UsedAxes(schema *Schema, ids ...AxisID) Mask {
	out := newMask(schema)
	if schema == nil {
		return out
	}
	for _, id := range ids {
		if i, ok := schema.byID[id]; ok {
			out.set(i)
		}
	}
	return out
}

// ProjectBoundary projects supported used lanes and records every lane that
// requires contextual fallback. Unsupported unused lanes are omitted.
func ProjectBoundary(s State, ctx ProjectCtx) Projection {
	p := Projection{Schema: s.schema, Fallback: newMask(s.schema)}
	used := ctx.Used
	if used.schema != s.schema {
		used = AllUsed(s.schema)
	}
	for i, spec := range s.schema.specs {
		if !used.Has(i) {
			continue
		}
		if spec.boundary == nil {
			p.Fallback.set(i)
			continue
		}
		payload, support := spec.boundary.project(ctx, s.slots[i].value)
		if support != Exact {
			p.Fallback.set(i)
			continue
		}
		p.Lanes = append(p.Lanes, LanePayload{ID: spec.id, Payload: payload})
	}
	return p
}

// InstantiateBoundary instantiates every projected lane or, on any unsupported
// lane/error, discards all partial work and invokes contextual exactly once.
// The bool reports whether contextual fallback was used.
func InstantiateBoundary(arena *Arena, p Projection, ctx InstantiateCtx, contextual func() State) (State, bool) {
	fallback := func() (State, bool) {
		if contextual == nil {
			panic("axiscompose: contextual fallback required but nil")
		}
		return contextual(), true
	}
	if p.Schema == nil || !p.Fallback.Empty() {
		return fallback()
	}
	out := Bottom(arena, p.Schema)
	for _, lane := range p.Lanes {
		i, ok := p.Schema.byID[lane.ID]
		if !ok {
			return fallback()
		}
		spec := p.Schema.specs[i]
		if spec.boundary == nil {
			return fallback()
		}
		value, support := spec.boundary.instantiate(ctx, lane.Payload)
		if support != Exact {
			return fallback()
		}
		if !spec.equal(out.slots[i].value, value) {
			out.slots[i] = slot{value: value, stamp: arena.fresh()}
		}
	}
	return out, false
}

package static

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// sealRuntimeKinds records the one runtime observation summary for every
// Class while canonical type bytes and construction graphs are still present.
// The completed ClassSet retains only this dense table; MayRuntimeKinds is
// consequently independent of typ and typeauthority after Static seals.
func (s *ClassSet) sealRuntimeKinds() error {
	if s == nil || len(s.rows) == 0 {
		return errors.New("static: unavailable Class runtime-kind construction")
	}
	masks := make([]runtimekind.Set, len(s.rows))
	masks[0] = runtimekind.All // ClassAnyValue.
	for index := 1; index < len(s.rows); index++ {
		row := s.rows[index]
		switch row.kind {
		case ClassConcrete:
			decoded, err := typ.DecodeCanonicalStructural(context.Background(), row.encoded)
			if err != nil || decoded == nil {
				return errors.New("static: concrete Class runtime-kind decode failed")
			}
			masks[index] = staticTypeMayRuntimeKinds(decoded, make(map[typ.Type]bool))
		case ClassOpaque:
			// An opaque static residual has no proven runtime representation.
			// All is the only sound closed Value-vocabulary projection.
			masks[index] = runtimekind.All
		default:
			return errors.New("static: invalid Class runtime-kind row")
		}
	}
	s.runtimeKinds = masks
	return nil
}

// sealRuntimeAtomKinds projects each canonical Runtime atom independently.
// A Class row may be a union and therefore its row mask is not a valid mask
// for every atom in that row.  Keeping this atom-indexed table makes derived
// Class descriptors exact without scanning Class rows at query time.
func (s *ClassSet) sealRuntimeAtomKinds() error {
	if s == nil || s.runtime == nil {
		return errors.New("static: unavailable Runtime atom-kind construction")
	}
	masks := make([]runtimekind.Set, s.runtime.Count()+1)
	for index := 1; index <= s.runtime.Count(); index++ {
		inner, ok := s.runtime.InnerAtIndex(uint32(index))
		if !ok {
			return errors.New("static: Runtime atom-kind index unavailable")
		}
		encoded, ok := s.runtime.CanonicalEncoding(inner)
		if !ok || len(encoded) == 0 {
			masks[index] = runtimekind.All
			continue
		}
		shape, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
		if err != nil || shape == nil {
			masks[index] = runtimekind.All
			continue
		}
		masks[index] = staticTypeMayRuntimeKinds(shape, make(map[typ.Type]bool))
	}
	s.runtimeAtomKinds = masks
	return nil
}

// staticTypeMayRuntimeKinds computes a sound may projection during sealing.
// It deliberately answers All for an unmodelled or cyclic structural form:
// this projection may lose precision but must never exclude a possible Lua
// runtime representation.  Its seen set is seal-only; recurrent queries are
// the table lookup above.
func staticTypeMayRuntimeKinds(value typ.Type, active map[typ.Type]bool) runtimekind.Set {
	value = typ.UnwrapStructuralWrappers(typ.NormalizeNil(value))
	if value == nil {
		return runtimekind.All
	}
	if active[value] {
		return runtimekind.All
	}

	switch typed := value.(type) {
	case *typ.Optional:
		active[value] = true
		result := runtimekind.Bit(runtimekind.Nil) | staticTypeMayRuntimeKinds(typed.Inner, active)
		delete(active, value)
		return result
	case *typ.Union:
		active[value] = true
		var result runtimekind.Set
		for _, member := range typed.Members {
			result |= staticTypeMayRuntimeKinds(member, active)
		}
		delete(active, value)
		return result
	case *typ.Intersection:
		active[value] = true
		result := runtimekind.All
		for _, member := range typed.Members {
			// A value inhabiting an intersection inhabits every member, so
			// intersecting their may sets remains a sound over-approximation.
			result &= staticTypeMayRuntimeKinds(member, active)
		}
		delete(active, value)
		return result
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(typed)
		if expanded == nil || expanded == value {
			return runtimekind.All
		}
		active[value] = true
		result := staticTypeMayRuntimeKinds(expanded, active)
		delete(active, value)
		return result
	case *typ.Recursive:
		if typed.Body == nil {
			return runtimekind.All
		}
		active[value] = true
		result := staticTypeMayRuntimeKinds(typed.Body, active)
		delete(active, value)
		return result
	case *typ.Literal:
		return runtimeKindForBase(typed.Base())
	}

	switch value.Kind() {
	case kind.Never:
		return 0
	case kind.Nil:
		return runtimekind.Bit(runtimekind.Nil)
	case kind.Boolean:
		return runtimekind.Bit(runtimekind.Boolean)
	case kind.Number, kind.Integer:
		return runtimekind.Bit(runtimekind.Number)
	case kind.String:
		return runtimekind.Bit(runtimekind.String)
	case kind.Function:
		return runtimekind.Bit(runtimekind.Function)
	case kind.Array, kind.Map, kind.ReadonlyMap, kind.Record:
		return runtimekind.Bit(runtimekind.Table)
	case kind.Any, kind.Unknown:
		return runtimekind.All
	default:
		// Tuple, interface, meta, type parameter, unresolved reference, and
		// every future type constructor remain conservatively unclassified
		// until Static gives them a proved Lua runtime representation.
		return runtimekind.All
	}
}

func runtimeKindForBase(base kind.Kind) runtimekind.Set {
	switch base {
	case kind.Boolean:
		return runtimekind.Bit(runtimekind.Boolean)
	case kind.Number, kind.Integer:
		return runtimekind.Bit(runtimekind.Number)
	case kind.String:
		return runtimekind.Bit(runtimekind.String)
	default:
		return runtimekind.All
	}
}

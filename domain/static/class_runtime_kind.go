package static

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/domain/runtimekind"
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
			masks[index] = typ.MayRuntimeKinds(decoded)
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
		masks[index] = typ.MayRuntimeKinds(shape)
	}
	s.runtimeAtomKinds = masks
	return nil
}

// MayRuntimeKinds is the sound may-projection of one static type onto the
// closed Lua runtime vocabulary: the families a value of that type may carry
// at run time. It is the same projection the Class table is sealed from, so a
// consumer that holds a type graph rather than a sealed Class reads the one
// answer this domain gives.
//
// The fold itself belongs to the type domain, which owns the graph and derives
// the projection as a column over it. Static keeps the name its consumers
// already call and adds no second traversal.
func MayRuntimeKinds(value typ.Type) runtimekind.Set {
	return typ.MayRuntimeKinds(value)
}

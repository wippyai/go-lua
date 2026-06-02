package paramevidence

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// PathEvidence wraps a consumed parameter-path obligation under a structural
// path. The generated fields are readonly because this evidence proves a read
// requirement on the caller shape; write requirements are owned by assignment
// and mutation evidence, not by parameter-read contracts.
func PathEvidence(segments []constraint.Segment, leaf typ.Type) typ.Type {
	if len(segments) == 0 {
		return leaf
	}
	seg := segments[0]
	var field string
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		field = seg.Name
	default:
		return nil
	}
	if field == "" {
		return nil
	}
	child := PathEvidence(segments[1:], leaf)
	if child == nil {
		return nil
	}
	return typ.NewRecord().ReadonlyField(field, child).Build()
}

// IndexedIteratorEvidence converts ipairs-style iterator variable evidence
// back into the container evidence required from the source parameter.
func IndexedIteratorEvidence(varIndex int, local typ.Type) typ.Type {
	if varIndex != 1 || local == nil {
		return nil
	}
	return typ.NewArray(local)
}

// KeyedIteratorEvidence converts pairs-style iterator variable evidence back
// into the read-only iterable evidence required from the source parameter.
// Enumerating a key/value does not grant write permission to arbitrary map
// entries, so this deliberately returns ReadonlyMap instead of mutable Map.
func KeyedIteratorEvidence(varIndex int, local typ.Type) typ.Type {
	if local == nil || typ.IsAny(local) || typ.IsUnknown(local) {
		return nil
	}
	switch varIndex {
	case 0:
		return typ.NewReadonlyMap(local, typ.Any)
	case 1:
		return typ.NewReadonlyMap(typ.Any, local)
	default:
		return nil
	}
}

// MapElementEvidence converts element evidence into map evidence with the
// observed key domain.
func MapElementEvidence(keyType, local typ.Type) typ.Type {
	if local == nil {
		return nil
	}
	if keyType == nil {
		keyType = typ.Any
	}
	return typ.NewMap(keyType, local)
}

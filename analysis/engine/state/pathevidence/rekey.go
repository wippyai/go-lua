package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// RekeyValueLanes re-interns the refinement and static-member value-lane keys
// from one keyspace into another, formatting each key via from and re-interning
// the spelling via to. It lets a path-evidence lane built in one analysis's
// keyspace be consumed in another (cross-summary entry-state transfer) without
// the per-keyspace intern ids diverging. Proof and presence-implication sublanes
// carry path-key strings and need no rekeying. A nil keyspace, or from == to,
// returns the lane unchanged.
func (l Lane) RekeyValueLanes(from, to *keyspace.KeySpace) Lane {
	if from == nil || to == nil || from == to {
		return l
	}
	out := l
	out.refinements = rekeyValueMap(from, to, l.refinements)
	out.staticMembers = rekeyValueMap(from, to, l.staticMembers)
	return out
}

func rekeyValueMap(from, to *keyspace.KeySpace, in map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[keyspace.Key]product.Value, len(in))
	for key, value := range in {
		rekeyed, ok := rekeyValueLaneKey(from, to, key)
		if !ok {
			continue
		}
		out[rekeyed] = value
	}
	return out
}

func rekeyValueLaneKey(from, to *keyspace.KeySpace, key keyspace.Key) (keyspace.Key, bool) {
	if from == nil || to == nil {
		return keyspace.Key{}, false
	}
	switch key.Kind {
	case keyspace.KindStableSym:
		segments, ok := from.SegmentsView(key)
		if !ok {
			return keyspace.Key{}, false
		}
		return to.FromStableSymbol(key.Sym, segments)
	default:
		return to.FromPathKey(from.Format(key))
	}
}

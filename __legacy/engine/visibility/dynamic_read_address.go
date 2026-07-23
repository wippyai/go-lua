package visibility

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// DynamicReadAddress is the one frozen address vocabulary used by concrete
// and guarded dynamic reads. Coordinate is the flow-sensitive path coordinate;
// StateKeys are the ordered, deduplicated equivalent fact spellings; Visible
// is deliberately a single SSA spelling for paired relational proofs.
type DynamicReadAddress struct {
	Coordinate       keyspace.Key
	StateKeys        []pathaddr.StateKey
	Visible          pathaddr.StateKey
	HasVisible       bool
	RootOrVisible    pathaddr.StateKey
	HasRootOrVisible bool
}

// FreezeDynamicReadAddress resolves p once under resolver. When no local SSA
// spelling exists (for example a certified boundary root), its structural
// keyspace address remains a valid coordinate, but no exact Visible proof
// address is invented.
func FreezeDynamicReadAddress(
	keys *keyspace.KeySpace,
	resolver *Resolver,
	point cfg.Point,
	p pathdom.Path,
) (DynamicReadAddress, bool) {
	if keys == nil || !keys.Valid() || p.IsEmpty() ||
		resolver != nil && resolver.KeySpace() != keys {
		return DynamicReadAddress{}, false
	}
	view := AddressAt(resolver, point, p)
	coordinate, coordinateOK := view.VisibleLocalKeyspaceKey()
	if !coordinateOK {
		coordinate = keys.FromPath(p)
		coordinateOK = coordinate.Kind != keyspace.KindInvalid
	}
	if !coordinateOK {
		return DynamicReadAddress{}, false
	}
	stateKeys := view.StateKeys(StateKeyVisible, StateKeyRootOrVisible)
	if len(stateKeys) == 0 {
		if structural, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(coordinate)); ok {
			stateKeys = []pathaddr.StateKey{structural}
		}
	}
	visible, hasVisible := view.VisibleStateKey()
	rootOrVisible, hasRootOrVisible := view.RootOrVisibleStateKey()
	return DynamicReadAddress{
		Coordinate:       coordinate,
		StateKeys:        stateKeys,
		Visible:          visible,
		HasVisible:       hasVisible,
		RootOrVisible:    rootOrVisible,
		HasRootOrVisible: hasRootOrVisible,
	}, true
}

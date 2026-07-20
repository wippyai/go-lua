package pathevidence

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// CloseRefinementsAcrossTransientEquality computes the finite, certificate-
// filtered image of path refinements under one equality. Root transfer is
// always sound; descendant congruence additionally requires reference-safe
// table equality.
func (l Lane) CloseRefinementsAcrossTransientEquality(
	reg *axis.Registry, keys *keyspace.KeySpace, left, right keyspace.Key,
	memberSafe bool, allow func(keyspace.Key) bool,
) (Lane, bool) {
	if reg == nil || keys == nil || !keys.Valid() || left == right || allow == nil {
		return l, false
	}
	snapshot := cloneLocalValueMap(l.refinements)
	out, changed := l, false
	for source, value := range snapshot {
		for _, direction := range [][2]keyspace.Key{{left, right}, {right, left}} {
			target, ok := rebaseCoordinateRefinementAcrossEquality(keys, source, direction[0], direction[1])
			authorized := ok && allow(target)
			if !ok || target == source || !authorized || source != direction[0] && !memberSafe {
				continue
			}
			current := out.ReadPathKey(reg, target)
			merged := value
			if !product.Equal(reg, current, product.Bottom(reg)) {
				merged = product.Meet(reg, current, value)
				if product.Equal(reg, merged, product.Bottom(reg)) {
					continue
				}
			}
			var wrote bool
			out, wrote = out.WritePathKey(reg, target, merged)
			changed = changed || wrote
		}
	}
	return out, changed
}

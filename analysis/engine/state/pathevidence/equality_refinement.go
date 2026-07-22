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
	out, changed := l, false
	for handle, value := range l.refinements {
		source, ok := keys.KeyByHandle(handle)
		if !ok {
			continue
		}
		for _, direction := range [][2]keyspace.Key{{left, right}, {right, left}} {
			target, valid := rebaseCoordinateRefinementAcrossEquality(keys, source, direction[0], direction[1])
			authorized := valid && allow(target)
			if !valid || target == source || !authorized || source != direction[0] && !memberSafe {
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

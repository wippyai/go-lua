package relation

import (
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

type productCoalescer struct {
	records             map[returnJoinKey]recordJoinResult
	recursiveRewrites   map[recursiveRewriteKey]Type
	discriminants       *discriminant.Detector
	recursiveFamilyFold bool
}

func newProductCoalescer() *productCoalescer {
	return &productCoalescer{}
}

func (c *productCoalescer) discriminantDetector() *discriminant.Detector {
	if c == nil {
		return discriminant.NewDetector()
	}
	if c.discriminants == nil {
		c.discriminants = discriminant.NewDetector()
	}
	return c.discriminants
}

func (c *productCoalescer) slotJoinOrDefault(slotJoin SlotJoinFunc) SlotJoinFunc {
	if slotJoin != nil {
		return slotJoin
	}
	return JoinReturnSlot
}

func (c *productCoalescer) joinKey(a, b Type) returnJoinKey {
	return makeReturnJoinKey(a, b)
}

func (c *productCoalescer) sameJoinInput(a, b Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		if c != nil && c.recursiveFamilyFold {
			return false
		}
		return sameProductFamily(a, b)
	}
	return EqualityHash(a) == EqualityHash(b) && TypeEquals(a, b)
}

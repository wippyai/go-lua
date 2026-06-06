package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

func SetStaticMemberFact(out *PointState, addr StableAddress, value product.AbstractValue) bool {
	if out == nil {
		return false
	}
	before := out.StaticMembers
	out.StaticMembers = out.StaticMembers.WithAddress(addr, value)
	return !StaticMemberFactsDomain.Equal(before, out.StaticMembers)
}

func KillStaticMemberSubtree(out *PointState, addr StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.StaticMembers
	out.StaticMembers = out.StaticMembers.KillSubtreeAddress(addr)
	return !StaticMemberFactsDomain.Equal(before, out.StaticMembers)
}

func SetStaticMemberPath(out *PointState, path constraint.Path, value product.AbstractValue) bool {
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return false
	}
	return SetStaticMemberFact(out, addr, value)
}

func SetStaticMemberSymbolPath(out *PointState, sym cfg.SymbolID, segments []constraint.Segment, value product.AbstractValue) bool {
	addr, ok := StableAddressOfSymbol(sym, segments)
	if !ok {
		return false
	}
	return SetStaticMemberFact(out, addr, value)
}

// InvalidateStaticMemberWritePath applies the static-member write law: ancestor
// member facts become contradictory before the exact written subtree is removed.
// This stays separate from generic address invalidation because StaticMembers is
// a must-fact refinement domain, not ordinary provenance/readback metadata.
func InvalidateStaticMemberWritePath(out *PointState, path constraint.Path) bool {
	if out == nil || path.Symbol == 0 {
		return false
	}
	changed := false
	for i := 1; i < len(path.Segments); i++ {
		prefix := constraint.Path{
			Root:     path.Root,
			Symbol:   path.Symbol,
			Version:  path.Version,
			Segments: append([]constraint.Segment(nil), path.Segments[:i]...),
		}
		changed = SetStaticMemberPath(out, prefix, product.Domain.Bottom()) || changed
	}
	addr, ok := StableAddressOfPath(path)
	if !ok {
		return changed
	}
	return KillStaticMemberSubtree(out, addr) || changed
}

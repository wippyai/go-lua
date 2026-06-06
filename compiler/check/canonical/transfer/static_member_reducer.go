package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

// staticMemberReducer is the transfer-owned reducer for StaticMembers. These
// facts are intentionally not part of flow's generic write invalidation because
// branch refinement uses prefix-bottoming before subtree kills.
type staticMemberReducer struct{}

var staticMembers = staticMemberReducer{}

func (staticMemberReducer) set(out *flow.PointState, addr flow.StableAddress, value product.AbstractValue) bool {
	if out == nil {
		return false
	}
	before := out.StaticMembers
	out.StaticMembers = out.StaticMembers.WithAddress(addr, value)
	return !flow.StaticMemberFactsDomain.Equal(before, out.StaticMembers)
}

func (staticMemberReducer) killSubtree(out *flow.PointState, addr flow.StableAddress) bool {
	if out == nil {
		return false
	}
	before := out.StaticMembers
	out.StaticMembers = out.StaticMembers.KillSubtreeAddress(addr)
	return !flow.StaticMemberFactsDomain.Equal(before, out.StaticMembers)
}

func (r staticMemberReducer) setPath(out *flow.PointState, path constraint.Path, value product.AbstractValue) bool {
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return false
	}
	return r.set(out, addr, value)
}

func (r staticMemberReducer) setSymbolPath(out *flow.PointState, sym cfg.SymbolID, segments []constraint.Segment, value product.AbstractValue) bool {
	addr, ok := flow.StableAddressOfSymbol(sym, segments)
	if !ok {
		return false
	}
	return r.set(out, addr, value)
}

func (r staticMemberReducer) invalidateWritePath(out *flow.PointState, path constraint.Path) bool {
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
		changed = r.setPath(out, prefix, product.Domain.Bottom()) || changed
	}
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return changed
	}
	return r.killSubtree(out, addr) || changed
}

package observation

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func (p Projector) resolveLocalRefs(t typ.Type, point cfg.Point) typ.Type {
	if t == nil {
		return t
	}
	sc := p.scopeAt(point)
	if sc == nil {
		return t
	}

	visiting := make(map[string]bool)
	var resolve func(typ.Type, int) typ.Type
	resolve = func(current typ.Type, depth int) typ.Type {
		if current == nil || typ.DepthExceeded(depth) {
			return current
		}
		return typ.Rewrite(current, func(node typ.Type) (typ.Type, bool) {
			ref, ok := node.(*typ.Ref)
			if !ok || ref.Module != "" {
				return nil, false
			}
			target, exists := sc.LookupType(ref.Name)
			if !exists || target == nil || visiting[ref.Name] {
				return nil, false
			}
			visiting[ref.Name] = true
			resolved := resolve(target, depth+1)
			delete(visiting, ref.Name)
			if resolved == nil {
				return nil, false
			}
			return resolved, true
		})
	}
	return resolve(t, 0)
}

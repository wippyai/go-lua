package product

import "github.com/wippyai/go-lua/analysis/domain/value/axis/presence"

func (rt *registryRuntime) sameNode(n *node, shape Shape, p presence.Value, slots []slot) bool {
	if n.reg != rt.reg || n.shape != shape || !presence.Equal(n.presence, p) || len(n.slots) != len(slots) {
		return false
	}
	for i, left := range n.slots {
		right := slots[i]
		if left.key != right.key {
			return false
		}
		info, ok := rt.axis(left.key)
		if !ok || !info.spec.EqualAny(left.value, right.value) {
			return false
		}
	}
	return true
}

func (rt *registryRuntime) isProductBottom(p presence.Value, slots []slot) bool {
	if presence.Equal(p, presence.Bottom()) {
		return true
	}
	for _, slot := range slots {
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if info.spec.EqualAny(slot.value, info.bottomAny) {
			return true
		}
	}
	return false
}

package product

func (rt *registryRuntime) validateValue(v Value) {
	if v.n == nil {
		return
	}
	if v.n.reg != rt.reg {
		panic("product: value belongs to a different registry")
	}
	for _, slot := range v.n.slots {
		if _, ok := rt.axis(slot.key); !ok {
			panic("product: value contains slot outside registry: " + slot.key)
		}
	}
}

func (rt *registryRuntime) axisValue(info axisRuntimeAxis, v Value) any {
	if v.n != nil {
		for _, slot := range v.n.slots {
			if slot.key == info.id {
				return slot.value
			}
		}
	}
	return info.topAny
}

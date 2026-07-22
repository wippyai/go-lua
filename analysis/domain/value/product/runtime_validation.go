package product

func (rt *registryRuntime) validateValue(v Value) {
	if v.n == nil {
		return
	}
	if v.n.reg != rt.reg {
		panic("product: value belongs to a different registry")
	}
	for _, slot := range v.n.slots {
		_ = rt.axisOrdinal(slot.ordinal)
	}
}

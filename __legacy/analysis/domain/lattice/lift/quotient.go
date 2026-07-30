package lift

type quotientContribution[D, F comparable] struct {
	destination D
	fiber       F
}

// QuotientMustSet computes the universal image of a finite must set. A
// destination fact is retained exactly when every member of its inverse
// structural fiber emits that fact.
func QuotientMustSet[S, D, F comparable](
	source map[S]struct{},
	image func(S) ([]D, bool),
	sourceFiber func(S) F,
	inverseFiber func(D) ([]F, bool),
) (map[D]struct{}, bool) {
	if len(source) == 0 {
		return nil, true
	}
	// Quotient relations are overwhelmingly injective. Prove that locally and
	// emit directly without allocating coverage tables. A genuine quotient
	// class falls through to the universal construction below.
	direct := make(map[D]struct{}, len(source))
	injective := true
	for value := range source {
		destinations, ok := image(value)
		if !ok {
			return nil, false
		}
		fiber := sourceFiber(value)
		for _, destination := range destinations {
			fibers, valid := inverseFiber(destination)
			if !valid || len(fibers) == 0 {
				return nil, false
			}
			if len(fibers) != 1 || fibers[0] != fiber {
				injective = false
				break
			}
			direct[destination] = struct{}{}
		}
		if !injective {
			break
		}
	}
	if injective {
		return direct, true
	}
	candidates := make(map[D]struct{})
	covered := make(map[quotientContribution[D, F]]struct{})
	for value := range source {
		destinations, ok := image(value)
		if !ok {
			return nil, false
		}
		fiber := sourceFiber(value)
		for _, destination := range destinations {
			candidates[destination] = struct{}{}
			covered[quotientContribution[D, F]{destination: destination, fiber: fiber}] = struct{}{}
		}
	}
	out := make(map[D]struct{})
	for candidate := range candidates {
		fibers, ok := inverseFiber(candidate)
		if !ok || len(fibers) == 0 {
			return nil, false
		}
		complete := true
		for _, fiber := range fibers {
			if _, exists := covered[quotientContribution[D, F]{destination: candidate, fiber: fiber}]; !exists {
				complete = false
				break
			}
		}
		if complete {
			out[candidate] = struct{}{}
		}
	}
	return out, true
}

type quotientMapContribution[D, F comparable] struct {
	destination D
	fiber       F
}

// QuotientMustMap computes the universal image of a finite must map. Missing
// preimages denote Top and remove the destination key. Values present at every
// preimage are combined using the element lattice's Join.
func QuotientMustMap[S, D, F comparable, V any](
	source map[S]V,
	imageKey func(S) ([]D, bool),
	imageValue func(V) (V, bool),
	sourceFiber func(S) F,
	inverseFiber func(D) ([]F, bool),
	join func(V, V) V,
) (map[D]V, bool) {
	if len(source) == 0 {
		return nil, true
	}
	direct := make(map[D]V, len(source))
	injective := true
	for sourceKey, value := range source {
		destinations, ok := imageKey(sourceKey)
		if !ok {
			return nil, false
		}
		mapped, ok := imageValue(value)
		if !ok {
			return nil, false
		}
		fiber := sourceFiber(sourceKey)
		for _, destination := range destinations {
			fibers, valid := inverseFiber(destination)
			if !valid || len(fibers) == 0 {
				return nil, false
			}
			if len(fibers) != 1 || fibers[0] != fiber {
				injective = false
				break
			}
			candidate := mapped
			if existing, found := direct[destination]; found {
				candidate = join(existing, candidate)
			}
			direct[destination] = candidate
		}
		if !injective {
			break
		}
	}
	if injective {
		return direct, true
	}
	contributions := make(map[quotientMapContribution[D, F]]V)
	candidates := make(map[D]struct{})
	for sourceKey, value := range source {
		destinations, ok := imageKey(sourceKey)
		if !ok {
			return nil, false
		}
		mapped, ok := imageValue(value)
		if !ok {
			return nil, false
		}
		fiber := sourceFiber(sourceKey)
		for _, destination := range destinations {
			key := quotientMapContribution[D, F]{destination: destination, fiber: fiber}
			candidate := mapped
			if existing, found := contributions[key]; found {
				candidate = join(existing, candidate)
			}
			contributions[key] = candidate
			candidates[destination] = struct{}{}
		}
	}
	out := make(map[D]V)
	for candidate := range candidates {
		fibers, ok := inverseFiber(candidate)
		if !ok || len(fibers) == 0 {
			return nil, false
		}
		var joined V
		complete := true
		for i, fiber := range fibers {
			value, exists := contributions[quotientMapContribution[D, F]{destination: candidate, fiber: fiber}]
			if !exists {
				complete = false
				break
			}
			if i == 0 {
				joined = value
			} else {
				joined = join(joined, value)
			}
		}
		if complete {
			out[candidate] = joined
		}
	}
	return out, true
}

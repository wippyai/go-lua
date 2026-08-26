package region

// Conjoin and Entails operate only on already-sealed Regions.  Their builder
// is transaction-local: no cache, map, or mutable node storage escapes the
// call.  The resulting graph is frozen in deterministic low-then-high
// postorder before its canonical identity is issued.

// Conjoin returns the canonical Boolean conjunction of left and right.  It is
// commutative at the function level even though the two input values may have
// different transport histories.
func Conjoin(left, right Region) (Region, bool) {
	if !left.Available() || !right.Available() {
		return Region{}, false
	}
	builder, roots, ok := importRegions(left, right)
	if !ok {
		return Region{}, false
	}
	root, ok := builder.ite(roots[0], roots[1], falseReference)
	if !ok {
		return Region{}, false
	}
	return builder.freeze(root)
}

func not(value Region) (Region, bool) {
	if !value.Available() {
		return Region{}, false
	}
	builder, roots, ok := importRegions(value)
	if !ok {
		return Region{}, false
	}
	root, ok := builder.ite(roots[0], falseReference, trueReference)
	if !ok {
		return Region{}, false
	}
	return builder.freeze(root)
}

// Entails reports exact Boolean implication between two sealed Regions.
func Entails(available, requested Region) bool {
	if !available.Available() || !requested.Available() {
		return false
	}
	notRequested, ok := not(requested)
	if !ok {
		return false
	}
	violation, ok := Conjoin(available, notRequested)
	return ok && violation.IsFalse()
}

type operationKey struct {
	test, yes, no uint32
}

type builder struct {
	nodes  []node
	unique map[nodeKey]uint32
	cache  map[operationKey]uint32
}

func newBuilder() *builder {
	return &builder{
		unique: make(map[nodeKey]uint32),
		cache:  make(map[operationKey]uint32),
	}
}

func (builder *builder) validReference(reference uint32) bool {
	return reference <= uint32(len(builder.nodes))+1
}

func (builder *builder) node(atom Atom, low, high uint32) (uint32, bool) {
	if !atom.Available() || !builder.validReference(low) || !builder.validReference(high) {
		return 0, false
	}
	if low == high {
		return low, true
	}
	for _, child := range []uint32{low, high} {
		if child < firstNodeReference {
			continue
		}
		childAtom, ok := builder.atom(child)
		if !ok || !atomLess(atom, childAtom) {
			return 0, false
		}
	}
	key := nodeKey{atom: atom, low: low, high: high}
	if existing, ok := builder.unique[key]; ok {
		return existing, true
	}
	reference := uint32(len(builder.nodes)) + firstNodeReference
	builder.nodes = append(builder.nodes, node{atom: atom, low: low, high: high})
	builder.unique[key] = reference
	return reference, true
}

func (builder *builder) atom(reference uint32) (Atom, bool) {
	if reference < firstNodeReference || reference > uint32(len(builder.nodes))+1 {
		return Atom{}, false
	}
	return builder.nodes[reference-firstNodeReference].atom, true
}

func (builder *builder) branch(reference uint32, high bool) (uint32, bool) {
	if reference < firstNodeReference || reference > uint32(len(builder.nodes))+1 {
		return 0, false
	}
	value := builder.nodes[reference-firstNodeReference]
	if high {
		return value.high, true
	}
	return value.low, true
}

func (builder *builder) ite(test, yes, no uint32) (uint32, bool) {
	if !builder.validReference(test) || !builder.validReference(yes) || !builder.validReference(no) {
		return 0, false
	}
	if test == trueReference || yes == no {
		return yes, true
	}
	if test == falseReference {
		return no, true
	}
	key := operationKey{test: test, yes: yes, no: no}
	// The general ITE cache is keyed by all three operands.  The op field is
	// retained for the operation-specific wrappers below and future extension.
	if result, ok := builder.cache[key]; ok {
		return result, true
	}
	decision, ok := builder.atom(test)
	if !ok {
		return 0, false
	}
	for _, candidate := range []uint32{yes, no} {
		if candidate < firstNodeReference {
			continue
		}
		candidateAtom, candidateOK := builder.atom(candidate)
		if !candidateOK {
			return 0, false
		}
		if atomLess(candidateAtom, decision) {
			decision = candidateAtom
		}
	}
	cofactor := func(value uint32, high bool) (uint32, bool) {
		if value < firstNodeReference {
			return value, true
		}
		valueAtom, valueOK := builder.atom(value)
		if !valueOK {
			return 0, false
		}
		if atomEqual(valueAtom, decision) {
			return builder.branch(value, high)
		}
		return value, true
	}
	testLow, ok := cofactor(test, false)
	if !ok {
		return 0, false
	}
	testHigh, ok := cofactor(test, true)
	if !ok {
		return 0, false
	}
	yesLow, ok := cofactor(yes, false)
	if !ok {
		return 0, false
	}
	yesHigh, ok := cofactor(yes, true)
	if !ok {
		return 0, false
	}
	noLow, ok := cofactor(no, false)
	if !ok {
		return 0, false
	}
	noHigh, ok := cofactor(no, true)
	if !ok {
		return 0, false
	}
	low, ok := builder.ite(testLow, yesLow, noLow)
	if !ok {
		return 0, false
	}
	high, ok := builder.ite(testHigh, yesHigh, noHigh)
	if !ok {
		return 0, false
	}
	result, ok := builder.node(decision, low, high)
	if ok {
		builder.cache[key] = result
	}
	return result, ok
}

func importRegions(regions ...Region) (*builder, []uint32, bool) {
	builder := newBuilder()
	roots := make([]uint32, len(regions))
	for index, region := range regions {
		if !region.Available() {
			return nil, nil, false
		}
		root, ok := builder.importRegion(region)
		if !ok {
			return nil, nil, false
		}
		roots[index] = root
	}
	return builder, roots, true
}

func (builder *builder) importRegion(region Region) (uint32, bool) {
	if !region.Available() || !region.rootValid() {
		return 0, false
	}
	seen := map[uint32]uint32{falseReference: falseReference, trueReference: trueReference}
	var copyNode func(uint32) (uint32, bool)
	copyNode = func(reference uint32) (uint32, bool) {
		if value, ok := seen[reference]; ok {
			return value, true
		}
		if reference < firstNodeReference || reference > uint32(len(region.nodes))+1 {
			return 0, false
		}
		value := region.nodes[reference-firstNodeReference]
		low, ok := copyNode(value.low)
		if !ok {
			return 0, false
		}
		high, ok := copyNode(value.high)
		if !ok {
			return 0, false
		}
		result, ok := builder.node(value.atom, low, high)
		if !ok {
			return 0, false
		}
		seen[reference] = result
		return result, true
	}
	return copyNode(region.root)
}

func (builder *builder) freeze(root uint32) (Region, bool) {
	if !builder.validReference(root) {
		return Region{}, false
	}
	remap := map[uint32]uint32{falseReference: falseReference, trueReference: trueReference}
	nodes := make([]node, 0, len(builder.nodes))
	var visit func(uint32) (uint32, bool)
	visit = func(reference uint32) (uint32, bool) {
		if value, ok := remap[reference]; ok {
			return value, true
		}
		if reference < firstNodeReference || reference > uint32(len(builder.nodes))+1 {
			return 0, false
		}
		value := builder.nodes[reference-firstNodeReference]
		low, ok := visit(value.low)
		if !ok {
			return 0, false
		}
		high, ok := visit(value.high)
		if !ok {
			return 0, false
		}
		if low == high {
			remap[reference] = low
			return low, true
		}
		canonical, ok := (func() (uint32, bool) {
			atom := value.atom
			if !atom.Available() {
				return 0, false
			}
			if low >= firstNodeReference {
				child := nodes[low-firstNodeReference].atom
				if !atomLess(atom, child) {
					return 0, false
				}
			}
			if high >= firstNodeReference {
				child := nodes[high-firstNodeReference].atom
				if !atomLess(atom, child) {
					return 0, false
				}
			}
			result := uint32(len(nodes)) + firstNodeReference
			nodes = append(nodes, node{atom: atom, low: low, high: high})
			return result, true
		})()
		if !ok {
			return 0, false
		}
		remap[reference] = canonical
		return canonical, true
	}
	canonicalRoot, ok := visit(root)
	if !ok {
		return Region{}, false
	}
	result := Region{nodes: nodes, root: canonicalRoot}
	result.digest = digestRegion(result)
	if !result.digest.Available() {
		return Region{}, false
	}
	result.valid = true
	return result, true
}

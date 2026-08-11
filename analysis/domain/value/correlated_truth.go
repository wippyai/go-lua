package value

// Not applies Lua logical-not to the complete correlated Value relation.
// Capabilities and rooted identity deliberately do not travel through a
// Boolean result. The result contains precisely true for each falsy input
// alternative and false for each truthy alternative.
func (schema *Schema) Not(input Value) (Value, bool) {
	if schema == nil || !schema.owns(input) {
		return Value{}, false
	}
	if schema.Equal(input, schema.Bottom()) {
		return input, true
	}
	falseAtom := schema.atomByRow[atomRow{kind: atomFalse}]
	trueAtom := schema.atomByRow[atomRow{kind: atomTrue}]
	if falseAtom == 0 || trueAtom == 0 {
		return Value{}, false
	}
	includeFalse, includeTrue := false, false
	if input.top {
		includeFalse, includeTrue = true, true
	} else {
		stride := schema.stride()
		for offset := 0; offset < len(input.image); offset += stride {
			truth := schema.atomTruth(uint32(input.image[offset]))
			if truth.MayBeTrue() {
				includeFalse = true
			}
			if truth.MayBeFalse() {
				includeTrue = true
			}
		}
	}
	atoms := make([]Atom, 0, 2)
	if includeFalse {
		atoms = append(atoms, Atom{schema: schema, id: falseAtom})
	}
	if includeTrue {
		atoms = append(atoms, Atom{schema: schema, id: trueAtom})
	}
	return schema.Alternatives(atoms...)
}

// FilterTruth retains exactly the input alternatives observable on one Lua
// truth edge. It preserves each retained atom's capability correlation; no
// projection/rebuild may smear a capability between alternatives.
func (schema *Schema) FilterTruth(input Value, truthy bool) (Value, bool) {
	if schema == nil || !schema.owns(input) {
		return Value{}, false
	}
	if schema.Equal(input, schema.Bottom()) {
		return input, true
	}
	if input.top {
		image := make([]uint64, 0, len(schema.atoms)*schema.stride())
		for index := range schema.atoms {
			truth := schema.atomTruth(uint32(index + 1))
			if (truthy && !truth.MayBeTrue()) || (!truthy && !truth.MayBeFalse()) {
				continue
			}
			row := make([]uint64, schema.stride())
			row[0] = uint64(index + 1)
			// Top denotes every capability attachment admissible for every
			// atom. Filtering by truth may remove atoms, never capability
			// possibilities of those retained atoms. A zero capability tail
			// here would be a false must-not-have conclusion after `and`/`or`.
			for word := 0; word < schema.capWords; word++ {
				row[1+word] = schema.fullCapabilityWord(word)
			}
			image = append(image, row...)
		}
		return schema.canonical(image), true
	}
	stride := schema.stride()
	image := make([]uint64, 0, len(input.image))
	for offset := 0; offset < len(input.image); offset += stride {
		truth := schema.atomTruth(uint32(input.image[offset]))
		if (truthy && !truth.MayBeTrue()) || (!truthy && !truth.MayBeFalse()) {
			continue
		}
		image = append(image, input.image[offset:offset+stride]...)
	}
	return schema.canonical(image), true
}

func (schema *Schema) fullCapabilityWord(word int) uint64 {
	if schema == nil || word < 0 || word >= schema.capWords {
		return 0
	}
	if word+1 < schema.capWords || len(schema.capabilities)%64 == 0 {
		return ^uint64(0)
	}
	return uint64(1)<<uint(len(schema.capabilities)%64) - 1
}

package typevalue

import (
	"math"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/keyspace"
)

type innerDisposition uint8

const (
	innerExact innerDisposition = iota + 1
	innerOther
)

// NameDisposition is the exact finite name observation of one correlated
// descriptor. Other is a declared summary image, never a wildcard coordinate.
type NameDisposition uint8

const (
	NameNone NameDisposition = iota + 1
	NameExact
	NameOther
)

type resolverDisposition uint8

const (
	resolverNone resolverDisposition = iota + 1
	resolverExact
	resolverOther
)

type descriptorRow struct {
	innerKind    innerDisposition
	inner        typeauthority.RuntimeInner
	nameKind     NameDisposition
	name         uint32 // one-based into names when exact
	resolverKind resolverDisposition
}

type descriptorKey struct {
	innerKind    innerDisposition
	inner        typeauthority.RuntimeInner
	nameKind     NameDisposition
	name         uint32
	resolverKind resolverDisposition
}

// Descriptor is one correlated (inner,name,resolver) row. Coordinates cannot
// be recombined independently, so impossible Cartesian states are unnameable.
type Descriptor struct {
	owner *Authority
	index uint32
}

type seedRow struct {
	valueID    keyspace.ContentID
	descriptor Descriptor
	root       uint32
}

// Seed is one binder-authorized Program TypeValue source row.
type Seed struct {
	owner *Authority
	index uint32
}

// forEachStaticSeed consumes Static's total occurrence relation. TypeValue
// performs no Program or Static-expression rescans of its own.
func (a *Authority) forEachStaticSeed(visit func(keyspace.ContentID, string, keyspace.ContentID, typeauthority.RuntimeInner, bool) bool) bool {
	if a == nil || a.static == nil || a.runtime == nil || visit == nil {
		return false
	}
	seeds := a.static.TypeValueSeeds()
	for index := 0; index < seeds.Count(); index++ {
		seed, ok := seeds.At(index)
		if !ok {
			return false
		}
		valueID, ok := seeds.ValueIdentity(seed)
		if !ok {
			return false
		}
		name, ok := seeds.Name(seed)
		if !ok {
			return false
		}
		root, ok := seeds.RootIdentity(seed)
		if !ok {
			return false
		}
		inner, exact := seeds.ExactInner(seed)
		if exact && !a.runtime.Equal(inner, inner) {
			return false
		}
		if !visit(valueID, name, root, inner, exact) {
			return false
		}
	}
	return true
}

func (a *Authority) sealDescriptors() bool {
	if a == nil || a.runtime == nil {
		return false
	}
	a.nameIndex = make(map[string]uint32)
	byDescriptor := make(map[descriptorKey]uint32)
	appendDescriptor := func(row descriptorRow) (Descriptor, bool) {
		key := descriptorKey(row)
		if index, found := byDescriptor[key]; found {
			return Descriptor{owner: a, index: index}, true
		}
		if uint64(len(a.descriptors)) > uint64(math.MaxUint32) {
			return Descriptor{}, false
		}
		index := uint32(len(a.descriptors))
		a.descriptors = append(a.descriptors, row)
		byDescriptor[key] = index
		return Descriptor{owner: a, index: index}, true
	}
	// The two declared summary images close unrepresented runtime objects and
	// their structural reflection children without inventing exact authority.
	if _, ok := appendDescriptor(descriptorRow{innerKind: innerOther, nameKind: NameOther, resolverKind: resolverOther}); !ok {
		return false
	}
	if _, ok := appendDescriptor(descriptorRow{innerKind: innerOther, nameKind: NameNone, resolverKind: resolverNone}); !ok {
		return false
	}
	if !a.forEachStaticSeed(func(valueID keyspace.ContentID, name string, _ keyspace.ContentID, inner typeauthority.RuntimeInner, exactInner bool) bool {
		rootIndex, admitted := a.runtimeRoots[valueID]
		if !admitted {
			return false
		}
		nameIndex, ok := a.internName(name)
		if !ok {
			return false
		}
		row := descriptorRow{
			innerKind: innerOther,
			nameKind:  NameExact, name: nameIndex,
			resolverKind: resolverNone,
		}
		if exactInner {
			row.innerKind = innerExact
			row.inner = inner
		}
		descriptor, ok := appendDescriptor(row)
		if !ok {
			return false
		}
		a.seeds = append(a.seeds, seedRow{valueID: valueID, descriptor: descriptor, root: rootIndex})
		return true
	}) {
		return false
	}
	runtime := a.runtime
	processed := 0
	for {
		// Reflection closure is a graph traversal over exact Runtime inners.
		// Newly appended descriptors are consumed on the next pass.
		for processed < len(a.descriptors) {
			row := a.descriptors[processed]
			processed++
			if row.innerKind != innerExact {
				continue
			}
			children, ok := reflectionChildren(runtime, row.inner)
			if !ok {
				return false
			}
			for _, child := range children {
				if _, ok := appendDescriptor(descriptorRow{innerKind: innerExact, inner: child, nameKind: NameNone, resolverKind: resolverNone}); !ok {
					return false
				}
			}
			form, ok := runtime.Form(row.inner)
			if !ok {
				return false
			}
			if form == typeauthority.FormGeneric {
				if _, ok := appendDescriptor(descriptorRow{innerKind: innerOther, nameKind: row.nameKind, name: row.name, resolverKind: resolverNone}); !ok {
					return false
				}
			}
		}
		// Existing canonical instantiated classes are exact only when their base
		// and every argument are already reachable descriptors. This is a cold
		// monotone reachability projection over Runtime's canonical relation rows,
		// not a second tuple matcher: every hot exact application goes exclusively
		// through Runtime's owner-fenced trie.
		beforePass := len(a.descriptors)
		reachable := make(map[typeauthority.RuntimeInner]struct{}, len(a.descriptors))
		for _, row := range a.descriptors {
			if row.innerKind == innerExact {
				reachable[row.inner] = struct{}{}
			}
		}
		for index := 0; index < runtime.InstantiationCount(); index++ {
			result, base, argumentCount, ok := runtime.InstantiationAt(index)
			if !ok {
				return false
			}
			if _, present := reachable[base]; !present {
				continue
			}
			all := true
			for argumentIndex := 0; argumentIndex < argumentCount; argumentIndex++ {
				argument, valid := runtime.InstantiationArgumentAt(index, argumentIndex)
				if !valid {
					return false
				}
				if _, present := reachable[argument]; !present {
					all = false
					break
				}
			}
			if !all {
				continue
			}
			for _, subject := range a.descriptors {
				if subject.innerKind != innerExact || !runtime.Equal(subject.inner, base) {
					continue
				}
				if _, ok := appendDescriptor(descriptorRow{innerKind: innerExact, inner: result, nameKind: subject.nameKind, name: subject.name, resolverKind: resolverNone}); !ok {
					return false
				}
			}
		}
		if processed == len(a.descriptors) && len(a.descriptors) == beforePass {
			break
		}
	}
	a.descriptorIndex = byDescriptor
	return len(a.descriptors) != 0
}

func (a *Authority) internName(name string) (uint32, bool) {
	if a == nil || name == "" {
		return 0, false
	}
	if index, ok := a.nameIndex[name]; ok {
		return index, true
	}
	if uint64(len(a.names)) >= uint64(math.MaxUint32) {
		return 0, false
	}
	index := uint32(len(a.names) + 1)
	a.names = append(a.names, name)
	a.nameIndex[name] = index
	return index, true
}

func reflectionChildren(runtime *typeauthority.Runtime, inner typeauthority.RuntimeInner) ([]typeauthority.RuntimeInner, bool) {
	form, ok := runtime.Form(inner)
	if !ok {
		return nil, false
	}
	var out []typeauthority.RuntimeInner
	appendOne := func(child typeauthority.RuntimeInner, present bool) bool {
		if present {
			out = append(out, child)
		}
		return present
	}
	switch form {
	case typeauthority.FormArray:
		child, present := runtime.Element(inner)
		if !appendOne(child, present) {
			return nil, false
		}
	case typeauthority.FormMap:
		key, value, present := runtime.Mapping(inner)
		if !present {
			return nil, false
		}
		out = append(out, key, value)
	case typeauthority.FormOptional:
		child, present := runtime.Inner(inner)
		if !appendOne(child, present) {
			return nil, false
		}
	case typeauthority.FormFunction:
		if child, present := runtime.Return(inner); present {
			out = append(out, child)
		}
		for index := 0; index < runtime.ParameterCount(inner); index++ {
			child, childPresent, valid := runtime.ParameterAt(inner, index)
			if !valid {
				return nil, false
			}
			appendOne(child, childPresent)
		}
	case typeauthority.FormRecord:
		for index := 0; index < runtime.FieldCount(inner); index++ {
			_, child, childPresent, valid := runtime.FieldAt(inner, index)
			if !valid {
				return nil, false
			}
			appendOne(child, childPresent)
		}
	case typeauthority.FormUnion:
		for index := 0; index < runtime.VariantCount(inner); index++ {
			child, childPresent, valid := runtime.VariantAt(inner, index)
			if !valid {
				return nil, false
			}
			appendOne(child, childPresent)
		}
	case typeauthority.FormGeneric:
		for index := 0; index < runtime.TypeParameterCount(inner); index++ {
			_, child, childPresent, valid := runtime.TypeParameterAt(inner, index)
			if !valid {
				return nil, false
			}
			appendOne(child, childPresent)
		}
	}
	return out, true
}

func (a *Authority) DescriptorCount() int {
	if a == nil {
		return 0
	}
	return len(a.descriptors)
}

func (a *Authority) DescriptorAt(index int) (Descriptor, bool) {
	if a == nil || index < 0 || index >= len(a.descriptors) {
		return Descriptor{}, false
	}
	return Descriptor{owner: a, index: uint32(index)}, true
}

func (a *Authority) DescriptorInner(descriptor Descriptor) (typeauthority.RuntimeInner, bool) {
	row, ok := a.descriptor(descriptor)
	return row.inner, ok && row.innerKind == innerExact
}

func (a *Authority) DescriptorName(descriptor Descriptor) (string, NameDisposition, bool) {
	row, ok := a.descriptor(descriptor)
	if !ok {
		return "", 0, false
	}
	if row.nameKind != NameExact {
		return "", row.nameKind, true
	}
	if row.name == 0 || uint64(row.name) > uint64(len(a.names)) {
		return "", 0, false
	}
	return a.names[row.name-1], NameExact, true
}

func (a *Authority) descriptor(descriptor Descriptor) (descriptorRow, bool) {
	if a == nil || descriptor.owner != a || uint64(descriptor.index) >= uint64(len(a.descriptors)) {
		return descriptorRow{}, false
	}
	return a.descriptors[descriptor.index], true
}

func (a *Authority) SeedCount() int {
	if a == nil {
		return 0
	}
	return len(a.seeds)
}

func (a *Authority) SeedAt(index int) (Seed, bool) {
	if a == nil || index < 0 || index >= len(a.seeds) {
		return Seed{}, false
	}
	return Seed{owner: a, index: uint32(index)}, true
}

func (a *Authority) SeedRoot(seed Seed) (Root, bool) {
	row, ok := a.seed(seed)
	if !ok || uint64(row.root) >= uint64(len(a.roots)) {
		return Root{}, false
	}
	return Root{owner: a, index: row.root}, true
}

func (a *Authority) SeedDescriptor(seed Seed) (Descriptor, bool) {
	row, ok := a.seed(seed)
	return row.descriptor, ok
}

func (a *Authority) SeedValueIdentity(seed Seed) (keyspace.ContentID, bool) {
	row, ok := a.seed(seed)
	return row.valueID, ok && row.valueID.Available()
}

func (a *Authority) seed(seed Seed) (seedRow, bool) {
	if a == nil || seed.owner != a || uint64(seed.index) >= uint64(len(a.seeds)) {
		return seedRow{}, false
	}
	return a.seeds[seed.index], true
}

// StructuralEqual and Subtype expose three-valued Runtime judgments without
// inventing proof tokens. decided=false is the only answer involving Other.
func (a *Authority) StructuralEqual(left, right Descriptor) (answer, decided bool) {
	leftRow, leftOK := a.descriptor(left)
	rightRow, rightOK := a.descriptor(right)
	if !leftOK || !rightOK || leftRow.innerKind != innerExact || rightRow.innerKind != innerExact {
		return false, false
	}
	return a.runtime.StructuralEqual(leftRow.inner, rightRow.inner)
}

func (a *Authority) Subtype(left, right Descriptor) (answer, decided bool) {
	leftRow, leftOK := a.descriptor(left)
	rightRow, rightOK := a.descriptor(right)
	if !leftOK || !rightOK || leftRow.innerKind != innerExact || rightRow.innerKind != innerExact {
		return false, false
	}
	return a.runtime.Subtype(leftRow.inner, rightRow.inner)
}

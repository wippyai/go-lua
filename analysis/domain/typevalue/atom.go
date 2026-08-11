package typevalue

import (
	"math"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
)

// Selector is the closed runtime LType member vocabulary. Record members are
// described by Runtime fields and never extend this method selector family.
type Selector uint8

const (
	SelectorIs Selector = iota + 1
	SelectorKind
	SelectorName
	SelectorElem
	SelectorKey
	SelectorVal
	SelectorInner
	SelectorRet
	SelectorFields
	SelectorVariants
	SelectorParams
	SelectorTparams
	selectorEnd
)

const selectorCount = int(selectorEnd - 1)

var selectorNames = [...]string{
	SelectorIs:       "is",
	SelectorKind:     "kind",
	SelectorName:     "name",
	SelectorElem:     "elem",
	SelectorKey:      "key",
	SelectorVal:      "val",
	SelectorInner:    "inner",
	SelectorRet:      "ret",
	SelectorFields:   "fields",
	SelectorVariants: "variants",
	SelectorParams:   "params",
	SelectorTparams:  "tparams",
}

func (selector Selector) Valid() bool { return selector > 0 && selector < selectorEnd }

func (selector Selector) String() string {
	if !selector.Valid() {
		return ""
	}
	return selectorNames[selector]
}

func SelectorForName(name string) (Selector, bool) {
	for selector := SelectorIs; selector < selectorEnd; selector++ {
		if selectorNames[selector] == name {
			return selector, true
		}
	}
	return 0, false
}

func iteratorSelectorOrdinal(selector Selector) (uint64, bool) {
	switch selector {
	case SelectorFields:
		return 0, true
	case SelectorVariants:
		return 1, true
	case SelectorParams:
		return 2, true
	case SelectorTparams:
		return 3, true
	default:
		return 0, false
	}
}

// Cursor is one exact VM-order iterator position or the single absorbing
// unknown position. It is authority-fenced because its exact range is sealed.
type Cursor struct {
	owner *Authority
	index uint32
}

func (a *Authority) CursorAt(index int) (Cursor, bool) {
	if a == nil || index < 0 || uint64(index) >= uint64(a.cursorEnd)-1 {
		return Cursor{}, false
	}
	return Cursor{owner: a, index: uint32(index)}, true
}

func (a *Authority) UnknownCursor() (Cursor, bool) {
	if a == nil || a.cursorEnd == 0 {
		return Cursor{}, false
	}
	return Cursor{owner: a, index: a.cursorEnd - 1}, true
}

func (a *Authority) CursorPosition(cursor Cursor) (position int, unknown bool, ok bool) {
	if a == nil || cursor.owner != a || cursor.index >= a.cursorEnd {
		return 0, false, false
	}
	if cursor.index == a.cursorEnd-1 {
		return 0, true, true
	}
	return int(cursor.index), false, true
}

type AtomKind uint8

const (
	AtomObject AtomKind = iota + 1
	AtomMethod
	AtomIterator
)

// Atom is one compact member of the finite homogeneous TypeValue powerset.
// Its integer is decoded by the authority; no root×selector rows are stored.
type Atom struct {
	owner *Authority
	id    uint64
}

func (a *Authority) sealAtomRange() bool {
	if a == nil || len(a.descriptors) == 0 || len(a.roots) > math.MaxUint32 {
		return false
	}
	maximum := 0
	runtime := a.runtime
	for _, row := range a.descriptors {
		if row.innerKind != innerExact {
			continue
		}
		form, ok := runtime.Form(row.inner)
		if !ok {
			return false
		}
		count := 0
		switch form {
		case typeauthority.FormRecord:
			count = runtime.FieldCount(row.inner)
		case typeauthority.FormUnion:
			count = runtime.VariantCount(row.inner)
		case typeauthority.FormFunction:
			count = runtime.ParameterCount(row.inner)
		case typeauthority.FormGeneric:
			count = runtime.TypeParameterCount(row.inner)
		}
		if count < 0 || count > maximum {
			maximum = count
		}
	}
	if uint64(maximum)+2 > uint64(math.MaxUint32) {
		return false
	}
	a.cursorEnd = uint32(maximum + 2) // exact 0..maximum, then absorbing unknown
	a.objectEnd = uint64(len(a.descriptors))
	methods, ok := checkedMul(uint64(selectorCount), uint64(len(a.roots)))
	if !ok {
		return false
	}
	a.methodEnd, ok = checkedAdd(a.objectEnd, methods)
	if !ok {
		return false
	}
	iterators, ok := checkedMul(4, uint64(len(a.roots)))
	if !ok {
		return false
	}
	iterators, ok = checkedMul(iterators, uint64(a.cursorEnd))
	if !ok {
		return false
	}
	a.atomEnd, ok = checkedAdd(a.methodEnd, iterators)
	return ok && a.atomEnd != math.MaxUint64
}

func (a *Authority) AtomCount() uint64 {
	if a == nil {
		return 0
	}
	return a.atomEnd
}

func (a *Authority) Object(descriptor Descriptor) (Atom, bool) {
	if _, ok := a.descriptor(descriptor); !ok {
		return Atom{}, false
	}
	return Atom{owner: a, id: uint64(descriptor.index)}, true
}

func (a *Authority) Method(selector Selector, object Root) (Atom, bool) {
	if !selector.Valid() || !a.ownsRoot(object) {
		return Atom{}, false
	}
	id := a.objectEnd + uint64(selector-1)*uint64(len(a.roots)) + uint64(object.index)
	return Atom{owner: a, id: id}, id < a.methodEnd
}

func (a *Authority) Iterator(selector Selector, object Root, cursor Cursor) (Atom, bool) {
	ordinal, ok := iteratorSelectorOrdinal(selector)
	if !ok || !a.ownsRoot(object) || cursor.owner != a || cursor.index >= a.cursorEnd {
		return Atom{}, false
	}
	id := a.methodEnd + (ordinal*uint64(len(a.roots))+uint64(object.index))*uint64(a.cursorEnd) + uint64(cursor.index)
	return Atom{owner: a, id: id}, id < a.atomEnd
}

func (a *Authority) AtomKind(atom Atom) (AtomKind, bool) {
	if !a.ownsAtom(atom) {
		return 0, false
	}
	switch {
	case atom.id < a.objectEnd:
		return AtomObject, true
	case atom.id < a.methodEnd:
		return AtomMethod, true
	default:
		return AtomIterator, true
	}
}

func (a *Authority) ObjectDescriptor(atom Atom) (Descriptor, bool) {
	kind, ok := a.AtomKind(atom)
	if !ok || kind != AtomObject {
		return Descriptor{}, false
	}
	return Descriptor{owner: a, index: uint32(atom.id)}, true
}

func (a *Authority) MethodCapture(atom Atom) (Selector, Root, bool) {
	kind, ok := a.AtomKind(atom)
	if !ok || kind != AtomMethod || len(a.roots) == 0 {
		return 0, Root{}, false
	}
	local := atom.id - a.objectEnd
	selector := Selector(local/uint64(len(a.roots))) + 1
	root := Root{owner: a, index: uint32(local % uint64(len(a.roots)))}
	return selector, root, selector.Valid() && a.ownsRoot(root)
}

func (a *Authority) IteratorCapture(atom Atom) (Selector, Root, Cursor, bool) {
	kind, ok := a.AtomKind(atom)
	if !ok || kind != AtomIterator || len(a.roots) == 0 || a.cursorEnd == 0 {
		return 0, Root{}, Cursor{}, false
	}
	local := atom.id - a.methodEnd
	cursor := Cursor{owner: a, index: uint32(local % uint64(a.cursorEnd))}
	local /= uint64(a.cursorEnd)
	root := Root{owner: a, index: uint32(local % uint64(len(a.roots)))}
	ordinal := local / uint64(len(a.roots))
	selectors := [...]Selector{SelectorFields, SelectorVariants, SelectorParams, SelectorTparams}
	if ordinal >= uint64(len(selectors)) {
		return 0, Root{}, Cursor{}, false
	}
	return selectors[ordinal], root, cursor, a.ownsRoot(root)
}

func (a *Authority) ownsAtom(atom Atom) bool {
	return a != nil && atom.owner == a && atom.id < a.atomEnd
}

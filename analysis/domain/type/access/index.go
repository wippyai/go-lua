package access

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

type indexMode uint8

const (
	indexStatic indexMode = iota
	indexRuntime
	indexWrite
)

// Index resolves a bracket-index projection against a type using static type
// facts only.
func Index(container typ.Type, key typ.Type) (typ.Type, bool) {
	if value, ok, handled := directIndexFast(container, key, indexStatic); handled {
		return value, ok
	}
	return newQuery().resolveIndex(container, key, indexStatic).materialize()
}

// RuntimeIndex resolves a bracket-index projection with Lua read semantics.
func RuntimeIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	if value, ok, handled := directIndexFast(container, key, indexRuntime); handled {
		return value, ok
	}
	return newQuery().resolveIndex(container, key, indexRuntime).materialize()
}

// WritableIndex resolves the value contract for a bracket-index assignment.
func WritableIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	if value, ok, handled := directIndexFast(container, key, indexWrite); handled {
		return value, ok
	}
	return newQuery().resolveIndex(container, key, indexWrite).materialize()
}

func directIndexFast(container typ.Type, key typ.Type, mode indexMode) (typ.Type, bool, bool) {
	array, ok := container.(*typ.Array)
	if !ok || key != typ.Integer {
		return nil, false, false
	}
	element := array.Element
	if element == nil {
		element = typ.Unknown
	}
	if mode == indexWrite {
		return element, true, true
	}
	return typeexpr.Optional(element), true, true
}

package typ

import (
	"sync/atomic"
	"unsafe"
)

type typeProperties struct {
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsGeneric       bool
	containsRecursive     bool
	containsOpenRecursive bool

	// The construction-time bits above are conservative for a node that
	// reaches a recursive placeholder: the placeholder can receive its body
	// after a product containing it has been built. The resolved answer for
	// such a node is the derived column, published here once the graph closes.
	// Points to an immutable typeColumns record. unsafe.Pointer is used
	// instead of embedding atomic.Pointer because typeProperties is copied by
	// value while product nodes are constructed; published records themselves
	// are never mutated.
	columnsMemo unsafe.Pointer

	// runtimeKinds carries the published may-runtime-kind column. Zero means
	// unpublished; a published value sets runtimeKindsPublished so the empty
	// projection of never is a distinguishable answer.
	runtimeKinds uint32

	// predicates carries the published monotone Boolean predicate column: one
	// value bit and one published bit per predicateKind.
	predicates uint32
}

func (p *typeProperties) loadColumns() *typeColumns {
	if p == nil {
		return nil
	}
	return (*typeColumns)(atomic.LoadPointer(&p.columnsMemo))
}

func (p *typeProperties) storeColumns(columns *typeColumns) {
	if p != nil {
		atomic.StorePointer(&p.columnsMemo, unsafe.Pointer(columns))
	}
}

// copyStatic returns only construction-time properties. It deliberately does
// not copy the asynchronously published memo slot when a canonical product is
// ownership-cloned.
func (p *typeProperties) copyStatic() typeProperties {
	if p == nil {
		return typeProperties{}
	}
	return typeProperties{
		containsAny:           p.containsAny,
		containsNever:         p.containsNever,
		containsTypeParam:     p.containsTypeParam,
		containsInstantiated:  p.containsInstantiated,
		containsGeneric:       p.containsGeneric,
		containsRecursive:     p.containsRecursive,
		containsOpenRecursive: p.containsOpenRecursive,
	}
}

func (p *typeProperties) invalidateColumns() {
	if p == nil {
		return
	}
	p.storeColumns(nil)
	atomic.StoreUint32(&p.runtimeKinds, 0)
}

func typePropertiesOf(types ...Type) typeProperties {
	var props typeProperties
	for _, t := range types {
		props.include(t)
	}
	return props
}

func typePropertiesOfFields(fields []Field) typeProperties {
	var props typeProperties
	props.includeFields(fields)
	return props
}

func typePropertiesOfMethods(methods []Method) typeProperties {
	var props typeProperties
	props.includeMethods(methods)
	return props
}

func typePropertiesOfUnionMembers(types []Type) typeProperties {
	var props typeProperties
	props.includeUnionMembers(types)
	return props
}

func typePropertiesOfTypeParams(params []*TypeParam) typeProperties {
	var props typeProperties
	props.includeTypeParams(params)
	return props
}

func (p *typeProperties) include(t Type) {
	p.includeWithOpenRecursive(t, mayContainOpenRecursive)
}

func (p *typeProperties) includeWithOpenRecursive(t Type, openRecursive func(Type) bool) {
	if !p.containsAny && knownContainsAny(t) {
		p.containsAny = true
	}
	if !p.containsNever && knownContainsNever(t) {
		p.containsNever = true
	}
	if !p.containsTypeParam && knownContainsTypeParam(t) {
		p.containsTypeParam = true
	}
	if !p.containsInstantiated && knownContainsInstantiated(t) {
		p.containsInstantiated = true
	}
	if !p.containsGeneric && knownContainsGeneric(t) {
		p.containsGeneric = true
	}
	if !p.containsRecursive && knownContainsRecursive(t) {
		p.containsRecursive = true
	}
	if !p.containsOpenRecursive && openRecursive != nil && openRecursive(t) {
		p.containsOpenRecursive = true
	}
}

func (p *typeProperties) includeTypes(types ...Type) {
	for _, t := range types {
		p.include(t)
	}
}

func (p *typeProperties) includeFields(fields []Field) {
	for _, f := range fields {
		p.include(f.Type)
	}
}

func (p *typeProperties) includeStaticMembers(members []StaticMember) {
	for _, m := range members {
		p.include(m.Type)
	}
}

func (p *typeProperties) includeMethods(methods []Method) {
	for _, m := range methods {
		p.include(m.Type)
	}
}

func (p *typeProperties) includeUnionMembers(types []Type) {
	for _, t := range types {
		p.includeUnionMember(t)
	}
}

func (p *typeProperties) includeUnionMember(t Type) {
	p.includeWithOpenRecursive(t, unionMemberContainsOpenRecursive)
}

func (p *typeProperties) includeParams(params []Param) {
	for _, param := range params {
		p.include(param.Type)
	}
}

func (p *typeProperties) includeTypeParams(params []*TypeParam) {
	for _, param := range params {
		if param != nil {
			p.include(param)
		}
	}
}

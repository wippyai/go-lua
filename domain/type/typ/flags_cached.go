package typ

// cachedContainsFlags returns the cached containment flags for a composite type
// node: whether it transitively contains any, never, a type parameter, or an
// instantiated generic. Non-composite types return all false. An *Instantiated
// node trivially contains an instantiated type, and a *TypeParam node trivially
// contains a type parameter.
func cachedContainsFlags(t Type) (containsAny, containsNever, containsTypeParam, containsInstantiated bool) {
	switch n := t.(type) {
	case *Recursive:
		columns := columnsOf(n)
		return columns.containsAny, columns.containsNever, columns.containsFreeFormal, columns.containsInstantiated
	case *Instantiated:
		// A Generic's declaration-owned formals are bound by an
		// Instantiated node.  They are not free formals of the application;
		// only an argument that itself remains open keeps the application open.
		// containsAny/containsNever are read live rather than from the
		// construction-time snapshot: a self application can be built before
		// its own Generic.Body exists, and the Generic's own flags self-heal in
		// place on SetBody, but a snapshot copied out of it earlier would not.
		return instantiatedContainsAny(n), instantiatedContainsNever(n), instantiatedContainsTypeParam(n), true
	case *TypeParam:
		return n.containsAny, n.containsNever, true, n.containsInstantiated
	}
	properties := nodeProperties(t)
	if properties == nil {
		return false, false, false, false
	}
	return properties.containsAny, properties.containsNever, properties.containsTypeParam, properties.containsInstantiated
}

func knownContainsAny(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if IsAny(t) {
		return true
	}
	containsAny, _, _, _ := cachedContainsFlags(t)
	return containsAny
}

func knownContainsNever(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if IsNever(t) {
		return true
	}
	_, containsNever, _, _ := cachedContainsFlags(t)
	return containsNever
}

func knownContainsTypeParam(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if instantiated, ok := t.(*Instantiated); ok {
		return instantiatedContainsTypeParam(instantiated)
	}
	_, _, containsTypeParam, _ := cachedContainsFlags(t)
	return containsTypeParam
}

// instantiatedContainsTypeParam reports free formal containment for one
// application. The application's Generic is a binder: its declaration
// formals are substituted by TypeArgs and therefore do not, by themselves,
// make a concrete application open. ValidateStaticGenericRecurrence remains
// the owner-side law for malformed foreign formals in the generic body.
func instantiatedContainsTypeParam(value *Instantiated) bool {
	if value == nil {
		return false
	}
	for _, argument := range value.TypeArgs {
		if ContainsTypeParam(argument) {
			return true
		}
	}
	return false
}

// instantiatedContainsAny and instantiatedContainsNever read the Generic's
// current flags rather than the copy captured at construction. Generic
// mutates its own flags in place when SetBody completes a forward-declared
// body, so a live read is always current at O(1) cost; a copy taken before
// SetBody would freeze the pre-completion answer forever.
func instantiatedContainsAny(value *Instantiated) bool {
	if value == nil {
		return false
	}
	if knownContainsAny(value.Generic) {
		return true
	}
	for _, argument := range value.TypeArgs {
		if knownContainsAny(argument) {
			return true
		}
	}
	return false
}

func instantiatedContainsNever(value *Instantiated) bool {
	if value == nil {
		return false
	}
	if knownContainsNever(value.Generic) {
		return true
	}
	for _, argument := range value.TypeArgs {
		if knownContainsNever(argument) {
			return true
		}
	}
	return false
}

func knownContainsInstantiated(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	_, _, _, containsInstantiated := cachedContainsFlags(t)
	return containsInstantiated
}

// ContainsAny reports whether t is, or transitively contains, the gradual any
// type. A cached positive is definitive. A cached negative is definitive only
// for a product that reaches no recursive placeholder; a placeholder can
// receive a later body, so those graphs read the derived column instead.
func ContainsAny(t Type) bool {
	if knownContainsAny(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return columnsOf(t).containsAny
}

// ContainsNever reports whether t is, or transitively contains, never.
func ContainsNever(t Type) bool {
	if knownContainsNever(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return columnsOf(t).containsNever
}

// ContainsTypeParam reports whether t is, or transitively contains, a type
// parameter that no enclosing generic application binds. See typeColumns for
// the binder rule.
func ContainsTypeParam(t Type) bool {
	if knownContainsTypeParam(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return columnsOf(t).containsFreeFormal
}

// ContainsInstantiated reports whether t is, or transitively contains, a
// generic instantiation. A cached positive is definitive. A cached negative is
// definitive only for a product that reaches no recursive placeholder.
func ContainsInstantiated(t Type) bool {
	if knownContainsInstantiated(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return columnsOf(t).containsInstantiated
}

// ContainsGeneric reports whether t is, or transitively contains, a generic
// declaration or instantiation node.
func ContainsGeneric(t Type) bool {
	if knownContainsGeneric(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return columnsOf(t).containsGeneric
}

// knownContainsGeneric reports whether t transitively contains a *Generic node,
// mirroring the transitive predicate computed by containsGenericNode. A *Generic
// node trivially satisfies it, and an *Instantiated node always wraps a *Generic
// and so satisfies it too. The result is read from a construction-time cached
// flag rather than walking the type on every call.
func knownContainsGeneric(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	return cachedContainsGeneric(t)
}

func cachedContainsGeneric(t Type) bool {
	switch n := t.(type) {
	case *Recursive:
		return columnsOf(n).containsGeneric
	case *Generic:
		return true
	case *Instantiated:
		return true
	}
	properties := nodeProperties(t)
	return properties != nil && properties.containsGeneric
}

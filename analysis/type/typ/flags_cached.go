package typ

func knownContainsAny(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if IsAny(t) {
		return true
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsAny
	case *Optional:
		return n.containsAny
	case *Union:
		return n.containsAny
	case *Intersection:
		return n.containsAny
	case *Array:
		return n.containsAny
	case *Map:
		return n.containsAny
	case *ReadonlyMap:
		return n.containsAny
	case *Tuple:
		return n.containsAny
	case *Function:
		return n.containsAny
	case *Record:
		return n.containsAny
	case *Alias:
		return n.containsAny
	case *Meta:
		return n.containsAny
	case *Generic:
		return n.containsAny
	case *Instantiated:
		return n.containsAny
	case *TypeParam:
		return n.containsAny
	case *FieldAccess:
		return n.containsAny
	case *IndexAccess:
		return n.containsAny
	case *Sum:
		return n.containsAny
	case *Interface:
		return n.containsAny
	default:
		return false
	}
}

func knownContainsNever(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	if IsNever(t) {
		return true
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsNever
	case *Optional:
		return n.containsNever
	case *Union:
		return n.containsNever
	case *Intersection:
		return n.containsNever
	case *Array:
		return n.containsNever
	case *Map:
		return n.containsNever
	case *ReadonlyMap:
		return n.containsNever
	case *Tuple:
		return n.containsNever
	case *Function:
		return n.containsNever
	case *Record:
		return n.containsNever
	case *Alias:
		return n.containsNever
	case *Meta:
		return n.containsNever
	case *Generic:
		return n.containsNever
	case *Instantiated:
		return n.containsNever
	case *TypeParam:
		return n.containsNever
	case *FieldAccess:
		return n.containsNever
	case *IndexAccess:
		return n.containsNever
	case *Sum:
		return n.containsNever
	case *Interface:
		return n.containsNever
	default:
		return false
	}
}

func knownContainsTypeParam(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *TypeParam:
		return true
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsTypeParam
	case *Optional:
		return n.containsTypeParam
	case *Union:
		return n.containsTypeParam
	case *Intersection:
		return n.containsTypeParam
	case *Array:
		return n.containsTypeParam
	case *Map:
		return n.containsTypeParam
	case *ReadonlyMap:
		return n.containsTypeParam
	case *Tuple:
		return n.containsTypeParam
	case *Function:
		return n.containsTypeParam
	case *Record:
		return n.containsTypeParam
	case *Alias:
		return n.containsTypeParam
	case *Meta:
		return n.containsTypeParam
	case *Generic:
		return n.containsTypeParam
	case *Instantiated:
		return n.containsTypeParam
	case *FieldAccess:
		return n.containsTypeParam
	case *IndexAccess:
		return n.containsTypeParam
	case *Sum:
		return n.containsTypeParam
	case *Interface:
		return n.containsTypeParam
	default:
		return false
	}
}

func knownContainsInstantiated(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Instantiated:
		return true
	case *Recursive:
		n.ensureContainsFlags()
		return n.containsInstantiated
	case *Optional:
		return n.containsInstantiated
	case *Union:
		return n.containsInstantiated
	case *Intersection:
		return n.containsInstantiated
	case *Array:
		return n.containsInstantiated
	case *Map:
		return n.containsInstantiated
	case *ReadonlyMap:
		return n.containsInstantiated
	case *Tuple:
		return n.containsInstantiated
	case *Function:
		return n.containsInstantiated
	case *Record:
		return n.containsInstantiated
	case *Alias:
		return n.containsInstantiated
	case *Meta:
		return n.containsInstantiated
	case *Generic:
		return n.containsInstantiated
	case *TypeParam:
		return n.containsInstantiated
	case *FieldAccess:
		return n.containsInstantiated
	case *IndexAccess:
		return n.containsInstantiated
	case *Sum:
		return n.containsInstantiated
	case *Interface:
		return n.containsInstantiated
	default:
		return false
	}
}

func knownContainsRecursive(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		return true
	case *Optional:
		return n.containsRecursive
	case *Union:
		return n.containsRecursive
	case *Intersection:
		return n.containsRecursive
	case *Array:
		return n.containsRecursive
	case *Map:
		return n.containsRecursive
	case *ReadonlyMap:
		return n.containsRecursive
	case *Tuple:
		return n.containsRecursive
	case *Function:
		return n.containsRecursive
	case *Record:
		return n.containsRecursive
	case *Alias:
		return n.containsRecursive
	case *Meta:
		return n.containsRecursive
	case *Generic:
		return n.containsRecursive
	case *Instantiated:
		return n.containsRecursive
	case *TypeParam:
		return n.containsRecursive
	case *FieldAccess:
		return n.containsRecursive
	case *IndexAccess:
		return n.containsRecursive
	case *Sum:
		return n.containsRecursive
	case *Interface:
		return n.containsRecursive
	default:
		return false
	}
}

func knownContainsOpenRecursive(t Type) bool {
	if t == nil {
		return false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsClosedFlag()
		return !n.containsFlagsClosed
	case *Optional:
		return n.containsOpenRecursive
	case *Union:
		return n.containsOpenRecursive
	case *Intersection:
		return n.containsOpenRecursive
	case *Array:
		return n.containsOpenRecursive
	case *Map:
		return n.containsOpenRecursive
	case *ReadonlyMap:
		return n.containsOpenRecursive
	case *Tuple:
		return n.containsOpenRecursive
	case *Function:
		return n.containsOpenRecursive
	case *Record:
		return n.containsOpenRecursive
	case *Alias:
		return n.containsOpenRecursive
	case *Meta:
		return n.containsOpenRecursive
	case *Generic:
		return n.containsOpenRecursive
	case *Instantiated:
		return n.containsOpenRecursive
	case *TypeParam:
		return n.containsOpenRecursive
	case *FieldAccess:
		return n.containsOpenRecursive
	case *IndexAccess:
		return n.containsOpenRecursive
	case *Sum:
		return n.containsOpenRecursive
	case *Interface:
		return n.containsOpenRecursive
	default:
		return false
	}
}

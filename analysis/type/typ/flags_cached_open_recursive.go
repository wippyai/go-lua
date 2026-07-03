package typ

func knownContainsOpenRecursive(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	switch n := t.(type) {
	case *Recursive:
		n.ensureContainsClosedFlag()
		return !n.containsFlagsClosed
	case *Optional:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Union:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Intersection:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Array:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Map:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *ReadonlyMap:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Tuple:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Function:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Record:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Alias:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Meta:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Generic:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Instantiated:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *TypeParam:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	case *Interface:
		return currentContainsOpenRecursive(n, n.containsOpenRecursive)
	default:
		return false
	}
}

func currentContainsOpenRecursive(t Type, cached bool) bool {
	if !cached && !knownContainsRecursive(t) {
		return false
	}
	var scan openRecursiveScan
	return scan.contains(t)
}

type openRecursiveScan struct {
	small    [64]recursiveTraversalMemoKey
	smallLen int
	entries  map[recursiveTraversalMemoKey]struct{}
}

func (s *openRecursiveScan) contains(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if rec, ok := t.(*Recursive); ok {
		rec.ensureContainsClosedFlag()
		return !rec.containsFlagsClosed
	}
	if !knownContainsRecursive(t) {
		return false
	}
	if !s.enter(t) {
		return false
	}
	return WalkChildren(t, func(child Type) bool {
		return s.contains(child)
	})
}

func (s *openRecursiveScan) enter(t Type) bool {
	if s == nil || t == nil {
		return true
	}
	key, ok := recursiveTraversalMemo(t)
	if !ok {
		return true
	}
	for i := 0; i < s.smallLen; i++ {
		if s.small[i] == key {
			return false
		}
	}
	if _, ok := s.entries[key]; ok {
		return false
	}
	if s.entries == nil && s.smallLen < len(s.small) {
		s.small[s.smallLen] = key
		s.smallLen++
		return true
	}
	if s.entries == nil {
		s.entries = make(map[recursiveTraversalMemoKey]struct{}, len(s.small)+1)
		for i := 0; i < s.smallLen; i++ {
			s.entries[s.small[i]] = struct{}{}
			s.small[i] = recursiveTraversalMemoKey{}
		}
		s.smallLen = 0
	}
	s.entries[key] = struct{}{}
	return true
}

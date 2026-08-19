package typ

type recursiveContainsMemo struct {
	containsAny          bool
	containsNever        bool
	containsTypeParam    bool
	containsInstantiated bool
	containsGeneric      bool
}

type recursiveClosedMemo struct {
	closed bool
}

func (r *Recursive) containsFlags() recursiveContainsMemo {
	if r == nil {
		return recursiveContainsMemo{}
	}
	if cached := r.containsMemo.Load(); cached != nil {
		return *cached
	}
	memo, complete := recursiveContainsScan(r)
	// A scan over an incomplete graph (an unresolved nested placeholder) is
	// never published: SetBody may still change the answer. Once every
	// reachable placeholder has a body the fact is permanent, because Body is
	// write-once.
	if complete {
		r.containsMemo.Store(&memo)
	}
	return memo
}

// recursiveContainsScan derives all recursive content flags for one graph
// walk, and reports whether every reachable recursive placeholder already
// has a body.
func recursiveContainsScan(r *Recursive) (recursiveContainsMemo, bool) {
	var memo recursiveContainsMemo
	if r == nil || r.Body == nil {
		return memo, false
	}

	seen := map[Type]bool{r: true}
	seenTypeParam := map[Type]bool{r: true}
	work := []recursiveContainsWork{{typ: r.Body, typeParam: true}}
	complete := true
	var flags [containmentGeneric]bool

	for len(work) != 0 {
		last := len(work) - 1
		item := work[last]
		work = work[:last]
		current := unwrapAnnotated(item.typ)
		if current == nil {
			continue
		}
		visited := seen
		if item.typeParam {
			visited = seenTypeParam
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		if recursive, ok := current.(*Recursive); ok {
			if recursive.Body == nil {
				complete = false
				continue
			}
			work = append(work, recursiveContainsWork{typ: recursive.Body, typeParam: item.typeParam})
			continue
		}

		graph := knownContainsRecursive(current)
		for flag := containmentAny; flag <= containmentGeneric; flag++ {
			if flag == containmentTypeParam && !item.typeParam {
				continue
			}
			if flag.direct(current) || (!graph && flag.known(current)) {
				flags[flag-1] = true
			}
		}
		if !graph {
			continue
		}
		if instantiated, ok := current.(*Instantiated); ok {
			work = append(work, recursiveContainsWork{typ: instantiated.Generic})
			for _, argument := range instantiated.TypeArgs {
				work = append(work, recursiveContainsWork{typ: argument, typeParam: item.typeParam})
			}
			continue
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, recursiveContainsWork{typ: child, typeParam: item.typeParam})
			return false
		})
	}

	memo.containsAny = flags[containmentAny-1]
	memo.containsNever = flags[containmentNever-1]
	memo.containsTypeParam = flags[containmentTypeParam-1]
	memo.containsInstantiated = flags[containmentInstantiated-1]
	memo.containsGeneric = flags[containmentGeneric-1]
	return memo, complete
}

type recursiveContainsWork struct {
	typ       Type
	typeParam bool
}

func (r *Recursive) containsClosedFlag() bool {
	if r == nil {
		return false
	}
	if cached := r.closedMemo.Load(); cached != nil {
		return cached.closed
	}
	if r.Body == nil {
		return false
	}
	// An unclosed graph is not cached: a still-open nested placeholder can
	// still receive a body. A closed graph is permanent under write-once and
	// is cached unconditionally.
	closed := recursiveGraphClosureForRecursive(r)
	if closed {
		r.closedMemo.Store(&recursiveClosedMemo{closed: true})
	}
	return closed
}

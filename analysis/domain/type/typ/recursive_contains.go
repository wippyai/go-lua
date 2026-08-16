package typ

type recursiveContainsMemo struct {
	rev                  uint64
	deps                 []recursiveHashDep
	containsAny          bool
	containsNever        bool
	containsTypeParam    bool
	containsInstantiated bool
	containsGeneric      bool
}

type recursiveClosedMemo struct {
	rev    uint64
	closed bool
	deps   []recursiveHashDep
}

func (r *Recursive) containsFlags() recursiveContainsMemo {
	if r == nil {
		return recursiveContainsMemo{}
	}
	rev := r.rev
	if cached := r.containsMemo.Load(); cached != nil && cached.rev == rev && recursiveHashDepsValid(cached.deps) {
		return *cached
	}
	memo, complete := recursiveContainsScan(r, rev)
	// SetBody is a construction operation and must not race with queries. The
	// revision check still prevents a sequential rewrite from publishing an
	// obsolete computation.
	if complete && r.rev == rev {
		r.containsMemo.Store(&memo)
	}
	return memo
}

// recursiveContainsScan derives all recursive content flags and the revision
// fence for one graph walk.
func recursiveContainsScan(r *Recursive, rev uint64) (recursiveContainsMemo, bool) {
	memo := recursiveContainsMemo{rev: rev}
	if r == nil || r.Body == nil {
		return memo, false
	}

	seen := map[Type]bool{r: true}
	seenTypeParam := map[Type]bool{r: true}
	seenRecursive := map[*Recursive]bool{r: true}
	deps := []recursiveHashDep{{rec: r, rev: r.rev}}
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
			if !seenRecursive[recursive] {
				seenRecursive[recursive] = true
				deps = append(deps, recursiveHashDep{rec: recursive, rev: recursive.rev})
			}
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
	if complete {
		memo.deps = deps
	}
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
	rev := r.rev
	if cached := r.closedMemo.Load(); cached != nil && cached.rev == rev && recursiveHashDepsValid(cached.deps) {
		return cached.closed
	}
	if r.Body == nil {
		memo := &recursiveClosedMemo{rev: rev}
		r.closedMemo.Store(memo)
		return false
	}
	closed, deps := recursiveGraphClosureForRecursive(r)
	memo := &recursiveClosedMemo{rev: rev, closed: closed, deps: deps}
	if r.rev == rev {
		r.closedMemo.Store(memo)
	}
	return closed
}

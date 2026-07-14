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
	memo := &recursiveContainsMemo{rev: rev}
	if r.Body == nil {
		return *memo
	}
	seen := map[Type]bool{r: true}
	memo.containsAny = containsDynamicFlag(r.Body, seen, 1, -1, containmentAny)
	seen = map[Type]bool{r: true}
	memo.containsNever = containsNeverDynamic(r.Body, seen)
	seen = map[Type]bool{r: true}
	memo.containsTypeParam = containsDynamicFlag(r.Body, seen, 1, -1, containmentTypeParam)
	seen = map[Type]bool{r: true}
	memo.containsInstantiated = containsDynamicFlag(r.Body, seen, 1, -1, containmentInstantiated)
	seen = map[Type]bool{r: true}
	memo.containsGeneric = containsDynamicFlag(r.Body, seen, 1, -1, containmentGeneric)
	// SetBody is a construction operation and must not race with queries. The
	// revision check still prevents a sequential rewrite from publishing an
	// obsolete computation.
	deps, complete := recursiveHashDeps(r)
	if complete && r.rev == rev {
		memo.deps = deps
		r.containsMemo.Store(memo)
	}
	return *memo
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

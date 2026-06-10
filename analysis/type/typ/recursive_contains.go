package typ

func (r *Recursive) ensureContainsFlags() {
	if r == nil || !r.containsFlagsDirty || r.containsFlagsComputing {
		return
	}
	r.refreshContainsFlags()
}

func (r *Recursive) ensureContainsClosedFlag() {
	if r == nil || !r.containsClosedDirty || r.containsClosedComputing {
		return
	}
	r.refreshContainsClosedFlag()
}

func (r *Recursive) refreshContainsFlags() {
	if r == nil || r.Body == nil {
		r.containsAny = false
		r.containsNever = false
		r.containsTypeParam = false
		r.containsInstantiated = false
		r.containsFlagsDirty = false
		return
	}
	r.containsFlagsComputing = true
	defer func() {
		r.containsFlagsComputing = false
		r.containsFlagsDirty = false
	}()
	seen := map[Type]bool{r: true}
	r.containsAny = containsAnyDynamic(r.Body, seen, 1)
	seen = map[Type]bool{r: true}
	r.containsNever = containsNeverDynamic(r.Body, seen)
	seen = map[Type]bool{r: true}
	r.containsTypeParam = containsTypeParamDynamic(r.Body, seen, 1)
	seen = map[Type]bool{r: true}
	r.containsInstantiated = containsInstantiatedDynamic(r.Body, seen, 1)
}

func (r *Recursive) refreshContainsClosedFlag() {
	if r == nil || r.Body == nil {
		r.containsFlagsClosed = false
		r.containsClosedDirty = false
		return
	}
	r.containsClosedComputing = true
	defer func() {
		r.containsClosedComputing = false
		r.containsClosedDirty = false
	}()
	r.containsFlagsClosed = recursiveContainsGraphClosed(r.Body, map[*Recursive]bool{r: true}, 1)
}

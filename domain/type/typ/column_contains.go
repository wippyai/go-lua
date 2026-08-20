package typ

// typeColumns is the derived containment-and-closure column every public
// containment query reads. One derivation answers all of them, so a graph is
// walked at most once instead of once per flag and once more for closure.
//
// Two regimes produce it, and they are the discipline the node hashes already
// use. A node whose reachable graph holds no recursive placeholder has an
// exact permanent answer the moment it is constructed: typeProperties folds it
// from the children's own columns and the query is a field read. A node that
// does reach a placeholder has no permanent answer until every reachable
// placeholder has a body, because SetBody may still introduce a marker; that
// column is derived on demand and published once the graph closes. Body is
// write-once, so a published column is permanent and needs no invalidation.
//
// containsFreeFormal is the one containment fact that is not plain
// reachability. An *Instantiated node is a binder: it substitutes its
// Generic's declaration formals with its type arguments, so a formal
// occurrence reached through that Generic is bound, and only the arguments
// contribute free formals. That is the single rule for formal containment;
// every consumer reads this column and none restates it.
type typeColumns struct {
	containsAny          bool
	containsNever        bool
	containsFreeFormal   bool
	containsInstantiated bool
	containsGeneric      bool
	// closed reports that every Recursive placeholder reachable from the node
	// already has a body.
	closed bool
}

// merge folds a subtree's own column into this one. bound drops the free
// formal bit: the subtree sits under a Generic whose declaration formals this
// application has already substituted.
func (c *typeColumns) merge(other typeColumns, bound bool) {
	c.containsAny = c.containsAny || other.containsAny
	c.containsNever = c.containsNever || other.containsNever
	c.containsInstantiated = c.containsInstantiated || other.containsInstantiated
	c.containsGeneric = c.containsGeneric || other.containsGeneric
	c.closed = c.closed && other.closed
	if !bound && other.containsFreeFormal {
		c.containsFreeFormal = true
	}
}

// includeIntrinsic records what a node satisfies by being the node it is,
// independently of anything it reaches.
func (c *typeColumns) includeIntrinsic(t Type, bound bool) {
	if IsAny(t) {
		c.containsAny = true
	}
	if IsNever(t) {
		c.containsNever = true
	}
	switch t.(type) {
	case *TypeParam:
		if !bound {
			c.containsFreeFormal = true
		}
	case *Instantiated:
		c.containsInstantiated = true
		c.containsGeneric = true
	case *Generic:
		c.containsGeneric = true
	}
}

// includeConstruction folds in the construction-time properties of a node that
// reaches no recursive placeholder. Those bits are exact and permanent, so the
// subtree below such a node is never re-walked.
func (c *typeColumns) includeConstruction(t Type, bound bool) {
	containsAny, containsNever, containsFreeFormal, containsInstantiated := cachedContainsFlags(t)
	c.merge(typeColumns{
		containsAny:          containsAny,
		containsNever:        containsNever,
		containsFreeFormal:   containsFreeFormal,
		containsInstantiated: containsInstantiated,
		containsGeneric:      cachedContainsGeneric(t),
		closed:               true,
	}, bound)
}

// IsGraphClosed proves that every reachable Recursive node has a body. A
// productive backedge to an already visited Recursive node is closed; a
// dangling placeholder anywhere in the finite graph is not. This is the
// owner-side admission predicate for callers that must not retain an open
// mutable graph as a closed value.
func IsGraphClosed(t Type) bool {
	return columnsOf(t).closed
}

// knownContainsOpenRecursive is the negation of graph closure: some reachable
// Recursive placeholder still lacks a body.
func knownContainsOpenRecursive(t Type) bool {
	return !columnsOf(t).closed
}

// columnsOf returns the containment-and-closure column of t.
func columnsOf(t Type) typeColumns {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return typeColumns{closed: true}
	}
	if recursive, ok := t.(*Recursive); ok {
		if recursive == nil {
			// A typed nil placeholder reaches no body, so nothing closes below it.
			return typeColumns{}
		}
	} else if !knownContainsRecursive(t) {
		// No reachable placeholder: the construction fold is the answer.
		columns := typeColumns{closed: true}
		columns.includeIntrinsic(t, false)
		columns.includeConstruction(t, false)
		return columns
	}
	if published := publishedColumns(t); published != nil {
		return *published
	}
	columns, publishable := deriveColumns(t)
	if publishable {
		publishColumns(t, columns)
	}
	return columns
}

type columnWork struct {
	typ Type
	// bound marks a node reached through the Generic of an *Instantiated,
	// whose declaration formals that application has already substituted.
	bound bool
}

// deriveColumns walks the finite graph reachable from root once and reports
// the column together with whether it may be published. Publication requires
// every reachable Recursive AND Generic declaration to have a body: either
// SetBody can still introduce a marker, and a Generic body can even introduce
// the first recursive placeholder, which would falsify closed as well.
func deriveColumns(root Type) (typeColumns, bool) {
	columns := typeColumns{closed: true}
	declarationsClosed := true
	// One visited set per binder scope: a node reached both free and bound
	// contributes different formal containment and must be visited in each.
	seen := [2]map[Type]bool{0: make(map[Type]bool)}
	work := []columnWork{{typ: root}}

	for len(work) != 0 {
		last := len(work) - 1
		item := work[last]
		work = work[:last]
		current := unwrapAnnotated(item.typ)
		if current == nil {
			continue
		}
		scope := 0
		if item.bound {
			scope = 1
		}
		if seen[scope] == nil {
			seen[scope] = make(map[Type]bool)
		}
		if seen[scope][current] {
			continue
		}
		seen[scope][current] = true

		if recursive, ok := current.(*Recursive); ok {
			if recursive == nil || recursive.Body == nil {
				columns.closed = false
				continue
			}
			if published := recursive.columnsMemo.Load(); published != nil {
				columns.merge(*published, item.bound)
				continue
			}
			work = append(work, columnWork{typ: recursive.Body, bound: item.bound})
			continue
		}

		// A node can intrinsically satisfy a query while also reaching a
		// recursive back-edge, so its own identity is recorded before any
		// decision about descending.
		columns.includeIntrinsic(current, item.bound)
		if declarationOpen(current) {
			declarationsClosed = false
		}

		if !knownContainsRecursive(current) {
			columns.includeConstruction(current, item.bound)
			continue
		}
		if published := publishedColumns(current); published != nil {
			columns.merge(*published, item.bound)
			continue
		}
		if instantiated, ok := current.(*Instantiated); ok {
			work = append(work, columnWork{typ: instantiated.Generic, bound: true})
			for _, argument := range instantiated.TypeArgs {
				work = append(work, columnWork{typ: argument, bound: item.bound})
			}
			continue
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, columnWork{typ: child, bound: item.bound})
			return false
		})
	}

	return columns, columns.closed && declarationsClosed
}

// declarationOpen reports a generic declaration still awaiting SetBody. Such a
// declaration heals its own construction-time properties in place when the
// body arrives, which can change what the graph containing it reaches, so a
// column derived over one is never published.
func declarationOpen(t Type) bool {
	switch node := t.(type) {
	case *Generic:
		return node == nil || node.Body == nil
	case *Instantiated:
		return node == nil || node.Generic == nil || node.Generic.Body == nil
	default:
		return false
	}
}

func publishedColumns(t Type) *typeColumns {
	if recursive, ok := t.(*Recursive); ok {
		return recursive.columnsMemo.Load()
	}
	return columnProperties(t).loadColumns()
}

func publishColumns(t Type, columns typeColumns) {
	if recursive, ok := t.(*Recursive); ok {
		recursive.columnsMemo.Store(&columns)
		return
	}
	columnProperties(t).storeColumns(&columns)
}

// columnProperties returns the atomic immutable-memo slot each product node
// carries. The node structure itself is immutable once built; a memo record is
// published whole and never mutated afterwards.
func columnProperties(t Type) *typeProperties {
	switch n := t.(type) {
	case *Optional:
		return &n.typeProperties
	case *Union:
		return &n.typeProperties
	case *Intersection:
		return &n.typeProperties
	case *Array:
		return &n.typeProperties
	case *Map:
		return &n.typeProperties
	case *ReadonlyMap:
		return &n.typeProperties
	case *Tuple:
		return &n.typeProperties
	case *Function:
		return &n.typeProperties
	case *Record:
		return &n.typeProperties
	case *Alias:
		return &n.typeProperties
	case *Meta:
		return &n.typeProperties
	case *Generic:
		return &n.typeProperties
	case *Instantiated:
		return &n.typeProperties
	case *TypeParam:
		return &n.typeProperties
	case *Interface:
		return &n.typeProperties
	default:
		return nil
	}
}

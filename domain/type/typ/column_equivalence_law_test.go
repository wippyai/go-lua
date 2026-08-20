package typ

import (
	"fmt"
	"testing"
)

// This file is the stage-2 equivalence gate. It carries verbatim copies of the
// pre-column walkers (recursiveContainsScan, containsDynamicFlag,
// recursiveGraphClosedWalk) and asserts the derived columns answer identically
// over a generated corpus. It is deleted with the walkers it certifies.

type gateFlag uint8

const (
	gateAny gateFlag = iota + 1
	gateNever
	gateTypeParam
	gateInstantiated
	gateGeneric
)

func (f gateFlag) direct(t Type) bool {
	switch f {
	case gateAny:
		return IsAny(t)
	case gateNever:
		return IsNever(t)
	case gateTypeParam:
		_, ok := t.(*TypeParam)
		return ok
	case gateInstantiated:
		_, ok := t.(*Instantiated)
		return ok
	case gateGeneric:
		switch t.(type) {
		case *Generic, *Instantiated:
			return true
		}
	}
	return false
}

func (f gateFlag) known(t Type) bool {
	switch f {
	case gateAny:
		return gateKnownContainsAny(t)
	case gateNever:
		return gateKnownContainsNever(t)
	case gateTypeParam:
		return gateKnownContainsTypeParam(t)
	case gateInstantiated:
		return gateKnownContainsInstantiated(t)
	case gateGeneric:
		return gateKnownContainsGeneric(t)
	default:
		return false
	}
}

type gateMemo struct {
	any, never, typeParam, instantiated, generic bool
}

// gateRecursiveScan is recursive_contains.go's recursiveContainsScan.
func gateRecursiveScan(r *Recursive) (gateMemo, bool) {
	var memo gateMemo
	if r == nil || r.Body == nil {
		return memo, false
	}
	seen := map[Type]bool{r: true}
	seenTypeParam := map[Type]bool{r: true}
	type work struct {
		typ       Type
		typeParam bool
	}
	stack := []work{{typ: r.Body, typeParam: true}}
	complete := true
	var flags [gateGeneric]bool

	for len(stack) != 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
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
			stack = append(stack, work{typ: recursive.Body, typeParam: item.typeParam})
			continue
		}

		graph := knownContainsRecursive(current)
		for flag := gateAny; flag <= gateGeneric; flag++ {
			if flag == gateTypeParam && !item.typeParam {
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
			stack = append(stack, work{typ: instantiated.Generic})
			for _, argument := range instantiated.TypeArgs {
				stack = append(stack, work{typ: argument, typeParam: item.typeParam})
			}
			continue
		}
		WalkChildren(current, func(child Type) bool {
			stack = append(stack, work{typ: child, typeParam: item.typeParam})
			return false
		})
	}

	memo.any = flags[gateAny-1]
	memo.never = flags[gateNever-1]
	memo.typeParam = flags[gateTypeParam-1]
	memo.instantiated = flags[gateInstantiated-1]
	memo.generic = flags[gateGeneric-1]
	return memo, complete
}

// gateCachedContainsFlags is flags_cached.go's cachedContainsFlags with the
// *Recursive case routed through the pre-column scan.
func gateCachedContainsFlags(t Type) (bool, bool, bool, bool) {
	switch n := t.(type) {
	case *Recursive:
		memo, _ := gateRecursiveScan(n)
		return memo.any, memo.never, memo.typeParam, memo.instantiated
	case *Instantiated:
		return gateInstantiatedAny(n), gateInstantiatedNever(n), gateInstantiatedTypeParam(n), true
	case *TypeParam:
		return n.containsAny, n.containsNever, true, n.containsInstantiated
	case *Optional:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Union:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Intersection:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Array:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Map:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *ReadonlyMap:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Tuple:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Function:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Record:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Alias:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Meta:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Generic:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	case *Interface:
		return n.containsAny, n.containsNever, n.containsTypeParam, n.containsInstantiated
	default:
		return false, false, false, false
	}
}

func gateCachedContainsGeneric(t Type) bool {
	switch n := t.(type) {
	case *Recursive:
		memo, _ := gateRecursiveScan(n)
		return memo.generic
	case *Generic, *Instantiated:
		return true
	case *TypeParam:
		return n.containsGeneric
	case *Optional:
		return n.containsGeneric
	case *Union:
		return n.containsGeneric
	case *Intersection:
		return n.containsGeneric
	case *Array:
		return n.containsGeneric
	case *Map:
		return n.containsGeneric
	case *ReadonlyMap:
		return n.containsGeneric
	case *Tuple:
		return n.containsGeneric
	case *Function:
		return n.containsGeneric
	case *Record:
		return n.containsGeneric
	case *Alias:
		return n.containsGeneric
	case *Meta:
		return n.containsGeneric
	case *Interface:
		return n.containsGeneric
	default:
		return false
	}
}

func gateInstantiatedTypeParam(value *Instantiated) bool {
	if value == nil {
		return false
	}
	for _, argument := range value.TypeArgs {
		if gateContainsTypeParam(argument) {
			return true
		}
	}
	return false
}

func gateInstantiatedAny(value *Instantiated) bool {
	if value == nil {
		return false
	}
	if gateKnownContainsAny(value.Generic) {
		return true
	}
	for _, argument := range value.TypeArgs {
		if gateKnownContainsAny(argument) {
			return true
		}
	}
	return false
}

func gateInstantiatedNever(value *Instantiated) bool {
	if value == nil {
		return false
	}
	if gateKnownContainsNever(value.Generic) {
		return true
	}
	for _, argument := range value.TypeArgs {
		if gateKnownContainsNever(argument) {
			return true
		}
	}
	return false
}

func gateKnownContainsAny(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if IsAny(t) {
		return true
	}
	a, _, _, _ := gateCachedContainsFlags(t)
	return a
}

func gateKnownContainsNever(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if IsNever(t) {
		return true
	}
	_, n, _, _ := gateCachedContainsFlags(t)
	return n
}

func gateKnownContainsTypeParam(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if instantiated, ok := t.(*Instantiated); ok {
		return gateInstantiatedTypeParam(instantiated)
	}
	_, _, p, _ := gateCachedContainsFlags(t)
	return p
}

func gateKnownContainsInstantiated(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	_, _, _, i := gateCachedContainsFlags(t)
	return i
}

func gateKnownContainsGeneric(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	return gateCachedContainsGeneric(t)
}

// gateDynamic is flags_dynamic.go's containsDynamicFlag.
func gateDynamic(t Type, flag gateFlag) bool {
	if t == nil || flag == 0 {
		return false
	}
	seen := make(map[Type]bool)
	work := []Type{t}
	for len(work) != 0 {
		last := len(work) - 1
		current := unwrapAnnotated(work[last])
		work = work[:last]
		if current == nil {
			continue
		}
		if recursive, ok := current.(*Recursive); ok {
			if seen[current] {
				continue
			}
			seen[current] = true
			work = append(work, recursive.Body)
			continue
		}
		if flag.direct(current) {
			return true
		}
		if flag == gateTypeParam {
			if instantiated, ok := current.(*Instantiated); ok {
				if seen[current] {
					continue
				}
				seen[current] = true
				work = append(work, instantiated.TypeArgs...)
				continue
			}
		}
		if !knownContainsRecursive(current) {
			if flag.known(current) {
				return true
			}
			continue
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
	return false
}

func gateContainsAny(t Type) bool {
	if gateKnownContainsAny(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return gateDynamic(t, gateAny)
}

func gateContainsNever(t Type) bool {
	if gateKnownContainsNever(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return gateDynamic(t, gateNever)
}

func gateContainsTypeParam(t Type) bool {
	if gateKnownContainsTypeParam(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return gateDynamic(t, gateTypeParam)
}

func gateContainsInstantiated(t Type) bool {
	if gateKnownContainsInstantiated(t) {
		return true
	}
	if !knownContainsRecursive(t) {
		return false
	}
	return gateDynamic(t, gateInstantiated)
}

func gateContainsGeneric(t Type) bool {
	return gateKnownContainsGeneric(t)
}

// gateGraphClosed is recursive_graph_closed.go's recursiveGraphClosedWalk.
func gateGraphClosed(root Type) bool {
	type memoKey struct {
		kind uint16
		ptr  uintptr
	}
	key := func(t Type) (memoKey, bool) {
		if t == nil {
			return memoKey{}, false
		}
		ptr := typePointer(t)
		if ptr == 0 {
			ptr = uintptr(t.Kind())
		}
		return memoKey{kind: uint16(t.Kind()), ptr: ptr}, true
	}
	seen := make(map[*Recursive]bool)
	memo := make(map[memoKey]bool)
	work := []Type{root}
	active := make(map[memoKey]bool)
	visited := make([]memoKey, 0, 8)
	for len(work) != 0 {
		last := len(work) - 1
		current := unwrapAnnotatedOrNil(work[last])
		work = work[:last]
		if current == nil {
			continue
		}
		k, keyed := key(current)
		if keyed {
			if closed, found := memo[k]; found {
				if !closed {
					return false
				}
				continue
			}
			if active[k] {
				continue
			}
			active[k] = true
			visited = append(visited, k)
		}
		if recursive, ok := current.(*Recursive); ok {
			if recursive.Body == nil {
				seen[recursive] = true
				if keyed {
					memo[k] = false
				}
				return false
			}
			if seen[recursive] {
				continue
			}
			seen[recursive] = true
			work = append(work, recursive.Body)
			continue
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
	}
	for _, k := range visited {
		memo[k] = true
	}
	return true
}

// gateCorpus produces the graph shapes the package's own constructors build,
// each as a factory so the reference and the column each see an untouched
// instance. Every reachable subterm of every shape is compared, so the corpus
// covers far more nodes than it names.
func gateCorpus() []struct {
	name  string
	build func() []Type
} {
	atom := func() []Type {
		param := NewTypeParam("T", nil)
		box := NewGeneric("Box", []*TypeParam{param}, newRecord().Field("value", param).Build())
		return []Type{
			Any, Never, Nil, String, Number, Integer, Boolean, Unknown,
			NewTypeParam("Free", nil),
			NewTypeParam("Bounded", Any),
			box,
			Instantiate(box, String),
			Instantiate(box, Any),
			Instantiate(box, Never),
			Instantiate(box, NewTypeParam("Outer", nil)),
			NewGeneric("Empty", nil, nil),
		}
	}

	wrap := func(inner Type) []Type {
		other := NewTypeParam("W", nil)
		return []Type{
			MaterializeOptional(inner),
			NewArray(inner),
			NewMap(String, inner),
			NewMap(inner, String),
			NewReadonlyMap(String, inner),
			NewTuple(inner, String),
			newRecord().Field("f", inner).Build(),
			RebuildRecord(RecordParts{Fields: []Field{{Name: "f", Type: String}}, MapKey: String, MapValue: inner}),
			Func().Param("p", inner).Returns(String).Build(),
			Func().Param("p", String).Returns(inner).Build(),
			Func().Variadic(inner).Returns(String).Build(),
			NewMeta(inner),
			NewInterface("I", []Method{{Name: "m", Type: Func().Returns(inner).Build()}}),
			NewGeneric("G", []*TypeParam{other}, inner),
			NewAlias("A", inner),
		}
	}

	var cases []struct {
		name  string
		build func() []Type
	}
	add := func(name string, build func() []Type) {
		cases = append(cases, struct {
			name  string
			build func() []Type
		}{name, build})
	}

	add("atoms", atom)
	add("wrapped atoms", func() []Type {
		var out []Type
		for _, a := range atom() {
			out = append(out, wrap(a)...)
		}
		return out
	})
	add("twice wrapped atoms", func() []Type {
		var out []Type
		for _, a := range atom() {
			for _, w := range wrap(a) {
				out = append(out, MaterializeOptional(w), NewArray(w), newRecord().Field("g", w).Build())
			}
		}
		return out
	})

	for _, marker := range []struct {
		name string
		make func() Type
	}{
		{"any", func() Type { return Any }},
		{"never", func() Type { return Never }},
		{"formal", func() Type { return NewTypeParam("M", nil) }},
		{"application", func() Type {
			p := NewTypeParam("T", nil)
			return Instantiate(NewGeneric("Box", []*TypeParam{p}, newRecord().Field("v", p).Build()), String)
		}},
		{"declaration", func() Type {
			p := NewTypeParam("T", nil)
			return NewGeneric("Box", []*TypeParam{p}, newRecord().Field("v", p).Build())
		}},
		{"plain", func() Type { return String }},
	} {
		name, make := marker.name, marker.make

		add("closed self recursion carrying "+name, func() []Type {
			node := NewRecursivePlaceholder("Node")
			node.SetBody(newRecord().Field("marker", make()).OptField("next", node).Build())
			return []Type{node, NewArray(node), MaterializeOptional(node)}
		})
		add("open placeholder beside "+name, func() []Type {
			child := NewRecursivePlaceholder("Child")
			return []Type{child, newRecord().Field("child", child).Field("marker", make()).Build(), NewArray(child)}
		})
		add("late sealed child carrying "+name, func() []Type {
			child := NewRecursivePlaceholder("Child")
			parent := NewRecursive("Parent", func(Type) Type {
				return newRecord().Field("child", child).Build()
			})
			wrapper := NewArray(parent)
			child.SetBody(newRecord().Field("marker", make()).Build())
			return []Type{parent, wrapper, child}
		})
		add("mutual recursion carrying "+name, func() []Type {
			left := NewRecursivePlaceholder("Left")
			right := NewRecursivePlaceholder("Right")
			left.SetBody(newRecord().Field("right", right).Build())
			right.SetBody(newRecord().Field("marker", make()).Field("left", left).Build())
			return []Type{left, right, NewTuple(left, right)}
		})
		add("recursive union and intersection with "+name, func() []Type {
			node := NewRecursivePlaceholder("Node")
			node.SetBody(MaterializeUnion([]Type{String, MaterializeOptional(node), make()}))
			other := NewRecursivePlaceholder("Other")
			other.SetBody(MaterializeIntersection([]Type{newRecord().Field("a", make()).Build(), newRecord().OptField("b", other).Build()}))
			return []Type{node, other}
		})
		add("recursion under a generic body carrying "+name, func() []Type {
			node := NewRecursivePlaceholder("Node")
			param := NewTypeParam("T", nil)
			generic := NewGeneric("Box", []*TypeParam{param}, nil)
			generic.SetBody(newRecord().Field("next", MaterializeOptional(node)).Field("value", param).Field("marker", make()).Build())
			node.SetBody(newRecord().Field("box", Instantiate(generic, String)).Build())
			return []Type{node, generic, Instantiate(generic, make()), Instantiate(generic, param)}
		})
		add("self application carrying "+name, func() []Type {
			param := NewTypeParam("T", nil)
			list := NewGeneric("List", []*TypeParam{param}, nil)
			list.SetBody(newRecord().Field("head", param).Field("marker", make()).OptField("tail", Instantiate(list, param)).Build())
			return []Type{list, Instantiate(list, String), Instantiate(list, param)}
		})
		add("deeply nested "+name+" behind recursion", func() []Type {
			node := NewRecursivePlaceholder("Deep")
			node.SetBody(newRecord().Field("marker", nestInFunctions(make(), 40)).OptField("next", node).Build())
			return []Type{node, NewArray(node)}
		})
	}

	return cases
}

func gateReachable(root Type) []Type {
	seen := map[Type]bool{}
	var out []Type
	var work []Type
	if root != nil {
		work = append(work, root)
	}
	for len(work) != 0 {
		last := len(work) - 1
		current := work[last]
		work = work[:last]
		if current == nil || seen[current] {
			continue
		}
		seen[current] = true
		out = append(out, current)
		if recursive, ok := current.(*Recursive); ok {
			if recursive.Body != nil {
				work = append(work, recursive.Body)
			}
			continue
		}
		WalkChildren(current, func(child Type) bool {
			work = append(work, child)
			return false
		})
		if instantiated, ok := current.(*Instantiated); ok && instantiated.Generic != nil {
			work = append(work, instantiated.Generic)
		}
	}
	return out
}

// TestColumnsAnswerExactlyAsThePreColumnWalkers is the deletion gate for
// recursiveContainsScan, containsDynamicFlag and recursiveGraphClosedWalk.
func TestColumnsAnswerExactlyAsThePreColumnWalkers(t *testing.T) {
	queries := []struct {
		name      string
		reference func(Type) bool
		column    func(Type) bool
		// staleNegative marks the one query whose pre-column form had no
		// recursive fallback at all: ContainsGeneric read only the
		// construction-time bit, which is stale-false for a product built
		// around a recursive placeholder that later received a generic-bearing
		// body. The column gives that query the same fallback as its four
		// siblings, so it is allowed to answer true where the reference
		// answered false - and the corpus must actually witness it.
		staleNegative bool
	}{
		{name: "ContainsAny", reference: gateContainsAny, column: ContainsAny},
		{name: "ContainsNever", reference: gateContainsNever, column: ContainsNever},
		{name: "ContainsTypeParam", reference: gateContainsTypeParam, column: ContainsTypeParam},
		{name: "ContainsInstantiated", reference: gateContainsInstantiated, column: ContainsInstantiated},
		{name: "ContainsGeneric", reference: gateContainsGeneric, column: ContainsGeneric, staleNegative: true},
		{name: "IsGraphClosed", reference: gateGraphClosed, column: IsGraphClosed},
	}

	compared, witnessed := 0, 0
	for _, shape := range gateCorpus() {
		for _, query := range queries {
			for rootIndex, root := range shape.build() {
				for nodeIndex, node := range gateReachable(root) {
					want := query.reference(node)
					got := query.column(node)
					compared++
					if got == want {
						continue
					}
					if query.staleNegative && got && !want {
						witnessed++
						continue
					}
					t.Fatalf("%s: %s on %s root %d node %d (%s) = %v, reference = %v",
						shape.name, query.name, node.Kind(), rootIndex, nodeIndex,
						truncate(node.String()), got, want)
				}
			}
		}
	}
	if compared < 5000 {
		t.Fatalf("equivalence gate compared only %d answers; the corpus is too small to certify a deletion", compared)
	}
	if witnessed == 0 {
		t.Fatal("no stale-negative ContainsGeneric answer was witnessed; the exemption is unproven and must be removed")
	}
	t.Logf("equivalence gate compared %d answers, %d stale-negative reference answers repaired", compared, witnessed)
}

// TestContainsGenericReadsTheColumnLikeItsSiblings states the one answer the
// column changes. Before it, ContainsGeneric was the only containment query
// with no fallback for a graph that reaches a recursive placeholder: it read
// the construction-time bit alone, which a product built around a placeholder
// records as false and never revisits when the placeholder's body later brings
// a generic in. The column gives the query the same derivation as ContainsAny,
// ContainsNever, ContainsTypeParam and ContainsInstantiated, so it sees the
// sealed body.
func TestContainsGenericReadsTheColumnLikeItsSiblings(t *testing.T) {
	param := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{param}, newRecord().Field("value", param).Build())

	child := NewRecursivePlaceholder("Child")
	root := newRecord().Field("child", child).Build()
	if ContainsGeneric(root) {
		t.Fatal("no generic is reachable before the placeholder has a body")
	}

	child.SetBody(newRecord().Field("boxed", Instantiate(box, String)).Build())
	if !ContainsGeneric(root) {
		t.Fatal("a generic reachable only through the sealed recursive body was not reported")
	}
	if !ContainsInstantiated(root) {
		t.Fatal("the sibling application query disagrees about the same sealed body")
	}
	// The declaration's formal is bound by the application, so the same sealed
	// body carries no free formal: one column, one binder rule.
	if ContainsTypeParam(root) {
		t.Fatal("a formal the application substitutes was reported as free")
	}
}

// gateUncachedPredicate runs the predicate fixpoint machine without reading or
// writing the published column, so the column can be compared against the
// equations it caches rather than against itself.
func gateUncachedPredicate(t Type, which predicateKind) bool {
	t = NormalizeNil(t)
	if value, known := predicateLeaf(t, which); known {
		return value
	}
	work := predicateWork{kind: which, nodes: make(map[Type]*predicateNode)}
	root := work.intern(t)
	work.expandAll()
	work.propagate()
	return root.value
}

// TestPredicateColumnAnswersAsTheFixpointItCaches states that publishing the
// monotone Boolean predicates as a column changes no answer, cold or warm.
func TestPredicateColumnAnswersAsTheFixpointItCaches(t *testing.T) {
	predicates := []struct {
		name   string
		which  predicateKind
		column func(Type) bool
	}{
		{"AdmitsFalse", predicateAdmitsFalse, AdmitsFalse},
		{"IsBooleanType", predicateBoolean, IsBooleanType},
		{"IsIntegerIndexType", predicateIntegerIndex, IsIntegerIndexType},
	}

	compared := 0
	for _, shape := range gateCorpus() {
		for _, predicate := range predicates {
			for rootIndex, root := range shape.build() {
				for nodeIndex, node := range gateReachable(root) {
					want := gateUncachedPredicate(node, predicate.which)
					for pass := 0; pass < 2; pass++ {
						got := predicate.column(node)
						compared++
						if got != want {
							t.Fatalf("%s: %s pass %d on %s root %d node %d (%s) = %v, fixpoint = %v",
								shape.name, predicate.name, pass, node.Kind(), rootIndex, nodeIndex,
								truncate(node.String()), got, want)
						}
					}
					if again := gateUncachedPredicate(node, predicate.which); again != want {
						t.Fatalf("%s: %s fixpoint disagreed with itself after the column published",
							shape.name, predicate.name)
					}
				}
			}
		}
	}
	if compared < 5000 {
		t.Fatalf("predicate column gate compared only %d answers", compared)
	}
	t.Logf("predicate column gate compared %d answers", compared)
}

func truncate(s string) string {
	if len(s) <= 90 {
		return s
	}
	return fmt.Sprintf("%s...", s[:90])
}

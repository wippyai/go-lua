package typ

import (
	"fmt"
	"sync/atomic"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// recursiveIDCounter generates unique IDs for recursive types.
var recursiveIDCounter uint64

// Recursive represents a self-referential (mu) type.
// Recursive types are identified by a unique ID to allow cycle detection
// during equality comparison and hashing without infinite recursion.
//
// Example: type Node = { next: Node? } is represented as:
//
//	Recursive{ID: 1, Name: "Node", Body: Record{Fields: [{name: "next", type: <self-ref>}]}}
type Recursive struct {
	ID   uint64
	Name string
	Body Type
	hash uint64
	rev  uint64

	// keyed marks a family interned by a RecursiveFamilyInterner: its identity is
	// familyKey, so Equal and Hash use the key, not the body. Body refinement
	// mutates the slot in place under the stable identity.
	keyed     bool
	familyKey FamilyKey

	// owner is the compilation-scoped interner that minted a keyed family. Only
	// the owning interner may widen the family body; a node with no owner (a
	// declared type, an instantiated stdlib product) is immutable to the widen path.
	owner *RecursiveFamilyInterner

	// frozen marks an immutable input graph (stdlib/manifest/DB/cache). SetBody on
	// a frozen node is a no-op so a shared recursive body cannot be mutated by a
	// later compilation that reaches it.
	frozen bool

	containsAny             bool
	containsNever           bool
	containsTypeParam       bool
	containsInstantiated    bool
	containsFlagsClosed     bool
	containsFlagsDirty      bool
	containsClosedDirty     bool
	containsFlagsComputing  bool
	containsClosedComputing bool
	hashDeps                []recursiveHashDep
}

type recursiveHashDep struct {
	rec *Recursive
	rev uint64
}

// RecursiveBuilder is used during construction to provide a self-reference.
type RecursiveBuilder func(self Type) Type

// NewRecursive creates a new recursive type.
// The builder function receives a placeholder that represents self-references
// and should return the body type using that placeholder where needed.
func NewRecursive(name string, builder RecursiveBuilder) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)

	rec := &Recursive{
		ID:   id,
		Name: name,
	}

	rec.SetBody(builder(rec))
	return rec
}

// NewRecursiveWithBody creates a recursive type with a pre-built body.
// Use this when the body is already constructed with proper self-references.
func NewRecursiveWithBody(name string, body Type) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)

	rec := &Recursive{
		ID:   id,
		Name: name,
	}
	rec.SetBody(body)
	return rec
}

// NewRecursivePlaceholder creates an empty recursive type for deferred body assignment.
// Use SetBody to assign the body after creation. This is useful for mutual recursion.
func NewRecursivePlaceholder(name string) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)
	return &Recursive{
		ID:                  id,
		Name:                name,
		containsFlagsDirty:  true,
		containsClosedDirty: true,
	}
}

// SetBody assigns the body to a placeholder recursive type.
//
// A frozen node is an immutable input graph (stdlib/manifest/DB/cache); SetBody on
// it is a no-op so a shared recursive body cannot be mutated by a compilation that
// reaches it. The freeze guard is what makes stdlib immutability explicit.
func (r *Recursive) SetBody(body Type) {
	if r.frozen {
		return
	}
	r.Body = body
	r.hash = 0
	r.rev++
	r.hashDeps = nil
	r.containsFlagsDirty = true
	r.containsClosedDirty = true
}

// Freeze marks the recursive node and its reachable recursive descendants as
// immutable input. After freezing, SetBody is a no-op, so a shared stdlib,
// manifest, DB, or cache type graph cannot be mutated by any compilation that
// references it.
func (r *Recursive) Freeze() {
	FreezeType(r)
}

// FreezeType walks t and freezes every reachable recursive node, marking the whole
// type graph as immutable input. Stdlib types are frozen once at library init so
// any recursive body reached through them is immutable to every compilation.
func FreezeType(t Type) {
	if t == nil {
		return
	}
	// Contains is the canonical cycle-safe structural scanner; a predicate that
	// freezes each recursive node it visits and never short-circuits walks the
	// whole graph exactly once.
	Contains(t, func(n Type) bool {
		if rec, ok := n.(*Recursive); ok && rec != nil {
			rec.frozen = true
		}
		return false
	})
}

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

func recursiveContainsGraphClosed(t Type, seen map[*Recursive]bool, depth int) bool {
	return recursiveContainsGraphClosedMemo(t, seen, make(map[graphClosedKey]bool), depth)
}

type graphClosedKey struct {
	kind kind.Kind
	ptr  uintptr
}

func recursiveContainsGraphClosedMemo(t Type, seen map[*Recursive]bool, memo map[graphClosedKey]bool, depth int) bool {
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return true
	}
	if key, ok := graphClosedMemoKey(t); ok {
		if closed, found := memo[key]; found {
			return closed
		}
	}

	result := true
	switch n := t.(type) {
	case nil:
		result = true
	case *Recursive:
		if n.Body == nil {
			result = false
			break
		}
		if seen[n] {
			result = true
			break
		}
		seen[n] = true
		result = recursiveContainsGraphClosedMemo(n.Body, seen, memo, depth+1)
	case *Alias:
		result = recursiveContainsGraphClosedMemo(n.Target, seen, memo, depth+1)
	case *Optional:
		result = recursiveContainsGraphClosedMemo(n.Inner, seen, memo, depth+1)
	case *Union:
		for _, member := range n.Members {
			if !recursiveContainsGraphClosedMemo(member, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *Intersection:
		for _, member := range n.Members {
			if !recursiveContainsGraphClosedMemo(member, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *Array:
		result = recursiveContainsGraphClosedMemo(n.Element, seen, memo, depth+1)
	case *Map:
		result = recursiveContainsGraphClosedMemo(n.Key, seen, memo, depth+1) &&
			recursiveContainsGraphClosedMemo(n.Value, seen, memo, depth+1)
	case *Tuple:
		for _, elem := range n.Elements {
			if !recursiveContainsGraphClosedMemo(elem, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *Function:
		for _, param := range n.Params {
			if !recursiveContainsGraphClosedMemo(param.Type, seen, memo, depth+1) {
				result = false
				break
			}
		}
		if result {
			for _, ret := range n.Returns {
				if !recursiveContainsGraphClosedMemo(ret, seen, memo, depth+1) {
					result = false
					break
				}
			}
		}
		if result && n.Variadic != nil && !recursiveContainsGraphClosedMemo(n.Variadic, seen, memo, depth+1) {
			result = false
		}
	case *Record:
		for _, field := range n.Fields {
			if !recursiveContainsGraphClosedMemo(field.Type, seen, memo, depth+1) {
				result = false
				break
			}
		}
		if result && n.Metatable != nil && !recursiveContainsGraphClosedMemo(n.Metatable, seen, memo, depth+1) {
			result = false
		}
		if result && n.HasMapComponent() {
			result = recursiveContainsGraphClosedMemo(n.MapKey, seen, memo, depth+1) &&
				recursiveContainsGraphClosedMemo(n.MapValue, seen, memo, depth+1)
		}
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && !recursiveContainsGraphClosedMemo(param.Constraint, seen, memo, depth+1) {
				result = false
				break
			}
		}
		if result {
			result = recursiveContainsGraphClosedMemo(n.Body, seen, memo, depth+1)
		}
	case *Instantiated:
		if !recursiveContainsGraphClosedMemo(n.Generic, seen, memo, depth+1) {
			result = false
			break
		}
		for _, arg := range n.TypeArgs {
			if !recursiveContainsGraphClosedMemo(arg, seen, memo, depth+1) {
				result = false
				break
			}
		}
	case *TypeParam:
		result = recursiveContainsGraphClosedMemo(n.Constraint, seen, memo, depth+1)
	case *FieldAccess:
		result = recursiveContainsGraphClosedMemo(n.Base, seen, memo, depth+1)
	case *IndexAccess:
		result = recursiveContainsGraphClosedMemo(n.Base, seen, memo, depth+1) &&
			recursiveContainsGraphClosedMemo(n.Index, seen, memo, depth+1)
	case *Sum:
		for _, variant := range n.Variants {
			for _, t := range variant.Types {
				if !recursiveContainsGraphClosedMemo(t, seen, memo, depth+1) {
					result = false
					break
				}
			}
			if !result {
				break
			}
		}
	case *Interface:
		for _, method := range n.Methods {
			if method.Type != nil && !recursiveContainsGraphClosedMemo(method.Type, seen, memo, depth+1) {
				result = false
				break
			}
		}
	}
	if key, ok := graphClosedMemoKey(t); ok {
		memo[key] = result
	}
	return result
}

func graphClosedMemoKey(t Type) (graphClosedKey, bool) {
	if t == nil {
		return graphClosedKey{}, false
	}
	ptr := typePointer(t)
	if ptr == 0 {
		ptr = uintptr(t.Kind())
	}
	return graphClosedKey{kind: t.Kind(), ptr: ptr}, true
}

// hashWithVisited computes hash with cycle detection for recursive types.
// Uses structural traversal to ensure order-independent hashing for mutual recursion.
func hashWithVisited(t Type, visited map[*Recursive]bool) uint64 {
	return hashWithVisitedMemo(t, visited, make(map[Type]uint64))
}

func hashWithVisitedMemo(t Type, visited map[*Recursive]bool, memo map[Type]uint64) uint64 {
	if t == nil {
		return 0
	}

	// Check if this is a recursive type we've already seen
	if rec, ok := t.(*Recursive); ok {
		if rec.keyed {
			// Keyed identity: hash by owner key, stable across body refinement, so a
			// keyed family contributes a fixed hash to any product that embeds it.
			return internal.HashCombine(uint64(kind.Recursive), rec.familyKey.hash())
		}
		if visited[rec] {
			// Self-reference: use a sentinel hash value
			return internal.HashCombine(uint64(kind.Recursive), internal.FnvString("$self"))
		}
		if h, ok := memo[t]; ok {
			return h
		}
		visited[rec] = true
		defer delete(visited, rec)

		// Compute structurally rather than using pre-computed hash.
		// This ensures correct hashing during mutual recursion setup
		// when the other recursive type's hash may not be computed yet.
		h := internal.HashCombine(uint64(kind.Recursive), internal.FnvString(rec.Name))
		if rec.Body != nil {
			h = internal.HashCombine(h, hashBodyWithVisitedMemo(rec.Body, visited, memo))
		}
		memo[t] = h
		return h
	}

	if h, ok := memo[t]; ok {
		return h
	}
	h := hashBodyWithVisitedMemo(t, visited, memo)
	memo[t] = h
	return h
}

// hashBodyWithVisited hashes a type's structure with cycle detection.
// Handles compound types that may contain recursive references.
// Mirrors the real Hash() semantics of each type constructor for consistency.
func hashBodyWithVisited(t Type, visited map[*Recursive]bool) uint64 {
	return hashBodyWithVisitedMemo(t, visited, make(map[Type]uint64))
}

func hashBodyWithVisitedMemo(t Type, visited map[*Recursive]bool, memo map[Type]uint64) uint64 {
	t = normalizeNilType(t)
	if t == nil {
		return 0
	}
	t = unwrapTransparentWrappers(t)
	if alias, ok := t.(*Alias); ok {
		return hashBodyWithVisitedMemo(alias.UnaliasedTarget(), visited, memo)
	}

	// Check for recursive type reference
	if rec, ok := t.(*Recursive); ok {
		return hashWithVisitedMemo(rec, visited, memo)
	}

	if h, ok := memo[t]; ok {
		return h
	}

	// For compound types, traverse their components
	h := Visit(t, Visitor[uint64]{
		Optional: func(o *Optional) uint64 {
			return internal.HashCombine(uint64(kind.Optional), hashBodyWithVisitedMemo(o.Inner, visited, memo))
		},
		Union: func(u *Union) uint64 {
			h := uint64(kind.Union)
			for _, m := range u.Members {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(m, visited, memo))
			}
			return h
		},
		Intersection: func(in *Intersection) uint64 {
			h := uint64(kind.Intersection)
			for _, m := range in.Members {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(m, visited, memo))
			}
			return h
		},
		Record: func(r *Record) uint64 {
			h := uint64(kind.Record)
			for _, f := range r.Fields {
				h = internal.HashCombine(h, internal.FnvString(f.Name))
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(f.Type, visited, memo))
				if f.Optional {
					h = internal.HashCombine(h, 1)
				}
				if f.Readonly {
					h = internal.HashCombine(h, 2)
				}
			}
			if r.Metatable != nil {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(r.Metatable, visited, memo))
			}
			if r.Open {
				h = internal.HashCombine(h, 3)
			}
			if r.HasMapComponent() {
				h = internal.HashCombine(h, internal.FnvString("$mapKey"))
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(r.MapKey, visited, memo))
				h = internal.HashCombine(h, internal.FnvString("$mapValue"))
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(r.MapValue, visited, memo))
			}
			return h
		},
		Array: func(a *Array) uint64 {
			return internal.HashCombine(uint64(kind.Array), hashBodyWithVisitedMemo(a.Element, visited, memo))
		},
		Map: func(m *Map) uint64 {
			h := uint64(kind.Map)
			h = internal.HashCombine(h, hashBodyWithVisitedMemo(m.Key, visited, memo))
			h = internal.HashCombine(h, hashBodyWithVisitedMemo(m.Value, visited, memo))
			return h
		},
		Tuple: func(t *Tuple) uint64 {
			h := uint64(kind.Tuple)
			for _, e := range t.Elements {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(e, visited, memo))
			}
			return h
		},
		Function: func(fn *Function) uint64 {
			h := uint64(kind.Function)
			// Type parameters
			for _, tp := range fn.TypeParams {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(tp, visited, memo))
			}
			// Parameters with optional flags
			for _, p := range fn.Params {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(p.Type, visited, memo))
				if p.Optional {
					h = internal.HashCombine(h, 1)
				}
			}
			// Variadic
			if fn.Variadic != nil {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(fn.Variadic, visited, memo))
			}
			// Returns
			for _, r := range fn.Returns {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(r, visited, memo))
			}
			return h
		},
		Meta: func(m *Meta) uint64 {
			return internal.HashCombine(uint64(kind.Meta), hashBodyWithVisitedMemo(m.Of, visited, memo))
		},
		Generic: func(g *Generic) uint64 {
			h := internal.HashCombine(uint64(kind.Generic), internal.FnvString(g.Name))
			for _, p := range g.TypeParams {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(p, visited, memo))
			}
			if g.Name == "" && g.Body != nil {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(g.Body, visited, memo))
			}
			return h
		},
		Instantiated: func(in *Instantiated) uint64 {
			h := internal.HashCombine(uint64(kind.Instantiated), hashBodyWithVisitedMemo(in.Generic, visited, memo))
			for _, arg := range in.TypeArgs {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(arg, visited, memo))
			}
			return h
		},
		TypeParam: func(tp *TypeParam) uint64 {
			h := internal.HashCombine(uint64(kind.TypeParam), internal.FnvString(tp.Name))
			if tp.Constraint != nil {
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(tp.Constraint, visited, memo))
			}
			return h
		},
		FieldAccess: func(f *FieldAccess) uint64 {
			h := internal.HashCombine(uint64(kind.FieldAccess), hashBodyWithVisitedMemo(f.Base, visited, memo))
			return internal.HashCombine(h, internal.FnvString(f.Field))
		},
		IndexAccess: func(i *IndexAccess) uint64 {
			h := internal.HashCombine(uint64(kind.IndexAccess), hashBodyWithVisitedMemo(i.Base, visited, memo))
			return internal.HashCombine(h, hashBodyWithVisitedMemo(i.Index, visited, memo))
		},
		Sum: func(s *Sum) uint64 {
			h := internal.HashCombine(uint64(kind.Sum), internal.FnvString(s.Name))
			for _, variant := range s.Variants {
				h = internal.HashCombine(h, internal.FnvString(variant.Tag))
				for _, vt := range variant.Types {
					h = internal.HashCombine(h, hashBodyWithVisitedMemo(vt, visited, memo))
				}
			}
			return h
		},
		Interface: func(i *Interface) uint64 {
			h := internal.HashCombine(uint64(kind.Interface), internal.FnvString(i.Name))
			for _, method := range i.Methods {
				h = internal.HashCombine(h, internal.FnvString(method.Name))
				h = internal.HashCombine(h, hashBodyWithVisitedMemo(method.Type, visited, memo))
			}
			return h
		},
		Default: func(t Type) uint64 {
			return t.Hash()
		},
	})
	memo[t] = h
	return h
}

// EqualityHash returns the canonical hash used by structural equality and
// deduplication. It matches Hash for immutable closed products, but recomputes
// wrappers around open recursive placeholders so SetBody cannot leave stale
// construction-time hashes in the type algebra.
func EqualityHash(t Type) uint64 {
	return typeEqualityHash(t)
}

func typeEqualityHash(t Type) uint64 {
	t = unwrapAliasForEquals(t, NewGuard())
	if t == nil {
		return 0
	}
	if knownContainsOpenRecursive(t) {
		return hashBodyWithVisited(t, make(map[*Recursive]bool))
	}
	return t.Hash()
}

func (r *Recursive) Kind() kind.Kind { return kind.Recursive }

func (r *Recursive) String() string {
	return fmt.Sprintf("%s#%d", r.Name, r.ID)
}

func (r *Recursive) Hash() uint64 {
	if r.keyed {
		// Keyed identity: the hash is the family key, stable across every body
		// refinement so the inter-procedural fixpoint sees a fixed point on the
		// family while the body slot still widens.
		return internal.HashCombine(uint64(kind.Recursive), r.familyKey.hash())
	}
	if r.hash != 0 && recursiveHashDepsValid(r.hashDeps) {
		return r.hash
	}
	// Compute hash on demand with cycle detection. Recursive types are mutable
	// only until SetBody completes, then share the same cached-hash contract as
	// other type nodes.
	h := hashWithVisited(r, make(map[*Recursive]bool))
	if deps, ok := recursiveHashDeps(r); ok {
		r.hash = h
		r.hashDeps = deps
	}
	return h
}

func recursiveHashDepsValid(deps []recursiveHashDep) bool {
	for _, dep := range deps {
		if dep.rec == nil || dep.rec.rev != dep.rev {
			return false
		}
	}
	return true
}

func recursiveHashDeps(r *Recursive) ([]recursiveHashDep, bool) {
	if r == nil {
		return nil, true
	}
	seen := make(map[*Recursive]bool)
	if !collectRecursiveHashDepsMemo(r, seen, make(map[graphClosedKey]bool)) {
		return nil, false
	}
	deps := make([]recursiveHashDep, 0, len(seen))
	for rec := range seen {
		deps = append(deps, recursiveHashDep{rec: rec, rev: rec.rev})
	}
	return deps, true
}

func collectRecursiveHashDepsMemo(r *Recursive, seen map[*Recursive]bool, memo map[graphClosedKey]bool) bool {
	if r == nil {
		return true
	}
	if r.Body == nil {
		return false
	}
	if seen[r] {
		return true
	}
	seen[r] = true
	return collectRecursiveHashDepsInTypeMemo(r.Body, seen, memo)
}

func collectRecursiveHashDepsInTypeMemo(t Type, seen map[*Recursive]bool, memo map[graphClosedKey]bool) bool {
	if t == nil {
		return true
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return true
	}
	if key, ok := graphClosedMemoKey(t); ok {
		if closed, found := memo[key]; found {
			return closed
		}
	}

	result := true
	switch n := t.(type) {
	case nil:
		result = true
	case *Recursive:
		result = collectRecursiveHashDepsMemo(n, seen, memo)
	case *Alias:
		result = collectRecursiveHashDepsInTypeMemo(n.Target, seen, memo)
	case *Optional:
		result = collectRecursiveHashDepsInTypeMemo(n.Inner, seen, memo)
	case *Union:
		for _, member := range n.Members {
			if !collectRecursiveHashDepsInTypeMemo(member, seen, memo) {
				result = false
				break
			}
		}
	case *Intersection:
		for _, member := range n.Members {
			if !collectRecursiveHashDepsInTypeMemo(member, seen, memo) {
				result = false
				break
			}
		}
	case *Array:
		result = collectRecursiveHashDepsInTypeMemo(n.Element, seen, memo)
	case *Map:
		result = collectRecursiveHashDepsInTypeMemo(n.Key, seen, memo) &&
			collectRecursiveHashDepsInTypeMemo(n.Value, seen, memo)
	case *Tuple:
		for _, elem := range n.Elements {
			if !collectRecursiveHashDepsInTypeMemo(elem, seen, memo) {
				result = false
				break
			}
		}
	case *Function:
		for _, param := range n.Params {
			if !collectRecursiveHashDepsInTypeMemo(param.Type, seen, memo) {
				result = false
				break
			}
		}
		if result {
			for _, ret := range n.Returns {
				if !collectRecursiveHashDepsInTypeMemo(ret, seen, memo) {
					result = false
					break
				}
			}
		}
		if result && n.Variadic != nil && !collectRecursiveHashDepsInTypeMemo(n.Variadic, seen, memo) {
			result = false
		}
	case *Record:
		for _, field := range n.Fields {
			if !collectRecursiveHashDepsInTypeMemo(field.Type, seen, memo) {
				result = false
				break
			}
		}
		if result && n.Metatable != nil && !collectRecursiveHashDepsInTypeMemo(n.Metatable, seen, memo) {
			result = false
		}
		if result && n.HasMapComponent() {
			result = collectRecursiveHashDepsInTypeMemo(n.MapKey, seen, memo) &&
				collectRecursiveHashDepsInTypeMemo(n.MapValue, seen, memo)
		}
	case *Generic:
		for _, param := range n.TypeParams {
			if param != nil && !collectRecursiveHashDepsInTypeMemo(param.Constraint, seen, memo) {
				result = false
				break
			}
		}
		if result {
			result = collectRecursiveHashDepsInTypeMemo(n.Body, seen, memo)
		}
	case *Instantiated:
		if !collectRecursiveHashDepsInTypeMemo(n.Generic, seen, memo) {
			result = false
			break
		}
		for _, arg := range n.TypeArgs {
			if !collectRecursiveHashDepsInTypeMemo(arg, seen, memo) {
				result = false
				break
			}
		}
	case *TypeParam:
		result = collectRecursiveHashDepsInTypeMemo(n.Constraint, seen, memo)
	case *FieldAccess:
		result = collectRecursiveHashDepsInTypeMemo(n.Base, seen, memo)
	case *IndexAccess:
		result = collectRecursiveHashDepsInTypeMemo(n.Base, seen, memo) &&
			collectRecursiveHashDepsInTypeMemo(n.Index, seen, memo)
	case *Sum:
		for _, variant := range n.Variants {
			for _, t := range variant.Types {
				if !collectRecursiveHashDepsInTypeMemo(t, seen, memo) {
					result = false
					break
				}
			}
			if !result {
				break
			}
		}
	case *Interface:
		for _, method := range n.Methods {
			if method.Type != nil && !collectRecursiveHashDepsInTypeMemo(method.Type, seen, memo) {
				result = false
				break
			}
		}
	}
	if key, ok := graphClosedMemoKey(t); ok {
		memo[key] = result
	}
	return result
}

// Equals compares two recursive types by their structural identity.
// Two recursive types are equal if they have the same structure when
// the self-references are treated as equivalent.
func (r *Recursive) Equals(other Type) bool {
	return TypeEquals(r, other)
}

// IsRecursiveRef returns true if t is a reference to the given recursive type.
func IsRecursiveRef(t Type, rec *Recursive) bool {
	if t == rec {
		return true
	}
	if r, ok := t.(*Recursive); ok {
		return r.ID == rec.ID
	}
	return false
}

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

	// hash caches Hash once every recursive type reachable from Body has a
	// body; zero means not cached.
	hash atomic.Uint64
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

	rec.Body = builder(rec)
	return rec
}

// NewRecursiveWithBody creates a recursive type with a pre-built body.
// Use this when the body is already constructed with proper self-references.
func NewRecursiveWithBody(name string, body Type) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)

	return &Recursive{
		ID:   id,
		Name: name,
		Body: body,
	}
}

// NewRecursivePlaceholder creates an empty recursive type for deferred body assignment.
// Use SetBody to assign the body after creation. This is useful for mutual recursion.
func NewRecursivePlaceholder(name string) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)
	return &Recursive{
		ID:   id,
		Name: name,
	}
}

// SetBody assigns the body to a placeholder recursive type.
func (r *Recursive) SetBody(body Type) {
	r.Body = body
	r.hash.Store(0)
}

// recursiveHashState carries the recursive types on the current hashing path,
// whether a placeholder without a body was reached, and the hashes of subterms
// already computed in this pass.
//
// A reference to a recursive type on the path hashes to a sentinel, so a
// subterm's hash depends only on which of the recursive types it refers to are
// on the path. Each memoized subterm records those types, and its hash is
// reused while all of them are still on the path. This keeps hashing linear in
// the number of distinct nodes when a type shares substructure.
type recursiveHashState struct {
	visited    map[*Recursive]bool
	incomplete bool
	memo       map[Type]memoHash
	hits       []*Recursive
}

type memoHash struct {
	hash uint64
	refs []*Recursive
}

func newRecursiveHashState() *recursiveHashState {
	return &recursiveHashState{visited: make(map[*Recursive]bool), memo: make(map[Type]memoHash)}
}

// hashMemoized hashes t with compute, reusing a result whose references are on the path.
func (st *recursiveHashState) hashMemoized(t Type, compute func() uint64) uint64 {
	if m, ok := st.memo[t]; ok && st.onPath(m.refs) {
		st.hits = append(st.hits, m.refs...)
		return m.hash
	}
	mark := len(st.hits)
	h := compute()
	var refs []*Recursive
	seen := make(map[*Recursive]bool)
	for _, r := range st.hits[mark:] {
		if st.visited[r] && !seen[r] {
			seen[r] = true
			refs = append(refs, r)
		}
	}
	st.hits = append(st.hits[:mark], refs...)
	if !st.incomplete {
		st.memo[t] = memoHash{hash: h, refs: refs}
	}
	return h
}

func (st *recursiveHashState) onPath(refs []*Recursive) bool {
	for _, r := range refs {
		if !st.visited[r] {
			return false
		}
	}
	return true
}

// hashWithVisited computes hash with cycle detection for recursive types.
// Uses structural traversal to ensure order-independent hashing for mutual recursion.
func hashWithVisited(t Type, st *recursiveHashState) uint64 {
	if t == nil {
		return 0
	}

	// Check if this is a recursive type we've already seen
	if rec, ok := t.(*Recursive); ok {
		if st.visited[rec] {
			st.hits = append(st.hits, rec)
			// Self-reference: use a sentinel hash value
			return internal.HashCombine(uint64(kind.Recursive), internal.FnvString("$self"))
		}
		return st.hashMemoized(rec, func() uint64 {
			st.visited[rec] = true
			defer delete(st.visited, rec)

			// Compute structurally rather than using pre-computed hash.
			// This ensures correct hashing during mutual recursion setup
			// when the other recursive type's hash may not be computed yet.
			h := internal.HashCombine(uint64(kind.Recursive), internal.FnvString(rec.Name))
			if rec.Body != nil {
				h = internal.HashCombine(h, hashBodyWithVisited(rec.Body, st))
			} else {
				st.incomplete = true
			}
			return h
		})
	}

	// For non-recursive types, use their standard hash
	return t.Hash()
}

// hashBodyWithVisited hashes a type's structure with cycle detection.
// Handles compound types that may contain recursive references.
// Mirrors the real Hash() semantics of each type constructor for consistency.
func hashBodyWithVisited(t Type, st *recursiveHashState) uint64 {
	if t == nil {
		return 0
	}

	// Check for recursive type reference
	if rec, ok := t.(*Recursive); ok {
		return hashWithVisited(rec, st)
	}

	// For compound types, traverse their components
	return st.hashMemoized(t, func() uint64 { return hashCompound(t, st) })
}

func hashCompound(t Type, st *recursiveHashState) uint64 {
	return Visit(t, Visitor[uint64]{
		Optional: func(o *Optional) uint64 {
			return internal.HashCombine(uint64(kind.Optional), hashBodyWithVisited(o.Inner, st))
		},
		Union: func(u *Union) uint64 {
			h := uint64(kind.Union)
			for _, m := range u.Members {
				h = internal.HashCombine(h, hashBodyWithVisited(m, st))
			}
			return h
		},
		Intersection: func(in *Intersection) uint64 {
			h := uint64(kind.Intersection)
			for _, m := range in.Members {
				h = internal.HashCombine(h, hashBodyWithVisited(m, st))
			}
			return h
		},
		Record: func(r *Record) uint64 {
			h := uint64(kind.Record)
			for _, f := range r.Fields {
				h = internal.HashCombine(h, internal.FnvString(f.Name))
				h = internal.HashCombine(h, hashBodyWithVisited(f.Type, st))
				if f.Optional {
					h = internal.HashCombine(h, 1)
				}
				if f.Readonly {
					h = internal.HashCombine(h, 2)
				}
			}
			if r.Metatable != nil {
				h = internal.HashCombine(h, hashBodyWithVisited(r.Metatable, st))
			}
			if r.Open {
				h = internal.HashCombine(h, 3)
			}
			if r.HasMapComponent() {
				h = internal.HashCombine(h, internal.FnvString("$mapKey"))
				h = internal.HashCombine(h, hashBodyWithVisited(r.MapKey, st))
				h = internal.HashCombine(h, internal.FnvString("$mapValue"))
				h = internal.HashCombine(h, hashBodyWithVisited(r.MapValue, st))
			}
			return h
		},
		Array: func(a *Array) uint64 {
			return internal.HashCombine(uint64(kind.Array), hashBodyWithVisited(a.Element, st))
		},
		Map: func(m *Map) uint64 {
			h := uint64(kind.Map)
			h = internal.HashCombine(h, hashBodyWithVisited(m.Key, st))
			h = internal.HashCombine(h, hashBodyWithVisited(m.Value, st))
			return h
		},
		Tuple: func(t *Tuple) uint64 {
			h := uint64(kind.Tuple)
			for _, e := range t.Elements {
				h = internal.HashCombine(h, hashBodyWithVisited(e, st))
			}
			return h
		},
		Function: func(fn *Function) uint64 {
			h := uint64(kind.Function)
			// Type parameters
			for _, tp := range fn.TypeParams {
				h = internal.HashCombine(h, tp.Hash())
			}
			// Parameters with optional flags
			for _, p := range fn.Params {
				h = internal.HashCombine(h, hashBodyWithVisited(p.Type, st))
				if p.Optional {
					h = internal.HashCombine(h, 1)
				}
			}
			// Variadic
			if fn.Variadic != nil {
				h = internal.HashCombine(h, hashBodyWithVisited(fn.Variadic, st))
			}
			// Returns
			for _, r := range fn.Returns {
				h = internal.HashCombine(h, hashBodyWithVisited(r, st))
			}
			return h
		},
		Default: func(t Type) uint64 {
			return t.Hash()
		},
	})
}

func (r *Recursive) Kind() kind.Kind { return kind.Recursive }

func (r *Recursive) String() string {
	return fmt.Sprintf("%s#%d", r.Name, r.ID)
}

// Hash computes the structural hash with cycle detection, so mutually
// recursive types hash independently of construction order. The result is
// cached once no reachable placeholder is missing its body.
func (r *Recursive) Hash() uint64 {
	if h := r.hash.Load(); h != 0 {
		return h
	}
	st := newRecursiveHashState()
	h := hashWithVisited(r, st)
	if !st.incomplete {
		r.hash.Store(h)
	}
	return h
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

// FoldApproximations returns mu X. t[T' := X], where T' ranges over the types
// nested in t that isApprox accepts, or t itself when none is nested.
//
// Fixpoint inference approximates a self-embedding type as an ascending chain
// T_0 <: T_1 <: ..., where T_n nests approximations of T_(n-1). Replacing the
// nested approximations by the recursion variable yields an upper bound of the
// whole chain and its limit.
//
// Only guarded occurrences are replaced. The root, and the members of a root
// union or optional, are the unguarded top level of the body: replacing one of
// them would produce mu X. X | ..., which is not contractive.
func FoldApproximations(name string, t Type, isApprox func(Type) bool) Type {
	if t == nil {
		return t
	}
	self := NewRecursivePlaceholder(name)
	body := foldGuarded(t, self, isApprox)
	if body == t {
		return t
	}
	self.SetBody(body)
	return self
}

// foldGuarded rewrites the guarded positions of top: it descends through the
// unguarded union and optional structure and replaces approximations only
// below a type constructor.
func foldGuarded(top Type, self *Recursive, isApprox func(Type) bool) Type {
	switch tt := unwrapTransparentWrappers(top).(type) {
	case *Union:
		var members []Type
		for i, m := range tt.Members {
			folded := foldGuarded(m, self, isApprox)
			if folded != m && members == nil {
				members = make([]Type, len(tt.Members))
				copy(members, tt.Members[:i])
			}
			if members != nil {
				members[i] = folded
			}
		}
		if members == nil {
			return top
		}
		return NewUnion(members...)
	case *Optional:
		inner := foldGuarded(tt.Inner, self, isApprox)
		if inner == tt.Inner {
			return top
		}
		return NewOptional(inner)
	}
	return Rewrite(top, func(node Type) (Type, bool) {
		if node == top {
			return nil, false
		}
		if isApprox(node) {
			return self, true
		}
		return nil, false
	})
}

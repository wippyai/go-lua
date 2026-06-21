package typ

import (
	"fmt"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/type/kind"
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

	containsAny             bool
	containsNever           bool
	containsTypeParam       bool
	containsInstantiated    bool
	containsGeneric         bool
	containsFlagsClosed     bool
	containsFlagsDirty      bool
	containsClosedDirty     bool
	containsFlagsComputing  bool
	containsClosedComputing bool
	hashDeps                []recursiveHashDep
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
func (r *Recursive) SetBody(body Type) {
	r.Body = body
	r.hash = 0
	r.rev++
	r.hashDeps = nil
	r.containsFlagsDirty = true
	r.containsClosedDirty = true
}

func (r *Recursive) Kind() kind.Kind { return kind.Recursive }

func (r *Recursive) String() string {
	return fmt.Sprintf("%s#%d", r.Name, r.ID)
}

// Equals compares two recursive types by their structural identity.
// Two recursive types are equal if they have the same structure when
// the self-references are treated as equivalent.
func (r *Recursive) Equals(other Type) bool {
	return typeEquals(r, other)
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

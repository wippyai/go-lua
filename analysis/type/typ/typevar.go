package typ

import "github.com/wippyai/go-lua/analysis/type/kind"

// TypeVar represents an inference variable during type checking.
//
// Type variables are placeholders created during generic instantiation
// and constraint solving. They are unified with concrete types as the
// checker gathers information. ID distinguishes different variables.
type TypeVar struct {
	ID   int
	hash uint64
}

func (t *TypeVar) Kind() kind.Kind { return kind.TypeVar }
func (t *TypeVar) String() string  { return "$" + string(rune('a'+t.ID%26)) }
func (t *TypeVar) Hash() uint64    { return t.hash }
func (t *TypeVar) Equals(other Type) bool {
	if other.Kind() != kind.TypeVar {
		return false
	}

	return t.ID == other.(*TypeVar).ID
}

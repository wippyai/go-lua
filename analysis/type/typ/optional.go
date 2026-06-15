package typ

import (
	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Optional represents a nullable type: T | nil.
//
// Optional is syntactic sugar for a union with nil. It provides cleaner
// representation for the common case of a value that may be absent.
//
// The Inner field holds the non-nil type. An Optional never contains
// another Optional (they are flattened during construction).
type Optional struct {
	Inner                 Type
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// MaterializeOptional builds the hash-stable optional node for an already-selected inner type.
//
// It performs only low-level node materialization owned by typ: hash/cache/
// contains flag computation plus the canonical Optional node invariants for
// nil/Nil and already-Optional inputs. It is not semantic type-expression
// construction: Any is kept as the optional inner type, and Union is not
// interpreted as nil plus its members.
func MaterializeOptional(inner Type) Type {
	return newRawOptionalNode(inner)
}

// typ owns hash-stable node materialization; callers here already decided the optional shape.
func newRawOptionalNode(inner Type) Type {
	if inner == nil || inner.Kind() == kind.Nil {
		return Nil
	}

	if inner.Kind() == kind.Optional {
		return inner
	}

	h := hash.MixHash(uint64(kind.Optional), inner.Hash())

	return &Optional{
		Inner:                 inner,
		hash:                  h,
		containsAny:           knownContainsAny(inner),
		containsNever:         knownContainsNever(inner),
		containsTypeParam:     knownContainsTypeParam(inner),
		containsInstantiated:  knownContainsInstantiated(inner),
		containsRecursive:     knownContainsRecursive(inner),
		containsOpenRecursive: knownContainsOpenRecursive(inner),
	}
}

func (o *Optional) Kind() kind.Kind { return kind.Optional }

func (o *Optional) String() string {
	return o.strCache.get(func() string {
		if o.Inner == nil {
			return "nil?"
		}
		return o.Inner.String() + "?"
	})
}

func (o *Optional) Hash() uint64 { return o.hash }

func (o *Optional) Equals(other Type) bool {
	return typeEquals(o, other)
}

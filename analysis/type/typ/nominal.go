package typ

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Method represents a method in an interface type.
type Method struct {
	Name string    // Method name
	Type *Function // Method signature
}

// Interface represents a structural interface type (trait/protocol).
//
// Interfaces define a set of methods that a type must implement.
// Unlike nominal interfaces, subtyping is structural: any type with
// matching methods satisfies the interface.
//
// Named interfaces (Name != "") use nominal identity for marker interfaces
// (interfaces with no methods, like Channel<T>).
type Interface struct {
	Name                  string   // Interface name (empty for anonymous)
	Methods               []Method // Required methods
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsGeneric       bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// NewInterface creates an interface type.
func NewInterface(name string, methods []Method) *Interface {
	h := hash.MixHash(uint64(kind.Interface), hash.FnvString(name))
	containsAny := false
	containsNever := false
	containsTypeParam := false
	containsInstantiated := false
	containsGeneric := false
	containsRecursive := false
	containsOpenRecursive := false
	for _, m := range methods {
		h = hash.MixHash(h, hash.FnvString(m.Name))
		h = hash.MixHash(h, m.Type.Hash())
		if !containsAny && knownContainsAny(m.Type) {
			containsAny = true
		}
		if !containsNever && knownContainsNever(m.Type) {
			containsNever = true
		}
		if !containsTypeParam && knownContainsTypeParam(m.Type) {
			containsTypeParam = true
		}
		if !containsInstantiated && knownContainsInstantiated(m.Type) {
			containsInstantiated = true
		}
		if !containsGeneric && knownContainsGeneric(m.Type) {
			containsGeneric = true
		}
		if !containsRecursive && knownContainsRecursive(m.Type) {
			containsRecursive = true
		}
		if !containsOpenRecursive && knownContainsOpenRecursive(m.Type) {
			containsOpenRecursive = true
		}
	}
	// Defensive copy to prevent external mutation
	copied := make([]Method, len(methods))
	copy(copied, methods)

	return &Interface{
		Name:                  name,
		Methods:               copied,
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsGeneric:       containsGeneric,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func (i *Interface) Kind() kind.Kind { return kind.Interface }

func (i *Interface) String() string {
	return i.strCache.get(func() string {
		if i.Name != "" {
			return i.Name
		}

		var sb strings.Builder
		sb.WriteString("interface { ")

		for j, m := range i.Methods {
			if j > 0 {
				sb.WriteString("; ")
			}

			sb.WriteString(m.Name)
			sb.WriteString(": ")
			sb.WriteString(m.Type.String())
		}

		sb.WriteString(" }")

		return sb.String()
	})
}

func (i *Interface) Hash() uint64 { return i.hash }

func (i *Interface) Equals(other Type) bool {
	if other.Kind() != kind.Interface {
		return false
	}

	oi := other.(*Interface)
	if i.Name != oi.Name || len(i.Methods) != len(oi.Methods) {
		return false
	}

	for j, m := range i.Methods {
		om := oi.Methods[j]
		if m.Name != om.Name || !m.Type.Equals(om.Type) {
			return false
		}
	}

	return true
}

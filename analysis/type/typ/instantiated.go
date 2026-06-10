package typ

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
)

// Instantiated represents a generic type with concrete type arguments applied.
//
// When a generic like Array<T> is used as Array<number>, an Instantiated
// type is created with Generic=Array and TypeArgs=[number]. The body can
// be expanded by substituting type parameters with arguments.
type Instantiated struct {
	Generic               *Generic // The generic being instantiated
	TypeArgs              []Type   // Concrete types for each type parameter
	hash                  uint64
	containsAny           bool
	containsNever         bool
	containsTypeParam     bool
	containsInstantiated  bool
	containsRecursive     bool
	containsOpenRecursive bool
	strCache              stringCache
}

// Instantiate creates an instantiated generic type with the given arguments.
func Instantiate(g *Generic, args ...Type) *Instantiated {
	h := hash.MixHash(uint64(kind.Instantiated), g.Hash())
	containsAny := knownContainsAny(g)
	containsNever := knownContainsNever(g)
	containsTypeParam := knownContainsTypeParam(g)
	containsInstantiated := true
	containsRecursive := knownContainsRecursive(g)
	containsOpenRecursive := knownContainsOpenRecursive(g)
	for _, a := range args {
		h = hash.MixHash(h, a.Hash())
		if !containsAny && knownContainsAny(a) {
			containsAny = true
		}
		if !containsNever && knownContainsNever(a) {
			containsNever = true
		}
		if !containsTypeParam && knownContainsTypeParam(a) {
			containsTypeParam = true
		}
		if !containsInstantiated && knownContainsInstantiated(a) {
			containsInstantiated = true
		}
		if !containsRecursive && knownContainsRecursive(a) {
			containsRecursive = true
		}
		if !containsOpenRecursive && knownContainsOpenRecursive(a) {
			containsOpenRecursive = true
		}
	}

	return &Instantiated{
		Generic:               g,
		TypeArgs:              args,
		hash:                  h,
		containsAny:           containsAny,
		containsNever:         containsNever,
		containsTypeParam:     containsTypeParam,
		containsInstantiated:  containsInstantiated,
		containsRecursive:     containsRecursive,
		containsOpenRecursive: containsOpenRecursive,
	}
}

func (i *Instantiated) Kind() kind.Kind { return kind.Instantiated }
func (i *Instantiated) String() string {
	return i.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString(i.Generic.Name)
		sb.WriteString("<")

		for j, a := range i.TypeArgs {
			if j > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(a.String())
		}

		sb.WriteString(">")

		return sb.String()
	})
}
func (i *Instantiated) Hash() uint64 { return i.hash }
func (i *Instantiated) Equals(other Type) bool {
	return TypeEquals(i, other)
}

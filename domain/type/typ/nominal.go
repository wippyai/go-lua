package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
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
	Name    string   // Interface name (empty for anonymous)
	Methods []Method // Required methods
	hash    uint64
	typeProperties
	strCache stringCache
}

// NewInterface creates an interface type.
func NewInterface(name string, methods []Method) *Interface {
	h := hash.MixHash(uint64(kind.Interface), hash.FnvString(name))
	copied := make([]Method, len(methods))
	copy(copied, methods)
	for _, m := range methods {
		h = hash.MixHash(h, hash.FnvString(m.Name))
		h = hash.MixHash(h, m.Type.Hash())
	}
	props := typePropertiesOfMethods(copied)

	zzProbeConstruct(uint64(kind.Interface), h) // ZZPROBE
	return &Interface{
		Name:           name,
		Methods:        copied,
		hash:           h,
		typeProperties: props,
	}
}

func (i *Interface) Kind() kind.Kind { return kind.Interface }

func (i *Interface) String() string {
	return i.strCache.get(func() string { return renderTypeString(i) })
}

func (i *Interface) Hash() uint64 { return i.hash }

func (i *Interface) Equals(other Type) bool { return typeEquals(i, other) }

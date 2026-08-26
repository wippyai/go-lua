package relationadmission

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Registry is the sealed owner of the semantic value authorities used by one
// relation admission. Algebra and equality are separate authority planes:
// an owner may provide both for one TypeID, or equality alone for an
// Equatable-only TypeID. The registry never derives one plane from the other.
//
// The constructor snapshots the supplied slices into private TypeID-keyed
// tables. There are no mutation or projection methods, so a successful value
// can be shared by mount and solve consumers as one immutable registry.
type Registry struct {
	algebras   map[model.TypeID]binding.ValueAlgebra
	equalities map[model.TypeID]binding.ValueEquality
	sealed     bool
}

var _ binding.AlgebraRegistry = Registry{}
var _ binding.EqualityRegistry = Registry{}

// NewRegistry seals the owner-issued ascent and equality authorities into one
// TypeID-keyed registry. Every authority must be non-nil and report an
// available TypeID. A TypeID may occur once in each plane (for example, an
// Ascending type with an explicit equality witness), but duplicate entries in
// either plane are refused.
//
// The returned registry retains the exact authority values supplied by the
// owner. In particular, this constructor does not wrap an algebra as an
// equality witness, compare token/hash identity, or synthesize a missing
// authority.
func NewRegistry(algebras []binding.ValueAlgebra, equalities []binding.ValueEquality) (Registry, bool) {
	sealedAlgebras := make(map[model.TypeID]binding.ValueAlgebra, len(algebras))
	for _, algebra := range algebras {
		if nilAuthority(algebra) {
			return Registry{}, false
		}
		typeID := algebra.Type()
		if !typeID.Available() {
			return Registry{}, false
		}
		if _, duplicate := sealedAlgebras[typeID]; duplicate {
			return Registry{}, false
		}
		sealedAlgebras[typeID] = algebra
	}

	sealedEqualities := make(map[model.TypeID]binding.ValueEquality, len(equalities))
	for _, equality := range equalities {
		if nilAuthority(equality) {
			return Registry{}, false
		}
		typeID := equality.Type()
		if !typeID.Available() {
			return Registry{}, false
		}
		if _, duplicate := sealedEqualities[typeID]; duplicate {
			return Registry{}, false
		}
		sealedEqualities[typeID] = equality
	}

	return Registry{algebras: sealedAlgebras, equalities: sealedEqualities, sealed: true}, true
}

// Available reports whether the registry was successfully sealed. A zero
// Registry is intentionally unavailable; an empty successful constructor is
// still a valid registry for a schema with no semantic authority obligations.
func (registry Registry) Available() bool {
	return registry.sealed && registry.algebras != nil && registry.equalities != nil
}

// Resolve returns the exact owner algebra for typeID. Lookup is nominal: an
// unavailable or unknown TypeID never falls through to another authority.
func (registry Registry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	if !registry.Available() || !typeID.Available() {
		return nil, false
	}
	algebra, ok := registry.algebras[typeID]
	if !ok || nilAuthority(algebra) || algebra.Type() != typeID {
		return nil, false
	}
	return algebra, true
}

// ResolveEquality returns the exact owner equality witness for typeID.
// Equality is explicit: this lookup never projects equality from an algebra
// and never treats token identity or a hash as a semantic fallback.
func (registry Registry) ResolveEquality(typeID model.TypeID) (binding.ValueEquality, bool) {
	if !registry.Available() || !typeID.Available() {
		return nil, false
	}
	equality, ok := registry.equalities[typeID]
	if !ok || nilAuthority(equality) || equality.Type() != typeID {
		return nil, false
	}
	return equality, true
}

// nilAuthority rejects the unavailable interface form. The semantic ABI
// requires every concrete owner authority to report its TypeID; generated
// pointer authorities report a zero TypeID when their receiver is nil, which
// the constructor's availability check rejects without reflection.
func nilAuthority(value any) bool { return value == nil }

// Package carrier owns the schema-neutral carrier vocabulary shared by
// surfaces that declare, import, or consume typed coordinate carriers.
//
// A carrier key is only a nominal spelling. Its owner-qualified authority and
// capability are declarations sealed by the owning surface; consumers carry a
// Ref or Binding rather than restating either one.
package carrier

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Key is the nominal name of one owner-issued coordinate carrier.
type Key schema.Key

func (key Key) Available() bool { return schema.Key(key).Available() }

// CapabilityKind is the closed vocabulary of operations an owner promises a
// carrier can support. The capability is part of the owner's declaration;
// users of a carrier never infer it from a Go type.
type CapabilityKind uint8

const (
	CapabilityInvalid CapabilityKind = iota
	// Equatable is the capability required by an owner-key carrier.
	Equatable
	// Ascending is the capability required by a fact carrier whose order can be
	// widened or narrowed by the owning algebra.
	Ascending
	// DecodeOnly is a carrier that may be read from an owner row but is not an
	// axis key or fact algebra coordinate.
	DecodeOnly
)

func (capability CapabilityKind) Available() bool {
	return capability >= Equatable && capability <= DecodeOnly
}

// Ref is an owner-qualified reference to one carrier. Owner is intentionally
// schema-generic: axis-owned carriers and carriers issued by the Program or
// issuance surface share this reference vocabulary.
type Ref struct {
	Owner   schema.EntryReference
	Carrier Key
}

func (reference Ref) Available() bool {
	return reference.Owner.Available() && reference.Carrier.Available()
}

func (reference Ref) Declared() bool {
	return reference.Owner.Declared() || reference.Carrier.Available()
}

// Binding is the local spelling of one carrier use. Use is the alias occurring
// in a member declaration; Ref names the one owner-qualified authority that
// alias resolves to. The two carrier keys may differ.
type Binding struct {
	Use Key
	Ref Ref
}

func (binding Binding) Available() bool {
	return binding.Use.Available() && binding.Ref.Available()
}

// Authority is one local carrier declaration. id is private so an authored
// catalog cannot forge or import an authority identity; the owning catalog is
// the only issuer. Imports are Bindings and never become authorities in the
// importing catalog.
type Authority struct {
	id         schema.EntryID
	Carrier    Key
	Capability CapabilityKind
}

func (authority Authority) ID() schema.EntryID { return authority.id }

func (authority Authority) Available() bool {
	return authority.Carrier.Available() && authority.Capability.Available()
}

// Issued reports whether this authority crossed its owner's issuance
// boundary.
func (authority Authority) Issued() bool { return authority.id.Available() }

const authorityIdentityDomain = "wippy.analysis/schema/carrier/authority/v1"

// Issue owner-qualifies one raw local authority. Program/issuance owners are
// valid owners as well as axes. Imports do not call this function: only the
// catalog that owns an authority may issue it. The returned value carries the
// private identity; callers cannot forge it or assign it to another row.
func Issue(owner schema.EntryReference, authority Authority) (Authority, bool) {
	if !owner.Available() || !authority.Available() || authority.Issued() {
		return Authority{}, false
	}
	entry := schema.NewEntryID(owner.Surface, owner.Key)
	if !entry.Available() {
		return Authority{}, false
	}
	value := identity.ContentID(entry)
	issued, ok := identity.DeriveContentID(authorityIdentityDomain, value[:], []byte(authority.Carrier), []byte{byte(authority.Capability)})
	if !ok {
		return Authority{}, false
	}
	authority.id = schema.EntryID(issued)
	return authority, true
}

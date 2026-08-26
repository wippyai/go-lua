package region

import "github.com/wippyai/go-lua/analysis/identity"

// Atom is one explicit owner-issued Boolean proposition.  The region package
// carries the identity exactly as issued; it has no owner lookup or identity
// derivation authority.
type Atom struct {
	id identity.ContentID
}

// NewAtom adopts one already-issued atom identity.  The zero ContentID is an
// unavailable atom and is refused at this boundary.
func NewAtom(id identity.ContentID) (Atom, bool) {
	atom := Atom{id: id}
	return atom, atom.Available()
}

// Available reports whether this atom carries an issued identity.
func (atom Atom) Available() bool { return atom.id.Available() }

// ID returns the owner-issued content identity, or the zero identity for an
// unavailable atom.
func (atom Atom) ID() identity.ContentID {
	if !atom.Available() {
		return identity.ContentID{}
	}
	return atom.id
}

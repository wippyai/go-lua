package model

import "github.com/wippyai/go-lua/analysis/identity"

// OwnerID names the authority that issued one logical identity.  The zero
// value is unavailable.  Owners issue content-addressed tokens; this package
// only carries and validates them and never derives an identity from physical
// position or declaration order.
type OwnerID struct {
	content identity.ContentID
}

// issued is the common owner/content pair carried by nominal identities. It
// is private so callers cannot accidentally construct a foreign identity by
// copying fields across nominal types.
type issued struct {
	owner   OwnerID
	content identity.ContentID
}

func (value issued) Available() bool {
	return value.owner.Available() && value.content.Available()
}

func issue(owner OwnerID, content identity.ContentID) (issued, bool) {
	if !owner.Available() || !content.Available() {
		return issued{}, false
	}
	return issued{owner: owner, content: content}, true
}

// IssueOwnerID adopts a non-zero owner-issued content token.
func IssueOwnerID(content identity.ContentID) (OwnerID, bool) {
	if !content.Available() {
		return OwnerID{}, false
	}
	return OwnerID{content: content}, true
}

// Available reports whether id names an issuing owner.
func (id OwnerID) Available() bool { return id.content.Available() }

// Content returns the owner-issued token carried by id.
func (id OwnerID) Content() identity.ContentID { return id.content }

// RelationID is the nominal identity of one logical relation.  Its owner
// fence prevents equal content tokens issued by different owners from being
// treated as the same relation.
type RelationID struct {
	issued
}

// IssueRelationID adopts a non-zero token issued by owner.
func IssueRelationID(owner OwnerID, content identity.ContentID) (RelationID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return RelationID{}, false
	}
	return RelationID{issued: value}, true
}

// Available reports whether id names a relation.
func (id RelationID) Available() bool {
	return id.issued.Available()
}

// Owner returns the authority that issued id.
func (id RelationID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued relation token.
func (id RelationID) Content() identity.ContentID { return id.issued.content }

// ColumnID is the nominal identity of a relation column.  A column is issued
// in the context of exactly one relation; the relation fence is part of the
// identity and is checked whenever schemas compose columns.
type ColumnID struct {
	relation RelationID
	issued
}

// IssueColumnID adopts a non-zero token issued for relation.
func IssueColumnID(relation RelationID, content identity.ContentID) (ColumnID, bool) {
	value, ok := issue(relation.Owner(), content)
	if !relation.Available() || !ok {
		return ColumnID{}, false
	}
	return ColumnID{relation: relation, issued: value}, true
}

// Available reports whether id names a column.
func (id ColumnID) Available() bool {
	return id.relation.Available() && id.issued.Available()
}

// Relation returns the relation that owns id.
func (id ColumnID) Relation() RelationID { return id.relation }

// Owner returns the relation owner that issued id.
func (id ColumnID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued column token.
func (id ColumnID) Content() identity.ContentID { return id.issued.content }

// KeyID is the nominal identity of one logical key vector in a relation.
// Key-column order is a logical property of the key definition, not a
// physical ordinal.
type KeyID struct {
	relation RelationID
	issued
}

// IssueKeyID adopts a non-zero token issued for relation.
func IssueKeyID(relation RelationID, content identity.ContentID) (KeyID, bool) {
	value, ok := issue(relation.Owner(), content)
	if !relation.Available() || !ok {
		return KeyID{}, false
	}
	return KeyID{relation: relation, issued: value}, true
}

// Available reports whether id names a key.
func (id KeyID) Available() bool {
	return id.relation.Available() && id.issued.Available()
}

// Relation returns the relation that owns id.
func (id KeyID) Relation() RelationID { return id.relation }

// Owner returns the relation owner that issued id.
func (id KeyID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued key token.
func (id KeyID) Content() identity.ContentID { return id.issued.content }

// ScopeID is the nominal identity of one decision-scope schema.
type ScopeID struct {
	issued
}

// IssueScopeID adopts a non-zero token issued by owner.
func IssueScopeID(owner OwnerID, content identity.ContentID) (ScopeID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return ScopeID{}, false
	}
	return ScopeID{issued: value}, true
}

// Available reports whether id names a scope schema.
func (id ScopeID) Available() bool {
	return id.issued.Available()
}

// Owner returns the authority that issued id.
func (id ScopeID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued scope token.
func (id ScopeID) Content() identity.ContentID { return id.issued.content }

// RefusalID is an owner-issued identity for a refusal reason.  Refusal
// reasons are identities, not free-form strings, so equal reasons remain
// stable across declaration and physical order.
type RefusalID struct {
	issued
}

// IssueRefusalID adopts a non-zero token issued by owner.
func IssueRefusalID(owner OwnerID, content identity.ContentID) (RefusalID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return RefusalID{}, false
	}
	return RefusalID{issued: value}, true
}

// Available reports whether id names a refusal reason.
func (id RefusalID) Available() bool {
	return id.issued.Available()
}

// Owner returns the authority that issued id.
func (id RefusalID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued refusal token.
func (id RefusalID) Content() identity.ContentID { return id.issued.content }

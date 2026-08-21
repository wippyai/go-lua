package lifecycle

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// SubjectAliasRouteScopeMember is one dense ordered route in exactly one
// scope. It contains no copied denominator or inverse index.
type SubjectAliasRouteScopeMember struct {
	id      identity.ContentID
	scope   identity.ContentID
	route   identity.ContentID
	ordinal uint32
	proven  bool
}

func SubjectAliasRouteScopeMemberIdentity(scope identity.ContentID, ordinal uint32, route identity.ContentID) (identity.ContentID, bool) {
	if !scope.Available() || !route.Available() {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-alias-route-scope-member-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(scope[:]) != nil || writer.Uint(uint64(ordinal)) != nil || writer.Bytes(route[:]) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectAliasRouteScopeMember(id, scope identity.ContentID, ordinal uint32, route identity.ContentID) (SubjectAliasRouteScopeMember, bool) {
	derived, derivedOK := SubjectAliasRouteScopeMemberIdentity(scope, ordinal, route)
	row := SubjectAliasRouteScopeMember{id: id, scope: scope, route: route, ordinal: ordinal, proven: derivedOK && derived == id}
	return row, row.Available()
}

func (row SubjectAliasRouteScopeMember) Available() bool {
	return row.proven && row.id.Available() && row.scope.Available() && row.route.Available()
}

func (row SubjectAliasRouteScopeMember) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectAliasRouteScopeMember) ScopeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.scope
}

func (row SubjectAliasRouteScopeMember) RouteID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.route
}

func (row SubjectAliasRouteScopeMember) Ordinal() (uint32, bool) {
	return row.ordinal, row.Available()
}

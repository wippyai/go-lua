package lifecycle

import (
	"bytes"
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// SubjectAliasRouteScopeKind is the complete alias-denominator scope
// vocabulary. Body scopes carry one body identity; the global fallback does
// not fabricate one.
type SubjectAliasRouteScopeKind uint8

const (
	SubjectAliasRouteScopeInvalid SubjectAliasRouteScopeKind = iota
	SubjectAliasRouteScopeBody
	SubjectAliasRouteScopeGlobal
)

func (kind SubjectAliasRouteScopeKind) Valid() bool {
	return kind == SubjectAliasRouteScopeBody || kind == SubjectAliasRouteScopeGlobal
}

// SubjectAliasRouteScope is the scalar header for one normalized ordered
// route denominator. Members live only in SubjectAliasRouteScopeMemberFamily.
type SubjectAliasRouteScope struct {
	id           identity.ContentID
	sourceScope  identity.ContentID
	kind         SubjectAliasRouteScopeKind
	body         identity.ContentID
	memberOffset uint32
	memberCount  uint32
	proven       bool
}

func SubjectAliasRouteScopeIdentity(sourceScope identity.ContentID, kind SubjectAliasRouteScopeKind, body identity.ContentID, routes []identity.ContentID) (identity.ContentID, bool) {
	if !sourceScope.Available() || !kind.Valid() || kind == SubjectAliasRouteScopeBody && !body.Available() || kind == SubjectAliasRouteScopeGlobal && body.Available() || !canonicalAliasRoutes(routes) {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/subject-alias-route-scope-v1", 1) != nil || writer.Record(1) != nil ||
		writer.Bytes(sourceScope[:]) != nil || writer.Uint(uint64(kind)) != nil || writer.Bool(body.Available()) != nil ||
		writer.Bytes(body[:]) != nil || writer.Uint(uint64(len(routes))) != nil {
		return identity.ContentID{}, false
	}
	for _, route := range routes {
		if writer.Bytes(route[:]) != nil {
			return identity.ContentID{}, false
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}

func NewSubjectAliasRouteScope(id, sourceScope identity.ContentID, kind SubjectAliasRouteScopeKind, body identity.ContentID, memberOffset uint32, routes []identity.ContentID) (SubjectAliasRouteScope, bool) {
	if uint64(len(routes)) > uint64(^uint32(0)) {
		return SubjectAliasRouteScope{}, false
	}
	derived, derivedOK := SubjectAliasRouteScopeIdentity(sourceScope, kind, body, routes)
	row := SubjectAliasRouteScope{id: id, sourceScope: sourceScope, kind: kind, body: body, memberOffset: memberOffset, memberCount: uint32(len(routes))}
	row.proven = derivedOK && derived == id
	return row, row.Available()
}

func (row SubjectAliasRouteScope) Available() bool {
	return row.proven && row.id.Available() && row.sourceScope.Available() && row.kind.Valid() &&
		(row.kind == SubjectAliasRouteScopeBody && row.body.Available() || row.kind == SubjectAliasRouteScopeGlobal && !row.body.Available())
}

func (row SubjectAliasRouteScope) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row SubjectAliasRouteScope) SourceScopeID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.sourceScope
}

func (row SubjectAliasRouteScope) Kind() SubjectAliasRouteScopeKind {
	if !row.Available() {
		return SubjectAliasRouteScopeInvalid
	}
	return row.kind
}

func (row SubjectAliasRouteScope) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row SubjectAliasRouteScope) MemberSpan() (uint32, uint32, bool) {
	if !row.Available() {
		return 0, 0, false
	}
	return row.memberOffset, row.memberCount, true
}

func canonicalAliasRoutes(routes []identity.ContentID) bool {
	for index, route := range routes {
		if !route.Available() || index > 0 && bytes.Compare(routes[index-1][:], route[:]) >= 0 {
			return false
		}
	}
	return true
}

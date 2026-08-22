package contextfiber

import "github.com/wippyai/go-lua/analysis/identity"

const pointOwnerDomain = "analysis/engine/internal/contextfiber/point-owner/v1"

// PointOwnerKind names the two point-owner lanes admitted by the compact
// execution state layout. Mounted points belong to one module context; link
// globals belong to the Link and therefore have one state row shared by every
// context.
type PointOwnerKind uint8

const (
	pointOwnerInvalid PointOwnerKind = iota
	PointOwnerMounted
	PointOwnerLinkGlobal
)

// PointOwner is an authenticated owner value for one point position. Its
// private identity is derived from the lane and key, and issued is set only by
// the owner constructors, so a mounted/global classification cannot be
// manufactured by writing fields or by replaying the derivation.
type PointOwner struct {
	kind   PointOwnerKind
	key    identity.ContentID
	id     identity.ContentID
	issued bool
}

// Mounted constructs an authenticated owner for a point mounted by moduleKey.
func Mounted(moduleKey identity.ContentID) (PointOwner, bool) {
	return newPointOwner(PointOwnerMounted, moduleKey)
}

// LinkGlobal constructs an authenticated owner for a Link-global point plane.
func LinkGlobal(linkID identity.ContentID) (PointOwner, bool) {
	return newPointOwner(PointOwnerLinkGlobal, linkID)
}

func newPointOwner(kind PointOwnerKind, key identity.ContentID) (PointOwner, bool) {
	if (kind != PointOwnerMounted && kind != PointOwnerLinkGlobal) || !key.Available() {
		return PointOwner{}, false
	}
	id, ok := identity.DeriveContentID(pointOwnerDomain, []byte{byte(kind)}, key[:])
	if !ok {
		return PointOwner{}, false
	}
	owner := PointOwner{kind: kind, key: key, id: id, issued: true}
	return owner, owner.Available()
}

// Available reports whether owner is an authenticated lane/key tuple issued by
// this package. The zero owner is unavailable.
func (owner PointOwner) Available() bool { return owner.issued }

// Kind returns the authenticated owner lane.
func (owner PointOwner) Kind() PointOwnerKind {
	if !owner.Available() {
		return pointOwnerInvalid
	}
	return owner.kind
}

// Mounted reports whether owner is an authenticated module-mounted owner.
func (owner PointOwner) Mounted() bool { return owner.Available() && owner.kind == PointOwnerMounted }

// LinkGlobal reports whether owner is an authenticated Link-global owner.
func (owner PointOwner) LinkGlobal() bool {
	return owner.Available() && owner.kind == PointOwnerLinkGlobal
}

// ModuleKey returns the module identity for a mounted owner.
func (owner PointOwner) ModuleKey() identity.ContentID {
	if !owner.Mounted() {
		return identity.ContentID{}
	}
	return owner.key
}

// LinkID returns the Link identity for a global owner.
func (owner PointOwner) LinkID() identity.ContentID {
	if !owner.LinkGlobal() {
		return identity.ContentID{}
	}
	return owner.key
}

// ID returns the authenticated point-owner identity.
func (owner PointOwner) ID() identity.ContentID {
	if !owner.Available() {
		return identity.ContentID{}
	}
	return owner.id
}

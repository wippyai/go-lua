package subtype

import (
	"github.com/wippyai/go-lua/domain/type/internal/nodeid"
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func (c *checker) check(sub, super typ.Type) bool {
	return c.prove(subtypeOf(sub, super))
}

type typePair struct {
	sub   typeNodeID
	super typeNodeID
}

// typeNodeID is an identity in the closed type universe. Product nodes use
// their allocation identity; singleton primitives use their kind. Keeping
// those spaces separate avoids treating a pointer value as a primitive tag.
type typeNodeID struct {
	pointer     uintptr
	primitive   kind.Kind
	isPrimitive bool
}

func newTypePair(sub, super typ.Type) (typePair, bool) {
	subID, subOK := typeNodeIdentity(sub)
	superID, superOK := typeNodeIdentity(super)
	if !subOK || !superOK {
		return typePair{}, false
	}
	return typePair{sub: subID, super: superID}, true
}

func typeNodeIdentity(t typ.Type) (typeNodeID, bool) {
	if pointer := nodeid.Pointer(t); pointer != 0 {
		return typeNodeID{pointer: pointer}, true
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String,
		kind.Any, kind.Unknown, kind.Never, kind.Self:
		return typeNodeID{primitive: t.Kind(), isPrimitive: true}, true
	default:
		return typeNodeID{}, false
	}
}

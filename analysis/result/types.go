package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// ValueCoordinate is the Link substitution for one Value factor coordinate.
type ValueCoordinate struct {
	id    identity.ContentID
	mount identity.ContentID
}

// NewValueCoordinate admits one value identity at a mount.
func NewValueCoordinate(id, mount identity.ContentID) (ValueCoordinate, bool) {
	if !id.Available() || !mount.Available() {
		return ValueCoordinate{}, false
	}
	return ValueCoordinate{id: id, mount: mount}, true
}

// ID returns the canonical Value identity carried by this coordinate.
func (coordinate ValueCoordinate) ID() identity.ContentID { return coordinate.id }

// MountID returns the Link mount substitution carried by this coordinate.
func (coordinate ValueCoordinate) MountID() identity.ContentID { return coordinate.mount }

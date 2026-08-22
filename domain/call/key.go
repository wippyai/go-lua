// Package call owns the finite dispatch-completeness relation.
package call

import (
	"github.com/wippyai/go-lua/analysis/identity"
)

// Key is one arm of Call's closed source sum: a Project base Application, a
// Target callback, or a Target resume.  The dense slot is private and valid
// only with the issuing Algebra; portable identity is ContentID.
type Key struct {
	owner *Algebra
	slot  uint32
}

func (key Key) Valid() bool { return key.owner != nil && key.owner.validKey(key) }

// IsApplication reports the arm discriminator without exposing the private
// dense coordinate.
func (key Key) IsApplication() bool {
	return key.Valid() && key.owner.keys[key.slot-1].kind == keyApplication
}

// ContentID is the cold identity of this exact Call source-sum arm.
func (key Key) ContentID() (identity.ContentID, bool) {
	if !key.Valid() {
		return identity.ContentID{}, false
	}
	return key.owner.keys[key.slot-1].id, true
}

// ApplicationID is the exact existing Project base-application identity.
func (key Key) ApplicationID() (identity.ContentID, bool) {
	if !key.IsApplication() {
		return identity.ContentID{}, false
	}
	return key.owner.keys[key.slot-1].id, true
}

// selector is the private dense identity for one global callable target.
// Zero is invalid and it never crosses the Call package boundary.
type selector uint32

func (selector selector) valid() bool { return selector != 0 }

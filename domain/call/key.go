// Package call owns the finite dispatch-completeness relation.
package call

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// Key is one arm of Call's closed source sum: a Project base Application, a
// Target callback, or a Target resume.  The dense slot is private and valid
// only with the issuing Algebra; portable identity is ContentID.
type Key struct {
	owner *Algebra
	slot  uint32
}

func (key Key) Valid() bool { return key.owner != nil && key.owner.validKey(key) }

// Callback returns the exact Target operation/callback pair for a callback
// arm. It never projects a callback Subedge or manufactures an Application.
func (key Key) Callback() (vocabulary.Operation, vocabulary.CallbackID, bool) {
	if !key.Valid() || key.owner.keys[key.slot-1].kind != keyCallback {
		return 0, 0, false
	}
	row := key.owner.keys[key.slot-1]
	return row.operation, row.callback, true
}

// Resume returns the exact Target operation/resume pair for a resume arm.
func (key Key) Resume() (vocabulary.Operation, vocabulary.ResumeID, bool) {
	if !key.Valid() || key.owner.keys[key.slot-1].kind != keyResume {
		return 0, 0, false
	}
	row := key.owner.keys[key.slot-1]
	return row.operation, row.resume, true
}

// Operation returns the Target operation owning this Call source arm. It is
// useful to neutral consumers that need to classify operation declarations
// without reopening Call's private key row.
func (key Key) Operation() (vocabulary.Operation, bool) {
	if !key.Valid() {
		return 0, false
	}
	return key.owner.keys[key.slot-1].operation, true
}

// IsApplication reports the arm discriminator without exposing the private
// dense coordinate.
func (key Key) IsApplication() bool {
	return key.Valid() && key.owner.keys[key.slot-1].kind == keyApplication
}

// IsCallback reports the arm discriminator without exposing the private
// dense coordinate.
func (key Key) IsCallback() bool {
	return key.Valid() && key.owner.keys[key.slot-1].kind == keyCallback
}

// IsResume reports the arm discriminator without exposing the private dense
// coordinate.
func (key Key) IsResume() bool {
	return key.Valid() && key.owner.keys[key.slot-1].kind == keyResume
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
	return key.owner.keys[key.slot-1].applicationID, true
}

// selector is the private dense identity for one global callable target.
// Zero is invalid and it never crosses the Call package boundary.
type selector uint32

func (selector selector) valid() bool { return selector != 0 }

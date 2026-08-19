package flow

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/internal/framing"
)

var errInvalidArtifactComponent = errors.New("program/flow: invalid artifact component")

// WriteArtifactSection writes Flow's authored payload into an enclosing
// canonical stream. The enclosing artifact owns Reset/Header and Finish;
// this child section emits only its nine authored records. It consumes the
// direct immutable Flow View and rejects an unavailable view before writing.
func WriteArtifactSection(writer *framing.Writer, view View) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if !view.available() {
		return errInvalidArtifactComponent
	}
	// The artifact codec is an owner-local consumer of the live View.  Pass
	// the canonical authored storage directly; going through View.Authored()
	// would create a second public adapter route for the same payload.
	return authored.WriteArtifactSection(writer, view.component.authored)
}

// ReadArtifactSection reads Flow's authored payload from an enclosing stream.
// It leaves Input.Counts zero so the root artifact can inject the dense term
// universes before calling Build. Stream completion remains the caller's job.
func ReadArtifactSection(reader *framing.Reader) (Input, error) {
	return authored.ReadArtifactSection(reader)
}

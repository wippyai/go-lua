package imports

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Finalizer claims Draft's owner-defined publication capability. The claim is
// shared by every copied Draft, so a copied value cannot open a second path.
func (d *Draft) Finalizer() (Finalizer, error) {
	if !d.active() {
		return Finalizer{}, errors.New("program/imports: invalid draft")
	}
	state := d.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.claimed || state.terminal || state.authored == nil {
		return Finalizer{}, errors.New("program/imports: finalizer already claimed")
	}
	state.claimed = true
	return Finalizer{state: state}, nil
}

// View returns the authored Module surface while this Finalizer is active.
// After Commit or Abort it becomes an empty view.
func (f Finalizer) View() View { return View{state: f.state} }

// Commit validates the complete derived projection and publishes one
// immutable Component. Commit is terminal even when validation fails.
func (f Finalizer) Commit(input CommitInput) (*Component, error) {
	if f.state == nil {
		return nil, errors.New("program/imports: invalid finalizer")
	}
	state := f.state
	state.mu.Lock()
	if state.terminal || !state.claimed || state.authored == nil {
		state.mu.Unlock()
		return nil, errors.New("program/imports: finalizer is closed")
	}
	authored := state.authored
	state.authored = nil
	state.terminal = true
	state.mu.Unlock()

	if err := validateDerived(authored.imports, input); err != nil {
		return nil, err
	}
	imports := append([]Import(nil), authored.imports...)
	for index, resolution := range input.Resolutions {
		imports[index].Key = resolution.Key
	}
	component := &Component{
		imports: imports,
		entry:   cloneEntry(input.Entry),
		content: authored.content,
	}
	return component, nil
}

// Abort closes the owner capability without publishing any Component. It is
// idempotent only in the sense that a later call returns false and cannot
// reopen construction.
func (f Finalizer) Abort() bool {
	if f.state == nil {
		return false
	}
	state := f.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal || !state.claimed || state.authored == nil {
		return false
	}
	state.authored = nil
	state.terminal = true
	return true
}

func validateDerived(imports []Import, input CommitInput) error {
	if len(input.Resolutions) != len(imports) {
		return errors.New("program/imports: incomplete resolution denominator")
	}
	for index, resolution := range input.Resolutions {
		if resolution.Request != imports[index].Request {
			return errors.New("program/imports: resolution Request disagrees with authored Import")
		}
		if !validFamilyTerm(resolution.Request, keyspace.FamilyString) {
			return errors.New("program/imports: invalid resolution Request")
		}
		if resolution.Key == 0 {
			return errors.New("program/imports: missing derived import Key")
		}
	}
	if !input.Entry.valid() {
		return errors.New("program/imports: invalid derived Entry")
	}
	return nil
}

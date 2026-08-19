package source

import "errors"

// Finalizer claims the Draft's one-shot lifecycle and returns the only
// capability allowed to install the root-sealed Source index. Claiming is a
// separate operation from Commit so Flow can inspect authored Source views
// while deriving cross-owner geometry without exposing a published Component.
func (d *Draft) Finalizer() (Finalizer, error) {
	if d == nil || d.state == nil {
		return Finalizer{}, errors.New("program/source: invalid finalizer")
	}
	state := d.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftOpen || state.authority == nil {
		return Finalizer{}, errors.New("program/source: finalizer already claimed")
	}
	state.phase = draftFinalizerClaimed
	return Finalizer{state: state}, nil
}

// Preimage returns the one authored Source query bundle for this claimed
// Finalizer. The bundle stores only the shared lifecycle fence; each typed
// subview resolves the current owner from that same fence, so copied or
// foreign capabilities cannot be combined into another Source owner.
func (f Finalizer) Preimage() Preimage {
	if f.authority() == nil {
		return Preimage{}
	}
	return Preimage(f)
}

// Identity returns the authored identity view while the Preimage is live.
func (p Preimage) Identity() Identity { return Identity{state: p.state} }

// Order returns authored direct Body source order while the Preimage is live.
func (p Preimage) Order() Order { return Order{state: p.state} }

// Binds returns authored Bind cell order while the Preimage is live.
func (p Preimage) Binds() BindOrder { return BindOrder{state: p.state} }

// Formals returns authored Function formal order while the Preimage is live.
func (p Preimage) Formals() FormalOrder { return FormalOrder{state: p.state} }

// Spellings returns the authored debug-spelling rows while the Preimage is
// live. It shares the same lifecycle fence as every other Source view.
func (p Preimage) Spellings() Spellings { return Spellings{state: p.state} }

// Keys returns Source's authored key and exact-atom authority while the
// Preimage is live.
func (p Preimage) Keys() Keys { return Keys{state: p.state} }

// Faults returns authored control-fault provenance while the Preimage is live.
func (p Preimage) Faults() Faults { return Faults{state: p.state} }

// Literals returns authored literal rows while the Preimage is live.
func (p Preimage) Literals() Literals { return Literals{state: p.state} }

// Commit consumes this Finalizer exactly once. Both success and validation
// failure are terminal: no later copy can retry with a different IndexInput.
// The caller-owned batch is validated and compacted into Source's private
// index; no batch or containment rows survive publication.
func (f Finalizer) Commit(input IndexInput) (*Component, error) {
	return f.commit(input)
}

func (f Finalizer) commit(input IndexInput) (*Component, error) {
	if f.state == nil {
		return nil, errors.New("program/source: invalid finalizer")
	}
	state := f.state
	state.mu.Lock()
	if state.phase != draftFinalizerClaimed || state.authority == nil {
		state.mu.Unlock()
		return nil, errors.New("program/source: finalizer is terminal")
	}
	// Keep the original authored authority immutable for any query that
	// already captured it before Commit acquired the fence. Seal projection
	// installs the derived Outcome identity and sparse source index, so build that
	// projection on a private shallow authority copy before publishing the
	// terminal transition. The copied identity store owns its scalar/slice
	// headers; all authored row slices are immutable and safely shared. The
	// claimed state's original owner is invalidated below, and only this one
	// candidate is published through Component. It carries the same authored
	// ContentID; the private copy is not a second externally usable authority.
	authority := *state.authority
	if err := installIndex(&authority, input); err != nil {
		state.phase = draftTerminal
		state.authority = nil
		state.mu.Unlock()
		return nil, err
	}
	cellRoles, err := buildCellRoleAuthority(&authority)
	if err != nil {
		state.phase = draftTerminal
		state.authority = nil
		state.mu.Unlock()
		return nil, err
	}
	authority.cellRoles = cellRoles
	state.phase = draftTerminal
	state.authority = nil
	state.mu.Unlock()
	return &Component{authority: &authority}, nil
}

// Abort consumes this Finalizer without publishing a Component. Abort is
// terminal and idempotence is deliberately rejected so misuse cannot be
// mistaken for a successful lifecycle transition.
func (f Finalizer) Abort() error {
	if f.state == nil {
		return errors.New("program/source: invalid finalizer")
	}
	state := f.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftFinalizerClaimed || state.authority == nil {
		return errors.New("program/source: finalizer is terminal")
	}
	state.phase = draftTerminal
	state.authority = nil
	return nil
}

// authority returns the uncommitted owner for one authored Finalizer view.
// The owner rows are immutable after Build, so the lock only protects the
// lifecycle check and capability claim; published Components use views with
// no lifecycle state and do not pay this check.
func (f Finalizer) authority() *authority {
	if f.state == nil {
		return nil
	}
	state := f.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftFinalizerClaimed {
		return nil
	}
	return state.authority
}

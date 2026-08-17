package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Build seals only authored Source rows. The explicit typed index batch is
// consumed later, after Flow's position seal has supplied its local geometry.
func Build(input Input) (*Draft, error) {
	a := &authority{}
	if err := buildIdentity(&a.identity, input.Name, input.Families); err != nil {
		return nil, err
	}
	if err := buildLiterals(a, input); err != nil {
		return nil, err
	}
	if err := buildOrder(a, input); err != nil {
		return nil, err
	}
	if err := buildKeyFault(a, input); err != nil {
		return nil, err
	}
	if err := validateFaultSourceOwnership(a); err != nil {
		return nil, err
	}
	a.content = authoredContentID(a)
	if !a.content.Available() {
		return nil, errors.New("program/source: unavailable authored identity")
	}
	return &Draft{state: &draftState{authority: a}}, nil
}

// validateFaultSourceOwnership closes the authored side of control-fault
// containment. Every dense fault ordinal occurs once in its declared owner
// Body's direct source sequence; Finalize then requires that same direct term
// to have exactly one sealed source Position.
func validateFaultSourceOwnership(a *authority) error {
	if a == nil || a.count(keyspace.FamilyControlFault) == 0 {
		return nil
	}
	owners := make([]keyspace.Term, a.count(keyspace.FamilyControlFault))
	for bodyOrdinal, sourceRange := range a.order.bodyRanges {
		if !validRange(a.order.sourceTerms, sourceRange) {
			return errors.New("program/source: invalid Body source range")
		}
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1))
		for _, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			if keyspace.TermFamily(term) != keyspace.FamilyControlFault {
				continue
			}
			ordinal := keyspace.TermOrdinal(term)
			if ordinal == 0 || uint64(ordinal) > uint64(len(owners)) || owners[ordinal-1] != 0 {
				return errors.New("program/source: duplicate or invalid direct control fault")
			}
			owners[ordinal-1] = body
		}
	}
	for index, row := range a.keys.faults {
		if owners[index] == 0 || owners[index] != row.Owner {
			return errors.New("program/source: control fault lacks its owner Body source occurrence")
		}
	}
	return nil
}

func buildIdentity(store *identityStore, name string, rows []FamilySpans) error {
	if store == nil || name == "" || len(rows) != int(keyspace.FamilyCount-1) {
		return errors.New("program/source: incomplete family spans")
	}
	store.name = name
	for index, row := range rows {
		family := keyspace.Family(index + 1)
		if row.Family != family || !keyspace.TermOrdinalFits(len(row.Spans)) {
			return errors.New("program/source: invalid family spans")
		}
		// Outcome is the sole derived Term family. Its identity is assigned by
		// Flow's canonical control topology during Finalize, never by authored
		// Source input. Keep the explicit family row in the denominator so a
		// caller cannot silently omit the derived family, but require it empty.
		if family == keyspace.FamilyOutcome && len(row.Spans) != 0 {
			return errors.New("program/source: authored Outcome spans are forbidden")
		}
		spans := make([]storedSpan, len(row.Spans))
		for at, span := range row.Spans {
			stored, ok := compactSpan(name, span)
			if !ok {
				return errors.New("program/source: invalid source span")
			}
			spans[at] = stored
		}
		store.counts[family] = uint32(len(spans))
		store.spans[family] = spans
		if uint64(store.termCount)+uint64(len(spans)) > uint64(^uint32(0)) {
			return errors.New("program/source: Term cardinality overflow")
		}
		store.termCount += uint32(len(spans))
	}
	if store.termCount == 0 {
		return errors.New("program/source: empty Term cardinality")
	}
	return nil
}

func buildLiterals(a *authority, input Input) error {
	if a == nil || len(input.Nil) != a.count(keyspace.FamilyNil) ||
		len(input.Bool) != a.count(keyspace.FamilyBool) ||
		len(input.Integer) != a.count(keyspace.FamilyInteger) ||
		len(input.Float) != a.count(keyspace.FamilyFloat) ||
		len(input.String) != a.count(keyspace.FamilyString) {
		return errors.New("program/source: literal family cardinality mismatch")
	}
	a.literals.nil = append([]NilLiteral(nil), input.Nil...)
	a.literals.bool = append([]BoolLiteral(nil), input.Bool...)
	a.literals.integer = append([]IntegerLiteral(nil), input.Integer...)
	a.literals.float = append([]FloatLiteral(nil), input.Float...)
	a.literals.string = append([]StringLiteral(nil), input.String...)
	for _, owner := range a.literals.nil {
		if !a.validFamilyTerm(owner.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid nil owner")
		}
	}
	for _, row := range a.literals.bool {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid bool owner")
		}
	}
	for _, row := range a.literals.integer {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid integer owner")
		}
	}
	for _, row := range a.literals.float {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid float owner")
		}
	}
	for _, row := range a.literals.string {
		if !a.validFamilyTerm(row.Owner, keyspace.FamilyBody) {
			return errors.New("program/source: invalid string owner")
		}
	}
	return nil
}

func buildOrder(a *authority, input Input) error {
	if a == nil {
		return errors.New("program/source: nil authority")
	}
	if err := buildBodyOrder(a, input.Bodies); err != nil {
		return err
	}
	cells := make([]bool, a.count(keyspace.FamilyCell))
	if err := buildBindOrder(a, input.Binds, cells); err != nil {
		return err
	}
	return buildFormalOrder(a, input.Functions, cells)
}

func buildBodyOrder(a *authority, rows []BodySource) error {
	count := a.count(keyspace.FamilyBody)
	if len(rows) != count {
		return errors.New("program/source: Body source cardinality mismatch")
	}
	a.order.bodyRanges = make([]termRange, count)
	seen := newTermMarks(a.identity.counts)
	for index, row := range rows {
		if !a.validFamilyTerm(row.Body, keyspace.FamilyBody) || keyspace.TermOrdinal(row.Body) != uint32(index+1) {
			return errors.New("program/source: invalid Body source owner")
		}
		start := len(a.order.sourceTerms)
		for _, term := range row.Terms {
			if !a.validDirectBodyTerm(term) || seen.take(term) {
				return errors.New("program/source: duplicate or invalid direct source Term")
			}
			a.order.sourceTerms = append(a.order.sourceTerms, term)
		}
		r, ok := makeRange(start, len(row.Terms))
		if !ok {
			return errors.New("program/source: Body source range overflow")
		}
		a.order.bodyRanges[index] = r
	}
	return nil
}

func buildBindOrder(a *authority, rows []BindCells, cells []bool) error {
	count := a.count(keyspace.FamilyBind)
	if len(rows) != count {
		return errors.New("program/source: Bind order cardinality mismatch")
	}
	a.order.bindRanges = make([]termRange, count)
	for index, row := range rows {
		if !a.validFamilyTerm(row.Bind, keyspace.FamilyBind) || keyspace.TermOrdinal(row.Bind) != uint32(index+1) {
			return errors.New("program/source: invalid Bind order owner")
		}
		start := len(a.order.bindTerms)
		for _, cell := range row.Cells {
			if !a.validFamilyTerm(cell, keyspace.FamilyCell) || cells[keyspace.TermOrdinal(cell)-1] {
				return errors.New("program/source: duplicate or invalid Bind Cell")
			}
			cells[keyspace.TermOrdinal(cell)-1] = true
			a.order.bindTerms = append(a.order.bindTerms, cell)
			a.order.bindOwners = append(a.order.bindOwners, row.Bind)
		}
		r, ok := makeRange(start, len(row.Cells))
		if !ok {
			return errors.New("program/source: Bind range overflow")
		}
		a.order.bindRanges[index] = r
	}
	return nil
}

func buildFormalOrder(a *authority, rows []FunctionFormals, cells []bool) error {
	count := a.count(keyspace.FamilyFunction)
	if len(rows) != count {
		return errors.New("program/source: Function formal cardinality mismatch")
	}
	a.order.formalRanges = make([]termRange, count)
	for index, row := range rows {
		if !a.validFamilyTerm(row.Function, keyspace.FamilyFunction) || keyspace.TermOrdinal(row.Function) != uint32(index+1) {
			return errors.New("program/source: invalid Function formal owner")
		}
		start := len(a.order.formalTerms)
		for _, formal := range row.Formals {
			if !a.validFamilyTerm(formal, keyspace.FamilyCell) || cells[keyspace.TermOrdinal(formal)-1] {
				return errors.New("program/source: duplicate or invalid Function formal")
			}
			cells[keyspace.TermOrdinal(formal)-1] = true
			a.order.formalTerms = append(a.order.formalTerms, formal)
			a.order.formalOwners = append(a.order.formalOwners, row.Function)
		}
		r, ok := makeRange(start, len(row.Formals))
		if !ok {
			return errors.New("program/source: Function formal range overflow")
		}
		a.order.formalRanges[index] = r
	}
	return nil
}

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
	component, _, err := f.commit(input, false)
	return component, err
}

// CommitWithSemanticPathIssuance is the sole parent issuance point for
// Flow's structural semantic-path certificate. The token is returned only to
// the exact Commit caller, cannot be reconstructed from Component.View, and
// may be consumed once by semanticpath after all sibling proofs are present.
func (f Finalizer) CommitWithSemanticPathIssuance(input IndexInput) (*Component, *SemanticPathIssuance, error) {
	return f.commit(input, true)
}

func (f Finalizer) commit(input IndexInput, issueSemanticPath bool) (*Component, *SemanticPathIssuance, error) {
	if f.state == nil {
		return nil, nil, errors.New("program/source: invalid finalizer")
	}
	state := f.state
	state.mu.Lock()
	if state.phase != draftFinalizerClaimed || state.authority == nil {
		state.mu.Unlock()
		return nil, nil, errors.New("program/source: finalizer is terminal")
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
		return nil, nil, err
	}
	cellRoles, err := buildCellRoleAuthority(&authority)
	if err != nil {
		state.phase = draftTerminal
		state.authority = nil
		state.mu.Unlock()
		return nil, nil, err
	}
	authority.cellRoles = cellRoles
	state.phase = draftTerminal
	state.authority = nil
	state.mu.Unlock()
	component := &Component{authority: &authority}
	if !issueSemanticPath {
		return component, nil, nil
	}
	return component, &SemanticPathIssuance{state: &semanticPathIssuanceState{authority: &authority}}, nil
}

// ConsumeSemanticPathIssuance transfers the exact commit-only capability to
// Flow's semantic-path leaf. A same-content or foreign View is rejected by
// pointer authority, and every attempted consume is terminal.
func (issuance *SemanticPathIssuance) ConsumeSemanticPathIssuance(view View) bool {
	if issuance == nil || issuance.state == nil {
		return false
	}
	state := issuance.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		return false
	}
	// Consumption is terminal before any caller-controlled View comparison.
	// A foreign probe must not leave a live capability for an exact retry.
	authority := state.authority
	state.used = true
	state.authority = nil
	return authority != nil && view.authority == authority
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

func installIndex(a *authority, input IndexInput) error {
	if a == nil || !input.SourceID.Available() || input.SourceID != a.content {
		return errors.New("program/source: Index Source identity disagrees with authored authority")
	}
	if len(input.Bodies) != a.count(keyspace.FamilyBody) ||
		!a.validFamilyTerm(input.Entry, keyspace.FamilyBody) {
		return errors.New("program/source: incomplete Body index")
	}
	var next indexStore
	next.rootRanges = make([]termRange, a.count(keyspace.FamilyBody))
	next.parents = make([]keyspace.Term, a.count(keyspace.FamilyBody))
	if err := installBodyRoots(a, &next, input.Bodies); err != nil {
		return err
	}
	locations, err := buildDirectLocations(a, &next)
	if err != nil {
		return err
	}
	if err := validateBodyForest(a, &next, locations, input.Entry); err != nil {
		return err
	}
	next.entry = input.Entry
	if err := installOutcomeIdentity(a, input.OutcomeOrigins); err != nil {
		return err
	}
	if err := installPositions(a, &next, locations, input); err != nil {
		return err
	}
	a.index = next
	return nil
}

// installOutcomeIdentity installs the sole derived Term family from Flow's
// canonical ordered origin Bodies. Source does not know Outcome semantics or
// mint an order: it validates the typed Body foreign keys, copies each owning
// Body's authored coordinate, and assigns the dense Outcome ordinal implied by
// the supplied order. The resulting count/spans participate in final identity
// and position validation, but remain absent from authored ContentID.
func installOutcomeIdentity(a *authority, origins []keyspace.Term) error {
	if a == nil || a.identity.counts[keyspace.FamilyOutcome] != 0 ||
		len(a.identity.spans[keyspace.FamilyOutcome]) != 0 {
		return errors.New("program/source: Outcome identity was already installed")
	}
	if !keyspace.TermOrdinalFits(len(origins)) {
		return errors.New("program/source: Outcome cardinality overflow")
	}
	spans := make([]storedSpan, len(origins))
	for index, body := range origins {
		if !a.validFamilyTerm(body, keyspace.FamilyBody) {
			return errors.New("program/source: invalid Outcome origin Body")
		}
		spans[index] = a.identity.spans[keyspace.FamilyBody][keyspace.TermOrdinal(body)-1]
	}
	if uint64(a.identity.termCount)+uint64(len(spans)) > uint64(^uint32(0)) {
		return errors.New("program/source: final Term cardinality overflow")
	}
	a.identity.counts[keyspace.FamilyOutcome] = uint32(len(spans))
	a.identity.spans[keyspace.FamilyOutcome] = spans
	a.identity.termCount += uint32(len(spans))
	return nil
}

func installBodyRoots(a *authority, index *indexStore, rows []BodyRoots) error {
	for ordinal, row := range rows {
		if !a.validFamilyTerm(row.Body, keyspace.FamilyBody) || keyspace.TermOrdinal(row.Body) != uint32(ordinal+1) {
			return errors.New("program/source: invalid indexed Body")
		}
		if (row.Parent != 0 && !a.validFamilyTerm(row.Parent, keyspace.FamilyBody)) || row.Parent == row.Body {
			return errors.New("program/source: invalid Body parent")
		}
		start := len(index.rootTerms)
		for _, root := range row.Roots {
			if !a.validTerm(root) {
				return errors.New("program/source: invalid statement root")
			}
			index.rootTerms = append(index.rootTerms, root)
		}
		r, ok := makeRange(start, len(row.Roots))
		if !ok {
			return errors.New("program/source: Body root range overflow")
		}
		index.rootRanges[ordinal] = r
		index.parents[ordinal] = row.Parent
	}
	return nil
}

func installPositions(a *authority, index *indexStore, locations directLocations, input IndexInput) error {
	// Positions is the exact batch for Flow's reachable containment closure.
	// Identity/span cardinality is a separate denominator; direct Body source
	// occurrences are mandatory, while Terms outside that closure have no
	// source-position projection. Position.Term is the sole row identity, and
	// the explicit family/ordinal order is part of this boundary.
	// Allocate the retained batch exactly once. The input is already canonical
	// by (Family, Ordinal), so each family can be carved from this backing array
	// without per-family geometric growth or a sorting/counting pass.
	entries := make([]positionEntry, len(input.Positions))
	var directCounts [keyspace.FamilyCount]int
	familyStart := 0
	var installedFamily keyspace.Family
	var previousFamily keyspace.Family
	var previousOrdinal uint32
	for position, row := range input.Positions {
		if !a.validTerm(row.Term) || !a.validTerm(row.Root) || !a.validFamilyTerm(row.Body, keyspace.FamilyBody) ||
			!a.validFamilyTerm(row.FrontierBody, keyspace.FamilyBody) ||
			keyspace.TermFamily(row.Term) == keyspace.FamilyOutcome {
			return errors.New("program/source: invalid source position")
		}
		family, termOrdinal := keyspace.TermFamily(row.Term), keyspace.TermOrdinal(row.Term)
		if previousFamily != keyspace.FamilyInvalid &&
			(family < previousFamily || family == previousFamily && termOrdinal <= previousOrdinal) {
			return errors.New("program/source: noncanonical source position order")
		}
		if installedFamily != keyspace.FamilyInvalid && family != installedFamily {
			index.positions[installedFamily] = positionIndex(entries[familyStart:position:position])
			installedFamily = family
			familyStart = position
		} else if installedFamily == keyspace.FamilyInvalid {
			installedFamily = family
			familyStart = position
		}
		previousFamily, previousOrdinal = family, termOrdinal
		location, ok := locations.lookup(row.Root)
		if !ok {
			return errors.New("program/source: source position root is not a direct source Term")
		}
		if location.body != row.Body || location.offset != row.Offset || location.cursor != row.Cursor {
			return errors.New("program/source: inconsistent source position")
		}
		// A direct Body source occurrence is its own canonical source root. The
		// root lookup above proves that row.Term is a direct source row whenever
		// row.Term == row.Root. Counting those rows per family, then requiring
		// the exact direct-row count below, preserves direct omission,
		// substitution, and uniqueness without a second direct membership scan.
		if row.Term == row.Root {
			directCounts[family]++
		}
		if err := validateFrontier(index, row, location); err != nil {
			return err
		}
		entries[position] = positionEntry{
			ordinal: keyspace.TermOrdinal(row.Term),
			slot: positionSlot{
				root: row.Root, body: row.Body, offset: row.Offset, cursor: row.Cursor,
				frontierBody: row.FrontierBody, frontierCursor: row.FrontierCursor,
			},
		}
	}
	if installedFamily != keyspace.FamilyInvalid {
		index.positions[installedFamily] = positionIndex(entries[familyStart:len(entries):len(entries)])
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if directCounts[family] != len(locations[family].rows) {
			return errors.New("program/source: direct source Term lacks position")
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for _, entry := range index.positions[family] {
			slot := entry.slot
			rootFamily, rootOrdinal := keyspace.TermFamily(slot.root), keyspace.TermOrdinal(slot.root)
			if rootFamily == keyspace.FamilyInvalid {
				return errors.New("program/source: root lacks direct source position")
			}
			root, ok := index.positions[rootFamily].lookup(rootOrdinal)
			if !ok || root.root != slot.root || root.body != slot.body || root.offset != slot.offset || root.cursor != slot.cursor ||
				root.frontierBody != slot.frontierBody || root.frontierCursor != slot.frontierCursor {
				return errors.New("program/source: root position is not its direct source coordinate")
			}
		}
	}
	return nil
}

func validateFrontier(index *indexStore, row Position, location directLocation) error {
	// Flow's position seal supplies Repeat's kind and the exact Loop-to-child
	// selection. Source validates only the owner-local geometry represented by
	// this row: a direct Loop root, a Body child of the containing Body, and the
	// selected child's sealed root-tail cursor. It does not infer which of two
	// same-owner Body children Flow selected.
	if !row.Repeat {
		// A non-direct row inherits all six position fields from its direct
		// root. Defer its frontier check until the complete batch is installed;
		// this is what permits a descendant of a Repeat root to inherit that
		// root's adjusted frontier without opening a second frontier authority.
		if row.Term != row.Root {
			return nil
		}
		if row.FrontierBody != location.body || row.FrontierCursor != location.cursor {
			return errors.New("program/source: invalid ordinary source frontier")
		}
		return nil
	}
	if keyspace.TermFamily(row.Root) != keyspace.FamilyLoop || row.FrontierBody == location.body ||
		int(keyspace.TermOrdinal(row.FrontierBody)) > len(index.parents) ||
		index.parents[keyspace.TermOrdinal(row.FrontierBody)-1] != location.body {
		return errors.New("program/source: invalid Repeat source frontier")
	}
	r := index.rootRanges[keyspace.TermOrdinal(row.FrontierBody)-1]
	if row.FrontierCursor != r.end-r.start {
		return errors.New("program/source: invalid Repeat frontier cursor")
	}
	return nil
}

func validateBodyForest(a *authority, index *indexStore, locations directLocations, entry keyspace.Term) error {
	if a == nil || index == nil || !a.validFamilyTerm(entry, keyspace.FamilyBody) {
		return errors.New("program/source: invalid entry Body")
	}
	entryOrdinal := keyspace.TermOrdinal(entry) - 1
	rootCount := 0
	for ordinal, parent := range index.parents {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal+1))
		location, direct := locations.lookup(body)
		if parent == 0 {
			rootCount++
			if body != entry {
				return errors.New("program/source: non-entry root Body")
			}
			if direct {
				return errors.New("program/source: Entry Body has direct source occurrence")
			}
			continue
		}
		// A lexical Body parent is supplied by Flow's sealed Body forest. A
		// child Body may be represented only by a typed Function/Branch/Loop
		// witness, so it need not also occur as a direct Source term. When a
		// direct Body occurrence does exist, the sealed forest projection must
		// agree with that Source-owned witness.
		if direct && location.body != parent {
			return errors.New("program/source: direct Body parent mismatch")
		}
	}
	if rootCount != 1 || index.parents[entryOrdinal] != 0 {
		return errors.New("program/source: invalid Body root")
	}
	state := make([]uint8, len(index.parents))
	for start := range index.parents {
		if state[start] != 0 {
			continue
		}
		path := make([]uint32, 0, 4)
		for current := uint32(start); ; {
			if int(current) >= len(index.parents) {
				return errors.New("program/source: invalid Body parent ordinal")
			}
			switch state[current] {
			case 1:
				return errors.New("program/source: cyclic Body parent")
			case 2:
				for _, visited := range path {
					state[visited] = 2
				}
			default:
				state[current] = 1
				path = append(path, current)
				parent := index.parents[current]
				if parent == 0 {
					if current != entryOrdinal {
						return errors.New("program/source: Body forest does not reach entry")
					}
					for _, visited := range path {
						state[visited] = 2
					}
					path = nil
				}
				if path == nil {
					break
				}
				current = keyspace.TermOrdinal(parent) - 1
				continue
			}
			break
		}
	}
	return nil
}

// buildDirectLocations makes one temporary sparse validation index containing
// exactly the direct Body source occurrences. Build's authored order pass has
// already proved that those occurrences are valid and unique, so Commit need
// not allocate a second membership plane for every identity ordinal. The rows
// are discarded after position installation.
func buildDirectLocations(a *authority, index *indexStore) (directLocations, error) {
	var result directLocations
	for _, sourceRange := range a.order.bodyRanges {
		if !validRange(a.order.sourceTerms, sourceRange) {
			return directLocations{}, errors.New("program/source: invalid Body source range")
		}
		for _, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			family := keyspace.TermFamily(term)
			if !a.validDirectBodyTerm(term) || family == keyspace.FamilyInvalid {
				return directLocations{}, errors.New("program/source: invalid direct source Term")
			}
		}
	}
	for bodyOrdinal, sourceRange := range a.order.bodyRanges {
		rootRange := index.rootRanges[bodyOrdinal]
		rootAt := uint32(0)
		cursor := uint32(0)
		for offset, term := range a.order.sourceTerms[sourceRange.start:sourceRange.end] {
			family := keyspace.TermFamily(term)
			location := directLocation{
				term:   term,
				body:   keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal+1)),
				offset: uint32(offset),
				cursor: cursor,
			}
			if err := result[family].add(keyspace.TermOrdinal(term), location); err != nil {
				return directLocations{}, err
			}
			if rootAt < rootRange.end-rootRange.start && index.rootTerms[rootRange.start+rootAt] == term {
				rootAt++
				cursor++
			}
		}
		if rootAt != rootRange.end-rootRange.start {
			return directLocations{}, errors.New("program/source: unordered or non-direct statement root")
		}
	}
	return result, nil
}

func (a *authority) count(family keyspace.Family) int {
	if a == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount {
		return 0
	}
	return int(a.identity.counts[family])
}

func (a *authority) validTerm(term keyspace.Term) bool {
	return a != nil && keyspace.ValidTerm(term, keyspace.TermFamily(term), a.count(keyspace.TermFamily(term)))
}

// validDirectBodyTerm is the exact Source Body-order denominator.  It mirrors
// Flow's body sourceDirectFamily law without importing Flow: only the
// statement/owner families below may occur directly in authored Body order.
func (a *authority) validDirectBodyTerm(term keyspace.Term) bool {
	return a != nil && sourceDirectFamily(keyspace.TermFamily(term)) && a.validTerm(term)
}

// sourceDirectFamily is Source's copy of the canonical direct-Body family
// boundary.  It is shared by Build and the artifact decoder; neither side may
// admit a family merely because its ordinal exists in Source's census.
func sourceDirectFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyLabel, keyspace.FamilyControlFault,
		keyspace.FamilyTypeAlias, keyspace.FamilyTypeInterface:
		return true
	default:
		return false
	}
}

func (a *authority) validFamilyTerm(term keyspace.Term, family keyspace.Family) bool {
	return a != nil && keyspace.ValidTerm(term, family, a.count(family))
}

type termMarks struct{ rows [keyspace.FamilyCount][]bool }

func newTermMarks(counts [keyspace.FamilyCount]uint32) termMarks {
	var result termMarks
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		result.rows[family] = make([]bool, counts[family])
	}
	return result
}

func (s *termMarks) take(term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family == keyspace.FamilyInvalid || ordinal == 0 || uint64(ordinal) > uint64(len(s.rows[family])) {
		return true
	}
	at := ordinal - 1
	if s.rows[family][at] {
		return true
	}
	s.rows[family][at] = true
	return false
}

func makeRange(start, count int) (termRange, bool) {
	if start < 0 || count < 0 || uint64(start)+uint64(count) > uint64(^uint32(0)) {
		return termRange{}, false
	}
	return termRange{start: uint32(start), end: uint32(start + count)}, true
}

func compactSpan(name string, span Span) (storedSpan, bool) {
	allZero := span.StartLine == 0 && span.StartCol == 0 && span.EndLine == 0 && span.EndCol == 0
	if name == "" || span.File != name && (span.File != "" || !allZero) {
		return storedSpan{}, false
	}
	if allZero {
		return storedSpan{}, true
	}
	if span.StartLine == 0 || span.StartCol == 0 ||
		(span.EndLine == 0) != (span.EndCol == 0) ||
		span.EndLine != 0 && (span.EndLine < span.StartLine || span.EndLine == span.StartLine && span.EndCol < span.StartCol) {
		return storedSpan{}, false
	}
	return storedSpan{startLine: span.StartLine, startCol: span.StartCol, endLine: span.EndLine, endCol: span.EndCol}, true
}

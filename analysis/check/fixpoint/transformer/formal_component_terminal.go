package transformer

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

var (
	errFormalComponentMalformed      = errors.New("transformer: malformed formal component terminal")
	errFormalComponentForeignOwner   = errors.New("transformer: foreign formal component owner")
	errFormalSymbolicPathCorrelation = errors.New("transformer: symbolic path requires an atomic value/path binding carrier")
	errFormalSymbolicMeetUnproven    = errors.New("transformer: distinct symbolic sets have no proven pointwise meet")
	errFormalOccurrenceConflict      = errors.New("transformer: distinct occurrences overlap one formal fiber")
	errFormalCoordinateGroupRequired = errors.New("transformer: coordinate operation requires its complete registered family group")
)

// formalComponentKind is a closed physical terminal vocabulary. Registered
// product semantics never dispatch through this tag: their opaque descriptors
// are passed back to the owning ProductDomain.
type formalComponentKind uint8

const (
	formalComponentInvalid formalComponentKind = iota
	formalComponentBindings
	formalComponentPathTerms
	formalComponentOutcomeOccurrence
	formalComponentDiagnostics
	formalComponentCallOutcomes
	formalComponentRawCallOutcome
	formalComponentOrdinaryLane
	formalComponentCoordinateSkeleton
	formalComponentCoordinateScalar
	formalComponentGroundValue
	formalComponentSymbolicValue
	formalComponentTypestateResourceObservation
)

type formalComponentBinaryOp uint8

const (
	formalComponentJoin formalComponentBinaryOp = iota + 1
	formalComponentMeet
	formalComponentWiden
	formalComponentNarrow
)

type formalComponentDefaultKind uint8

const (
	formalComponentDefaultInvalid formalComponentDefaultKind = iota
	// Absent is the exact zero for symbolic bindings and occurrence syntax.
	formalComponentDefaultAbsent
	// BooleanFalse is consumed directly by the shared decision kernel; it is
	// not an interned typed payload.
	formalComponentDefaultBooleanFalse
	formalComponentDefaultTerminal
	// Coordinate defaults are resolved only while reconstructing the complete
	// registered family group because scalar defaults depend on its skeleton.
	formalComponentDefaultCoordinateGroup
)

type formalComponentDefault struct {
	kind formalComponentDefaultKind
	leaf decisionLeaf
}

// Arena qualification is part of symbolic identity. Numeric term references
// from two bodies are never interchangeable merely because their ordinals
// happen to match.
type formalQualifiedPathTerm struct {
	arena *Arena
	term  PathTerm
}

type formalQualifiedOutcomeOccurrence struct {
	code  *relationCode
	ref   boundaryOutcomeRef
	root  relationRootRef
	scope loopMuTerm
}

// formalComponentTerminal is private typed storage. decisionLeaf is only the
// dense index of one entry in its owning authority; it never encodes a product
// factor, term reference, route, or State.
type formalComponentTerminal struct {
	kind                         formalComponentKind
	bindings                     []formalQualifiedBinding
	pathTerms                    []formalQualifiedPathTerm
	outcome                      formalQualifiedOutcomeOccurrence
	diagnostics                  callpayload.DiagnosticOutput
	callOutcomes                 callpayload.CallOutcomeAlternativeSet
	rawCallOutcome               callpayload.CallOutcome
	lane                         state.LaneFactor
	skeleton                     state.CoordinateFamilySkeleton
	scalar                       state.CoordinateScalarFactor
	ground                       product.Value
	symbolicValue                formalSymbolicValueSet
	typestateResourceObservation state.TypestateResourceObservation
	fingerprint                  uint64
}

type formalComponentBucket struct {
	owner       *formalComponentTerminalBody
	kind        formalComponentKind
	lane        state.LaneOrdinal
	family      state.CoordinateFamilyOrdinal
	width       uint32
	fingerprint uint64
}

type formalOwnedComponentTerminal struct {
	owner    *formalComponentTerminalBody
	terminal formalComponentTerminal
}

// formalComponentTerminalSchema is immutable forest metadata and is safe to
// retain in RelationProgram. It owns no solve history, fingerprint scratch,
// decision nodes, terminal leaves, or persistent-directory roots.
type formalComponentTerminalSchema struct {
	fibers *formalFiberInventory
	bodies []*formalComponentTerminalBody
}

type formalComponentTerminalBody struct {
	schema         *formalComponentTerminalSchema
	variable       relationVar
	terms          *Arena
	code           *relationCode
	product        state.ProductDomain
	keys           *keyspace.KeySpace
	coordinateKeys *keyspace.KeySpace
	factors        formalFactorExecutionLayout
}

// formalFactorExecutionLayout is the immutable dense product layout shared by
// every tuple leaf of one body. Group descriptors are cloned once at schema
// freeze; hot evaluators neither rediscover groups nor allocate lane
// inventories.
type formalFactorExecutionLayout struct {
	values    formalFiberGroupDescriptor
	nonValues []formalFiberGroupDescriptor
	// members is the one frozen Values-then-residual dense product projection.
	// offsets index nonValues within that compact vector.
	members     []formalFiberOrdinal
	offsets     []int
	vectorWidth int
	variable    relationVar
	sealed      bool
}

func (l *formalFactorExecutionLayout) validFor(domain state.ProductDomain, variable relationVar) bool {
	return l != nil && l.sealed && domain.Valid() && l.variable == variable && l.values.valid() &&
		l.values.variable == variable && len(l.nonValues) == domain.NonValuesLaneCount()
}

func (l *formalFactorExecutionLayout) validateForFreeze(domain state.ProductDomain, variable relationVar) bool {
	if l == nil || !domain.Valid() || !l.values.valid() || l.values.variable != variable || len(l.nonValues) != domain.NonValuesLaneCount() {
		return false
	}
	width := 0
	for index, group := range l.nonValues {
		lane, ok := domain.NonValuesLaneAt(index)
		if !ok || !group.valid() || group.variable != variable || group.kind == formalFiberGroupValues || group.lane != lane {
			return false
		}
		width += len(group.members)
	}
	return width == l.vectorWidth
}

// formalComponentTerminalArena is one run/solve transaction. The shared guard
// DD consumes its forest-global leaves, but repeated or concurrent Runs receive
// distinct arenas and cannot retain or race on terminal/fingerprint history.
type formalComponentTerminalArena struct {
	schema      *formalComponentTerminalSchema
	terminals   []formalOwnedComponentTerminal
	buckets     map[formalComponentBucket][]decisionLeaf
	authorities []*formalComponentTerminalAuthority
}

// formalComponentTerminalAuthority is a run-bound view of one immutable body
// authority. Product ownership comes from body; leaf allocation and scratch
// come from arena.
type formalComponentTerminalAuthority struct {
	arena           *formalComponentTerminalArena
	body            *formalComponentTerminalBody
	variable        relationVar
	terms           *Arena
	code            *relationCode
	product         state.ProductDomain
	keys            *keyspace.KeySpace
	coordinateKeys  *keyspace.KeySpace
	workspace       *state.FingerprintWorkspace
	valueValidation map[ValueTerm]bool
}

func freezeFormalComponentTerminalSchema(program *RelationProgram) (*formalComponentTerminalSchema, error) {
	if program == nil || len(program.bodies) == 0 || program.formalFibers == nil {
		return nil, errFormalComponentForeignOwner
	}
	schema := &formalComponentTerminalSchema{fibers: program.formalFibers, bodies: make([]*formalComponentTerminalBody, len(program.bodies))}
	for index := range program.bodies {
		body, err := freezeFormalComponentTerminalBody(schema, &program.bodies[index])
		if err != nil {
			return nil, fmt.Errorf("transformer: formal terminal authority for relation %d: %w", index+1, err)
		}
		schema.bodies[index] = body
	}
	return schema, nil
}

func freezeFormalComponentTerminalBody(schema *formalComponentTerminalSchema, body *relationProgramBody) (*formalComponentTerminalBody, error) {
	if body == nil || body.variable == 0 || body.relation.arena == nil || body.relation.effects == nil ||
		body.relation.effects.Terms() != body.relation.arena || !body.relation.effects.Sealed() ||
		body.relation.code == nil || !body.relation.code.sealed || body.relation.code.terms != body.relation.arena ||
		body.relation.code.effects != body.relation.effects || body.keys == nil || !body.keys.Valid() ||
		!body.productDomain.Valid() || body.productDomain.Registry() != body.relation.arena.reg {
		return nil, errFormalComponentForeignOwner
	}
	if schema == nil || schema.fibers == nil {
		return nil, errFormalComponentForeignOwner
	}
	span, ok := schema.fibers.span(body.variable)
	if !ok || span.forest != schema.fibers || span.keys == nil || !span.keys.Valid() {
		return nil, errFormalComponentForeignOwner
	}
	if err := validateFormalFiberProductGroups(span, body.productDomain); err != nil {
		return nil, fmt.Errorf("%w: %v", errFormalComponentForeignOwner, err)
	}
	layout := formalFactorExecutionLayout{variable: body.variable}
	for _, group := range span.groupDescriptors() {
		if group.kind == formalFiberGroupValues {
			if layout.values.valid() {
				return nil, fmt.Errorf("%w: duplicate Values execution group", errFormalComponentForeignOwner)
			}
			layout.values = group
			layout.members = append(layout.members, group.members...)
			continue
		}
		layout.offsets = append(layout.offsets, len(layout.members))
		layout.nonValues = append(layout.nonValues, group)
		layout.members = append(layout.members, group.members...)
		layout.vectorWidth += len(group.members)
	}
	if !layout.validateForFreeze(body.productDomain, body.variable) {
		return nil, fmt.Errorf("%w: incomplete factor execution layout", errFormalComponentForeignOwner)
	}
	layout.sealed = true
	return &formalComponentTerminalBody{
		schema:   schema,
		variable: body.variable, terms: body.relation.arena,
		code: body.relation.code, product: body.productDomain, keys: body.keys, coordinateKeys: span.keys, factors: layout,
	}, nil
}

func newFormalComponentTerminalArena(schema *formalComponentTerminalSchema) (*formalComponentTerminalArena, error) {
	if schema == nil || schema.fibers == nil || len(schema.bodies) == 0 {
		return nil, errFormalComponentForeignOwner
	}
	arena := &formalComponentTerminalArena{
		schema: schema, terminals: make([]formalOwnedComponentTerminal, 2),
		buckets:     make(map[formalComponentBucket][]decisionLeaf),
		authorities: make([]*formalComponentTerminalAuthority, len(schema.bodies)),
	}
	for index, body := range schema.bodies {
		if body == nil || body.schema != schema || body.variable != relationVar(index+1) {
			return nil, errFormalComponentForeignOwner
		}
		// Every retained lane factor has already crossed the formal root-image
		// boundary. Fingerprint it in that destination keyspace; using the
		// concrete body keyspace silently worked only for key-independent lanes.
		workspace, err := state.NewFingerprintWorkspace(body.coordinateKeys)
		if err != nil {
			return nil, err
		}
		arena.authorities[index] = &formalComponentTerminalAuthority{
			arena: arena, body: body, variable: body.variable, terms: body.terms,
			code: body.code, product: body.product, keys: body.keys,
			coordinateKeys: body.coordinateKeys, workspace: workspace,
		}
	}
	return arena, nil
}

func (a *formalComponentTerminalArena) authority(variable relationVar) (*formalComponentTerminalAuthority, bool) {
	if a == nil || variable == 0 || int(variable) > len(a.authorities) {
		return nil, false
	}
	authority := a.authorities[variable-1]
	return authority, authority != nil && authority.arena == a && authority.variable == variable
}

func (a *formalComponentTerminalAuthority) terminal(leaf decisionLeaf) (formalComponentTerminal, error) {
	if a == nil || a.arena == nil || a.body == nil || leaf < 2 || int(leaf) >= len(a.arena.terminals) {
		return formalComponentTerminal{}, errFormalComponentForeignOwner
	}
	owned := a.arena.terminals[leaf]
	if owned.owner != a.body {
		return formalComponentTerminal{}, errFormalComponentForeignOwner
	}
	if owned.terminal.kind == formalComponentInvalid {
		return formalComponentTerminal{}, errFormalComponentMalformed
	}
	return owned.terminal, nil
}

func (a *formalComponentTerminalArena) append(owner *formalComponentTerminalAuthority, terminal formalComponentTerminal, bucket formalComponentBucket) (decisionLeaf, error) {
	if a == nil || owner == nil || owner.arena != a || owner.body == nil || uint64(len(a.terminals)) > uint64(^decisionLeaf(0)) {
		return 0, fmt.Errorf("%w: terminal inventory exceeds address space", errFormalComponentMalformed)
	}
	bucket.owner = owner.body
	leaf := decisionLeaf(len(a.terminals))
	a.terminals = append(a.terminals, formalOwnedComponentTerminal{owner: owner.body, terminal: terminal})
	a.buckets[bucket] = append(a.buckets[bucket], leaf)
	return leaf, nil
}

func formalComponentMix(left, right uint64) uint64 {
	left ^= right + 0x9e3779b97f4a7c15 + left<<6 + left>>2
	return left
}

func (a *formalComponentTerminalAuthority) internBinding(value formalQualifiedBinding) (decisionLeaf, error) {
	if a == nil || a.terms == nil || !a.terms.Sealed() || a.code == nil ||
		!value.validForAuthority(a) {
		return 0, errFormalComponentForeignOwner
	}
	node := value.value.arena.values[value.value.term]
	if value.apply.present() {
		// The frame is part of this immutable alternative.  Flattening a target
		// join here would discard that environment and recreate the very
		// caller-owned syntax which the Apply view removes.
		return a.internCanonicalBindings([]formalQualifiedBinding{value})
	}
	if node.op != valueJoin {
		return a.internCanonicalBindings([]formalQualifiedBinding{value})
	}
	if value.pathPresent {
		// A joined ValueTerm has no single correlated address. Its alternatives
		// must be introduced as distinct atomic bindings by the producer.
		return 0, errFormalSymbolicPathCorrelation
	}
	// Arena.JoinValue already seals this syntax as a flat, sorted, compact
	// argument vector. Preserve that canonical order directly: rebuilding a
	// map here paid an allocation on every symbolic read without proving any
	// additional property.
	values := make([]formalQualifiedBinding, len(node.args))
	for index, term := range node.args {
		values[index] = formalQualifiedBinding{value: relationArenaValueRef{owner: a.variable, arena: a.terms, term: term}}
	}
	return a.internCanonicalBindings(values)
}

// internBindings interns one exact finite collecting set of correlated value
// and optional-path bindings over the existing sealed caller arena.
func (a *formalComponentTerminalAuthority) internBindings(values []formalQualifiedBinding) (decisionLeaf, error) {
	if a == nil || a.terms == nil || !a.terms.Sealed() || a.code == nil {
		return 0, errFormalComponentForeignOwner
	}
	if len(values) == 1 {
		return a.internBinding(values[0])
	}
	terms := make([]formalQualifiedBinding, 0, len(values))
	for _, value := range values {
		if !value.validForAuthority(a) {
			return 0, errFormalComponentForeignOwner
		}
		node := value.value.arena.values[value.value.term]
		if !value.apply.present() && node.op == valueJoin && !value.pathPresent {
			for _, term := range node.args {
				terms = append(terms, formalQualifiedBinding{value: relationArenaValueRef{owner: a.variable, arena: a.terms, term: term}})
			}
			continue
		}
		terms = append(terms, value)
	}
	if len(terms) == 0 {
		return 0, nil
	}
	sort.Slice(terms, func(i, j int) bool { return formalQualifiedBindingLess(terms[i], terms[j]) })
	terms = compactFormalQualifiedBindings(terms)
	return a.internCanonicalBindings(terms)
}

func (a *formalComponentTerminalAuthority) validFormalValue(term ValueTerm) bool {
	if a == nil || a.terms == nil || term == 0 || int(term) >= len(a.terms.values) {
		return false
	}
	// Root terms are sealed leaves. Validate their shape directly instead of
	// entering the graph walk: allocating traversal scratch for the dominant
	// one-node lookup would make every formal operand read allocate.
	if node := a.terms.values[term]; node.op == valueRoot {
		return a.terms.Sealed() && a.terms.validRoot(a.code.shape, node.root)
	}
	// The authority is run-owned scratch, just like its fingerprint workspace.
	// Reusing this map removes one allocation per symbolic terminal while the
	// existing Arena validator remains the single ownership/shape authority.
	if a.valueValidation == nil {
		a.valueValidation = make(map[ValueTerm]bool)
	} else {
		clear(a.valueValidation)
	}
	return a.terms.validValue(term, a.code.shape, a.valueValidation)
}

func (a *formalComponentTerminalAuthority) internCanonicalBindings(values []formalQualifiedBinding) (decisionLeaf, error) {
	if len(values) == 0 {
		return 0, nil
	}
	if uint64(len(values)) > uint64(^uint32(0)) {
		return 0, errFormalComponentMalformed
	}
	fingerprint := formalComponentSetFingerprint(formalComponentBindings, func(index int) uint64 { return values[index].fingerprint() }, len(values))
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentBindings, width: uint32(len(values)), fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		terminal := a.arena.terminals[leaf].terminal
		if formalQualifiedBindingsEqual(terminal.bindings, values) {
			return leaf, nil
		}
	}
	owned := append([]formalQualifiedBinding(nil), values...)
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentBindings, bindings: owned, fingerprint: fingerprint}, bucket)
}

func (a *formalComponentTerminalAuthority) internPathTerm(value formalQualifiedPathTerm) (decisionLeaf, error) {
	if a == nil || a.terms == nil || !a.terms.Sealed() || a.code == nil ||
		value.arena != a.terms || value.term == 0 || !a.terms.validPath(value.term, a.code.shape) {
		return 0, errFormalComponentForeignOwner
	}
	return a.internCanonicalPathTerms([]formalQualifiedPathTerm{value})
}

func (a *formalComponentTerminalAuthority) internPathTerms(values []formalQualifiedPathTerm) (decisionLeaf, error) {
	if a == nil || a.terms == nil || !a.terms.Sealed() || a.code == nil {
		return 0, errFormalComponentForeignOwner
	}
	if len(values) == 1 {
		return a.internPathTerm(values[0])
	}
	terms := make([]formalQualifiedPathTerm, 0, len(values))
	for _, value := range values {
		if value.arena != a.terms || value.term == 0 || !a.terms.validPath(value.term, a.code.shape) {
			return 0, errFormalComponentForeignOwner
		}
		terms = append(terms, value)
	}
	if len(terms) == 0 {
		return 0, nil
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].term < terms[j].term })
	terms = compactFormalQualifiedPathTerms(terms)
	return a.internCanonicalPathTerms(terms)
}

func (a *formalComponentTerminalAuthority) internCanonicalPathTerms(terms []formalQualifiedPathTerm) (decisionLeaf, error) {
	if len(terms) == 0 {
		return 0, nil
	}
	if uint64(len(terms)) > uint64(^uint32(0)) {
		return 0, errFormalComponentMalformed
	}
	fingerprint := formalComponentSetFingerprint(formalComponentPathTerms, func(index int) uint64 { return uint64(terms[index].term) }, len(terms))
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentPathTerms, width: uint32(len(terms)), fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		terminal := a.arena.terminals[leaf].terminal
		if formalQualifiedPathTermsEqual(terminal.pathTerms, terms) {
			return leaf, nil
		}
	}
	owned := append([]formalQualifiedPathTerm(nil), terms...)
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentPathTerms, pathTerms: owned, fingerprint: fingerprint}, bucket)
}

func formalComponentSetFingerprint(kind formalComponentKind, term func(int) uint64, count int) uint64 {
	hash := formalComponentMix(0x666f726d616c7365, uint64(kind))
	hash = formalComponentMix(hash, uint64(count))
	for index := 0; index < count; index++ {
		hash = formalComponentMix(hash, term(index))
	}
	return hash
}

func formalQualifiedPathTermsEqual(left, right []formalQualifiedPathTerm) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func compactFormalQualifiedPathTerms(terms []formalQualifiedPathTerm) []formalQualifiedPathTerm {
	if len(terms) == 0 {
		return terms
	}
	out := terms[:1]
	for _, term := range terms[1:] {
		if term.term != out[len(out)-1].term {
			out = append(out, term)
		}
	}
	return out
}

func formalQualifiedPathTermsSubset(left, right []formalQualifiedPathTerm) bool {
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		switch {
		case left[leftIndex].term == right[rightIndex].term:
			leftIndex++
			rightIndex++
		case left[leftIndex].term > right[rightIndex].term:
			rightIndex++
		default:
			return false
		}
	}
	return leftIndex == len(left)
}

func unionFormalQualifiedPathTerms(left, right []formalQualifiedPathTerm) []formalQualifiedPathTerm {
	out := make([]formalQualifiedPathTerm, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		switch {
		case left[leftIndex].term == right[rightIndex].term:
			out = append(out, left[leftIndex])
			leftIndex++
			rightIndex++
		case left[leftIndex].term < right[rightIndex].term:
			out = append(out, left[leftIndex])
			leftIndex++
		default:
			out = append(out, right[rightIndex])
			rightIndex++
		}
	}
	out = append(out, left[leftIndex:]...)
	out = append(out, right[rightIndex:]...)
	return out
}

func (a *formalComponentTerminalAuthority) internOutcomeOccurrence(value formalQualifiedOutcomeOccurrence) (decisionLeaf, error) {
	if a == nil || value.code != a.code || value.ref == 0 || int(value.ref) >= len(a.code.outcomes) ||
		value.root == 0 || int(value.root) >= len(a.code.nodes) ||
		a.code.nodes[value.root].kind != relationNodeOutcome || a.code.nodes[value.root].outcome != value.ref {
		return 0, errFormalComponentForeignOwner
	}
	fingerprint := formalComponentMix(uint64(value.ref), uint64(value.root))
	fingerprint = formalComponentMix(fingerprint, uint64(value.scope))
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentOutcomeOccurrence, fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		if a.arena.terminals[leaf].terminal.outcome == value {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentOutcomeOccurrence, outcome: value, fingerprint: bucket.fingerprint}, bucket)
}

func (a *formalComponentTerminalAuthority) internDiagnostics(value callpayload.DiagnosticOutput) (decisionLeaf, error) {
	if a == nil || a.product.Registry() == nil || !value.Valid(a.product.Registry()) ||
		diagnosticContainsAllocationTemplate(a.product.Registry(), value) {
		return 0, errFormalComponentForeignOwner
	}
	value = value.Normalize(a.product.Registry())
	fingerprint := value.Fingerprint(a.product.Registry())
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentDiagnostics, fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		if prior.diagnostics.RepresentationEqual(a.product.Registry(), value) {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{
		kind: formalComponentDiagnostics, diagnostics: value.Clone(), fingerprint: fingerprint,
	}, bucket)
}

func (a *formalComponentTerminalAuthority) internCallOutcomes(value callpayload.CallOutcomeAlternativeSet) (decisionLeaf, error) {
	if a == nil || a.product.Registry() == nil {
		return 0, errFormalComponentForeignOwner
	}
	value = value.Normalize(a.product.Registry())
	fingerprint := value.Fingerprint(a.product.Registry())
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentCallOutcomes, fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		if prior.callOutcomes.Equal(a.product.Registry(), value) {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{
		kind: formalComponentCallOutcomes, callOutcomes: value.Normalize(a.product.Registry()), fingerprint: fingerprint,
	}, bucket)
}

// internRawCallOutcome retains provider syntax only until its enclosing
// composition tree has completed. Raw outcomes are never directory payloads
// or lattice values; normalization begins only after the complete root exists,
// at the canonical CallOutcomeAlternativeSet interning boundary.
func (a *formalComponentTerminalAuthority) internRawCallOutcome(value callpayload.CallOutcome) (decisionLeaf, error) {
	if a == nil || a.product.Registry() == nil {
		return 0, errFormalComponentForeignOwner
	}
	fingerprint := callpayload.FingerprintCallOutcomeRepresentation(a.product.Registry(), value)
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentRawCallOutcome, fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		if callpayload.CallOutcomeRepresentationEqual(prior.rawCallOutcome, value) {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{
		kind: formalComponentRawCallOutcome, rawCallOutcome: value.Clone(), fingerprint: fingerprint,
	}, bucket)
}

func (a *formalComponentTerminalAuthority) rawCallOutcomeTerminalCount() int {
	if a == nil || a.arena == nil {
		return 0
	}
	count := 0
	for _, owned := range a.arena.terminals {
		if owned.owner == a.body && owned.terminal.kind == formalComponentRawCallOutcome {
			count++
		}
	}
	return count
}

func (a *formalComponentTerminalAuthority) internTypestateResourceObservation(value state.TypestateResourceObservation) (decisionLeaf, error) {
	if a == nil {
		return 0, errFormalComponentForeignOwner
	}
	fingerprint := value.Fingerprint()
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentTypestateResourceObservation, fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		if prior.typestateResourceObservation.Equal(value) {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{
		kind: formalComponentTypestateResourceObservation, typestateResourceObservation: value, fingerprint: fingerprint,
	}, bucket)
}

func formalCallOutcomeSetSubset(reg *axis.Registry, left, right callpayload.CallOutcomeAlternativeSet) bool {
	if reg == nil {
		return false
	}
	leftOutcomes, rightOutcomes := left.Outcomes(), right.Outcomes()
	for _, candidate := range leftOutcomes {
		present := false
		for _, target := range rightOutcomes {
			if callpayload.EqualCallOutcome(reg, candidate, target) {
				present = true
				break
			}
		}
		if !present {
			return false
		}
	}
	return true
}

func formalCallOutcomeSetMeet(reg *axis.Registry, left, right callpayload.CallOutcomeAlternativeSet) callpayload.CallOutcomeAlternativeSet {
	if reg == nil {
		return callpayload.CallOutcomeAlternativeSet{}
	}
	var intersection []callpayload.CallOutcome
	for _, candidate := range left.Outcomes() {
		for _, target := range right.Outcomes() {
			if callpayload.EqualCallOutcome(reg, candidate, target) {
				intersection = append(intersection, candidate)
				break
			}
		}
	}
	return callpayload.NewCallOutcomeAlternativeSet(reg, intersection...)
}

func (a *formalComponentTerminalAuthority) internLane(ctx context.Context, value state.LaneFactor) (decisionLeaf, error) {
	if a == nil || ctx == nil {
		return 0, errFormalComponentForeignOwner
	}
	fingerprint, err := a.product.LaneFingerprint(state.FingerprintConfig{
		Context: ctx, Registry: a.product.Registry(), KeySpace: a.coordinateKeys, Workspace: a.workspace,
	}, value)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errFormalComponentForeignOwner, err)
	}
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentOrdinaryLane, lane: value.Lane().Ordinal(), fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		same, sameErr := a.product.LaneCanonicalRepresentationEqual(prior.lane, value)
		if sameErr != nil {
			return 0, sameErr
		}
		if same {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentOrdinaryLane, lane: value, fingerprint: fingerprint}, bucket)
}

func (a *formalComponentTerminalAuthority) internCoordinateSkeleton(value state.CoordinateFamilySkeleton) (decisionLeaf, error) {
	if a == nil || a.coordinateKeys == nil || !a.coordinateKeys.Valid() {
		return 0, errFormalComponentForeignOwner
	}
	// A coordinate payload is meaningful only in the formal keyspace retained by
	// this body's descriptor span.  Hashing the opaque payload alone validates
	// its ProductDomain owner, but not its keyspace owner: a concrete and formal
	// skeleton may otherwise have the same representation hash.  Pairing with
	// the authority-owned Bottom invokes the registered pair validator without
	// importing or rewriting the payload.
	ownedBottom, err := a.product.CoordinateSkeletonBottom(value.Family(), a.coordinateKeys)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errFormalComponentForeignOwner, err)
	}
	if _, err := a.product.CoordinateSkeletonRepresentationEqual(ownedBottom, value); err != nil {
		return 0, fmt.Errorf("%w: %v", errFormalComponentForeignOwner, err)
	}
	fingerprint, err := a.product.CoordinateSkeletonRepresentationHash(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errFormalComponentForeignOwner, err)
	}
	family := value.Family()
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentCoordinateSkeleton,
		lane: family.Lane().Ordinal(), family: family.Ordinal(), fingerprint: fingerprint}
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		same, sameErr := a.product.CoordinateSkeletonRepresentationEqual(prior.skeleton, value)
		if sameErr != nil {
			return 0, sameErr
		}
		if same {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentCoordinateSkeleton, skeleton: value, fingerprint: fingerprint}, bucket)
}

func (a *formalComponentTerminalAuthority) internCoordinateScalar(value state.CoordinateScalarFactor) (decisionLeaf, error) {
	if a == nil || a.coordinateKeys == nil || !a.coordinateKeys.Valid() {
		return 0, errFormalComponentForeignOwner
	}
	// Ownership is an exact factor/keyspace predicate. Building and sorting a
	// singleton inventory here would re-run an admission operation for every
	// scalar encountered by every WTO equation.
	if !a.product.OwnsCoordinateScalarFactor(a.coordinateKeys, value) {
		return 0, fmt.Errorf("%w: coordinate scalar keyspace", errFormalComponentForeignOwner)
	}
	fingerprint, err := a.product.CoordinateScalarHash(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errFormalComponentForeignOwner, err)
	}
	family := value.Slot().Family()
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentCoordinateScalar,
		lane: family.Lane().Ordinal(), family: family.Ordinal(), fingerprint: fingerprint}
	// ProductDomain intentionally exposes semantic scalar equality, not a
	// representation-identity predicate. A merely equal prior scalar therefore
	// cannot authorize operand reuse; complete family-group recombination owns
	// canonical scalar publication.
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		equal, equalErr := a.product.CoordinateScalarRepresentationEqual(prior.scalar, value)
		if equalErr != nil {
			return 0, equalErr
		}
		if equal {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentCoordinateScalar, scalar: value, fingerprint: fingerprint}, bucket)
}

func (a *formalComponentTerminalAuthority) internGroundValue(value product.Value) (decisionLeaf, error) {
	if a == nil || !product.BelongsToRegistry(a.product.Registry(), value) {
		return 0, errFormalComponentForeignOwner
	}
	fingerprint := product.Hash(a.product.Registry(), value)
	bucket := formalComponentBucket{owner: a.body, kind: formalComponentGroundValue, fingerprint: fingerprint}
	domain := product.Domain(a.product.Registry())
	for _, leaf := range a.arena.buckets[bucket] {
		prior := a.arena.terminals[leaf].terminal
		if domain.Same(prior.ground, value) {
			return leaf, nil
		}
	}
	return a.arena.append(a, formalComponentTerminal{kind: formalComponentGroundValue, ground: value, fingerprint: fingerprint}, bucket)
}

// same is representation identity, not lattice equality. It is the only
// lawful operand-reuse certificate for registered factors.
func (a *formalComponentTerminalAuthority) same(left, right decisionLeaf) (bool, error) {
	if left == right {
		_, err := a.terminal(left)
		return err == nil, err
	}
	l, err := a.terminal(left)
	if err != nil {
		return false, err
	}
	r, err := a.terminal(right)
	if err != nil || l.kind != r.kind {
		return false, errFormalComponentMalformed
	}
	switch l.kind {
	case formalComponentBindings:
		return formalQualifiedBindingsEqual(l.bindings, r.bindings), nil
	case formalComponentPathTerms:
		return formalQualifiedPathTermsEqual(l.pathTerms, r.pathTerms), nil
	case formalComponentOutcomeOccurrence:
		return l.outcome == r.outcome, nil
	case formalComponentDiagnostics:
		return l.diagnostics.RepresentationEqual(a.product.Registry(), r.diagnostics), nil
	case formalComponentCallOutcomes:
		return l.callOutcomes.Equal(a.product.Registry(), r.callOutcomes), nil
	case formalComponentOrdinaryLane:
		return a.product.LaneCanonicalRepresentationEqual(l.lane, r.lane)
	case formalComponentCoordinateSkeleton:
		return a.product.CoordinateSkeletonRepresentationEqual(l.skeleton, r.skeleton)
	case formalComponentCoordinateScalar:
		return a.product.CoordinateScalarRepresentationEqual(l.scalar, r.scalar)
	case formalComponentGroundValue:
		return product.Domain(a.product.Registry()).Same(l.ground, r.ground), nil
	case formalComponentSymbolicValue:
		return l.symbolicValue.equal(a.product.Registry(), r.symbolicValue), nil
	case formalComponentTypestateResourceObservation:
		return l.typestateResourceObservation.Equal(r.typestateResourceObservation), nil
	default:
		return false, errFormalComponentMalformed
	}
}

// equal is semantic lattice equality. It is deliberately distinct from same:
// a registered operator may produce a lattice-equal spelling which must remain
// a separate terminal even though convergence can stop on Equal.
func (a *formalComponentTerminalAuthority) equal(left, right decisionLeaf) (bool, error) {
	l, err := a.terminal(left)
	if err != nil {
		return false, err
	}
	r, err := a.terminal(right)
	if err != nil || l.kind != r.kind {
		return false, errFormalComponentMalformed
	}
	switch l.kind {
	case formalComponentBindings:
		return formalQualifiedBindingsEqual(l.bindings, r.bindings), nil
	case formalComponentPathTerms:
		return formalQualifiedPathTermsEqual(l.pathTerms, r.pathTerms), nil
	case formalComponentOutcomeOccurrence:
		return l.outcome == r.outcome, nil
	case formalComponentDiagnostics:
		return l.diagnostics.Equal(a.product.Registry(), r.diagnostics), nil
	case formalComponentCallOutcomes:
		return l.callOutcomes.Equal(a.product.Registry(), r.callOutcomes), nil
	case formalComponentOrdinaryLane:
		return a.product.LaneEqual(l.lane, r.lane)
	case formalComponentCoordinateSkeleton, formalComponentCoordinateScalar:
		return false, errFormalCoordinateGroupRequired
	case formalComponentGroundValue:
		return product.Equal(a.product.Registry(), l.ground, r.ground), nil
	case formalComponentSymbolicValue:
		return l.symbolicValue.equal(a.product.Registry(), r.symbolicValue), nil
	case formalComponentTypestateResourceObservation:
		return l.typestateResourceObservation.Equal(r.typestateResourceObservation), nil
	default:
		return false, errFormalComponentMalformed
	}
}

func (a *formalComponentTerminalAuthority) lessOrEq(left, right decisionLeaf) (bool, error) {
	l, err := a.terminal(left)
	if err != nil {
		return false, err
	}
	r, err := a.terminal(right)
	if err != nil || l.kind != r.kind {
		return false, errFormalComponentMalformed
	}
	switch l.kind {
	case formalComponentBindings:
		return formalQualifiedBindingsSubset(l.bindings, r.bindings), nil
	case formalComponentPathTerms:
		return formalQualifiedPathTermsSubset(l.pathTerms, r.pathTerms), nil
	case formalComponentOutcomeOccurrence:
		return l.outcome == r.outcome, nil
	case formalComponentDiagnostics:
		return l.diagnostics.LessOrEq(a.product.Registry(), r.diagnostics), nil
	case formalComponentCallOutcomes:
		return formalCallOutcomeSetSubset(a.product.Registry(), l.callOutcomes, r.callOutcomes), nil
	case formalComponentOrdinaryLane:
		return a.product.LaneLessOrEq(l.lane, r.lane)
	case formalComponentCoordinateSkeleton, formalComponentCoordinateScalar:
		return false, errFormalCoordinateGroupRequired
	case formalComponentGroundValue:
		return product.LessOrEq(a.product.Registry(), l.ground, r.ground), nil
	case formalComponentSymbolicValue:
		return l.symbolicValue.equal(a.product.Registry(), r.symbolicValue), nil
	default:
		return false, errFormalComponentMalformed
	}
}

// defaultFor resolves the semantic interpretation of physical directory zero.
// It never treats zero as a universal lattice Bottom.
func (a *formalComponentTerminalAuthority) defaultFor(ctx context.Context, descriptor formalFiberDescriptor) (formalComponentDefault, error) {
	if a == nil || a.arena == nil || a.arena.schema == nil || descriptor.forest != a.arena.schema.fibers || descriptor.variable != a.variable {
		return formalComponentDefault{}, errFormalComponentForeignOwner
	}
	switch descriptor.role {
	case formalFiberCare, formalFiberGroundValueTop:
		return formalComponentDefault{kind: formalComponentDefaultBooleanFalse}, nil
	case formalFiberMiddleValue, formalFiberMiddlePath, formalFiberOutcome:
		return formalComponentDefault{kind: formalComponentDefaultAbsent}, nil
	case formalFiberDiagnostics:
		domain := callpayload.DiagnosticOutputLattice(a.product.Registry())
		leaf, err := a.internDiagnostics(domain.Bottom())
		if err != nil {
			return formalComponentDefault{}, err
		}
		return formalComponentDefault{kind: formalComponentDefaultTerminal, leaf: leaf}, nil
	case formalFiberCallOutcome:
		leaf, err := a.internCallOutcomes(callpayload.CallOutcomeAlternativeSet{})
		if err != nil {
			return formalComponentDefault{}, err
		}
		return formalComponentDefault{kind: formalComponentDefaultTerminal, leaf: leaf}, nil
	case formalFiberOrdinaryLane:
		bottom, err := a.product.LaneBottom(descriptor.lane)
		if err != nil {
			return formalComponentDefault{}, err
		}
		leaf, err := a.internLane(ctx, bottom)
		if err != nil {
			return formalComponentDefault{}, err
		}
		return formalComponentDefault{kind: formalComponentDefaultTerminal, leaf: leaf}, nil
	case formalFiberCoordinate:
		return formalComponentDefault{kind: formalComponentDefaultCoordinateGroup}, nil
	case formalFiberGroundValue:
		leaf, err := a.internGroundValue(product.Bottom(a.product.Registry()))
		if err != nil {
			return formalComponentDefault{}, err
		}
		return formalComponentDefault{kind: formalComponentDefaultTerminal, leaf: leaf}, nil
	default:
		return formalComponentDefault{}, errFormalComponentMalformed
	}
}

func (a *formalComponentTerminalAuthority) combine(ctx context.Context, op formalComponentBinaryOp, left, right decisionLeaf) (decisionLeaf, error) {
	l, err := a.terminal(left)
	if err != nil {
		return 0, err
	}
	r, err := a.terminal(right)
	if err != nil || l.kind != r.kind || op < formalComponentJoin || op > formalComponentNarrow {
		return 0, errFormalComponentMalformed
	}
	// Every registered lattice operation is idempotent on an identical
	// retained representation. This is also the only coordinate-local combine
	// which does not require reconstructing its complete joint family group.
	if left == right {
		return left, nil
	}
	switch l.kind {
	case formalComponentBindings:
		if op == formalComponentMeet || op == formalComponentNarrow {
			// These are sets of symbolic functions, not disjoint concrete
			// atoms. Distinct terms may evaluate to overlapping or equal
			// values, so syntactic intersection would under-approximate.
			return 0, errFormalSymbolicMeetUnproven
		}
		if formalQualifiedBindingsSubset(l.bindings, r.bindings) {
			return right, nil
		}
		if formalQualifiedBindingsSubset(r.bindings, l.bindings) {
			return left, nil
		}
		bindings := unionFormalQualifiedBindings(l.bindings, r.bindings)
		return a.internCanonicalBindings(bindings)
	case formalComponentPathTerms:
		if op == formalComponentMeet || op == formalComponentNarrow {
			return 0, errFormalSymbolicMeetUnproven
		}
		if formalQualifiedPathTermsSubset(l.pathTerms, r.pathTerms) {
			return right, nil
		}
		if formalQualifiedPathTermsSubset(r.pathTerms, l.pathTerms) {
			return left, nil
		}
		paths := unionFormalQualifiedPathTerms(l.pathTerms, r.pathTerms)
		return a.internCanonicalPathTerms(paths)
	case formalComponentOutcomeOccurrence:
		if l.outcome != r.outcome {
			return 0, errFormalOccurrenceConflict
		}
		return left, nil
	case formalComponentDiagnostics:
		domain := callpayload.DiagnosticOutputLattice(a.product.Registry())
		var value callpayload.DiagnosticOutput
		switch op {
		case formalComponentJoin:
			value = domain.Join(l.diagnostics, r.diagnostics)
		case formalComponentWiden:
			value = domain.Widen(l.diagnostics, r.diagnostics)
		case formalComponentNarrow:
			value = domain.Narrow(l.diagnostics, r.diagnostics)
		case formalComponentMeet:
			return 0, errFormalComponentMalformed
		default:
			return 0, errFormalComponentMalformed
		}
		return a.internDiagnostics(value)
	case formalComponentCallOutcomes:
		var value callpayload.CallOutcomeAlternativeSet
		switch op {
		case formalComponentJoin, formalComponentWiden:
			value = l.callOutcomes.Join(a.product.Registry(), r.callOutcomes)
		case formalComponentMeet, formalComponentNarrow:
			value = formalCallOutcomeSetMeet(a.product.Registry(), l.callOutcomes, r.callOutcomes)
		default:
			return 0, errFormalComponentMalformed
		}
		return a.internCallOutcomes(value)
	case formalComponentOrdinaryLane:
		return a.combineLane(ctx, op, left, right, l.lane, r.lane)
	case formalComponentCoordinateSkeleton, formalComponentCoordinateScalar:
		return 0, errFormalCoordinateGroupRequired
	case formalComponentGroundValue:
		return a.combineGround(op, left, right, l.ground, r.ground)
	case formalComponentSymbolicValue:
		switch op {
		case formalComponentJoin, formalComponentWiden:
			return a.joinFormalValueLeaves(left, right)
		case formalComponentNarrow:
			return left, nil
		default:
			return 0, errFormalSymbolicMeetUnproven
		}
	default:
		return 0, errFormalComponentMalformed
	}
}

func (a *formalComponentTerminalAuthority) combineLane(ctx context.Context, op formalComponentBinaryOp, leftLeaf, rightLeaf decisionLeaf, left, right state.LaneFactor) (decisionLeaf, error) {
	var (
		value state.LaneFactor
		err   error
	)
	switch op {
	case formalComponentJoin:
		value, err = a.product.LaneJoin(left, right)
	case formalComponentMeet:
		value, err = a.product.LaneMeet(left, right)
	case formalComponentWiden:
		value, err = a.product.LaneWiden(left, right)
	case formalComponentNarrow:
		value, err = a.product.LaneNarrow(left, right)
	}
	if err != nil {
		return 0, err
	}
	for _, candidate := range []struct {
		leaf  decisionLeaf
		value state.LaneFactor
	}{{leftLeaf, left}, {rightLeaf, right}} {
		same, sameErr := a.product.LaneSame(value, candidate.value)
		if sameErr != nil {
			return 0, sameErr
		}
		if same {
			return candidate.leaf, nil
		}
	}
	// Deliberately do not reuse a merely LaneEqual operand. The registered
	// operator's actual spelling is retained and deterministic WTO order owns
	// that spelling.
	return a.internLane(ctx, value)
}

func (a *formalComponentTerminalAuthority) combineGround(op formalComponentBinaryOp, leftLeaf, rightLeaf decisionLeaf, left, right product.Value) (decisionLeaf, error) {
	domain := product.Domain(a.product.Registry())
	var value product.Value
	switch op {
	case formalComponentJoin:
		value = domain.Join(left, right)
	case formalComponentMeet:
		if domain.Meet == nil {
			return 0, errFormalComponentMalformed
		}
		value = domain.Meet(left, right)
	case formalComponentWiden:
		value = domain.Widen(left, right)
	case formalComponentNarrow:
		if domain.Narrow == nil {
			value = left
		} else {
			value = domain.Narrow(left, right)
		}
	}
	if domain.Same(value, left) {
		return leftLeaf, nil
	}
	if domain.Same(value, right) {
		return rightLeaf, nil
	}
	return a.internGroundValue(value)
}

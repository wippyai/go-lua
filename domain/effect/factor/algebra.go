// Package factor owns Effect's finite may-set factor algebra.
//
// It deliberately projects the canonical Link, Pack, Target, and Program
// Static authorities.  It does not copy their payloads or retain a derived
// application/operation relation.
package factor

import (
	"crypto/sha256"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/effect/internal/valuecore"
	"github.com/wippyai/go-lua/domain/pack"
	internalhash "github.com/wippyai/go-lua/internal/hash"
)

const (
	effectFactorIDDomain = "wippy.analysis.effect.factor.v5\x00"
	effectRootIDDomain   = "wippy.analysis.effect.root.v3\x00"
)

// Root is one Factor-owned executable Program body coordinate.
type Root struct {
	owner *Algebra
	slot  uint32
}

type rootRow struct {
	moduleKey identity.ContentID
	programID identity.ContentID
	bodyID    identity.ContentID
	context   identity.ContentID
	id        identity.ContentID
}

// Algebra owns the one finite Effect factor vocabulary.  The capacity is a
// mathematical upper bound, not a materialized product or convergence cap.
type Algebra struct {
	linkOwner link.OwnerCapability
	packs     *pack.Schema
	contract  *contract.Contract

	roots                []rootRow
	rootContextIndex     map[rootContextRef]uint32
	rootBodyIndex        map[rootBodyRef]uint32
	rootMountedBodyIndex map[rootMountedBodyRef]uint32
	capacity             uint64
	unknownID            identity.ContentID
	content              identity.ContentID
	applicationOps       map[identity.ContentID]map[vocabulary.Operation]int
	applicationCount     uint64
	mountedCalls         []mountedCallRow
	mountedCallIndex     map[mountedCallRef]uint32
	publications         PublicationDirectory
	publicationCallIndex map[mountedPublicationCallRef]uint32
	publicationSubjects  map[identity.ContentID]uint32
	publicationReceipts  map[identity.ContentID]MountedPublication
}

// mountedPublicationCallRef addresses one publication call by the mounted
// coordinate both this directory and Call's own directory are keyed by.
type mountedPublicationCallRef struct {
	module     identity.ContentID
	occurrence identity.ContentID
}

type artifactCallRow struct {
	programID identity.ContentID
	bodyID    identity.ContentID
	root      uint32
}

type rootContextRef struct {
	module  identity.ContentID
	context identity.ContentID
}

type rootBodyRef struct {
	module identity.ContentID
	bodyID identity.ContentID
}

type rootMountedBodyRef struct {
	moduleKey identity.ContentID
	bodyID    identity.ContentID
}

// MountedArtifact is the exact mounted reusable artifact handle consumed by
// Effect.  Body rows are enumerated and validated inside Effect; callers
// cannot self-attest Body/Context scalar correspondence.
type MountedArtifact struct {
	ModuleKey identity.ContentID
	Snapshot  *ingress.Snapshot
}

// MountedCall is Effect's detached, exact ordinary-call placement receipt.
// It is issued only from the cold Link census and contains neither Project
// nor Program proof.  The opaque slot is authenticated by its issuing
// Algebra, making content-equal foreign Links fail closed.
type MountedCall struct {
	owner *Algebra
	slot  uint32
}

type mountedCallRow struct {
	applicationID identity.ContentID
	moduleID      identity.ContentID
	contextID     identity.ContentID
	root          uint32
}

type mountedCallRef struct {
	moduleID  identity.ContentID
	contextID identity.ContentID
}

// NewWithMountedArtifacts constructs Effect from exact mounted artifacts.
// It is artifact-native: no Program census or caller-supplied Body proof
// bundle is accepted.
func NewWithMountedArtifacts(source *link.Link, packs *pack.Schema, contract *contract.Contract, mounts []MountedArtifact) (*Algebra, bool) {
	if source == nil || packs == nil || contract == nil || source.Boundary() == nil || source.Project() == nil {
		return nil, false
	}
	owner := source.OwnerCapability()
	if !owner.Available() || !packs.LinkOwner().Matches(owner) {
		return nil, false
	}
	linked, ok := source.Boundary().Target()
	if !ok || linked != contract {
		return nil, false
	}
	a := &Algebra{linkOwner: owner, packs: packs, contract: contract, rootContextIndex: make(map[rootContextRef]uint32), rootBodyIndex: make(map[rootBodyRef]uint32), rootMountedBodyIndex: make(map[rootMountedBodyRef]uint32), applicationOps: make(map[identity.ContentID]map[vocabulary.Operation]int), mountedCallIndex: make(map[mountedCallRef]uint32)}
	project := source.Project()
	if project == nil {
		return nil, false
	}
	artifactCalls, artifactsOK := a.sealMountedArtifacts(mounts)
	if !artifactsOK || !a.captureApplicationOps(project, source.Boundary(), artifactCalls) || !a.sealCapacity() {
		return nil, false
	}
	a.unknownID = externalID()
	if !a.unknownID.Available() {
		return nil, false
	}
	a.content = a.contentID()
	// The publication directory is sealed here because it is a function of the
	// mounted calls this algebra just sealed and of nothing else. A relation
	// owner binds this algebra alone, so a directory derived later would be
	// derived per read from state that cannot change.
	if !a.sealPublications() {
		return nil, false
	}
	return a, a.Valid()
}

func (a *Algebra) Valid() bool {
	return a != nil && a.linkOwner.Available() && a.packs != nil && a.packs.LinkOwner().Matches(a.linkOwner) && a.contract != nil &&
		a.linkOwner.ContentID().Available() && a.content.Available()
}

// LinkOwner returns Effect's exact detached Link owner witness.
func (a *Algebra) LinkOwner() link.OwnerCapability {
	if !a.Valid() {
		return link.OwnerCapability{}
	}
	return a.linkOwner
}

func (a *Algebra) captureApplicationOps(project *linkproject.Component, boundary *linkboundary.Component, artifactCalls map[mountedCallRef]artifactCallRow) bool {
	if a == nil || project == nil || boundary == nil || a.contract == nil {
		return false
	}
	apps := project.Applications()
	a.applicationCount = uint64(apps.Count())
	for i := 0; i < apps.Count(); i++ {
		app, ok := apps.At(i)
		id, idOK := project.ApplicationID(app)
		if !ok || !idOK || !id.Available() {
			return false
		}
		rows := make(map[vocabulary.Operation]int)
		for j := 0; j < a.contract.Operations.OperationCount(); j++ {
			op, opOK := a.contract.Operations.OperationAt(j)
			if !opOK {
				return false
			}
			if !boundary.ApplicationOperationAvailable(a.contract, app, op) {
				continue
			}
			args, argsOK := boundary.Calls().TypeFormalArguments(a.contract, app, op)
			if !argsOK {
				rows[op] = -1
				continue
			}
			rows[op] = args.Count()
		}
		a.applicationOps[id] = rows
		if apps.IsBase(app) {
			applicationID, moduleID, contextID, identityOK := apps.Calls().MountedIdentity(app)
			if !identityOK {
				// Non-call base applications are not mounted Program calls.
				continue
			}
			artifact, artifactOK := artifactCalls[mountedCallRef{moduleID: moduleID, contextID: contextID}]
			if applicationID != id || !artifactOK || artifact.root == 0 || uint64(artifact.root) > uint64(len(a.roots)) || !moduleID.Available() || !contextID.Available() {
				return false
			}
			ref := mountedCallRef{moduleID: moduleID, contextID: contextID}
			if _, duplicate := a.mountedCallIndex[ref]; duplicate {
				return false
			}
			a.mountedCalls = append(a.mountedCalls, mountedCallRow{applicationID: id, moduleID: moduleID, contextID: contextID, root: artifact.root})
			a.mountedCallIndex[ref] = uint32(len(a.mountedCalls))
		}
	}
	return true
}

func (a *Algebra) applicationOperation(applicationID identity.ContentID, operation vocabulary.Operation) (int, bool) {
	if a == nil || !applicationID.Available() || operation == 0 {
		return 0, false
	}
	count, found := a.applicationOps[applicationID]
	if !found {
		return 0, false
	}
	args, found := count[operation]
	return args, found && args >= 0
}

// LinkID returns the detached content identity paired with LinkOwner.
func (a *Algebra) LinkID() identity.ContentID {
	if !a.Valid() {
		return identity.ContentID{}
	}
	return a.linkOwner.ContentID()
}

// Owner is the concise alias used by cross-domain binders.
func (a *Algebra) Owner() link.OwnerCapability { return a.LinkOwner() }

func (a *Algebra) Pack() *pack.Schema {
	if !a.Valid() {
		return nil
	}
	return a.packs
}

func (a *Algebra) ContentID() identity.ContentID {
	if !a.Valid() {
		return identity.ContentID{}
	}
	return a.content
}

func (a *Algebra) RootCount() int {
	if !a.Valid() {
		return 0
	}
	return len(a.roots)
}

func (a *Algebra) RootAt(index int) (Root, bool) {
	if !a.Valid() || index < 0 || index >= len(a.roots) {
		return Root{}, false
	}
	return Root{owner: a, slot: uint32(index + 1)}, true
}

// RootForMountedBodyID is the production artifact bridge. It accepts only
// mounted and Program-issued scalar IDs and therefore cannot reopen a
// Program Body proof after compilation.
func (a *Algebra) RootForMountedBodyID(moduleKey, programID, bodyID identity.ContentID) (Root, bool) {
	if !a.Valid() || !moduleKey.Available() || !programID.Available() || !bodyID.Available() {
		return Root{}, false
	}
	slot := a.rootMountedBodyIndex[rootMountedBodyRef{moduleKey: moduleKey, bodyID: bodyID}]
	if slot == 0 || uint64(slot) > uint64(len(a.roots)) {
		return Root{}, false
	}
	row := a.roots[slot-1]
	if row.moduleKey != moduleKey || row.programID != programID || row.bodyID != bodyID {
		return Root{}, false
	}
	return Root{owner: a, slot: slot}, true
}

func (a *Algebra) RootIndex(root Root) (int, bool) {
	if !a.ownsRoot(root) {
		return 0, false
	}
	return int(root.slot - 1), true
}

// DenseKeyIndex is the axis member key-binding form of RootIndex: the same
// dense coordinate, in the uint32 width the generated relation owner
// normalizes against.
func (a *Algebra) DenseKeyIndex(root Root) (uint32, bool) {
	if !a.ownsRoot(root) {
		return 0, false
	}
	return root.slot - 1, true
}

// RootID is Effect's portable body-root proof. Dense root slots remain hot
// owner state and never enter rule operands or replay identities.
func (a *Algebra) RootID(root Root) (identity.ContentID, bool) {
	if !a.ownsRoot(root) {
		return identity.ContentID{}, false
	}
	row := a.roots[root.slot-1]
	return row.id, row.id.Available()
}

func (a *Algebra) Bottom() Value {
	if !a.Valid() {
		return Value{}
	}
	return a.value(0, false, nil)
}

// Default is Factor's least may-effect set.
func (a *Algebra) Default() Value { return a.Bottom() }
func (a *Algebra) Top() Value {
	if !a.Valid() {
		return Value{}
	}
	return a.value(0, true, nil)
}

func (a *Algebra) Singleton(atom Atom) (Value, bool) {
	if !atomValidFor(atom, a) {
		return Value{}, false
	}
	return a.value(atom.Root(), false, []Atom{atom}), true
}

func (a *Algebra) FromAtoms(atoms []Atom) (Value, bool) {
	if !a.Valid() {
		return Value{}, false
	}
	if len(atoms) == 0 {
		return a.Bottom(), true
	}
	out := make([]Atom, len(atoms))
	copy(out, atoms)
	root := out[0].Root()
	for _, atom := range out {
		if !atomValidFor(atom, a) {
			return Value{}, false
		}
		if atom.Root() != root {
			return Value{}, false
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessID(out[i].ID(), out[j].ID()) })
	n := 1
	for i := 1; i < len(out); i++ {
		if out[i].ID() != out[n-1].ID() {
			out[n] = out[i]
			n++
		}
	}
	if uint64(n) > a.capacity {
		return Value{}, false
	}
	return a.value(root, false, out[:n]), true
}

func (a *Algebra) AtomAt(value Value, index int) (Atom, bool) {
	atoms := value.Atoms()
	if !a.owns(value) || value.IsTop() || index < 0 || index >= len(atoms) {
		return Atom{}, false
	}
	return atoms[index], true
}

// AtomID exposes the portable certificate identity of an atom without
// exposing an inverse constructor or any of its cross-domain preimage.
func (a *Algebra) AtomID(atom Atom) (identity.ContentID, bool) {
	if !atomValidFor(atom, a) {
		return identity.ContentID{}, false
	}
	return atom.ID(), true
}

// TransportAtom rehomes an existing certificate to another sealed Effect
// coordinate without decoding it or minting a new identity.  It is the only
// transport operation for outcome/boundary rules; there is no raw-ID inlet.
func (a *Algebra) TransportAtom(atom Atom, root Root) (Atom, bool) {
	if !atomValidFor(atom, a) || !a.ownsRoot(root) {
		return Atom{}, false
	}
	return valuecore.NewAtom(a, root.slot, atom.ID()), true
}

// Transport rehomes one sparse owned value. Bottom and Top carry no local
// atoms and remain their canonical context-neutral values.
func (a *Algebra) Transport(value Value, root Root) (Value, bool) {
	if !a.owns(value) || !a.ownsRoot(root) {
		return Value{}, false
	}
	if value.IsTop() {
		return a.Top(), true
	}
	values := value.Atoms()
	if len(values) == 0 {
		return a.Bottom(), true
	}
	atoms := make([]Atom, len(values))
	for i, atom := range values {
		atoms[i] = valuecore.NewAtom(a, root.slot, atom.ID())
	}
	return a.value(root.slot, false, atoms), true
}

func (a *Algebra) Owns(value Value) bool { return a.owns(value) }
func (a *Algebra) Admit(root Root, value Value) bool {
	return a.ownsRoot(root) && a.owns(value) && (value.IsTop() || len(value.Atoms()) == 0 || value.Root() == root.slot)
}

func (a *Algebra) Equal(left, right Value) bool {
	leftAtoms, rightAtoms := left.Atoms(), right.Atoms()
	if !a.owns(left) || !a.owns(right) || left.IsTop() != right.IsTop() || left.Root() != right.Root() || len(leftAtoms) != len(rightAtoms) {
		return false
	}
	for i := range leftAtoms {
		if leftAtoms[i].ID() != rightAtoms[i].ID() {
			return false
		}
	}
	return true
}

func (a *Algebra) Same(left, right Value) bool { return a.Equal(left, right) }

func (a *Algebra) LessOrEq(left, right Value) bool {
	if !a.owns(left) || !a.owns(right) {
		return false
	}
	if left.IsTop() {
		return right.IsTop()
	}
	leftAtoms := left.Atoms()
	if right.IsTop() || len(leftAtoms) == 0 {
		return true
	}
	if left.Root() != right.Root() {
		return false
	}
	rightAtoms := right.Atoms()
	if len(leftAtoms) > len(rightAtoms) {
		return false
	}
	i, j := 0, 0
	for i < len(leftAtoms) && j < len(rightAtoms) {
		if leftAtoms[i].ID() == rightAtoms[j].ID() {
			i++
			j++
			continue
		}
		if lessID(leftAtoms[i].ID(), rightAtoms[j].ID()) {
			return false
		}
		j++
	}
	return i == len(leftAtoms)
}

func (a *Algebra) Join(left, right Value) (Value, bool) {
	if !a.owns(left) || !a.owns(right) {
		return Value{}, false
	}
	if left.IsTop() || right.IsTop() {
		return a.Top(), true
	}
	if a.LessOrEq(left, right) {
		return right, true
	}
	if a.LessOrEq(right, left) {
		return left, true
	}
	if left.Root() != right.Root() {
		return Value{}, false
	}
	leftAtoms, rightAtoms := left.Atoms(), right.Atoms()
	out := make([]Atom, 0, len(leftAtoms)+len(rightAtoms))
	i, j := 0, 0
	for i < len(leftAtoms) || j < len(rightAtoms) {
		if j == len(rightAtoms) || (i < len(leftAtoms) && lessID(leftAtoms[i].ID(), rightAtoms[j].ID())) {
			out = append(out, leftAtoms[i])
			i++
			continue
		}
		if i == len(leftAtoms) || lessID(rightAtoms[j].ID(), leftAtoms[i].ID()) {
			out = append(out, rightAtoms[j])
			j++
			continue
		}
		out = append(out, leftAtoms[i])
		i++
		j++
	}
	return a.value(left.Root(), false, out), true
}

func (a *Algebra) Widen(previous, next Value) (Value, bool) { return a.Join(previous, next) }

func (a *Algebra) Lattice() (lattice.Lattice[Value], bool) {
	if !a.Valid() {
		return lattice.Lattice[Value]{}, false
	}
	return lattice.Lattice[Value]{
		Bottom: a.Bottom, Top: a.Top, Equal: a.Equal, Same: a.Same, LessOrEq: a.LessOrEq,
		Join: func(left, right Value) Value {
			value, ok := a.Join(left, right)
			if !ok {
				panic("effect factor: foreign or cross-root value")
			}
			return value
		},
		Widen: func(left, right Value) Value {
			value, ok := a.Widen(left, right)
			if !ok {
				panic("effect factor: foreign or cross-root value")
			}
			return value
		},
	}, true
}

// WidenRank is the cap-free finite descent witness: capacity+1-cardinality,
// reserving rank zero for Top even when the sparse quotient has fewer atoms.
func (a *Algebra) WidenRank(root Root, value Value, component int) uint64 {
	if component != 0 || !a.Admit(root, value) {
		return 0
	}
	if value.IsTop() {
		return 0
	}
	return a.capacity + 1 - uint64(len(value.Atoms()))
}

func (a *Algebra) Fingerprint(value Value) uint64 {
	if !a.owns(value) {
		return 0
	}
	return value.Seal()
}

func (a *Algebra) sealMountedArtifacts(mounts []MountedArtifact) (map[mountedCallRef]artifactCallRow, bool) {
	rows := make([]rootRow, 0)
	artifactCalls := make(map[mountedCallRef]artifactCallRow)
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.Snapshot == nil || !mount.Snapshot.Available() || !mount.ModuleKey.Available() {
			return nil, false
		}
		if mount.Snapshot.ProgramID() == (identity.ContentID{}) {
			return nil, false
		}
		if _, duplicate := seenMounts[mount.ModuleKey]; duplicate {
			return nil, false
		}
		seenMounts[mount.ModuleKey] = struct{}{}
		programID := mount.Snapshot.ProgramID()
		program, programOK := programmount.ProgramFromSnapshot(mount.Snapshot, mount.ModuleKey)
		bodyCount, bodiesPublished := program.BodyCount()
		if !programOK || !bodiesPublished {
			return nil, false
		}
		for index := 0; index < bodyCount; index++ {
			body, ok := program.BodyAt(index)
			if !ok || !body.ID().Available() || !body.ContextID().Available() {
				return nil, false
			}
			rows = append(rows, rootRow{moduleKey: mount.ModuleKey, programID: programID, bodyID: body.ID(), context: body.ContextID()})
		}
		callCount, callsPublished := program.CallCount()
		if !callsPublished {
			return nil, false
		}
		for index := 0; index < callCount; index++ {
			call, callOK := program.CallAt(index)
			if !callOK || !call.ID().Available() || !call.BodyID().Available() {
				return nil, false
			}
			ref := mountedCallRef{moduleID: mount.ModuleKey, contextID: call.ID()}
			if _, duplicate := artifactCalls[ref]; duplicate {
				return nil, false
			}
			artifactCalls[ref] = artifactCallRow{programID: programID, bodyID: call.BodyID()}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].moduleKey != rows[j].moduleKey {
			return lessID(rows[i].moduleKey, rows[j].moduleKey)
		}
		if rows[i].bodyID != rows[j].bodyID {
			return lessID(rows[i].bodyID, rows[j].bodyID)
		}
		return lessID(rows[i].context, rows[j].context)
	})
	for _, row := range rows {
		if !row.programID.Available() || !row.bodyID.Available() || !row.context.Available() || !row.moduleKey.Available() {
			return nil, false
		}
		contextRef := rootContextRef{module: row.moduleKey, context: row.context}
		bodyRef := rootBodyRef{module: row.moduleKey, bodyID: row.bodyID}
		mountedRef := rootMountedBodyRef{moduleKey: row.moduleKey, bodyID: row.bodyID}
		if a.rootContextIndex[contextRef] != 0 || a.rootBodyIndex[bodyRef] != 0 || a.rootMountedBodyIndex[mountedRef] != 0 || uint64(len(a.roots)) >= uint64(math.MaxUint32) {
			return nil, false
		}
		id := effectRootID(row.programID, row.moduleKey, row.context)
		if !id.Available() {
			return nil, false
		}
		row.id = id
		a.roots = append(a.roots, row)
		slot := uint32(len(a.roots))
		a.rootContextIndex[contextRef] = slot
		a.rootBodyIndex[bodyRef] = slot
		a.rootMountedBodyIndex[mountedRef] = slot
	}
	for ref, call := range artifactCalls {
		slot := a.rootMountedBodyIndex[rootMountedBodyRef{moduleKey: ref.moduleID, bodyID: call.bodyID}]
		if slot == 0 || uint64(slot) > uint64(len(a.roots)) {
			return nil, false
		}
		root := a.roots[slot-1]
		if root.moduleKey != ref.moduleID || root.programID != call.programID || root.bodyID != call.bodyID {
			return nil, false
		}
		call.root = slot
		artifactCalls[ref] = call
	}
	return artifactCalls, true
}

func (a *Algebra) sealCapacity() bool {
	contexts := uint64(a.applicationCount)
	occurrences := uint64(0)
	for i := 0; i < a.contract.Operations.OperationCount(); i++ {
		op, ok := a.contract.Operations.OperationAt(i)
		if !ok {
			return false
		}
		occurrences, ok = checkedAdd(occurrences, uint64(a.contract.Operations.EffectCount(op)))
		if !ok {
			return false
		}
		callbacks := a.contract.Operations.CallbackCount(op)
		for callbackIndex := 0; callbackIndex < callbacks; callbackIndex++ {
			callback, callbackOK := a.contract.Operations.CallbackAt(op, callbackIndex)
			if !callbackOK {
				return false
			}
			occurrences, ok = checkedAdd(occurrences, uint64(a.contract.Operations.CallbackEffectCount(callback)))
			if !ok {
				return false
			}
		}
	}
	capacity, ok := checkedMul(contexts, occurrences)
	if !ok {
		return false
	}
	// Only selected Call constructors mint distinct known IDs. Body, outcome,
	// and boundary rules transport existing atoms; they do not enlarge this
	// denominator. UnknownExternal contributes the one extra vocabulary atom.
	capacity, ok = checkedAdd(capacity, 1)
	if !ok || capacity == math.MaxUint64 {
		return false
	} // +1 rank must fit.
	a.capacity = capacity
	return true
}

// inputTailArgument reports which position of an effect's Values argument
// vector substitutes the target operation's input tail. The vector is indexed
// by the target's own ValuesVar ordinals, so that position is the target's
// input var; every other position substitutes an outcome-scoped var, which
// names no input and therefore selects nothing out of the caller's Pack.
func (a *Algebra) inputTailArgument(operation vocabulary.Operation) (int, bool) {
	input, inputOK := a.contract.Operations.Input(operation)
	if !inputOK {
		return 0, false
	}
	tail, variable, tailOK := a.contract.Operations.ValuesTail(input)
	if !tailOK || tail != vocabulary.ValuesVariable {
		return 0, false
	}
	return int(variable), true
}

func (a *Algebra) validateOrdinaryInputs(owner vocabulary.Operation, effect int) bool {
	for i := 0; i < a.contract.Operations.EffectValueArgumentCount(owner, effect); i++ {
		formal, ok := a.contract.Operations.EffectValueArgumentAt(owner, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	targetOperation, targetOK := a.contract.Operations.EffectTarget(owner, effect)
	if !targetOK {
		return false
	}
	tailArgument, tailed := a.inputTailArgument(targetOperation)
	for i := 0; i < a.contract.Operations.EffectValuesArgumentCount(owner, effect); i++ {
		formal, ok := a.contract.Operations.EffectValuesArgumentAt(owner, effect, i)
		if !ok {
			return false
		}
		if !tailed || i != tailArgument {
			continue
		}
		if _, ok = a.packs.InputSelector(owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	return true
}

func (a *Algebra) validateCallbackInputs(owner vocabulary.Operation, callback vocabulary.CallbackID, effect int) bool {
	for i := 0; i < a.contract.Operations.CallbackEffectValueArgumentCount(callback, effect); i++ {
		formal, ok := a.contract.Operations.CallbackEffectValueArgumentAt(callback, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	targetOperation, targetOK := a.contract.Operations.CallbackEffectTarget(callback, effect)
	if !targetOK {
		return false
	}
	tailArgument, tailed := a.inputTailArgument(targetOperation)
	for i := 0; i < a.contract.Operations.CallbackEffectValuesArgumentCount(callback, effect); i++ {
		formal, ok := a.contract.Operations.CallbackEffectValuesArgumentAt(callback, effect, i)
		if !ok {
			return false
		}
		if !tailed || i != tailArgument {
			continue
		}
		if _, ok = a.packs.InputSelector(owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	return true
}

func (a *Algebra) ownsRoot(root Root) bool {
	return a.Valid() && root.owner == a && root.slot != 0 && uint64(root.slot) <= uint64(len(a.roots))
}
func (a *Algebra) owns(value Value) bool {
	atoms := value.Atoms()
	return a.Valid() && value.Owner() == a && value.Seal() != 0 &&
		uint64(len(atoms)) <= a.capacity &&
		((value.IsTop() && value.Root() == 0 && len(atoms) == 0) ||
			(!value.IsTop() && ((len(atoms) == 0 && value.Root() == 0) ||
				(len(atoms) != 0 && value.Root() != 0 && uint64(value.Root()) <= uint64(len(a.roots))))))
}

// atomValidFor is the free-function counterpart of Value's owns: Atom is
// defined in valuecore, so its validity predicate cannot be a method here.
func atomValidFor(atom Atom, a *Algebra) bool {
	return a != nil && atom.Owner() == a && atom.Root() != 0 && uint64(atom.Root()) <= uint64(len(a.roots)) && atom.ID().Available()
}

// value seals constructor-proven ordering and owner admission into an O(1)
// hot header.  No caller can fabricate this private seal outside this package.
func (a *Algebra) value(root uint32, top bool, atoms []Atom) Value {
	return valuecore.NewValue(a, root, top, atoms, a.valueSeal(root, top, atoms))
}

func (a *Algebra) valueSeal(root uint32, top bool, atoms []Atom) uint64 {
	if a == nil {
		return 0
	}
	h := internalhash.MixHash(0x6566666563742d76, uint64(root))
	if top {
		h = internalhash.MixHash(h, 1)
	} else {
		h = internalhash.MixHash(h, 0)
	}
	h = internalhash.MixHash(h, uint64(len(atoms)))
	for _, atom := range atoms { // constructor/cold verification only; hot callers compare the cached seal.
		id := atom.ID()
		for _, byte := range id {
			h = internalhash.MixHash(h, uint64(byte))
		}
	}
	if h == 0 {
		return 1
	}
	return h
}

func (a *Algebra) contentID() identity.ContentID {
	ownerID := a.linkOwner.ContentID()
	if !ownerID.Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	_, _ = h.Write([]byte(effectFactorIDDomain))
	_, _ = h.Write(ownerID[:])
	for _, root := range a.roots {
		_, _ = h.Write(root.moduleKey[:])
		_, _ = h.Write(root.id[:])
	}
	var out identity.ContentID
	copy(out[:], h.Sum(nil))
	return out
}

// effectRootID is the v3 concrete mounted root identity. The exact Program
// Body context remains opaque, while ModuleKey keeps duplicate mounts
// disjoint without using Link identity or a dense shard ordinal.
func effectRootID(programID, moduleKey, context identity.ContentID) identity.ContentID {
	if !programID.Available() || !moduleKey.Available() || !context.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(effectRootIDDomain))
	_, _ = hash.Write(programID[:])
	_, _ = hash.Write(moduleKey[:])
	_, _ = hash.Write(context[:])
	var result identity.ContentID
	copy(result[:], hash.Sum(nil))
	return result
}

func externalID() identity.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.effect.atom.v2.unknown\x00"))
	var out identity.ContentID
	copy(out[:], h.Sum(nil))
	return out
}
func checkedAdd(left, right uint64) (uint64, bool) {
	if right > math.MaxUint64-left {
		return 0, false
	}
	return left + right, true
}
func checkedMul(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}
func checkedIntAdd(left, right int) (int, bool) {
	if left < 0 || right < 0 || right > int(^uint(0)>>1)-left {
		return 0, false
	}
	return left + right, true
}
func lessID(left, right identity.ContentID) bool {
	for i := range left {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return false
}

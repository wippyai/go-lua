// Package factor owns Effect's finite may-set factor algebra.
//
// It deliberately projects the canonical Link, Pack, Target, and Program
// Static authorities.  It does not copy their payloads or retain a derived
// application/operation relation.
package factor

import (
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/call"
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

// Atom is one opaque, algebra-local effect-template identity.  It contains no
// decoded Target, Pack, Boundary, or Static payload.
type Atom struct {
	owner *Algebra
	root  uint32
	id    identity.ContentID
}

// Value is Bottom, an immutable sparse atom set, or Top.  UnknownExternal is
// an ordinary admitted atom and therefore can coexist with known atoms.
type Value struct {
	owner *Algebra
	root  uint32
	top   bool
	atoms []Atom
	seal  uint64
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
	contract  *target.Contract

	roots                []rootRow
	rootContextIndex     map[rootContextRef]uint32
	rootBodyIndex        map[rootBodyRef]uint32
	rootMountedBodyIndex map[rootMountedBodyRef]uint32
	capacity             uint64
	unknownID            identity.ContentID
	content              identity.ContentID
	applicationOps       map[identity.ContentID]map[vocabulary.Operation]int
	applicationCount     uint64
	callRows             map[identity.ContentID]callRow
	mountedCalls         []mountedCallRow
	mountedCallIndex     map[mountedCallRef]uint32
}

type callRow struct {
	moduleID identity.ContentID
	context  identity.ContentID
	root     uint32
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
	Artifact  *programartifact.Artifact
}

type bodyRootReceipt struct {
	moduleKey identity.ContentID
	programID identity.ContentID
	bodyID    identity.ContentID
	contextID identity.ContentID
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
}

type mountedCallRef struct {
	moduleID  identity.ContentID
	contextID identity.ContentID
}

// NewWithMountedArtifacts constructs Effect from exact mounted artifacts.
// It is artifact-native: no Program census or caller-supplied Body proof
// bundle is accepted.
func NewWithMountedArtifacts(source *link.Link, packs *pack.Schema, contract *target.Contract, mounts []MountedArtifact) (*Algebra, bool) {
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
	a := &Algebra{linkOwner: owner, packs: packs, contract: contract, rootContextIndex: make(map[rootContextRef]uint32), rootBodyIndex: make(map[rootBodyRef]uint32), rootMountedBodyIndex: make(map[rootMountedBodyRef]uint32), applicationOps: make(map[identity.ContentID]map[vocabulary.Operation]int), callRows: make(map[identity.ContentID]callRow), mountedCallIndex: make(map[mountedCallRef]uint32)}
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
		for j := 0; j < a.contract.OperationCount(); j++ {
			op, opOK := a.contract.OperationAt(j)
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
			if _, duplicate := a.callRows[id]; duplicate {
				return false
			}
			ref := mountedCallRef{moduleID: moduleID, contextID: contextID}
			if _, duplicate := a.mountedCallIndex[ref]; duplicate {
				return false
			}
			a.callRows[id] = callRow{moduleID: moduleID, context: contextID, root: artifact.root}
			a.mountedCalls = append(a.mountedCalls, mountedCallRow{applicationID: id, moduleID: moduleID, contextID: contextID})
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

// RootForCall derives the exact containing Effect body for one existing
// ordinary Project Call. It retains no application index or site relation.
func (a *Algebra) RootForCallID(applicationID identity.ContentID) (Root, bool) {
	if !a.Valid() || !applicationID.Available() {
		return Root{}, false
	}
	row, ok := a.callRows[applicationID]
	if !ok || !row.moduleID.Available() || !row.context.Available() || row.root == 0 || uint64(row.root) > uint64(len(a.roots)) {
		return Root{}, false
	}
	return Root{owner: a, slot: row.root}, true
}

func (a *Algebra) RootIndex(root Root) (int, bool) {
	if !a.ownsRoot(root) {
		return 0, false
	}
	return int(root.slot - 1), true
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

// ContainsCallID validates an existing detached ordinary-call identity.
func (a *Algebra) ContainsCallID(root Root, applicationID identity.ContentID) bool {
	return a.callInRootID(root, applicationID)
}

// OpaqueCallUnknown converts an exact admitted opaque Call alternative into
// the Factor's one unknown vocabulary atom. Call is evidence only: it is not
// retained, enumerated, or made into a Factor root.
func (a *Algebra) OpaqueCallUnknown(root Root, calls *call.Algebra, applicationID identity.ContentID, value call.Value) (Atom, bool) {
	if !a.ownsRoot(root) || calls == nil || !calls.Valid() || !calls.LinkOwner().Matches(a.linkOwner) || !value.HasOpaqueAlternative() {
		return Atom{}, false
	}
	key, ok := calls.KeyForApplicationID(applicationID)
	if !ok || !calls.Admits(key, value) || !a.callInRootID(root, applicationID) {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

// OpenOperationUnknown requires an exact selected operation with its explicit
// unknown-open effect row. Row variables remain unsupported and fail closed.
func (a *Algebra) OpenOperationUnknown(root Root, applicationID identity.ContentID, owner vocabulary.Operation) (Atom, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, applicationID, owner) {
		return Atom{}, false
	}
	tail, _, ok := a.contract.EffectTail(owner)
	if !ok || tail != vocabulary.RowUnknownOpen {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

func (a *Algebra) OpenCallbackUnknown(root Root, applicationID identity.ContentID, owner vocabulary.Operation, callback vocabulary.CallbackID) (Atom, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, applicationID, owner) {
		return Atom{}, false
	}
	callbackOwner, ownerOK := a.contract.CallbackOwner(callback)
	tail, _, tailOK := a.contract.CallbackEffectTail(callback)
	if !ownerOK || callbackOwner != owner || !tailOK || tail != vocabulary.RowUnknownOpen {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

// CallEffectAtom validates one selected ordinary-call effect. Row variables and
// row arguments fail closed; explicit closed/open rows retain known atoms.
func (a *Algebra) CallEffectAtom(root Root, applicationID identity.ContentID, owner vocabulary.Operation, effect int) (Atom, bool) {
	mounted, mountedOK := a.mountedCallForApplication(applicationID)
	if !mountedOK || !a.selectedMountedCall(root, mounted, owner) {
		return Atom{}, false
	}
	formal, formalOK := a.FormalCallEffectAtom(mounted, owner, effect)
	binding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	if !formalOK || !bindingOK {
		return Atom{}, false
	}
	return binding.Atom()
}

// CallbackEffectAtom validates and issues one callback-owned selected-call
// atom. Callback occurrence provenance does not enter atom identity.
func (a *Algebra) CallbackEffectAtom(root Root, applicationID identity.ContentID, owner vocabulary.Operation, callback vocabulary.CallbackID, effect int) (Atom, bool) {
	mounted, mountedOK := a.mountedCallForApplication(applicationID)
	if !mountedOK || !a.selectedMountedCall(root, mounted, owner) {
		return Atom{}, false
	}
	formal, formalOK := a.FormalCallbackEffectAtom(mounted, owner, callback, effect)
	binding, bindingOK := a.bindFormalAtom(root, mounted, formal, formal)
	if !formalOK || !bindingOK {
		return Atom{}, false
	}
	return binding.Atom()
}

// SelectedCallEffects reduces the explicit ordinary and callback effects of
// one operation already selected by a Rule-owned Call target. It keeps no
// selection state: every invocation revalidates the canonical witnesses.
// Unsupported authored rows fail closed so a caller cannot silently omit them.
func (a *Algebra) SelectedCallEffects(root Root, applicationID identity.ContentID, operation vocabulary.Operation) (Value, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, applicationID, operation) {
		return Value{}, false
	}
	tail, _, ok := a.contract.EffectTail(operation)
	if !ok || (tail != vocabulary.RowClosed && tail != vocabulary.RowUnknownOpen) {
		return Value{}, false
	}
	atomCount := a.contract.EffectCount(operation)
	callbacks := a.contract.CallbackCount(operation)
	for callbackIndex := 0; callbackIndex < callbacks; callbackIndex++ {
		callback, ok := a.contract.CallbackAt(operation, callbackIndex)
		if !ok {
			return Value{}, false
		}
		callbackTail, _, ok := a.contract.CallbackEffectTail(callback)
		if !ok || (callbackTail != vocabulary.RowClosed && callbackTail != vocabulary.RowUnknownOpen) {
			return Value{}, false
		}
		var added bool
		atomCount, added = checkedIntAdd(atomCount, a.contract.CallbackEffectCount(callback))
		if !added {
			return Value{}, false
		}
	}
	atoms := make([]Atom, 0, atomCount)
	for effect := 0; effect < a.contract.EffectCount(operation); effect++ {
		atom, ok := a.CallEffectAtom(root, applicationID, operation, effect)
		if !ok {
			return Value{}, false
		}
		atoms = append(atoms, atom)
	}
	for callbackIndex := 0; callbackIndex < callbacks; callbackIndex++ {
		callback, ok := a.contract.CallbackAt(operation, callbackIndex)
		if !ok {
			return Value{}, false
		}
		for effect := 0; effect < a.contract.CallbackEffectCount(callback); effect++ {
			atom, ok := a.CallbackEffectAtom(root, applicationID, operation, callback, effect)
			if !ok {
				return Value{}, false
			}
			atoms = append(atoms, atom)
		}
	}
	return a.FromAtoms(atoms)
}

// SelectedCallOpaque reduces only explicit unknown-open Target rows for one
// Rule-selected operation. Fully closed rows contribute Bottom; RowVariable
// is unsupported and therefore rejects the whole selected operation.
func (a *Algebra) SelectedCallOpaque(root Root, applicationID identity.ContentID, operation vocabulary.Operation) (Value, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, applicationID, operation) {
		return Value{}, false
	}
	tail, _, ok := a.contract.EffectTail(operation)
	if !ok || tail == vocabulary.RowVariable {
		return Value{}, false
	}
	var unknown Atom
	known := false
	if tail == vocabulary.RowUnknownOpen {
		atom, ok := a.OpenOperationUnknown(root, applicationID, operation)
		if !ok {
			return Value{}, false
		}
		unknown, known = atom, true
	}
	for callbackIndex := 0; callbackIndex < a.contract.CallbackCount(operation); callbackIndex++ {
		callback, ok := a.contract.CallbackAt(operation, callbackIndex)
		if !ok {
			return Value{}, false
		}
		callbackTail, _, ok := a.contract.CallbackEffectTail(callback)
		if !ok || callbackTail == vocabulary.RowVariable {
			return Value{}, false
		}
		if callbackTail == vocabulary.RowUnknownOpen {
			if !known {
				atom, ok := a.OpenCallbackUnknown(root, applicationID, operation, callback)
				if !ok {
					return Value{}, false
				}
				unknown, known = atom, true
			}
		}
	}
	if !known {
		return a.Bottom(), true
	}
	return a.Singleton(unknown)
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
	if !atom.validFor(a) {
		return Value{}, false
	}
	return a.value(atom.root, false, []Atom{atom}), true
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
	root := out[0].root
	for _, atom := range out {
		if !atom.validFor(a) {
			return Value{}, false
		}
		if atom.root != root {
			return Value{}, false
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessID(out[i].id, out[j].id) })
	n := 1
	for i := 1; i < len(out); i++ {
		if out[i].id != out[n-1].id {
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
	if !a.owns(value) || value.top || index < 0 || index >= len(value.atoms) {
		return Atom{}, false
	}
	return value.atoms[index], true
}

// AtomID exposes the portable certificate identity of an atom without
// exposing an inverse constructor or any of its cross-domain preimage.
func (a *Algebra) AtomID(atom Atom) (identity.ContentID, bool) {
	if !atom.validFor(a) {
		return identity.ContentID{}, false
	}
	return atom.id, true
}

// TransportAtom rehomes an existing certificate to another sealed Effect
// coordinate without decoding it or minting a new identity.  It is the only
// transport operation for outcome/boundary rules; there is no raw-ID inlet.
func (a *Algebra) TransportAtom(atom Atom, root Root) (Atom, bool) {
	if !atom.validFor(a) || !a.ownsRoot(root) {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: atom.id}, true
}

// Transport rehomes one sparse owned value. Bottom and Top carry no local
// atoms and remain their canonical context-neutral values.
func (a *Algebra) Transport(value Value, root Root) (Value, bool) {
	if !a.owns(value) || !a.ownsRoot(root) {
		return Value{}, false
	}
	if value.top {
		return a.Top(), true
	}
	if len(value.atoms) == 0 {
		return a.Bottom(), true
	}
	atoms := make([]Atom, len(value.atoms))
	for i, atom := range value.atoms {
		atoms[i] = Atom{owner: a, root: root.slot, id: atom.id}
	}
	return a.value(root.slot, false, atoms), true
}

// CompareAtoms gives the canonical identity order for two owned atoms.
func (a *Algebra) CompareAtoms(left, right Atom) (int, bool) {
	if !left.validFor(a) || !right.validFor(a) {
		return 0, false
	}
	if left.id == right.id {
		return 0, true
	}
	if lessID(left.id, right.id) {
		return -1, true
	}
	return 1, true
}

func (a *Algebra) Owns(value Value) bool { return a.owns(value) }
func (a *Algebra) Admit(root Root, value Value) bool {
	return a.ownsRoot(root) && a.owns(value) && (value.top || len(value.atoms) == 0 || value.root == root.slot)
}

func (a *Algebra) Equal(left, right Value) bool {
	if !a.owns(left) || !a.owns(right) || left.top != right.top || left.root != right.root || len(left.atoms) != len(right.atoms) {
		return false
	}
	for i := range left.atoms {
		if left.atoms[i].id != right.atoms[i].id {
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
	if left.top {
		return right.top
	}
	if right.top || len(left.atoms) == 0 {
		return true
	}
	if left.root != right.root {
		return false
	}
	if len(left.atoms) > len(right.atoms) {
		return false
	}
	i, j := 0, 0
	for i < len(left.atoms) && j < len(right.atoms) {
		if left.atoms[i].id == right.atoms[j].id {
			i++
			j++
			continue
		}
		if lessID(left.atoms[i].id, right.atoms[j].id) {
			return false
		}
		j++
	}
	return i == len(left.atoms)
}

func (a *Algebra) Join(left, right Value) (Value, bool) {
	if !a.owns(left) || !a.owns(right) {
		return Value{}, false
	}
	if left.top || right.top {
		return a.Top(), true
	}
	if a.LessOrEq(left, right) {
		return right, true
	}
	if a.LessOrEq(right, left) {
		return left, true
	}
	if left.root != right.root {
		return Value{}, false
	}
	out := make([]Atom, 0, len(left.atoms)+len(right.atoms))
	i, j := 0, 0
	for i < len(left.atoms) || j < len(right.atoms) {
		if j == len(right.atoms) || (i < len(left.atoms) && lessID(left.atoms[i].id, right.atoms[j].id)) {
			out = append(out, left.atoms[i])
			i++
			continue
		}
		if i == len(left.atoms) || lessID(right.atoms[j].id, left.atoms[i].id) {
			out = append(out, right.atoms[j])
			j++
			continue
		}
		out = append(out, left.atoms[i])
		i++
		j++
	}
	return a.value(left.root, false, out), true
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
	if value.top {
		return 0
	}
	return a.capacity + 1 - uint64(len(value.atoms))
}

func (a *Algebra) Fingerprint(value Value) uint64 {
	if !a.owns(value) {
		return 0
	}
	return value.seal
}

func (a *Algebra) sealMountedArtifacts(mounts []MountedArtifact) (map[mountedCallRef]artifactCallRow, bool) {
	receipts := make([]bodyRootReceipt, 0)
	artifactCalls := make(map[mountedCallRef]artifactCallRow)
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.Artifact == nil || !mount.Artifact.Available() || !mount.ModuleKey.Available() {
			return nil, false
		}
		if mount.Artifact.CompileKey().ProgramID() == (identity.ContentID{}) {
			return nil, false
		}
		if _, duplicate := seenMounts[mount.ModuleKey]; duplicate {
			return nil, false
		}
		seenMounts[mount.ModuleKey] = struct{}{}
		programID := mount.Artifact.CompileKey().ProgramID()
		for index := 0; index < mount.Artifact.BodyCount(); index++ {
			body, ok := mount.Artifact.BodyAt(index)
			if !ok || !body.Available() || !body.ID().Available() || !body.ContextID().Available() {
				return nil, false
			}
			receipts = append(receipts, bodyRootReceipt{moduleKey: mount.ModuleKey, programID: programID, bodyID: body.ID(), contextID: body.ContextID()})
		}
		for index := 0; index < mount.Artifact.CallCount(); index++ {
			call, callOK := mount.Artifact.CallAt(index)
			if !callOK || !call.Available() || !call.ID().Available() || !call.BodyID().Available() {
				return nil, false
			}
			ref := mountedCallRef{moduleID: mount.ModuleKey, contextID: call.ID()}
			if _, duplicate := artifactCalls[ref]; duplicate {
				return nil, false
			}
			artifactCalls[ref] = artifactCallRow{programID: programID, bodyID: call.BodyID()}
		}
	}
	ordered := append([]bodyRootReceipt(nil), receipts...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].moduleKey != ordered[j].moduleKey {
			return lessID(ordered[i].moduleKey, ordered[j].moduleKey)
		}
		if ordered[i].bodyID != ordered[j].bodyID {
			return lessID(ordered[i].bodyID, ordered[j].bodyID)
		}
		return lessID(ordered[i].contextID, ordered[j].contextID)
	})
	for _, receipt := range ordered {
		if !receipt.programID.Available() || !receipt.bodyID.Available() || !receipt.contextID.Available() || !receipt.moduleKey.Available() {
			return nil, false
		}
		contextRef := rootContextRef{module: receipt.moduleKey, context: receipt.contextID}
		bodyRef := rootBodyRef{module: receipt.moduleKey, bodyID: receipt.bodyID}
		mountedRef := rootMountedBodyRef{moduleKey: receipt.moduleKey, bodyID: receipt.bodyID}
		if a.rootContextIndex[contextRef] != 0 || a.rootBodyIndex[bodyRef] != 0 || a.rootMountedBodyIndex[mountedRef] != 0 || uint64(len(a.roots)) >= uint64(math.MaxUint32) {
			return nil, false
		}
		id := effectRootID(receipt.programID, receipt.moduleKey, receipt.contextID)
		if !id.Available() {
			return nil, false
		}
		a.roots = append(a.roots, rootRow{moduleKey: receipt.moduleKey, programID: receipt.programID, bodyID: receipt.bodyID, context: receipt.contextID, id: id})
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
	for i := 0; i < a.contract.OperationCount(); i++ {
		op, ok := a.contract.OperationAt(i)
		if !ok {
			return false
		}
		occurrences, ok = checkedAdd(occurrences, uint64(a.contract.EffectCount(op)))
		if !ok {
			return false
		}
		callbacks := a.contract.CallbackCount(op)
		for callbackIndex := 0; callbackIndex < callbacks; callbackIndex++ {
			callback, callbackOK := a.contract.CallbackAt(op, callbackIndex)
			if !callbackOK {
				return false
			}
			occurrences, ok = checkedAdd(occurrences, uint64(a.contract.CallbackEffectCount(callback)))
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

func (a *Algebra) selectedCall(root Root, applicationID identity.ContentID, owner vocabulary.Operation) bool {
	if _, available := a.applicationOperation(applicationID, owner); !available {
		return false
	}
	if !a.callInRootID(root, applicationID) {
		return false
	}
	row, rowOK := a.callRows[applicationID]
	packRoot, ok := a.packs.CallRootForMountedSemantic(row.moduleID, row.context)
	if !rowOK || !ok {
		return false
	}
	_, ok = a.packs.RootID(packRoot)
	return ok
}

func (a *Algebra) callInRootID(root Root, applicationID identity.ContentID) bool {
	if !a.ownsRoot(root) {
		return false
	}
	row, rowOK := a.callRows[applicationID]
	if !rowOK || !row.moduleID.Available() || !row.context.Available() || row.root == 0 || uint64(row.root) > uint64(len(a.roots)) {
		return false
	}
	return Root{owner: a, slot: row.root} == root
}

// inputTailArgument reports which position of an effect's Values argument
// vector substitutes the target operation's input tail. The vector is indexed
// by the target's own ValuesVar ordinals, so that position is the target's
// input var; every other position substitutes an outcome-scoped var, which
// names no input and therefore selects nothing out of the caller's Pack.
func (a *Algebra) inputTailArgument(operation vocabulary.Operation) (int, bool) {
	input, inputOK := a.contract.Input(operation)
	if !inputOK {
		return 0, false
	}
	tail, variable, tailOK := a.contract.ValuesTail(input)
	if !tailOK || tail != vocabulary.ValuesVariable {
		return 0, false
	}
	return int(variable), true
}

func (a *Algebra) validateOrdinaryInputs(owner vocabulary.Operation, effect int) bool {
	for i := 0; i < a.contract.EffectValueArgumentCount(owner, effect); i++ {
		formal, ok := a.contract.EffectValueArgumentAt(owner, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	targetOperation, targetOK := a.contract.EffectTarget(owner, effect)
	if !targetOK {
		return false
	}
	tailArgument, tailed := a.inputTailArgument(targetOperation)
	for i := 0; i < a.contract.EffectValuesArgumentCount(owner, effect); i++ {
		formal, ok := a.contract.EffectValuesArgumentAt(owner, effect, i)
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
	for i := 0; i < a.contract.CallbackEffectValueArgumentCount(callback, effect); i++ {
		formal, ok := a.contract.CallbackEffectValueArgumentAt(callback, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	targetOperation, targetOK := a.contract.CallbackEffectTarget(callback, effect)
	if !targetOK {
		return false
	}
	tailArgument, tailed := a.inputTailArgument(targetOperation)
	for i := 0; i < a.contract.CallbackEffectValuesArgumentCount(callback, effect); i++ {
		formal, ok := a.contract.CallbackEffectValuesArgumentAt(callback, effect, i)
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
	return a.Valid() && value.owner == a && value.seal != 0 &&
		uint64(len(value.atoms)) <= a.capacity &&
		((value.top && value.root == 0 && len(value.atoms) == 0) ||
			(!value.top && ((len(value.atoms) == 0 && value.root == 0) ||
				(len(value.atoms) != 0 && value.root != 0 && uint64(value.root) <= uint64(len(a.roots))))))
}
func (atom Atom) validFor(a *Algebra) bool {
	return a != nil && atom.owner == a && atom.root != 0 && uint64(atom.root) <= uint64(len(a.roots)) && atom.id.Available()
}

// value seals constructor-proven ordering and owner admission into an O(1)
// hot header.  No caller can fabricate this private seal outside this package.
func (a *Algebra) value(root uint32, top bool, atoms []Atom) Value {
	value := Value{owner: a, root: root, top: top, atoms: atoms}
	value.seal = a.valueSeal(root, top, atoms)
	return value
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
		for _, byte := range atom.id {
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

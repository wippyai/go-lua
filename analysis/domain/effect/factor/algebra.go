// Package factor owns Effect's finite may-set factor algebra.
//
// It deliberately projects the canonical Link, Pack, Target, and Program
// Static authorities.  It does not copy their payloads or retain a derived
// application/operation relation.
package factor

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/call"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
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
	id    keyspace.ContentID
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
	shard uint32
	body  keyspace.Term
}

// Algebra owns the one finite Effect factor vocabulary.  The capacity is a
// mathematical upper bound, not a materialized product or convergence cap.
type Algebra struct {
	source   *link.Link
	packs    *pack.Schema
	contract *target.Contract
	linkID   keyspace.ContentID

	roots     []rootRow
	rootIndex map[rootRef]uint32
	capacity  uint64
	unknownID keyspace.ContentID
	content   keyspace.ContentID
}

type rootRef struct {
	shard uint32
	body  keyspace.Term
}

// New seals the Factor's small owner-local vocabulary.  It stores no Target
// effect table and no Application×Operation matrix; such coordinates remain
// validated projections at atom issue time.
func New(source *link.Link, packs *pack.Schema, contract *target.Contract) (*Algebra, bool) {
	if source == nil || !source.ContentID().Available() || packs == nil || packs.Link() != source || contract == nil || source.Boundary() == nil || source.Project() == nil {
		return nil, false
	}
	linked, ok := source.Boundary().Target()
	if !ok || linked != contract {
		return nil, false
	}
	a := &Algebra{source: source, packs: packs, contract: contract, linkID: source.ContentID(), rootIndex: make(map[rootRef]uint32)}
	if !a.sealRoots() || !a.sealCapacity() {
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
	return a != nil && a.source != nil && a.packs != nil && a.packs.Link() == a.source && a.contract != nil &&
		a.linkID.Available() && a.source.ContentID() == a.linkID && a.content.Available()
}

func (a *Algebra) Link() *link.Link {
	if !a.Valid() {
		return nil
	}
	return a.source
}

func (a *Algebra) Pack() *pack.Schema {
	if !a.Valid() {
		return nil
	}
	return a.packs
}

func (a *Algebra) ContentID() keyspace.ContentID {
	if !a.Valid() {
		return keyspace.ContentID{}
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

func (a *Algebra) RootForBody(shard linkproject.Shard, body keyspace.Term) (Root, bool) {
	if !a.Valid() || a.source.Project() == nil || body == 0 {
		return Root{}, false
	}
	index, ok := a.source.Project().Mounts().Index(shard)
	if !ok || uint64(index+1) > uint64(math.MaxUint32) {
		return Root{}, false
	}
	slot := a.rootIndex[rootRef{shard: uint32(index + 1), body: body}]
	if slot == 0 {
		return Root{}, false
	}
	return Root{owner: a, slot: slot}, true
}

// RootForCall derives the exact containing Effect body for one existing
// ordinary Project Call. It retains no application index or site relation.
func (a *Algebra) RootForCall(application linkproject.Application) (Root, bool) {
	if !a.Valid() || !a.callInRootCandidate(application) {
		return Root{}, false
	}
	shard, term, ok := a.source.Project().Applications().Call(application)
	if !ok {
		return Root{}, false
	}
	p, ok := a.source.Project().Mounts().Program(shard)
	if !ok || p == nil {
		return Root{}, false
	}
	body, _, _, ok := p.Source().Index().Position(term)
	if !ok {
		return Root{}, false
	}
	return a.RootForBody(shard, body)
}

func (a *Algebra) RootIndex(root Root) (int, bool) {
	if !a.ownsRoot(root) {
		return 0, false
	}
	return int(root.slot - 1), true
}

func (a *Algebra) RootShard(root Root) (linkproject.Shard, bool) {
	if !a.ownsRoot(root) {
		return linkproject.Shard{}, false
	}
	shard, ok := a.source.Project().Mounts().At(int(a.roots[root.slot-1].shard) - 1)
	return shard, ok
}

func (a *Algebra) RootBody(root Root) (keyspace.Term, bool) {
	if !a.ownsRoot(root) {
		return 0, false
	}
	return a.roots[root.slot-1].body, true
}

// RootID is Effect's portable body-root proof. Dense root slots remain hot
// owner state and never enter rule operands or replay identities.
func (a *Algebra) RootID(root Root) (keyspace.ContentID, bool) {
	if !a.ownsRoot(root) {
		return keyspace.ContentID{}, false
	}
	row := a.roots[root.slot-1]
	if row.shard == 0 {
		return keyspace.ContentID{}, false
	}
	shard, ok := a.source.Project().Mounts().At(int(row.shard) - 1)
	if !ok {
		return keyspace.ContentID{}, false
	}
	p, ok := a.source.Project().Mounts().Program(shard)
	if !ok || p == nil || !p.ContentID().Available() {
		return keyspace.ContentID{}, false
	}
	const prefix = "wippy.analysis.effect.root.v1\x00"
	var payload [len(prefix) + sha256.Size + 4]byte
	copy(payload[:], prefix)
	id := p.ContentID()
	copy(payload[len(prefix):], id[:])
	binary.BigEndian.PutUint32(payload[len(prefix)+sha256.Size:], uint32(row.body))
	out := keyspace.ContentID(sha256.Sum256(payload[:]))
	return out, out.Available()
}

// ContainsCall validates that an existing ordinary Project Call belongs to
// this exact body root. It retains no application relation.
func (a *Algebra) ContainsCall(root Root, application linkproject.Application) bool {
	return a.callInRoot(root, application)
}

// OpaqueCallUnknown converts an exact admitted opaque Call alternative into
// the Factor's one unknown vocabulary atom. Call is evidence only: it is not
// retained, enumerated, or made into a Factor root.
func (a *Algebra) OpaqueCallUnknown(root Root, calls *call.Algebra, application linkproject.Application, value call.Value) (Atom, bool) {
	if !a.ownsRoot(root) || calls == nil || !calls.Valid() || calls.Link() != a.source || !value.HasOpaqueAlternative() {
		return Atom{}, false
	}
	key, ok := calls.KeyForApplication(application)
	if !ok || !calls.Admits(key, value) || !a.callInRoot(root, application) {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

// OpenOperationUnknown requires an exact selected operation with its explicit
// unknown-open effect row. Row variables remain unsupported and fail closed.
func (a *Algebra) OpenOperationUnknown(root Root, application linkproject.Application, owner target.Operation) (Atom, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, application, owner) {
		return Atom{}, false
	}
	tail, _, ok := a.contract.EffectTail(owner)
	if !ok || tail != target.RowUnknownOpen {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

func (a *Algebra) OpenCallbackUnknown(root Root, application linkproject.Application, owner target.Operation, callback target.CallbackID) (Atom, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, application, owner) {
		return Atom{}, false
	}
	callbackOwner, ownerOK := a.contract.CallbackOwner(callback)
	tail, _, tailOK := a.contract.CallbackEffectTail(callback)
	if !ownerOK || callbackOwner != owner || !tailOK || tail != target.RowUnknownOpen {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: a.unknownID}, true
}

// CallEffectAtom validates one selected ordinary-call effect. Row variables and
// row arguments fail closed; explicit closed/open rows retain known atoms.
func (a *Algebra) CallEffectAtom(root Root, application linkproject.Application, owner target.Operation, effect int) (Atom, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, application, owner) {
		return Atom{}, false
	}
	tail, _, tailOK := a.contract.EffectTail(owner)
	if !tailOK || (tail != target.RowClosed && tail != target.RowUnknownOpen) || effect < 0 || effect >= a.contract.EffectCount(owner) || a.contract.EffectRowArgumentCount(owner, effect) != 0 {
		return Atom{}, false
	}
	targetOp, ok := a.contract.EffectTarget(owner, effect)
	if !ok || !a.validateOrdinaryInputs(owner, effect) {
		return Atom{}, false
	}
	descriptor, ok := a.contract.EffectDescriptorID(owner, effect)
	if !ok {
		return Atom{}, false
	}
	correspondence, terms, ok := a.ordinaryTypeArguments(application, owner, effect)
	if !ok {
		return Atom{}, false
	}
	return a.issue(root, application, targetOp, descriptor, correspondence, terms)
}

// CallbackEffectAtom validates and issues one callback-owned selected-call
// atom. Callback occurrence provenance does not enter atom identity.
func (a *Algebra) CallbackEffectAtom(root Root, application linkproject.Application, owner target.Operation, callback target.CallbackID, effect int) (Atom, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, application, owner) || effect < 0 || a.contract.CallbackEffectCount(callback) <= effect {
		return Atom{}, false
	}
	callbackOwner, ownerOK := a.contract.CallbackOwner(callback)
	tail, _, tailOK := a.contract.CallbackEffectTail(callback)
	if !ownerOK || callbackOwner != owner || !tailOK || (tail != target.RowClosed && tail != target.RowUnknownOpen) || a.contract.CallbackEffectRowArgumentCount(callback, effect) != 0 {
		return Atom{}, false
	}
	targetOp, ok := a.contract.CallbackEffectTarget(callback, effect)
	if !ok || !a.validateCallbackInputs(owner, callback, effect) {
		return Atom{}, false
	}
	descriptor, ok := a.contract.CallbackEffectDescriptorID(callback, effect)
	if !ok {
		return Atom{}, false
	}
	correspondence, terms, ok := a.callbackTypeArguments(application, owner, callback, effect)
	if !ok {
		return Atom{}, false
	}
	return a.issue(root, application, targetOp, descriptor, correspondence, terms)
}

// SelectedCallEffects reduces the explicit ordinary and callback effects of
// one operation already selected by a Rule-owned Call target. It keeps no
// selection state: every invocation revalidates the canonical witnesses.
// Unsupported authored rows fail closed so a caller cannot silently omit them.
func (a *Algebra) SelectedCallEffects(root Root, application linkproject.Application, operation target.Operation) (Value, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, application, operation) {
		return Value{}, false
	}
	tail, _, ok := a.contract.EffectTail(operation)
	if !ok || (tail != target.RowClosed && tail != target.RowUnknownOpen) {
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
		if !ok || (callbackTail != target.RowClosed && callbackTail != target.RowUnknownOpen) {
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
		atom, ok := a.CallEffectAtom(root, application, operation, effect)
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
			atom, ok := a.CallbackEffectAtom(root, application, operation, callback, effect)
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
func (a *Algebra) SelectedCallOpaque(root Root, application linkproject.Application, operation target.Operation) (Value, bool) {
	if !a.ownsRoot(root) || !a.selectedCall(root, application, operation) {
		return Value{}, false
	}
	tail, _, ok := a.contract.EffectTail(operation)
	if !ok || tail == target.RowVariable {
		return Value{}, false
	}
	var unknown Atom
	known := false
	if tail == target.RowUnknownOpen {
		atom, ok := a.OpenOperationUnknown(root, application, operation)
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
		if !ok || callbackTail == target.RowVariable {
			return Value{}, false
		}
		if callbackTail == target.RowUnknownOpen {
			if !known {
				atom, ok := a.OpenCallbackUnknown(root, application, operation, callback)
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
func (a *Algebra) AtomID(atom Atom) (keyspace.ContentID, bool) {
	if !atom.validFor(a) {
		return keyspace.ContentID{}, false
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

func (a *Algebra) sealRoots() bool {
	mounts := a.source.Project().Mounts()
	for i := 0; i < mounts.Count(); i++ {
		shard, ok := mounts.At(i)
		if !ok || uint64(i+1) > uint64(math.MaxUint32) {
			return false
		}
		program, ok := mounts.Program(shard)
		if !ok || program == nil {
			return false
		}
		bodyCount := program.Source().Identity().FamilyCount(keyspace.FamilyBody)
		for ordinal := 1; ordinal <= bodyCount; ordinal++ {
			body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
			if !program.Flow().Executable().Contains(body) {
				continue
			}
			if uint64(len(a.roots)) >= uint64(math.MaxUint32) {
				return false
			}
			ref := rootRef{shard: uint32(i + 1), body: body}
			if a.rootIndex[ref] != 0 {
				return false
			}
			a.roots = append(a.roots, rootRow{shard: uint32(i + 1), body: body})
			a.rootIndex[ref] = uint32(len(a.roots))
		}
	}
	return true
}

func (a *Algebra) sealCapacity() bool {
	contexts := uint64(a.source.Project().Applications().Calls().Count())
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

func (a *Algebra) selectedCall(root Root, application linkproject.Application, owner target.Operation) bool {
	if !a.source.Boundary().ApplicationOperationAvailable(a.contract, application, owner) {
		return false
	}
	if !a.callInRoot(root, application) {
		return false
	}
	packRoot, ok := a.packs.CallRoot(application)
	if !ok {
		return false
	}
	_, ok = a.packs.RootID(packRoot)
	return ok
}

func (a *Algebra) callInRoot(root Root, application linkproject.Application) bool {
	if !a.ownsRoot(root) {
		return false
	}
	shard, call, ok := a.source.Project().Applications().Call(application)
	if !ok {
		return false
	}
	program, ok := a.source.Project().Mounts().Program(shard)
	if !ok || program == nil {
		return false
	}
	body, _, _, ok := program.Source().Index().Position(call)
	shardIndex, shardOK := a.source.Project().Mounts().Index(shard)
	if !ok || !shardOK || body != a.roots[root.slot-1].body || uint32(shardIndex+1) != a.roots[root.slot-1].shard {
		return false
	}
	return true
}

func (a *Algebra) callInRootCandidate(application linkproject.Application) bool {
	if !a.Valid() {
		return false
	}
	shard, term, ok := a.source.Project().Applications().Call(application)
	if !ok {
		return false
	}
	p, ok := a.source.Project().Mounts().Program(shard)
	if !ok || p == nil {
		return false
	}
	body, _, _, ok := p.Source().Index().Position(term)
	return ok && p.Flow().Executable().Contains(body)
}

func (a *Algebra) validateOrdinaryInputs(owner target.Operation, effect int) bool {
	for i := 0; i < a.contract.EffectValueArgumentCount(owner, effect); i++ {
		formal, ok := a.contract.EffectValueArgumentAt(owner, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	for i := 0; i < a.contract.EffectValuesArgumentCount(owner, effect); i++ {
		formal, ok := a.contract.EffectValuesArgumentAt(owner, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	return true
}

func (a *Algebra) validateCallbackInputs(owner target.Operation, callback target.CallbackID, effect int) bool {
	for i := 0; i < a.contract.CallbackEffectValueArgumentCount(callback, effect); i++ {
		formal, ok := a.contract.CallbackEffectValueArgumentAt(callback, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	for i := 0; i < a.contract.CallbackEffectValuesArgumentCount(callback, effect); i++ {
		formal, ok := a.contract.CallbackEffectValuesArgumentAt(callback, effect, i)
		if !ok {
			return false
		}
		if _, ok = a.packs.InputSelector(owner, target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: uint32(formal)}); !ok {
			return false
		}
	}
	return true
}

func (a *Algebra) ordinaryTypeArguments(application linkproject.Application, owner target.Operation, effect int) (keyspace.ContentID, []keyspace.Term, bool) {
	count := a.contract.EffectTypeArgumentCount(owner, effect)
	return a.selectedTypeArguments(application, owner, count, func(i int) (target.TypeFormal, bool) { return a.contract.EffectTypeArgumentAt(owner, effect, i) })
}

func (a *Algebra) callbackTypeArguments(application linkproject.Application, owner target.Operation, callback target.CallbackID, effect int) (keyspace.ContentID, []keyspace.Term, bool) {
	count := a.contract.CallbackEffectTypeArgumentCount(callback, effect)
	return a.selectedTypeArguments(application, owner, count, func(i int) (target.TypeFormal, bool) {
		return a.contract.CallbackEffectTypeArgumentAt(callback, effect, i)
	})
}

func (a *Algebra) selectedTypeArguments(application linkproject.Application, owner target.Operation, count int, at func(int) (target.TypeFormal, bool)) (keyspace.ContentID, []keyspace.Term, bool) {
	if count == 0 {
		return keyspace.ContentID{}, nil, true
	}
	arguments, ok := a.source.Boundary().Calls().TypeFormalArguments(a.contract, application, owner)
	if !ok {
		return keyspace.ContentID{}, nil, false
	}
	correspondence, ok := arguments.CorrespondenceID()
	if !ok {
		return keyspace.ContentID{}, nil, false
	}
	terms := make([]keyspace.Term, count)
	for i := 0; i < count; i++ {
		formal, ok := at(i)
		if !ok || uint64(formal) >= uint64(arguments.Count()) {
			return keyspace.ContentID{}, nil, false
		}
		term, ok := arguments.At(int(formal))
		if !ok {
			return keyspace.ContentID{}, nil, false
		}
		terms[i] = term
	}
	return correspondence, terms, true
}

func (a *Algebra) issue(root Root, application linkproject.Application, targetOp target.Operation, descriptor, correspondence keyspace.ContentID, terms []keyspace.Term) (Atom, bool) {
	operationID, ok := a.contract.EffectOperationID(targetOp)
	if !ok || !descriptor.Available() {
		return Atom{}, false
	}
	packRoot, ok := a.packs.CallRoot(application)
	if !ok {
		return Atom{}, false
	}
	rootID, ok := a.packs.RootID(packRoot)
	if !ok {
		return Atom{}, false
	}
	id := atomID(operationID, descriptor, rootID, correspondence, terms)
	if !id.Available() {
		return Atom{}, false
	}
	return Atom{owner: a, root: root.slot, id: id}, true
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

func (a *Algebra) contentID() keyspace.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.effect.factor.v3\x00"))
	_, _ = h.Write(a.linkID[:])
	for _, root := range a.roots {
		var word [8]byte
		binary.BigEndian.PutUint32(word[:4], root.shard)
		binary.BigEndian.PutUint32(word[4:], uint32(root.body))
		_, _ = h.Write(word[:])
	}
	var out keyspace.ContentID
	copy(out[:], h.Sum(nil))
	return out
}

func externalID() keyspace.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.effect.atom.v2.unknown\x00"))
	var out keyspace.ContentID
	copy(out[:], h.Sum(nil))
	return out
}
func atomID(operation, descriptor, root, static keyspace.ContentID, args []keyspace.Term) keyspace.ContentID {
	if !operation.Available() || !descriptor.Available() || !root.Available() || (len(args) != 0 && !static.Available()) {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.effect.atom.v1\x00"))
	_, _ = h.Write(operation[:])
	_, _ = h.Write(descriptor[:])
	_, _ = h.Write(root[:])
	var word [4]byte
	binary.BigEndian.PutUint32(word[:], uint32(len(args)))
	_, _ = h.Write(word[:])
	if len(args) != 0 {
		_, _ = h.Write(static[:])
	}
	for _, argument := range args {
		binary.BigEndian.PutUint32(word[:], uint32(argument))
		_, _ = h.Write(word[:])
	}
	var out keyspace.ContentID
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
func lessID(left, right keyspace.ContentID) bool {
	for i := range left {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return false
}

package call

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// keyKind is the closed source-sum discriminator.  It is deliberately kept
// inside Call: Project and Target retain their own relations and Call stores
// only the exact owner-issued identity of each arm.
type keyKind uint8

const (
	keyApplication keyKind = iota + 1
	keyCallback
	keyResume
)

type keyRow struct {
	kind        keyKind
	application linkproject.Application
	operation   target.Operation
	callback    target.CallbackID
	resume      target.ResumeID
	id          keyspace.ContentID
}

// Algebra is the single Link-scoped owner of the complete Call Factor family.
// Existing Project base Applications, Target callbacks, and Target resumes
// are the only Call keys. Every Value belongs only to this shared algebra.
// Global target selectors are factorized once and are never crossed with the
// closed source sum (there is no B×Operation/Port product).
type Algebra struct {
	source          *link.Link
	linkID          keyspace.ContentID
	content         keyspace.ContentID
	keys            []keyRow
	keyIndex        map[keyspace.ContentID]uint32
	targets         []targetRow
	targetIndex     map[targetKey]selector
	functionIndex   map[functionTargetKey]selector
	bodyTargetCount int
	bottom          Value
	top             Value
}

// New builds one immutable Call family from one sealed Link.
func New(source *link.Link) (*Algebra, bool) {
	if source == nil || !source.ContentID().Available() {
		return nil, false
	}
	algebra := &Algebra{
		source: source, linkID: source.ContentID(),
		keyIndex: make(map[keyspace.ContentID]uint32), targetIndex: make(map[targetKey]selector),
		functionIndex: make(map[functionTargetKey]selector),
	}
	if !algebra.buildTargets() || !algebra.buildKeys() {
		return nil, false
	}
	algebra.content = algebraContentID(algebra.linkID)
	if !algebra.content.Available() {
		return nil, false
	}
	algebra.bottom = Value{owner: algebra, known: true}
	algebra.top = Value{owner: algebra, top: true}
	return algebra, true
}

func (algebra *Algebra) buildKeys() bool {
	project := algebra.source.Project()
	if project == nil {
		return false
	}
	applications := project.Applications()
	// Bases is Project's canonical executable base subsequence.  Walking it
	// directly keeps admission O(B) and never scans/import-filters the full
	// Application table. IsBase remains the owner-issued membership fence for
	// each returned handle.
	bases := applications.Bases()
	for index := 0; index < bases.Count(); index++ {
		application, ok := bases.At(index)
		if !ok {
			return false
		}
		// IsBase is Project's closed membership relation.  In particular it
		// excludes imports without reconstructing an Application or inventing
		// a second Call-side classification table.
		if !applications.IsBase(application) {
			continue
		}
		row := keyRow{kind: keyApplication, application: application}
		if !algebra.appendKey(row) {
			return false
		}
	}
	contract, ok := algebra.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	// Target owns callback/resume correspondence.  Iterate the sealed
	// operation-local ranges exactly once; no Subedge or Application key is
	// synthesized, and no cross product is retained.
	for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
		operation, present := contract.OperationAt(operationIndex)
		if !present {
			return false
		}
		for callbackIndex := 0; callbackIndex < contract.CallbackCount(operation); callbackIndex++ {
			callback, present := contract.CallbackAt(operation, callbackIndex)
			if !present {
				return false
			}
			id, present := contract.CallbackContentID(operation, callback)
			if !present {
				return false
			}
			if !algebra.appendKey(keyRow{kind: keyCallback, operation: operation, callback: callback, id: id}) {
				return false
			}
		}
		for resumeIndex := 0; resumeIndex < contract.ResumeCount(operation); resumeIndex++ {
			resume, present := contract.ResumeIDAt(operation, resumeIndex)
			if !present {
				return false
			}
			id, present := contract.ResumeContentID(operation, resume)
			if !present {
				return false
			}
			if !algebra.appendKey(keyRow{kind: keyResume, operation: operation, resume: resume, id: id}) {
				return false
			}
		}
	}
	return true
}

func (algebra *Algebra) appendKey(row keyRow) bool {
	if algebra == nil || len(algebra.keys) == int(^uint32(0)) {
		return false
	}
	switch row.kind {
	case keyApplication:
		if row.application == (linkproject.Application{}) {
			return false
		}
	case keyCallback:
		if row.operation == 0 || row.callback == 0 {
			return false
		}
	case keyResume:
		if row.operation == 0 || row.resume == 0 {
			return false
		}
	default:
		return false
	}
	project := algebra.source.Project()
	if project == nil {
		return false
	}
	if row.id == (keyspace.ContentID{}) {
		id, ok := project.ApplicationID(row.application)
		if !ok || !id.Available() {
			return false
		}
		row.id = id
	}
	if !row.id.Available() || algebra.keyIndex[row.id] != 0 {
		return false
	}
	algebra.keys = append(algebra.keys, row)
	algebra.keyIndex[row.id] = uint32(len(algebra.keys))
	return true
}

func (algebra *Algebra) Valid() bool {
	return algebra != nil && algebra.source != nil && algebra.linkID.Available() && algebra.content.Available() &&
		algebra.bodyTargetCount >= 0 && algebra.bodyTargetCount <= len(algebra.targets)
}
func (algebra *Algebra) ContentID() keyspace.ContentID {
	if !algebra.Valid() {
		return keyspace.ContentID{}
	}
	return algebra.content
}
func (algebra *Algebra) LinkID() keyspace.ContentID {
	if !algebra.Valid() {
		return keyspace.ContentID{}
	}
	return algebra.linkID
}

// Link returns Call's one sealed structural authority.  The pointer is an
// owner fence, not a replay identity: callers that bind a Call coordinate to
// another domain must reject a separately sealed Link even when its content
// happens to be equal.
func (algebra *Algebra) Link() *link.Link {
	if !algebra.Valid() {
		return nil
	}
	return algebra.source
}
func (algebra *Algebra) KeyCount() int {
	if !algebra.Valid() {
		return 0
	}
	return len(algebra.keys)
}
func (algebra *Algebra) KeyAt(index int) (Key, bool) {
	if !algebra.Valid() || index < 0 || index >= len(algebra.keys) {
		return Key{}, false
	}
	return Key{owner: algebra, slot: uint32(index + 1)}, true
}

// KeyForApplication looks up one existing Project base Application.  The
// Project owner fence is checked before deriving its portable identity.
func (algebra *Algebra) KeyForApplication(application linkproject.Application) (Key, bool) {
	if !algebra.Valid() {
		return Key{}, false
	}
	project := algebra.source.Project()
	if project == nil {
		return Key{}, false
	}
	if !project.Applications().IsBase(application) {
		return Key{}, false
	}
	id, ok := project.ApplicationID(application)
	if !ok || !id.Available() {
		return Key{}, false
	}
	return algebra.keyForID(id, keyApplication)
}

// KeyForCallback looks up one exact Target callback correspondence. The
// issuing Contract pointer is an authority fence: equal numeric handles from
// an equivalent Contract cannot be spliced into this Link.
func (algebra *Algebra) KeyForCallback(issuing *target.Contract, operation target.Operation, callback target.CallbackID) (Key, bool) {
	if !algebra.Valid() {
		return Key{}, false
	}
	contract, ok := algebra.source.Boundary().Target()
	if !ok || contract == nil || issuing == nil || issuing != contract {
		return Key{}, false
	}
	id, ok := contract.CallbackContentID(operation, callback)
	if !ok || !id.Available() {
		return Key{}, false
	}
	return algebra.keyForID(id, keyCallback)
}

// KeyForResume looks up one exact Target resumption correspondence. The
// issuing Contract pointer is an authority fence for the raw operation and
// resume handles.
func (algebra *Algebra) KeyForResume(issuing *target.Contract, operation target.Operation, resume target.ResumeID) (Key, bool) {
	if !algebra.Valid() {
		return Key{}, false
	}
	contract, ok := algebra.source.Boundary().Target()
	if !ok || contract == nil || issuing == nil || issuing != contract {
		return Key{}, false
	}
	id, ok := contract.ResumeContentID(operation, resume)
	if !ok || !id.Available() {
		return Key{}, false
	}
	return algebra.keyForID(id, keyResume)
}

func (algebra *Algebra) keyForID(id keyspace.ContentID, kind keyKind) (Key, bool) {
	if !algebra.Valid() || !id.Available() {
		return Key{}, false
	}
	slot := algebra.keyIndex[id]
	if slot == 0 || uint64(slot) > uint64(len(algebra.keys)) || algebra.keys[slot-1].kind != kind {
		return Key{}, false
	}
	key := Key{owner: algebra, slot: slot}
	return key, key.Valid()
}

// FindKey restores one existing Call source-sum key by its portable identity.
func (algebra *Algebra) FindKey(id keyspace.ContentID) (Key, bool) {
	if !algebra.Valid() || !id.Available() {
		return Key{}, false
	}
	key := Key{owner: algebra, slot: algebra.keyIndex[id]}
	return key, key.Valid()
}
func (algebra *Algebra) validKey(key Key) bool {
	return algebra != nil && key.owner == algebra && key.slot != 0 && uint64(key.slot) <= uint64(len(algebra.keys))
}
func (algebra *Algebra) dynamic(key Key) bool {
	return algebra.validKey(key)
}
func (algebra *Algebra) contains(key Key, selector selector) bool {
	if !algebra.validKey(key) || !selector.valid() || uint64(selector) > uint64(len(algebra.targets)) {
		return false
	}
	return true
}
func (algebra *Algebra) SupportCount(key Key) int {
	if !algebra.validKey(key) {
		return 0
	}
	return len(algebra.targets)
}
func (algebra *Algebra) SupportTargetAt(key Key, index int) (Target, bool) {
	if !algebra.validKey(key) || index < 0 || index >= algebra.SupportCount(key) {
		return Target{}, false
	}
	return algebra.targetForSelector(selector(index + 1))
}
func (algebra *Algebra) OpaqueAdmitted(key Key) bool { return algebra.dynamic(key) }

// TargetForFunction projects one sealed Program function into an owner-bound
// Call capability. The dense target identity never crosses this API.
func (algebra *Algebra) TargetForFunction(shard linkproject.Shard, function keyspace.Term) (Target, bool) {
	if !algebra.Valid() {
		return Target{}, false
	}
	selector := algebra.functionIndex[functionTargetKey{shard: shard, function: function}]
	return algebra.targetForSelector(selector)
}

// bodyContentID is the portable identity for one exact Program body target.
// The dense Call target selector is intentionally absent: equivalent Links
// with the same mounted Program content and body term receive the same cold
// identity, while independent live Algebra owners still fence the hot Body
// capability through its owner pointer.
func (algebra *Algebra) bodyContentID(shard linkproject.Shard, body keyspace.Term) (keyspace.ContentID, bool) {
	if algebra == nil || !algebra.Valid() || shard == (linkproject.Shard{}) || body == 0 || keyspace.TermFamily(body) != keyspace.FamilyBody || algebra.source.Project() == nil {
		return keyspace.ContentID{}, false
	}
	mounts := algebra.source.Project().Mounts()
	program, ok := mounts.Program(shard)
	if !ok || program == nil || !program.ContentID().Available() || !program.Flow().Executable().Contains(body) {
		return keyspace.ContentID{}, false
	}
	const prefix = "wippy.analysis.call.body.v1\x00"
	var payload [len(prefix) + sha256.Size + 4]byte
	copy(payload[:], prefix)
	id := program.ContentID()
	copy(payload[len(prefix):], id[:])
	binary.BigEndian.PutUint32(payload[len(prefix)+sha256.Size:], uint32(body))
	out := keyspace.ContentID(sha256.Sum256(payload[:]))
	return out, out.Available()
}

// TargetForSeed projects one sealed Boundary seed into an owner-bound Call
// capability. Foreign equivalent Boundary seeds fail closed.
func (algebra *Algebra) TargetForSeed(seed linkboundary.Seed) (Target, bool) {
	if !algebra.Valid() {
		return Target{}, false
	}
	selector := algebra.targetIndex[targetKey{kind: targetSeed, seed: seed}]
	return algebra.targetForSelector(selector)
}

// Equivalent performs cold exact replay validation after the content prefilter.
func (algebra *Algebra) Equivalent(other *Algebra) bool {
	if !algebra.Valid() || !other.Valid() || algebra.content != other.content || algebra.linkID != other.linkID || len(algebra.keys) != len(other.keys) || len(algebra.targets) != len(other.targets) || algebra.bodyTargetCount != other.bodyTargetCount {
		return false
	}
	for index := range algebra.keys {
		left, right := algebra.keys[index], other.keys[index]
		// Project Application handles are hot owner fences and cannot be
		// compared across equivalent Links. Their stable ApplicationID is
		// the replay relation used here.
		if left.kind != right.kind || left.id != right.id {
			return false
		}
		if left.kind == keyCallback && (left.operation != right.operation || left.callback != right.callback) {
			return false
		}
		if left.kind == keyResume && (left.operation != right.operation || left.resume != right.resume) {
			return false
		}
	}
	for index := range algebra.targets {
		if !equivalentTargetRow(algebra, algebra.targets[index], other, other.targets[index]) {
			return false
		}
	}
	return true
}

// equivalentTargetRow compares portable constituents only after Link content
// has already matched.  Hot selector lookup still requires the exact Project
// or Boundary handle; cold rebind must not compare those owner fences by raw
// pointer, because equivalent independently sealed Links intentionally issue
// distinct handles.
func equivalentTargetRow(left *Algebra, leftRow targetRow, right *Algebra, rightRow targetRow) bool {
	if left == nil || right == nil || leftRow.key.kind != rightRow.key.kind || leftRow.function != rightRow.function {
		return false
	}
	switch leftRow.key.kind {
	case targetBody:
		leftProject, rightProject := left.source.Project(), right.source.Project()
		if leftProject == nil || rightProject == nil || leftRow.key.body != rightRow.key.body {
			return false
		}
		leftIndex, leftOK := leftProject.Mounts().Index(leftRow.key.shard)
		rightIndex, rightOK := rightProject.Mounts().Index(rightRow.key.shard)
		return leftOK && rightOK && leftIndex == rightIndex
	case targetSeed:
		leftBoundary, rightBoundary := left.source.Boundary(), right.source.Boundary()
		if leftBoundary == nil || rightBoundary == nil {
			return false
		}
		leftID, leftOK := leftBoundary.Seeds().ID(leftRow.key.seed)
		rightID, rightOK := rightBoundary.Seeds().ID(rightRow.key.seed)
		return leftOK && rightOK && leftID == rightID
	default:
		return false
	}
}

func (algebra *Algebra) Rebind(value Value) (Value, bool) {
	if !algebra.Valid() || !value.valid() || !algebra.Equivalent(value.owner) {
		return Value{}, false
	}
	if value.owner == algebra {
		return value, true
	}
	if value.top {
		return algebra.top, true
	}
	if !value.open && len(value.selectors) == 0 {
		return algebra.bottom, true
	}
	return Value{owner: algebra, known: true, open: value.open, selectors: value.selectors}, true
}

func algebraContentID(linkID keyspace.ContentID) (id keyspace.ContentID) {
	var payload [32 + 24]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], 0x63616c6c2d616c67) // "call-alg"
	binary.BigEndian.PutUint64(payload[40:48], 5)
	binary.BigEndian.PutUint64(payload[48:56], 3) // Base Application + Callback + Resume closed sum
	return sha256.Sum256(payload[:])
}

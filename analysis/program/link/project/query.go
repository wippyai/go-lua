package project

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// viewLive keeps construction views tied to the shared Draft lifecycle while
// leaving finalized Component views permanently readable over the same
// immutable authority. Opaque handles intentionally carry only authority, so
// a handle issued before Finalize can still be consumed by the Component that
// owns that exact authority.
func viewLive(authority *authority, draft *draftState) bool {
	if authority == nil {
		return false
	}
	return draft == nil || (!draft.consumed && draft.authority == authority)
}

func (v Mounts) live() bool       { return viewLive(v.authority, v.draft) }
func (v Keys) live() bool         { return viewLive(v.authority, v.draft) }
func (v Applications) live() bool { return viewLive(v.authority, v.draft) }
func (v Calls) live() bool        { return viewLive(v.authority, v.draft) }
func (v Imports) live() bool      { return viewLive(v.authority, v.draft) }
func (v Bases) live() bool        { return viewLive(v.authority, v.draft) }

func (d *Draft) Mounts() Mounts {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return Mounts{}
	}
	return Mounts{authority: d.state.authority, draft: d.state}
}
func (d *Draft) Keys() Keys {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return Keys{}
	}
	return Keys{authority: d.state.authority, draft: d.state}
}
func (d *Draft) Cold() Cold {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return Cold{}
	}
	return coldSnapshot(d.state.authority, d.state.fence)
}
func (d *Draft) Applications() Applications {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return Applications{}
	}
	return Applications{authority: d.state.authority, draft: d.state}
}
func (c *Component) Mounts() Mounts {
	if c == nil {
		return Mounts{}
	}
	return Mounts{authority: c.authority}
}
func (c *Component) Keys() Keys {
	if c == nil {
		return Keys{}
	}
	return Keys{authority: c.authority}
}
func (c *Component) Cold() Cold {
	if c == nil {
		return Cold{}
	}
	return coldSnapshot(c.authority, nil)
}
func (c *Component) Applications() Applications {
	if c == nil {
		return Applications{}
	}
	return Applications{authority: c.authority}
}

// MatchesTarget authenticates the exact immutable Target authority used while
// this Project was built. A ContentID match alone is deliberately insufficient
// for hot scalar-coordinate validation: equivalent reseals have distinct
// owner instances and must not exchange handles.
func (c *Component) MatchesTarget(contract *contract.Contract) bool {
	return c != nil && c.authority != nil && contract != nil && c.authority.target == contract
}

// MountRelationID exposes only the canonical authored-mount relation digest.
// It excludes Target, derived Applications, and the enclosing Link identity;
// callers needing a specific mount must still present an exact owner-fenced
// Shard to ModuleKey or Mounts.
func (c *Component) MountRelationID() (identity.ContentID, bool) {
	if c == nil || c.authority == nil || !c.authority.mountContentID.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.mountContentID, true
}

// ApplicationRelationID exposes only the narrow Project Application relation
// digest needed by the dependent Boundary component. It excludes Target's
// operation product and the enclosing Link identity.
func (c *Component) ApplicationRelationID() (identity.ContentID, bool) {
	if c == nil || c.authority == nil || !c.authority.applicationContentID.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.applicationContentID, true
}

func (v Mounts) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.authority.mounts)
}
func (v Mounts) At(index int) (Shard, bool) {
	if !v.live() || index < 0 || index >= len(v.authority.mounts) {
		return Shard{}, false
	}
	return Shard{authority: v.authority, ordinal: uint32(index + 1)}, true
}
func (v Mounts) Index(shard Shard) (int, bool) {
	if !v.live() || shard.authority != v.authority || shard.ordinal == 0 || uint64(shard.ordinal) > uint64(len(v.authority.mounts)) {
		return 0, false
	}
	return int(shard.ordinal - 1), true
}
func (v Mounts) Program(shard Shard) (*program.Program, bool) {
	index, ok := v.Index(shard)
	if !ok {
		return nil, false
	}
	p := v.authority.mounts[index].program
	return p, p != nil
}

// ProgramID returns the immutable reusable Program identity for one exact
// owner-fenced Project mount.  Artifact consumers use this scalar projection
// to authenticate a mounted artifact without reopening or retaining the
// Program object itself.
func (v Mounts) ProgramID(shard Shard) (identity.ContentID, bool) {
	index, ok := v.Index(shard)
	if !ok {
		return identity.ContentID{}, false
	}
	p := v.authority.mounts[index].program
	if p == nil || !p.ContentID().Available() {
		return identity.ContentID{}, false
	}
	return p.ContentID(), true
}

func (v Mounts) Name(shard Shard) (string, bool) {
	index, ok := v.Index(shard)
	if !ok {
		return "", false
	}
	return v.authority.mounts[index].name, true
}

func (v Keys) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.authority.keys)
}
func (v Keys) At(index int) (Key, bool) {
	if !v.live() || index < 0 || index >= len(v.authority.keys) {
		return Key{}, false
	}
	return Key{authority: v.authority, ordinal: uint32(index + 1)}, true
}
func (v Keys) Index(key Key) (int, bool) {
	if !v.live() || key.authority != v.authority || key.ordinal == 0 || uint64(key.ordinal) > uint64(len(v.authority.keys)) {
		return 0, false
	}
	return int(key.ordinal - 1), true
}

// Compare orders two Project keys by their canonical dense relation position
// after validating both owner fences.
func (v Keys) Compare(left, right Key) (int, bool) {
	leftIndex, leftOK := v.Index(left)
	rightIndex, rightOK := v.Index(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	if leftIndex < rightIndex {
		return -1, true
	}
	if leftIndex > rightIndex {
		return 1, true
	}
	return 0, true
}
func (v Keys) Exact(key Key) (keyspace.LiteralValue, bool) {
	index, ok := v.Index(key)
	if !ok {
		return keyspace.LiteralValue{}, false
	}
	return v.authority.keys[index].value, true
}

// ForProgram resolves through a dense, build-derived Program-Key to Link-Key
// quotient table. The exact mounted Program is required so an equivalent
// resealed Program from another authority cannot borrow the mapping.
func (v Keys) ForProgram(shard Shard, owner *program.Program, key keyspace.Key) (Key, bool) {
	if !v.live() || owner == nil || key == 0 {
		return Key{}, false
	}
	shardIndex, shardOK := Mounts(v).Index(shard)
	if !shardOK || v.authority.mounts[shardIndex].program != owner {
		return Key{}, false
	}
	keys := v.authority.programKeys[shardIndex]
	if uint64(key) > uint64(len(keys)) {
		return Key{}, false
	}
	return Key{authority: v.authority, ordinal: keys[key-1] + 1}, true
}

// ForMounted resolves an already-authenticated reusable Program key through
// its concrete ModuleKey. It is the artifact binding lane: no Program handle
// is reopened or accepted after the Project has sealed its key quotient.
func (v Keys) ForMounted(module identity.ContentID, key keyspace.Key) (Key, bool) {
	if !v.live() || !module.Available() || key == 0 {
		return Key{}, false
	}
	for index, mount := range v.authority.mounts {
		if mount.key != module {
			continue
		}
		keys := v.authority.programKeys[index]
		if uint64(key) > uint64(len(keys)) {
			return Key{}, false
		}
		return Key{authority: v.authority, ordinal: keys[key-1] + 1}, true
	}
	return Key{}, false
}
func (v Keys) ForTarget(contract *contract.Contract, key vocabulary.ExactKey) (Key, bool) {
	if !v.live() || contract == nil || contract != v.authority.target || key == 0 || uint64(key) > uint64(len(v.authority.targetKeys)) {
		return Key{}, false
	}
	return Key{authority: v.authority, ordinal: v.authority.targetKeys[key-1] + 1}, true
}

// TargetFor resolves an exact Project key back to the canonical Target key
// from the same sealed Project/Target pair.  The inverse is a compact,
// owner-local quotient index built at Project seal time; no Target row scan,
// literal reconstruction, or second identity is involved.  Project keys
// authored only by Program/source data have no Target counterpart and fail
// closed.
func (v Keys) TargetFor(contract *contract.Contract, key Key) (vocabulary.ExactKey, bool) {
	if !v.live() || contract == nil || contract != v.authority.target {
		return 0, false
	}
	index, ok := v.Index(key)
	if !ok || index < 0 || index >= len(v.authority.targetKeyByProject) {
		return 0, false
	}
	targetKey := v.authority.targetKeyByProject[index]
	if targetKey == 0 || uint64(targetKey) > uint64(contract.ExactKeyCount()) {
		return 0, false
	}
	return targetKey, true
}

func (v Keys) ForInitial(contract *contract.Contract, value vocabulary.InitialValue) (Key, bool) {
	if !v.live() || contract == nil || contract != v.authority.target || value == 0 {
		return Key{}, false
	}
	key, ok := v.authority.initialKeys[value]
	if !ok || uint64(key) >= uint64(len(v.authority.keys)) {
		return Key{}, false
	}
	return Key{authority: v.authority, ordinal: key + 1}, true
}

func (v Applications) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.authority.applications)
}
func (v Applications) At(index int) (Application, bool) {
	if !v.live() || index < 0 || index >= len(v.authority.applications) {
		return Application{}, false
	}
	return Application{authority: v.authority, ordinal: uint32(index + 1)}, true
}

// Index returns the canonical zero-based position of one validated
// Application. Persistence uses this position plus one to preserve the
// historical dense Link encoding without exposing the row discriminator.
func (v Applications) Index(application Application) (int, bool) {
	if _, ok := v.application(application); !ok {
		return 0, false
	}
	return int(application.ordinal - 1), true
}
func (v Applications) Calls() Calls {
	return Calls(v)
}
func (v Applications) Imports() Imports {
	return Imports(v)
}
func (v Applications) Operators() Operators {
	return Operators(v)
}
func (v Applications) Bases() Bases {
	return Bases(v)
}

// IsBase reports whether application is a member of the sealed Bases
// subsequence.  Application ordinals are the dense index of the canonical
// application rows, so the row's kind is the existing O(1) membership index;
// no second membership table or semantic identity is needed.  The common
// application lookup first enforces both the exact Project authority and the
// ordinal range, making same-ordinal handles from another Project fail closed.
func (v Applications) IsBase(application Application) bool {
	row, ok := v.application(application)
	if !ok {
		return false
	}
	switch row.kind {
	case applicationCall, applicationMeta, applicationGeneric:
		return true
	default:
		return false
	}
}

// ContentID returns the detached identity of this exact application row.
func (application Application) ContentID() (identity.ContentID, bool) {
	if application.authority == nil {
		return identity.ContentID{}, false
	}
	return (&Component{authority: application.authority}).ApplicationID(application)
}

// IsBase reports the canonical base subsequence membership for this exact
// application handle.
func (application Application) IsBase() bool {
	if application.authority == nil {
		return false
	}
	return (Applications{authority: application.authority}).IsBase(application)
}

func (v Applications) Call(application Application) (Shard, keyspace.Term, bool) {
	row, ok := v.application(application)
	if !ok || row.kind != applicationCall {
		return Shard{}, 0, false
	}
	return Shard{authority: v.authority, ordinal: row.shard}, row.term, true
}

// Generic identifies one exact executable generic-for application.
func (v Applications) Generic(application Application) (Shard, keyspace.Term, bool) {
	row, ok := v.application(application)
	if !ok || row.kind != applicationGeneric {
		return Shard{}, 0, false
	}
	return Shard{authority: v.authority, ordinal: row.shard}, row.term, true
}
func (v Applications) Import(application Application) (Shard, keyspace.Term, Application, bool) {
	row, ok := v.application(application)
	if !ok || row.kind != applicationImport {
		return Shard{}, 0, Application{}, false
	}
	if row.root == 0 || uint64(row.root) > uint64(len(v.authority.applications)) {
		return Shard{}, 0, Application{}, false
	}
	root := v.authority.applications[row.root-1]
	if root.kind != applicationCall || root.shard != row.shard {
		return Shard{}, 0, Application{}, false
	}
	call := Application{authority: v.authority, ordinal: row.root}
	return Shard{authority: v.authority, ordinal: row.shard}, row.term, call, true
}
func (v Applications) Compare(left, right Application) (int, bool) {
	leftRow, ok := v.application(left)
	if !ok {
		return 0, false
	}
	rightRow, ok := v.application(right)
	if !ok {
		return 0, false
	}
	return compareApplicationKey(applicationKey{kind: leftRow.kind, shard: leftRow.shard, term: leftRow.term, slot: leftRow.slot}, applicationKey{kind: rightRow.kind, shard: rightRow.shard, term: rightRow.term, slot: rightRow.slot}), true
}
func (v Applications) application(value Application) (applicationRow, bool) {
	if !v.live() || value.authority != v.authority || value.ordinal == 0 || uint64(value.ordinal) > uint64(len(v.authority.applications)) {
		return applicationRow{}, false
	}
	return v.authority.applications[value.ordinal-1], true
}

func (v Calls) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.authority.callApplications)
}
func (v Calls) At(index int) (Application, bool) {
	if !v.live() || index < 0 || index >= len(v.authority.callApplications) {
		return Application{}, false
	}
	return Application{authority: v.authority, ordinal: v.authority.callApplications[index]}, true
}

func (v Bases) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.authority.baseApplications)
}
func (v Bases) At(index int) (Application, bool) {
	if !v.live() || index < 0 || index >= len(v.authority.baseApplications) {
		return Application{}, false
	}
	return Application{authority: v.authority, ordinal: v.authority.baseApplications[index]}, true
}
func (v Imports) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.authority.importApplications)
}
func (v Imports) At(index int) (Application, bool) {
	if !v.live() || index < 0 || index >= len(v.authority.importApplications) {
		return Application{}, false
	}
	return Application{authority: v.authority, ordinal: v.authority.importApplications[index]}, true
}

func (v Operators) source(application Application, slot applicationSlot) (Shard, keyspace.Term, bool) {
	applications := Applications(v)
	row, ok := applications.application(application)
	if !ok || row.kind != applicationMeta || row.slot != slot {
		return Shard{}, 0, false
	}
	return Shard{authority: v.authority, ordinal: row.shard}, row.term, true
}

func (v Operators) UnaryNumeric(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotUnaryNumeric)
}

func (v Operators) Length(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotLength)
}

func (v Operators) Arithmetic(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotArithmetic)
}

func (v Operators) Bitwise(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotBitwise)
}

func (v Operators) Concat(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotConcat)
}

func (v Operators) Equality(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotEquality)
}

func (v Operators) OrderPrimary(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotOrderPrimary)
}

func (v Operators) OrderFallback(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotOrderFallback)
}

func (v Operators) IndexGet(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotIndexGet)
}

func (v Operators) IndexSet(application Application) (Shard, keyspace.Term, bool) {
	return v.source(application, applicationSlotIndexSet)
}

// TargetID is the scalar Target constituent from which the Project's direct
// key mappings were derived. It is safe to retain across finalization because
// Cold contains no hot authority.
func (v Cold) TargetID() identity.ContentID {
	if !v.live() {
		return identity.ContentID{}
	}
	return v.targetID
}

// ContentID is the Link-independent, versioned identity of the complete
// Project constituent. Root Link identity consumes this digest rather than
// reopening and re-encoding Project's authored inputs.
func (v Cold) ContentID() identity.ContentID {
	if !v.live() {
		return identity.ContentID{}
	}
	return v.contentID
}

// coldSnapshot copies only the scalar identities needed after Project's hot
// authority has been sealed. Keeping this constructor private prevents any
// caller from manufacturing a Cold that claims a different relation.
func (v Cold) live() bool {
	return v.targetID.Available() && (v.fence == nil || !v.fence.consumed)
}

func coldSnapshot(a *authority, fence *draftFence) Cold {
	if a == nil || a.target == nil {
		return Cold{}
	}
	return Cold{
		targetID:  a.target.ContentID(),
		contentID: a.contentID,
		counts:    a.counts,
		fence:     fence,
	}
}

package call

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	kind          keyKind
	applicationID identity.ContentID
	operation     vocabulary.Operation
	callback      vocabulary.CallbackID
	resume        vocabulary.ResumeID
	id            identity.ContentID
}

// Algebra is the single Link-scoped owner of the complete Call Factor family.
// Existing Project base Applications, Target callbacks, and Target resumes
// are the only Call keys. Every Value belongs only to this shared algebra.
// Global target selectors are factorized once and are never crossed with the
// closed source sum (there is no B×Operation/Port product).
type Algebra struct {
	contract                   *contract.Contract
	mountModules               []identity.ContentID
	mountModuleIndex           map[identity.ContentID]uint32
	mountedCalls               []mountedCallRow
	mountedCallIndex           map[identity.ContentID]uint32
	mountedCallOccurrenceIndex map[mountedCallOccurrenceRef]uint32
	requireOperation           vocabulary.Operation
	linkOwner                  link.OwnerCapability
	content                    identity.ContentID
	keys                       []keyRow
	keyIndex                   map[identity.ContentID]uint32
	targets                    []targetRow
	targetIndex                map[targetKey]selector
	roleIndex                  map[TargetRoleID]selector
	functionIndex              map[functionTargetKey]selector
	allocationIndex            map[allocationTargetKey]selector
	bodyTargetCount            int
	bottom                     Value
	top                        Value
}

// mountedCallRow is the seal-time projection of Project's ordinary-call
// relation. No Project/Shard/CallApplication authority is retained.
type mountedCallRow struct {
	applicationID identity.ContentID
	callID        identity.ContentID
	moduleID      identity.ContentID
	calleeValueID identity.ContentID
	loaderSeedID  identity.ContentID
}

type mountedCallOccurrenceRef struct {
	moduleID identity.ContentID
	callID   identity.ContentID
}

// mountedArtifactCallIndex is construction-only. Project owns the mounted
// application relation, while Program owns the reusable authored-call rows;
// call identities are resolved by scanning that canonical family and no
// secondary call directory survives into Algebra.
type mountedArtifactCallIndex struct {
	program programschema.Program
}

// NewWithMountedArtifacts builds Call from Link-owned boundary/key facts plus
// exact mounted artifacts. Call itself enumerates and validates every target
// row, so a caller cannot self-attest allocation/body correspondence.
func NewWithMountedArtifacts(source *link.Link, mounts []MountedArtifact) (*Algebra, bool) {
	if source == nil {
		return nil, false
	}
	linkOwner := source.OwnerCapability()
	if !linkOwner.Available() {
		return nil, false
	}
	boundary := source.Boundary()
	if boundary == nil {
		return nil, false
	}
	contract, contractOK := boundary.Target()
	if !contractOK || contract == nil {
		return nil, false
	}
	algebra := &Algebra{
		contract: contract, mountModuleIndex: make(map[identity.ContentID]uint32), mountedCallIndex: make(map[identity.ContentID]uint32), mountedCallOccurrenceIndex: make(map[mountedCallOccurrenceRef]uint32), linkOwner: linkOwner,
		keyIndex: make(map[identity.ContentID]uint32), targetIndex: make(map[targetKey]selector), roleIndex: make(map[TargetRoleID]selector),
		functionIndex: make(map[functionTargetKey]selector), allocationIndex: make(map[allocationTargetKey]selector),
	}
	algebra.requireOperation, _ = boundary.RequireOperation()
	// Content identity is independent of the target rows and is available
	// while those rows receive their stable semantic TargetRoleID proofs.
	algebra.content = algebraContentID(algebra.linkOwner)
	if !algebra.content.Available() {
		return nil, false
	}
	project := source.Project()
	if project == nil {
		return nil, false
	}
	mountsView := project.Mounts()
	if len(mounts) != mountsView.Count() {
		return nil, false
	}
	artifactCalls := make(map[identity.ContentID]mountedArtifactCallIndex, len(mounts))
	for index := 0; index < mountsView.Count(); index++ {
		shard, ok := mountsView.At(index)
		moduleID, moduleOK := project.ModuleKey(shard)
		programID, programOK := mountsView.ProgramID(shard)
		snapshot := mounts[index].Snapshot
		artifactProgramID := identity.ContentID{}
		if snapshot != nil && snapshot.Available() {
			artifactProgramID = snapshot.ProgramID()
		}
		if !ok || !moduleOK || !moduleID.Available() || mounts[index].ModuleKey != moduleID || algebra.mountModuleIndex[moduleID] != 0 ||
			!programOK || !programID.Available() || artifactProgramID != programID {
			return nil, false
		}
		program := snapshot.Program()
		callCount, callsPublished := program.CallCount()
		if !program.Available() || !callsPublished {
			return nil, false
		}
		for callIndex := 0; callIndex < callCount; callIndex++ {
			call, callOK := program.CallAt(callIndex)
			callID := call.ID()
			if !callOK || !callID.Available() {
				return nil, false
			}
			published, publishedOK := program.CallForID(callID)
			if !publishedOK || published.ID() != callID {
				return nil, false
			}
		}
		artifactCalls[moduleID] = mountedArtifactCallIndex{program: program}
		algebra.mountModules = append(algebra.mountModules, moduleID)
		algebra.mountModuleIndex[moduleID] = uint32(len(algebra.mountModules))
		require, requireOK := boundary.Seeds().ScopedLoader(shard)
		if !requireOK {
			return nil, false
		}
		seedID, seedOK := boundary.Seeds().ID(require)
		if !seedOK || !seedID.Available() {
			return nil, false
		}
	}
	if !algebra.buildTargets(mounts, boundary) || !algebra.buildKeys(project) {
		return nil, false
	}
	for i := 0; i < project.Applications().Calls().Count(); i++ {
		mounted, ok := project.Applications().Calls().MountedAt(i)
		if !ok {
			return nil, false
		}
		application, applicationOK := mounted.Application()
		applicationID, moduleID, callID, identityOK := project.Applications().Calls().MountedIdentity(application)
		issuedCallID := mounted.CallID()
		shard, shardOK := mounted.Mount()
		callProgram, callIndex, callIndexOK := mountedArtifactCallAt(artifactCalls, moduleID, callID)
		call := programschema.Call{}
		callRowOK := false
		if callIndexOK {
			call, callRowOK = callProgram.CallAt(callIndex)
		}
		calleeOperand, calleeOperandOK := mountedCalleeOperand(callProgram, callIndex, call)
		callee, calleeOK := boundary.Values().ForMountedSemantic(moduleID, calleeOperand.ValueID())
		if !calleeOK {
			callee, calleeOK = boundary.Values().ForMountedSpan(moduleID, calleeOperand.SpanID())
		}
		calleeValueID, calleeIDOK := boundary.Values().ID(callee)
		loader, loaderOK := boundary.Seeds().ScopedLoader(shard)
		loaderSeedID, loaderIDOK := boundary.Seeds().ID(loader)
		_, keyOK := algebra.KeyForApplicationID(applicationID)
		if !applicationOK || !identityOK || issuedCallID != callID || !shardOK || !callIndexOK || !callRowOK || !calleeOperandOK || !calleeOK || !calleeIDOK || !loaderOK || !loaderIDOK || !keyOK || !callID.Available() || !calleeValueID.Available() || !loaderSeedID.Available() {
			return nil, false
		}
		moduleIDFromShard, moduleIDOK := project.ModuleKey(shard)
		if !moduleIDOK || !moduleIDFromShard.Available() || moduleIDFromShard != moduleID || algebra.mountedCallIndex[applicationID] != 0 || algebra.mountedCallOccurrenceIndex[mountedCallOccurrenceRef{moduleID: moduleID, callID: callID}] != 0 {
			return nil, false
		}
		algebra.mountedCalls = append(algebra.mountedCalls, mountedCallRow{applicationID: applicationID, callID: callID, calleeValueID: calleeValueID, loaderSeedID: loaderSeedID, moduleID: moduleID})
		slot := uint32(len(algebra.mountedCalls))
		algebra.mountedCallIndex[applicationID] = slot
		algebra.mountedCallOccurrenceIndex[mountedCallOccurrenceRef{moduleID: moduleID, callID: callID}] = slot
	}
	algebra.bottom = Value{owner: algebra, known: true}
	algebra.top = Value{owner: algebra, top: true}
	return algebra, true
}

func mountedArtifactCallAt(index map[identity.ContentID]mountedArtifactCallIndex, moduleID, callID identity.ContentID) (programschema.Program, int, bool) {
	if index == nil || !moduleID.Available() || !callID.Available() {
		return programschema.Program{}, 0, false
	}
	mount, mountOK := index[moduleID]
	if !mountOK || !mount.program.Available() {
		return programschema.Program{}, 0, false
	}
	callCount, callsPublished := mount.program.CallCount()
	if !callsPublished {
		return programschema.Program{}, 0, false
	}
	callIndex := -1
	for index := 0; index < callCount; index++ {
		call, callOK := mount.program.CallAt(index)
		if !callOK || call.ID() != callID {
			continue
		}
		if callIndex >= 0 {
			return programschema.Program{}, 0, false
		}
		callIndex = index
	}
	return mount.program, callIndex, callIndex >= 0
}

func mountedCalleeOperand(program programschema.Program, callIndex int, call programschema.Call) (programschema.CallOperand, bool) {
	if !program.Available() || callIndex < 0 || !call.ID().Available() {
		return programschema.CallOperand{}, false
	}
	var callee programschema.CallOperand
	calleeOK := false
	for operandIndex := 0; operandIndex < call.OperandCount(); operandIndex++ {
		operand, operandOK := program.CallOperandFor(callIndex, operandIndex)
		if !operandOK || operand.CallID() != call.ID() {
			return programschema.CallOperand{}, false
		}
		if operand.Kind() != programschema.CallOperandCallee {
			continue
		}
		if calleeOK {
			return programschema.CallOperand{}, false
		}
		callee, calleeOK = operand, true
	}
	return callee, calleeOK && callee.ID() == call.CalleeID() && callee.ValueID() == call.CalleeID()
}

func (algebra *Algebra) MountModuleCount() int {
	if !algebra.Valid() {
		return 0
	}
	return len(algebra.mountModules)
}
func (algebra *Algebra) MountModuleAt(index int) (identity.ContentID, bool) {
	if !algebra.Valid() || index < 0 || index >= len(algebra.mountModules) {
		return identity.ContentID{}, false
	}
	return algebra.mountModules[index], true
}
func (algebra *Algebra) MountedCallCount() int {
	if !algebra.Valid() {
		return 0
	}
	return len(algebra.mountedCalls)
}

// RequireOperation returns the detached Target operation classified as the
// scoped loader during cold Link sealing.
func (algebra *Algebra) RequireOperation() (vocabulary.Operation, bool) {
	if !algebra.Valid() || algebra.requireOperation == 0 {
		return 0, false
	}
	return algebra.requireOperation, true
}

func (algebra *Algebra) buildKeys(project *linkproject.Component) bool {
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
		applicationID, applicationOK := project.ApplicationID(application)
		if !applicationOK {
			return false
		}
		row := keyRow{kind: keyApplication, applicationID: applicationID, id: applicationID}
		if !algebra.appendKey(row) {
			return false
		}
	}
	contract := algebra.contract
	if contract == nil {
		return false
	}
	// Target owns callback/resume correspondence.  Iterate the sealed
	// operation-local ranges exactly once; no Subedge or Application key is
	// synthesized, and no cross product is retained.
	for operationIndex := 0; operationIndex < contract.Operations.OperationCount(); operationIndex++ {
		operation, present := contract.Operations.OperationAt(operationIndex)
		if !present {
			return false
		}
		for callbackIndex := 0; callbackIndex < contract.Operations.CallbackCount(operation); callbackIndex++ {
			callback, present := contract.Operations.CallbackAt(operation, callbackIndex)
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
		for resumeIndex := 0; resumeIndex < contract.Operations.ResumeCount(operation); resumeIndex++ {
			resume, present := contract.Operations.ResumeIDAt(operation, resumeIndex)
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
		if !row.applicationID.Available() {
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
	if row.kind == keyApplication && row.id == (identity.ContentID{}) {
		row.id = row.applicationID
	}
	if !row.id.Available() || algebra.keyIndex[row.id] != 0 {
		return false
	}
	algebra.keys = append(algebra.keys, row)
	algebra.keyIndex[row.id] = uint32(len(algebra.keys))
	return true
}

func (algebra *Algebra) Valid() bool {
	return algebra != nil && algebra.contract != nil && algebra.linkOwner.Available() && algebra.content.Available() &&
		algebra.bodyTargetCount >= 0 && algebra.bodyTargetCount <= len(algebra.targets)
}
func (algebra *Algebra) ContentID() identity.ContentID {
	if !algebra.Valid() {
		return identity.ContentID{}
	}
	return algebra.content
}
func (algebra *Algebra) LinkID() identity.ContentID {
	if !algebra.Valid() {
		return identity.ContentID{}
	}
	return algebra.linkOwner.ContentID()
}

// LinkOwner returns Call's exact detached Link owner witness.
func (algebra *Algebra) LinkOwner() link.OwnerCapability {
	if !algebra.Valid() {
		return link.OwnerCapability{}
	}
	return algebra.linkOwner
}

// Owner is the concise alias used by cross-domain binders.
func (algebra *Algebra) Owner() link.OwnerCapability { return algebra.LinkOwner() }

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

// KeyIndex returns the canonical dense coordinate for one exact Algebra-owned
// key. It is the sole inverse projection of KeyAt; callers cannot use it with
// a key issued by an equivalent or foreign Algebra.
func (algebra *Algebra) KeyIndex(key Key) (int, bool) {
	if !algebra.Valid() || !algebra.validKey(key) {
		return 0, false
	}
	index := int(key.slot - 1)
	return index, index >= 0 && index < len(algebra.keys)
}

// OwnsKey authenticates a key against this exact Algebra owner.
func (algebra *Algebra) OwnsKey(key Key) bool {
	return algebra != nil && key.owner == algebra && algebra.validKey(key)
}

// KeyForApplicationID projects the compact application receipt produced at
// seal time.  It deliberately has no Project reach-back.
func (algebra *Algebra) KeyForApplicationID(id identity.ContentID) (Key, bool) {
	if !algebra.Valid() || !id.Available() {
		return Key{}, false
	}
	// keyForID rechecks the closed source-sum discriminator; callback and
	// resume content IDs therefore cannot be reinterpreted as applications.
	return algebra.keyForID(id, keyApplication)
}

// KeyForCallback looks up one exact Target callback correspondence. The
// issuing Contract pointer is an authority fence: equal numeric handles from
// an equivalent Contract cannot be spliced into this Link.
func (algebra *Algebra) KeyForCallback(issuing *contract.Contract, operation vocabulary.Operation, callback vocabulary.CallbackID) (Key, bool) {
	if !algebra.Valid() {
		return Key{}, false
	}
	contract := algebra.contract
	if contract == nil || issuing == nil || issuing != contract {
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
func (algebra *Algebra) KeyForResume(issuing *contract.Contract, operation vocabulary.Operation, resume vocabulary.ResumeID) (Key, bool) {
	if !algebra.Valid() {
		return Key{}, false
	}
	contract := algebra.contract
	if contract == nil || issuing == nil || issuing != contract {
		return Key{}, false
	}
	id, ok := contract.ResumeContentID(operation, resume)
	if !ok || !id.Available() {
		return Key{}, false
	}
	return algebra.keyForID(id, keyResume)
}

func (algebra *Algebra) keyForID(id identity.ContentID, kind keyKind) (Key, bool) {
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
func (algebra *Algebra) FindKey(id identity.ContentID) (Key, bool) {
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

// TargetForFunction projects one sealed function receipt into an owner-bound
// Call capability. The dense target identity never crosses this API.
func (algebra *Algebra) TargetForFunction(moduleKey, functionContext identity.ContentID) (Target, bool) {
	if algebra == nil || !algebra.Valid() || !moduleKey.Available() || !functionContext.Available() {
		return Target{}, false
	}
	selector := algebra.functionIndex[functionTargetKey{moduleKey: moduleKey, functionContext: functionContext}]
	return algebra.targetForSelector(selector)
}

// TargetForAllocation projects one exact compact closure-allocation receipt
// into this Algebra. The mount and allocation identities are already
// owner-fenced before the precomputed target lookup is used.
func (algebra *Algebra) TargetForAllocation(moduleKey, allocationID identity.ContentID) (Target, bool) {
	if algebra == nil || !algebra.Valid() || !moduleKey.Available() || !allocationID.Available() {
		return Target{}, false
	}
	return algebra.targetForSelector(algebra.allocationIndex[allocationTargetKey{moduleKey: moduleKey, allocationID: allocationID}])
}

// OwnsTarget authenticates a capability against this exact Algebra owner.
func (algebra *Algebra) OwnsTarget(target Target) bool {
	return algebra != nil && target.owner == algebra && target.Valid()
}

// TargetForSeed projects one sealed Boundary seed into an owner-bound Call
// capability. Foreign equivalent Boundary seeds fail closed.
// TargetForSeedID is the detached hot lookup. The ID must have been issued
// by the exact Boundary during cold construction.
func (algebra *Algebra) TargetForSeedID(seedID identity.ContentID) (Target, bool) {
	if !algebra.Valid() || !seedID.Available() {
		return Target{}, false
	}
	return algebra.targetForSelector(algebra.targetIndex[targetKey{kind: targetSeed, seedID: seedID}])
}

// Equivalent performs cold exact replay validation after the content prefilter.
func (algebra *Algebra) Equivalent(other *Algebra) bool {
	if !algebra.Valid() || !other.Valid() || algebra.content != other.content || len(algebra.mountModules) != len(other.mountModules) || len(algebra.mountedCalls) != len(other.mountedCalls) || len(algebra.keys) != len(other.keys) || len(algebra.targets) != len(other.targets) || algebra.bodyTargetCount != other.bodyTargetCount {
		return false
	}
	for index := range algebra.mountModules {
		if algebra.mountModules[index] != other.mountModules[index] {
			return false
		}
	}
	for index := range algebra.mountedCalls {
		left, right := algebra.mountedCalls[index], other.mountedCalls[index]
		if left.applicationID != right.applicationID || left.callID != right.callID || left.moduleID != right.moduleID || left.calleeValueID != right.calleeValueID || left.loaderSeedID != right.loaderSeedID {
			return false
		}
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
	if left == nil || right == nil || leftRow.key.kind != rightRow.key.kind || leftRow.role != rightRow.role || leftRow.functionContext != rightRow.functionContext {
		return false
	}
	switch leftRow.key.kind {
	case targetBody:
		if leftRow.bodyContext != rightRow.bodyContext {
			return false
		}
		return leftRow.key.moduleKey.Available() && rightRow.key.moduleKey.Available() && leftRow.key.moduleKey == rightRow.key.moduleKey
	case targetSeed:
		return leftRow.key.seedID == rightRow.key.seedID && leftRow.seedFormalID == rightRow.seedFormalID
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

func algebraContentID(owner link.OwnerCapability) (id identity.ContentID) {
	if !owner.Available() {
		return identity.ContentID{}
	}
	linkID := owner.ContentID()
	var payload [32 + 24]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], 0x63616c6c2d616c67) // "call-alg"
	binary.BigEndian.PutUint64(payload[40:48], 5)
	binary.BigEndian.PutUint64(payload[48:56], 3) // Base Application + Callback + Resume closed sum
	return sha256.Sum256(payload[:])
}

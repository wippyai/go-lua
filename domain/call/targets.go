package call

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

type targetKind uint8

const (
	targetBody targetKind = iota + 1
	targetSeed
)

// allocationTargetKey is the sealed Program allocation proof lookup used by
// dispatch. The allocation ID is owner-neutral; TargetForAllocation applies
// the exact mounted Program owner fence before projecting it to Call.
type allocationTargetKey struct {
	moduleKey    identity.ContentID
	allocationID identity.ContentID
}

// MountedArtifact is the exact reusable artifact mounted at one Link shard.
// Call internally derives all closure target rows from it; no raw
// allocation/body scalar bundle enters the domain boundary.
type MountedArtifact struct {
	programmount.Program
	Snapshot *ingress.Snapshot
}

type targetRow struct {
	kind          targetKind
	mount         uint32
	role          TargetRoleID
	bodyPath      identity.ContentID
	seedID        identity.ContentID
	seedOperation vocabulary.Operation
	scopedLoader  bool
}

func (algebra *Algebra) buildTargets(mounts []MountedArtifact, boundary *linkboundary.Component) bool {
	if algebra == nil || boundary == nil {
		return false
	}
	type bodySource struct {
		moduleKey   identity.ContentID
		bodyContext identity.ContentID
	}
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	seenBodies := make(map[bodySource]struct{})
	for _, mount := range mounts {
		if mount.Snapshot == nil || !mount.Snapshot.Available() || !mount.Program.Available() {
			return false
		}
		programID := mount.ProgramID
		if programID != mount.Snapshot.ProgramID() {
			return false
		}
		if _, duplicate := seenMounts[mount.ModuleKey]; duplicate {
			return false
		}
		seenMounts[mount.ModuleKey] = struct{}{}
		mountSlot := algebra.mountModuleIndex[mount.ModuleKey]
		mountRow, mountOK := algebra.mountRow(mountSlot)
		if !mountOK || mountRow.programID != programID {
			return false
		}
		bodyCount, bodiesPublished := mount.Program.BodyCount()
		if !bodiesPublished {
			return false
		}
		bodies := make(map[identity.ContentID]programschema.Body, bodyCount)
		for index := 0; index < bodyCount; index++ {
			body, ok := mount.Program.BodyAt(index)
			if !ok || !body.ID().Available() || !body.ContextID().Available() {
				return false
			}
			if _, duplicate := bodies[body.ID()]; duplicate {
				return false
			}
			bodies[body.ID()] = body
		}
		state, stateOK := mount.Program.ColdState()
		targets, targetsOK := calltarget.NewView(state)
		targetCount, targetsPublished := targets.Count()
		if !stateOK || !targetsOK || !targetsPublished {
			return false
		}
		for index := 0; index < targetCount; index++ {
			target, ok := targets.At(index)
			body, bodyOK := bodies[target.BodyID()]
			function, functionOK := body.FunctionContextID()
			formal, formalOK := body.CallFormalID()
			if !ok || !target.Available() || !bodyOK || !body.Callable() || !functionOK || !formalOK || target.ContextID() != body.ContextID() || target.FunctionID() != function || target.FormalID() != formal {
				return false
			}
			ref := bodySource{moduleKey: mount.ModuleKey, bodyContext: target.ContextID()}
			_, duplicateBody := seenBodies[ref]
			role, roleOK := bodyTargetRoleID(mount.ModuleKey, target.FormalID())
			if !programID.Available() || !target.BodyID().Available() || !target.FormalID().Available() || !target.ContextID().Available() ||
				!roleOK || duplicateBody || !algebra.appendTarget(targetRow{
				kind:     targetBody,
				mount:    mountSlot,
				role:     role,
				bodyPath: target.BodyID(),
			}) {
				return false
			}
			seenBodies[ref] = struct{}{}
			if algebra.allocationIndex[allocationTargetKey{moduleKey: mount.ModuleKey, allocationID: target.AllocationID()}].valid() {
				return false
			}
			algebra.allocationIndex[allocationTargetKey{moduleKey: mount.ModuleKey, allocationID: target.AllocationID()}] = selector(len(algebra.targets))
		}
	}
	// Executable function bodies are the sole body-target prefix.  Retaining
	// its width lets the public Bodies cursor reuse Call's canonical target
	// order without a second list, map, or per-Application projection.
	algebra.bodyTargetCount = len(algebra.targets)
	seeds := boundary.Seeds()
	for seedIndex := 0; seedIndex < seeds.Count(); seedIndex++ {
		seed, ok := seeds.At(seedIndex)
		if !ok {
			return false
		}
		// Boundary is the sole external-value classifier.  Operation covers
		// both direct Target operations and nominal provider endpoints; Loader
		// covers the scoped require ingress.  Denied bootstrap values satisfy
		// neither query and cannot enter Call's target universe.
		_, operation := seeds.Operation(seed)
		_, loader := seeds.Loader(seed)
		if !operation && !loader {
			continue
		}
		seedID, seedIDOK := seeds.ID(seed)
		formal, formalOK := seeds.CallTarget(seed)
		formalID, formalIDOK := formal.ID()
		operationValue, _ := seeds.Operation(seed)
		// A scoped loader is the Target-authored require operation selected for
		// this mounted shard. Retain that exact operation on the existing seed
		// target so downstream factors can distinguish require from every other
		// callable without reopening Boundary or forming a Call×operation table.
		if loader && operationValue == 0 {
			operationValue = algebra.requireOperation
		}
		role, roleOK := seedTargetRoleID(seedID, formalID, uint8(formal.Kind()))
		if !seedIDOK || !formalOK || !formalIDOK || !seedID.Available() || !roleOK || !algebra.appendTarget(targetRow{kind: targetSeed, seedID: seedID, role: role, seedOperation: operationValue, scopedLoader: loader}) {
			return false
		}
	}
	return true
}

func (algebra *Algebra) appendTarget(row targetRow) bool {
	if algebra == nil || len(algebra.targets) == int(^selector(0)) {
		return false
	}
	if !row.role.Valid() || algebra.roleIndex[row.role].valid() {
		return false
	}
	switch row.kind {
	case targetBody:
		if _, mountOK := algebra.mountRow(row.mount); !mountOK || !row.bodyPath.Available() || row.seedID.Available() || row.seedOperation != 0 || row.scopedLoader || row.role.Kind() != TargetRoleBody || len(algebra.seedIndex) != 0 {
			return false
		}
	case targetSeed:
		if row.mount != 0 || row.bodyPath.Available() || !row.seedID.Available() || row.role.Kind() != TargetRoleSeed || algebra.seedIndex[row.seedID].valid() {
			return false
		}
	default:
		return false
	}
	selector := selector(len(algebra.targets) + 1)
	algebra.targets = append(algebra.targets, row)
	if row.kind == targetSeed {
		algebra.seedIndex[row.seedID] = selector
	}
	algebra.roleIndex[row.role] = selector
	return true
}

// Role identities are issued before their raw construction inputs are
// discarded. They exclude Call's selector ordinal and every live owner.
func bodyTargetRoleID(moduleID, formalID identity.ContentID) (TargetRoleID, bool) {
	if !moduleID.Available() || !formalID.Available() {
		return TargetRoleID{}, false
	}
	var payload [80]byte
	copy(payload[:8], []byte("callbody"))
	binary.BigEndian.PutUint64(payload[8:16], 1)
	copy(payload[16:48], moduleID[:])
	copy(payload[48:80], formalID[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return newTargetRoleID(TargetRoleBody, id)
}

func seedTargetRoleID(seedID, formalID identity.ContentID, seedKind uint8) (TargetRoleID, bool) {
	if !seedID.Available() || !formalID.Available() || seedKind == 0 {
		return TargetRoleID{}, false
	}
	var payload [8 + 8 + 1 + 32]byte
	copy(payload[:8], []byte("callseed"))
	binary.BigEndian.PutUint64(payload[8:16], 1)
	payload[16] = seedKind
	copy(payload[17:], formalID[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return newTargetRoleID(TargetRoleSeed, id)
}

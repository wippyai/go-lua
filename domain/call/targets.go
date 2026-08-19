package call

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
)

type targetKind uint8

const (
	targetBody targetKind = iota + 1
	targetSeed
)

type targetKey struct {
	kind        targetKind
	moduleKey   identity.ContentID
	bodyContext identity.ContentID
	seedID      identity.ContentID
}

type functionTargetKey struct {
	moduleKey       identity.ContentID
	functionContext identity.ContentID
}

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
	ModuleKey identity.ContentID
	Snapshot  *ingress.Snapshot
}

type bodyTargetReceipt struct {
	moduleKey       identity.ContentID
	artifactID      identity.ContentID
	programID       identity.ContentID
	allocationID    identity.ContentID
	bodyPath        identity.ContentID
	bodyContext     identity.ContentID
	functionContext identity.ContentID
	formalID        identity.ContentID
}

func (receipt bodyTargetReceipt) valid() bool {
	return receipt.moduleKey.Available() && receipt.artifactID.Available() &&
		receipt.programID.Available() && receipt.allocationID.Available() && receipt.bodyPath.Available() &&
		receipt.bodyContext.Available() && receipt.functionContext.Available() && receipt.formalID.Available()
}

// bodyReceipt is the compact seal-time projection of a Program body. It is
// deliberately data-only: no Program, Body, Flow, or Link
// proof survives in Call's target rows.
type bodyReceipt struct {
	artifactID  identity.ContentID
	programID   identity.ContentID
	bodyPath    identity.ContentID
	formalID    identity.ContentID
	bodyContext identity.ContentID
}

func (receipt bodyReceipt) valid() bool {
	return receipt.programID.Available() && receipt.bodyPath.Available() &&
		receipt.formalID.Available() && receipt.bodyContext.Available()
}

type targetRow struct {
	key             targetKey
	role            TargetRoleID
	functionContext identity.ContentID
	bodyContext     identity.ContentID
	body            bodyReceipt
	seedOperation   vocabulary.Operation
	seedFormalID    identity.ContentID
	seedKind        uint8
}

func (algebra *Algebra) buildTargets(mounts []MountedArtifact, boundary *linkboundary.Component) bool {
	if algebra == nil || boundary == nil {
		return false
	}
	receipts := make([]bodyTargetReceipt, 0)
	seenMounts := make(map[identity.ContentID]struct{}, len(mounts))
	for _, mount := range mounts {
		if mount.Snapshot == nil || !mount.Snapshot.Available() || !mount.ModuleKey.Available() {
			return false
		}
		programID := mount.Snapshot.ProgramID()
		if !programID.Available() {
			return false
		}
		if _, duplicate := seenMounts[mount.ModuleKey]; duplicate {
			return false
		}
		seenMounts[mount.ModuleKey] = struct{}{}
		bodies := make(map[identity.ContentID]ingress.BodyTransport, mount.Snapshot.BodyTransportCount())
		for index := 0; index < mount.Snapshot.BodyTransportCount(); index++ {
			body, ok := mount.Snapshot.BodyTransportAt(index)
			if !ok || !body.BodyID().Available() || !body.ContextID().Available() {
				return false
			}
			if _, duplicate := bodies[body.BodyID()]; duplicate {
				return false
			}
			bodies[body.BodyID()] = body
		}
		for index := 0; index < mount.Snapshot.CallTargetCount(); index++ {
			target, ok := mount.Snapshot.CallTargetAt(index)
			body, bodyOK := bodies[target.BodyID()]
			function, formal := body.FunctionID(), body.CallFormalID()
			if !ok || !target.Available() || !bodyOK || !body.Callable() || !function.Available() || !formal.Available() || target.ContextID() != body.ContextID() || target.FunctionContextID() != function || target.CallFormalID() != formal {
				return false
			}
			receipts = append(receipts, bodyTargetReceipt{moduleKey: mount.ModuleKey, artifactID: mount.Snapshot.ArtifactID(), programID: programID, allocationID: target.AllocationID(), bodyPath: target.BodyID(), bodyContext: target.ContextID(), functionContext: target.FunctionContextID(), formalID: target.CallFormalID()})
		}
	}
	for _, issued := range receipts {
		if !issued.valid() {
			return false
		}
		receipt := bodyReceipt{artifactID: issued.artifactID, programID: issued.programID, bodyPath: issued.bodyPath, formalID: issued.formalID, bodyContext: issued.bodyContext}
		key := targetKey{kind: targetBody, moduleKey: issued.moduleKey, bodyContext: issued.bodyContext}
		if !receipt.valid() || algebra.targetIndex[key].valid() || !algebra.appendTarget(targetRow{key: key, functionContext: issued.functionContext, bodyContext: issued.bodyContext, body: receipt}) {
			return false
		}
		if algebra.allocationIndex[allocationTargetKey{moduleKey: issued.moduleKey, allocationID: issued.allocationID}].valid() {
			return false
		}
		algebra.allocationIndex[allocationTargetKey{moduleKey: issued.moduleKey, allocationID: issued.allocationID}] = selector(len(algebra.targets))
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
		if !seedIDOK || !formalOK || !formalIDOK || !seedID.Available() || !algebra.appendTarget(targetRow{key: targetKey{kind: targetSeed, seedID: seedID}, seedOperation: operationValue, seedFormalID: formalID, seedKind: uint8(formal.Kind())}) {
			return false
		}
	}
	return true
}

func (algebra *Algebra) appendTarget(row targetRow) bool {
	if algebra == nil || len(algebra.targets) == int(^selector(0)) || algebra.targetIndex[row.key].valid() {
		return false
	}
	role, roleOK := algebra.roleIDForTarget(row)
	if !roleOK || algebra.roleIndex[role].valid() {
		return false
	}
	functionKey := functionTargetKey{}
	if row.key.kind == targetBody {
		functionKey = functionTargetKey{moduleKey: row.key.moduleKey, functionContext: row.functionContext}
		if !row.functionContext.Available() || !row.bodyContext.Available() || !row.body.valid() || row.body.bodyContext != row.bodyContext || algebra.functionIndex[functionKey].valid() {
			return false
		}
	}
	selector := selector(len(algebra.targets) + 1)
	row.role = role
	algebra.targets = append(algebra.targets, row)
	algebra.targetIndex[row.key] = selector
	algebra.roleIndex[role] = selector
	if row.key.kind == targetBody {
		algebra.functionIndex[functionKey] = selector
	}
	return true
}

// roleIDForTarget derives the stable semantic role identity for one sealed
// target row. It deliberately excludes Call's selector ordinal and every live
// owner pointer. Body roles are framed from Project's stable ModuleKey and
// Program's formal Body target; seeds use Boundary's explicit formal target.
func (algebra *Algebra) roleIDForTarget(row targetRow) (TargetRoleID, bool) {
	if algebra == nil {
		return TargetRoleID{}, false
	}
	switch row.key.kind {
	case targetBody:
		if !row.key.moduleKey.Available() || !row.bodyContext.Available() {
			return TargetRoleID{}, false
		}
		moduleID := row.key.moduleKey
		if !moduleID.Available() || !row.bodyContext.Available() {
			return TargetRoleID{}, false
		}
		formalID := row.body.formalID
		if !formalID.Available() {
			return TargetRoleID{}, false
		}
		// Fixed frame: domain tag (8), version (8), ModuleKey (32),
		// Program-owned formal Body target ID (32). No raw term, mount name,
		// selector, or owner pointer participates in the mounted identity.
		var payload [80]byte
		copy(payload[:8], []byte("callbody"))
		binary.BigEndian.PutUint64(payload[8:16], 1)
		copy(payload[16:48], moduleID[:])
		copy(payload[48:80], formalID[:])
		id := identity.ContentID(sha256.Sum256(payload[:]))
		return newTargetRoleID(TargetRoleBody, id)
	case targetSeed:
		if !row.key.seedID.Available() || !row.seedFormalID.Available() {
			return TargetRoleID{}, false
		}
		formalID := row.seedFormalID
		// Boundary formal kind is framed into the mounted role so operation
		// and external seed categories cannot alias through equal payloads.
		var payload [8 + 8 + 1 + 32]byte
		copy(payload[:8], []byte("callseed"))
		binary.BigEndian.PutUint64(payload[8:16], 1)
		payload[16] = row.seedKind
		copy(payload[17:], formalID[:])
		id := identity.ContentID(sha256.Sum256(payload[:]))
		return newTargetRoleID(TargetRoleSeed, id)
	default:
		return TargetRoleID{}, false
	}
}

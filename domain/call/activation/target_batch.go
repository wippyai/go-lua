package activation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

// TargetBatchRow is one mounted Program body selector row. BodyPath is the
// parent-issued artifact body identity; Target and Endpoint are the sealed
// equation semantic locators admitted for this mounted row.
type TargetBatchRow struct {
	Body     calldomain.Body
	BodyPath identity.ContentID
	Role     calldomain.TargetRoleID
}

// MountedTargetBatch is one exact Link mount projection. Rows are in the
// mounted artifact's Body denominator order. Shard and ModuleKey are retained
// as the immutable substitution fence; this package never resolves them back
// through Link or Project state.
type MountedTargetBatch struct {
	Artifact  *programartifact.Artifact
	ModuleKey identity.ContentID
	Rows      []TargetBatchRow
}

// Route is the read-only hot projection of one target-batch row. It does not
// expose the artifact's backing storage or any target-batch ordinal.
type Route struct {
	Body     calldomain.Body
	Target   identity.SemanticKey
	Endpoint identity.SemanticKey
}

type targetBatchRow struct {
	body       calldomain.Body
	bodyID     identity.ContentID
	bodyPath   identity.ContentID
	role       calldomain.TargetRoleID
	target     identity.SemanticKey
	endpoint   identity.SemanticKey
	artifactID identity.ContentID
	programID  identity.ContentID
	moduleKey  identity.ContentID
}

// TargetBatchCatalog is the immutable Link-wide activation selector receipt
// for ordered mounted Program artifacts. It owns no legacy engine assembly,
// Link, or solve-local plan. Mount artifacts and target batches are validated
// together once and then traversed by the hot callback without reopening
// Program/Flow. The sealed scalar keeps hot validation O(1).
type TargetBatchCatalog struct {
	rows       []targetBatchRow
	bodyRoutes map[bodyRouteKey]uint32
	sealed     identity.ContentID
	self       *TargetBatchCatalog
}

type bodyRouteKey struct {
	moduleKey identity.ContentID
	id        identity.ContentID
}

// NewTargetBatchCatalog seals the one Link-wide selector catalog from ordered
// mounted Program rows. Rows cover only executable Call body targets; the
// artifact may contain non-callable Body rows which remain structural-only.
func NewTargetBatchCatalog(mounts []MountedTargetBatch) (*TargetBatchCatalog, bool) {
	if len(mounts) == 0 {
		return nil, false
	}
	count := 0
	for _, mount := range mounts {
		if mount.Artifact == nil || !mount.Artifact.Available() || !mount.ModuleKey.Available() {
			return nil, false
		}
		count += len(mount.Rows)
	}
	result := &TargetBatchCatalog{rows: make([]targetBatchRow, 0, count), bodyRoutes: make(map[bodyRouteKey]uint32, count)}
	for _, mount := range mounts {
		artifact := mount.Artifact
		artifactID := artifact.ID()
		programID := artifact.CompileKey().ProgramID()
		if !artifactID.Available() || !programID.Available() {
			return nil, false
		}
		seen := make(map[identity.ContentID]struct{}, len(mount.Rows))
		for _, candidate := range mount.Rows {
			bodyID, bodyIDOK := candidate.Body.ContentID()
			bodyArtifactID, bodyArtifactOK := candidate.Body.ArtifactID()
			moduleKey, moduleKeyOK := candidate.Body.ModuleKey()
			role, roleOK := candidate.Body.RoleID()
			target, endpoint, semanticOK := callActivationRoleSemantics(candidate.Role)
			bodyProgramID, bodyProgramOK := candidate.Body.ProgramID()
			bodyPath, bodyPathOK := candidate.Body.BodyPath()
			if !candidate.Body.Valid() || !bodyArtifactOK || bodyArtifactID != artifactID ||
				!bodyProgramOK || bodyProgramID != programID ||
				!candidate.Role.Valid() || !moduleKeyOK || moduleKey != mount.ModuleKey || !roleOK || role != candidate.Role ||
				!bodyPathOK || bodyPath != candidate.BodyPath || !candidate.BodyPath.Available() ||
				!bodyIDOK || !semanticOK {
				return nil, false
			}
			if _, duplicate := seen[candidate.BodyPath]; duplicate {
				return nil, false
			}
			seen[candidate.BodyPath] = struct{}{}
			bodyKey := bodyRouteKey{moduleKey: mount.ModuleKey, id: bodyID}
			if _, duplicate := result.bodyRoutes[bodyKey]; duplicate {
				return nil, false
			}
			result.rows = append(result.rows, targetBatchRow{
				body: candidate.Body, bodyID: bodyID, bodyPath: candidate.BodyPath, role: candidate.Role,
				target: target, endpoint: endpoint,
				artifactID: artifactID, programID: programID,
				moduleKey: mount.ModuleKey,
			})
			result.bodyRoutes[bodyKey] = uint32(len(result.rows))
		}
	}
	if len(result.rows) != 0 {
		result.sealed = result.rows[0].artifactID
	} else {
		result.sealed = mounts[0].Artifact.ID()
	}
	result.self = result
	return result, result.valid()
}

// SealMountedBatches derives the Link-wide selector catalog from a sealed Call
// algebra and the neutral mounted artifact view. Every row is resolved through
// the algebra's own target inverse, so the catalog states nothing the algebra
// did not already admit, and the mounted body denominator must match the
// algebra's exactly.
//
// It lives with the catalog it produces: the derivation reads one factor's
// sealed authority plus the artifacts it was sealed from, so no composition
// root needs to enumerate Call target rows on this package's behalf.
func SealMountedBatches(algebra *calldomain.Algebra, mounts []axis.MountedArtifact) (*TargetBatchCatalog, bool) {
	if len(mounts) == 0 || algebra == nil || !algebra.Valid() {
		return nil, false
	}
	batches := make([]MountedTargetBatch, 0, len(mounts))
	rowCount := 0
	for _, mount := range mounts {
		if !mount.Available() {
			return nil, false
		}
		rows, built := mountedTargetBatchRows(mount, algebra)
		if !built {
			return nil, false
		}
		rowCount += len(rows)
		batches = append(batches, MountedTargetBatch{Artifact: mount.Artifact, ModuleKey: mount.ModuleKey, Rows: rows})
	}
	if rowCount != algebra.Bodies().Count() {
		return nil, false
	}
	return NewTargetBatchCatalog(batches)
}

// mountedTargetBatchRows projects one mount's artifact call targets onto the
// algebra's sealed body capabilities. The artifact supplies the target rows and
// the algebra supplies every selector; a row whose body path or program does
// not match the artifact it was read from is rejected.
func mountedTargetBatchRows(mount axis.MountedArtifact, algebra *calldomain.Algebra) ([]TargetBatchRow, bool) {
	artifact := mount.Artifact
	if artifact == nil || algebra == nil {
		return nil, false
	}
	rows := make([]TargetBatchRow, 0, artifact.CallTargetCount())
	for index := 0; index < artifact.CallTargetCount(); index++ {
		target, targetOK := artifact.CallTargetAt(index)
		capability, capabilityOK := algebra.TargetForAllocation(mount.ModuleKey, target.AllocationID())
		body, bodyCapabilityOK := capability.Body()
		role, roleOK := body.RoleID()
		bodyPath, pathOK := body.BodyPath()
		programID, programOK := body.ProgramID()
		if !targetOK || !target.Available() || !capabilityOK || !bodyCapabilityOK || !roleOK || !pathOK || !programOK || bodyPath != target.BodyID() || programID != mount.ProgramID {
			return nil, false
		}
		rows = append(rows, TargetBatchRow{Body: body, BodyPath: target.BodyID(), Role: role})
	}
	return rows, true
}

const (
	callActivationTargetFrame   = uint64(1)
	callActivationEndpointFrame = uint64(2)
)

// callActivationRoleSemantics frames the exact Call-owned Body role twice;
// target and endpoint are distinct semantic axes but neither is a new hash
// authority or a caller-supplied scalar.
func callActivationRoleSemantics(role calldomain.TargetRoleID) (identity.SemanticKey, identity.SemanticKey, bool) {
	id, ok := role.ContentID()
	if !ok {
		return identity.SemanticKey{}, identity.SemanticKey{}, false
	}
	target, targetOK := identity.NewSemanticKey([32]byte(id), callActivationTargetFrame)
	endpoint, endpointOK := identity.NewSemanticKey([32]byte(id), callActivationEndpointFrame)
	return target, endpoint, targetOK && endpointOK && target != endpoint
}

func (catalog *TargetBatchCatalog) valid() bool {
	return catalog != nil && catalog.self == catalog && catalog.bodyRoutes != nil && catalog.sealed.Available()
}

func (catalog *TargetBatchCatalog) routeAt(index int) (route, bool) {
	if catalog == nil || !catalog.valid() || index < 0 || index >= len(catalog.rows) {
		return route{}, false
	}
	row := catalog.rows[index]
	derivedTarget, derivedEndpoint, semanticOK := callActivationRoleSemantics(row.role)
	if !row.body.Valid() || !row.bodyID.Available() || !row.bodyPath.Available() || !row.role.Valid() || !semanticOK || row.target != derivedTarget || row.endpoint != derivedEndpoint {
		return route{}, false
	}
	return route{body: row.body, target: row.target, endpoint: row.endpoint}, true
}

func (catalog *TargetBatchCatalog) routeForBody(moduleKey identity.ContentID, id identity.ContentID) (route, bool) {
	if catalog == nil || !catalog.valid() || !moduleKey.Available() || !id.Available() || catalog.bodyRoutes == nil {
		return route{}, false
	}
	row, ok := catalog.bodyRoutes[bodyRouteKey{moduleKey: moduleKey, id: id}]
	if !ok || row == 0 {
		return route{}, false
	}
	return catalog.routeAt(int(row - 1))
}

func (catalog *TargetBatchCatalog) routeCount() int {
	if catalog == nil || !catalog.valid() {
		return 0
	}
	return len(catalog.rows)
}

// RouteAt returns one exact selector row without exposing the backing batch.
func (catalog *TargetBatchCatalog) RouteAt(index int) (Route, bool) {
	item, ok := catalog.routeAt(index)
	if !ok {
		return Route{}, false
	}
	return Route{Body: item.body, Target: item.target, Endpoint: item.endpoint}, true
}

// RouteForBody returns the exact activation selector for one mounted Call
// body semantic ID. Shard remains part of the inverse because equal Program
// artifacts may be mounted more than once with different substitutions.
func (catalog *TargetBatchCatalog) RouteForBody(moduleKey identity.ContentID, id identity.ContentID) (Route, bool) {
	item, ok := catalog.routeForBody(moduleKey, id)
	if !ok {
		return Route{}, false
	}
	return Route{Body: item.body, Target: item.target, Endpoint: item.endpoint}, true
}

// route is the private callback traversal row. Its body is still fenced by
// the exact Call owner when BindHot validates the catalog.
type route struct {
	body     calldomain.Body
	target   identity.SemanticKey
	endpoint identity.SemanticKey
}

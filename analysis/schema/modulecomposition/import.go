package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// ResolvedImport is the canonical resolution of one compiled ModuleRequest
// in one mounted Program to a target module key. Construction accepts the
// canonical mount and request to validate this join, but the sealed row keeps
// only stable scalar identities; it never retains the mounted Frozen Program.
type ResolvedImport struct {
	id, link, sourceModuleKey, targetModuleKey identity.ContentID
	artifactID, programID                      identity.ContentID
	importID, requestID, valueID               identity.ContentID
	requestKey                                 keyspace.Key
}

// NewResolvedImport constructs the resolved import witness. It authenticates
// the request against the canonical Program carried by the mount and derives
// the row identity from Link, mount, exact request, and target geometry.
func NewResolvedImport(linkID identity.ContentID, mount programmount.Program, request programschema.ModuleRequest, targetModuleKey identity.ContentID) (ResolvedImport, bool) {
	if !linkID.Available() || !mount.Available() || !request.Available() || !targetModuleKey.Available() || !requestInProgram(mount.Program, request) {
		return ResolvedImport{}, false
	}
	row := ResolvedImport{
		link: linkID, sourceModuleKey: mount.ModuleKey, targetModuleKey: targetModuleKey,
		artifactID: mount.ArtifactID, programID: mount.ProgramID,
		importID: request.ImportID(), requestID: request.ID(), valueID: request.ValueID(), requestKey: request.Key(),
	}
	row.id = resolvedImportID(row)
	return row, row.Available()
}

// Available reports whether the row is a complete, self-authenticated
// canonical join. Membership in the canonical Program was checked by the
// constructor; this row-level law is intentionally scalar-only.
func (row ResolvedImport) Available() bool {
	return row.id.Available() && row.link.Available() && row.sourceModuleKey.Available() && row.targetModuleKey.Available() &&
		row.artifactID.Available() && row.programID.Available() && row.importID.Available() && row.requestID.Available() &&
		row.valueID.Available() && row.requestKey != 0 && row.id == resolvedImportID(row)
}

func (row ResolvedImport) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ResolvedImport) LinkID() identity.ContentID {
	if row.Available() {
		return row.link
	}
	return identity.ContentID{}
}
func (row ResolvedImport) SourceModuleKey() identity.ContentID {
	if row.Available() {
		return row.sourceModuleKey
	}
	return identity.ContentID{}
}
func (row ResolvedImport) ArtifactID() identity.ContentID {
	if row.Available() {
		return row.artifactID
	}
	return identity.ContentID{}
}
func (row ResolvedImport) ProgramID() identity.ContentID {
	if row.Available() {
		return row.programID
	}
	return identity.ContentID{}
}
func (row ResolvedImport) RequestID() identity.ContentID {
	if row.Available() {
		return row.requestID
	}
	return identity.ContentID{}
}
func (row ResolvedImport) TargetModuleKey() identity.ContentID {
	if row.Available() {
		return row.targetModuleKey
	}
	return identity.ContentID{}
}
func (row ResolvedImport) ImportID() identity.ContentID {
	if row.Available() {
		return row.importID
	}
	return identity.ContentID{}
}
func (row ResolvedImport) ValueID() identity.ContentID {
	if row.Available() {
		return row.valueID
	}
	return identity.ContentID{}
}
func (row ResolvedImport) RequestKey() keyspace.Key {
	if row.Available() {
		return row.requestKey
	}
	return 0
}

func requestInProgram(program programschema.Program, request programschema.ModuleRequest) bool {
	if !program.Available() || !request.Available() {
		return false
	}
	count, ok := program.ModuleRequestCount()
	if !ok {
		return false
	}
	found := false
	for index := 0; index < count; index++ {
		candidate, held := program.ModuleRequestAt(index)
		if !held || !candidate.Available() {
			return false
		}
		if candidate.ID() != request.ID() {
			continue
		}
		if found || candidate.ImportID() != request.ImportID() || candidate.ValueID() != request.ValueID() || candidate.Key() != request.Key() {
			return false
		}
		found = true
	}
	return found
}

package static

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

const namespaceLaw = "wippy.link/static-namespace/v1"

// Build issues exactly one detached namespace ID for every concrete Project
// mount. It intentionally never calls Mounts.Program or opens Program data.
func Build(input Input) (*Draft, error) {
	if input.Project == nil {
		return nil, errors.New("link/static: invalid project")
	}
	project := input.Project.Cold()
	if !project.ContentID().Available() || !project.TargetID().Available() {
		return nil, errors.New("link/static: unavailable project identity")
	}
	mounts := input.Project.Mounts()
	if mounts.Count() == 0 {
		return nil, errors.New("link/static: empty project")
	}
	schema := make([]identity.ContentID, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		programID, programOK := mounts.ProgramID(shard)
		moduleID, moduleOK := input.Project.ModuleKey(shard)
		id := namespaceID(project.TargetID(), programID, moduleID, uint32(index+1))
		if !shardOK || !programOK || !moduleOK || !id.Available() {
			return nil, errors.New("link/static: malformed mount namespace")
		}
		schema[index] = id
	}
	contentID := namespacePlaneID(project.ContentID(), schema)
	if !contentID.Available() {
		return nil, errors.New("link/static: unavailable namespace plane")
	}
	return &Draft{state: &draftState{component: &Component{contentID: contentID, schema: schema}, fence: &draftFence{}}}, nil
}

func (d *Draft) Finalize() (*Component, error) {
	if d == nil || d.state == nil {
		return nil, errors.New("link/static: invalid finalization")
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.consumed || d.state.component == nil {
		return nil, errors.New("link/static: consumed draft")
	}
	d.state.consumed = true
	d.state.fence.consumed = true
	component := d.state.component
	d.state.component = nil
	return component, nil
}

func namespaceID(targetID, programID, moduleID identity.ContentID, ordinal uint32) (id identity.ContentID) {
	if !targetID.Available() || !programID.Available() || !moduleID.Available() || ordinal == 0 {
		return id
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, namespaceLaw, 1) != nil || w.Record(1) != nil || w.Bytes(targetID[:]) != nil || w.Bytes(programID[:]) != nil || w.Bytes(moduleID[:]) != nil || w.Uint(uint64(ordinal)) != nil || w.Finish() != nil {
		return id
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

func namespacePlaneID(projectID identity.ContentID, schema []identity.ContentID) (id identity.ContentID) {
	if !projectID.Available() || len(schema) == 0 {
		return id
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, namespaceLaw, 2) != nil || w.Record(1) != nil || w.Bytes(projectID[:]) != nil || w.Count(uint64(len(schema))) != nil {
		return id
	}
	for _, namespace := range schema {
		if !namespace.Available() || w.Bytes(namespace[:]) != nil {
			return id
		}
	}
	if w.Finish() != nil {
		return id
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

package project

import (
	"bytes"
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

const contentVersion = 1

const (
	mountRelationVersion       = 1
	applicationRelationVersion = 1
)

// contentID names the complete authored Project constituent. Keys and
// Applications are deterministic projections of these inputs under this
// version, so encoding their derived rows again would create a second content
// authority.
func contentID(targetID identity.ContentID, mounts []mountRow) (id identity.ContentID) {
	if !targetID.Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/project", contentVersion) != nil ||
		w.Record(1) != nil || w.Bytes(targetID[:]) != nil ||
		w.Count(uint64(len(mounts))) != nil {
		return identity.ContentID{}
	}
	names := make(map[string]struct{}, len(mounts))
	for index, mount := range mounts {
		if mount.name == "" || mount.program == nil || !mount.id.Available() ||
			mount.program.ContentID() != mount.id ||
			w.Bytes(mount.id[:]) != nil || w.String(mount.name) != nil {
			return identity.ContentID{}
		}
		if _, duplicate := names[mount.name]; duplicate {
			return identity.ContentID{}
		}
		names[mount.name] = struct{}{}
		if index != 0 {
			prior := mounts[index-1]
			order := bytes.Compare(prior.id[:], mount.id[:])
			if order > 0 || (order == 0 && prior.name >= mount.name) {
				return identity.ContentID{}
			}
		}
	}
	if w.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// mountRelationID names only the canonical authored mount relation.  It is
// intentionally independent of Target and the enclosing Link: ModuleKey
// consumers must not acquire a whole-project or whole-Link dependency merely
// because the mount was assembled by Link.
func mountRelationID(mounts []mountRow) (id identity.ContentID) {
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/project/mounts", mountRelationVersion) != nil ||
		w.Record(1) != nil || w.Count(uint64(len(mounts))) != nil {
		return identity.ContentID{}
	}
	for index, mount := range mounts {
		if mount.name == "" || mount.program == nil || !mount.id.Available() || mount.program.ContentID() != mount.id ||
			w.Record(1) != nil || w.String(mount.name) != nil || w.Bytes(mount.id[:]) != nil || w.Uint(uint64(index+1)) != nil {
			return identity.ContentID{}
		}
	}
	if w.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// applicationRelationID names the complete canonical Project Application
// relation, including typed subsequence classification and Import-to-Call
// correspondence.  It stores no product with Target operations and no Link
// root identity.
func applicationRelationID(applications []applicationRow) (id identity.ContentID) {
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/project/applications", applicationRelationVersion) != nil ||
		w.Record(1) != nil || w.Count(uint64(len(applications))) != nil {
		return identity.ContentID{}
	}
	for index, application := range applications {
		if application.kind == 0 || application.shard == 0 || application.term == 0 || application.root > uint32(len(applications)) ||
			w.Record(1) != nil || w.Uint(uint64(index+1)) != nil || w.Uint(uint64(application.kind)) != nil ||
			w.Uint(uint64(application.shard)) != nil || w.Uint(uint64(application.term)) != nil ||
			w.Uint(uint64(application.slot)) != nil || w.Uint(uint64(application.root)) != nil {
			return identity.ContentID{}
		}
	}
	if w.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

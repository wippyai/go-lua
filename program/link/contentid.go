package link

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

const (
	// v32 makes Project, Boundary, and Static contribute only their immutable child
	// digests; no semantic child is mutated after root identity.
	linkContentVersion = 34
	linkPolicyVersion  = 34
)

// contentID names the complete authored Link input under one fixed sealing
// policy. Each semantic child contributes its own versioned authority rather
// than being reopened and independently encoded by the root.
func contentID(link *Link, project linkproject.Cold, boundary *linkboundary.Component, module linkmodule.Cold, host linkhost.Cold, static linkstatic.Cold) (id keyspace.ContentID) {
	defer func() {
		if recover() != nil {
			id = keyspace.ContentID{}
		}
	}()
	projectID := project.ContentID()
	if !projectID.Available() || link == nil {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	var w canonical.Writer
	if w.Reset(h, "program/link/v34", linkContentVersion) != nil {
		return keyspace.ContentID{}
	}
	if w.Record(1) != nil || w.Uint(linkPolicyVersion) != nil || w.Bytes(projectID[:]) != nil {
		return keyspace.ContentID{}
	}
	boundaryID := keyspace.ContentID{}
	if boundary != nil {
		boundaryID = boundary.ContentID()
	}
	if !boundaryID.Available() || w.Bytes(boundaryID[:]) != nil {
		return keyspace.ContentID{}
	}
	moduleID := module.ContentID()
	if !moduleID.Available() || w.Bytes(moduleID[:]) != nil {
		return keyspace.ContentID{}
	}
	if !contentStaticAuthority(&w, static) {
		return keyspace.ContentID{}
	}
	hostID := host.ContentID()
	if !hostID.Available() || w.Bytes(hostID[:]) != nil {
		return keyspace.ContentID{}
	}
	if link.project == nil || !contentDependencyDigest(&w, boundary, link.project.Mounts(), static, module) {
		return keyspace.ContentID{}
	}
	if w.Finish() != nil {
		return keyspace.ContentID{}
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

// contentDependencyDigest records only Link's closed typed relation. The
// target BindingSpec used during admission is intentionally absent: it is a
// replay witness, while the resolved Operation is the semantic dependency.
// The rows are derived from child authorities for this call and never stored.
func contentDependencyDigest(w *canonical.Writer, boundary *linkboundary.Component, mounts linkproject.Mounts, static linkstatic.Cold, module linkmodule.Cold) bool {
	if w == nil || boundary == nil {
		return false
	}
	contract, ok := boundary.Target()
	if !ok || contract == nil {
		return false
	}
	rows, err := deriveDependencyRows(boundary, mounts, static, module)
	if err != nil {
		return false
	}
	if w == nil || w.Count(uint64(len(rows))) != nil {
		return false
	}
	for index, row := range rows {
		if !validDependencyRow(contract, row) ||
			(index != 0 && compareDependencyRow(rows[index-1], row) >= 0) ||
			w.Uint(uint64(row.kind)) != nil || w.Bytes(row.id[:]) != nil || w.Uint(uint64(row.operation)) != nil {
			return false
		}
	}
	return true
}

// contentStaticAuthority includes the sealed static-only structural authority
// in Link identity. Artifact replay derives the rows again from the same
// Programs; it never serializes a duplicate static resolver.
func contentStaticAuthority(w *canonical.Writer, static linkstatic.Cold) bool {
	if w == nil {
		return false
	}
	id := static.ContentID()
	return id.Available() && w.Bytes(id[:]) == nil
}

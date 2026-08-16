package link

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	linkstatic "github.com/wippyai/go-lua/analysis/program/link/static"
)

const (
	// v35 deletes the legacy LinkStatic Program-reconstruction constituent.
	// ProgramArtifact is the only owner of Program-internal static facts.
	linkContentVersion = 35
	linkPolicyVersion  = 35
)

// contentID names the complete authored Link input under one fixed sealing
// policy. Each semantic child contributes its own versioned authority rather
// than being reopened and independently encoded by the root.
func contentID(link *Link, project linkproject.Cold, boundary *linkboundary.Component, module linkmodule.Cold, host linkhost.Cold, static linkstatic.Cold) (id identity.ContentID) {
	defer func() {
		if recover() != nil {
			id = identity.ContentID{}
		}
	}()
	projectID := project.ContentID()
	if !projectID.Available() || link == nil {
		return identity.ContentID{}
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/link/v34", linkContentVersion) != nil {
		return identity.ContentID{}
	}
	if w.Record(1) != nil || w.Uint(linkPolicyVersion) != nil || w.Bytes(projectID[:]) != nil {
		return identity.ContentID{}
	}
	boundaryID := identity.ContentID{}
	if boundary != nil {
		boundaryID = boundary.ContentID()
	}
	if !boundaryID.Available() || w.Bytes(boundaryID[:]) != nil {
		return identity.ContentID{}
	}
	moduleID := module.ContentID()
	if !moduleID.Available() || w.Bytes(moduleID[:]) != nil {
		return identity.ContentID{}
	}
	staticID := static.ContentID()
	if !staticID.Available() || w.Bytes(staticID[:]) != nil {
		return identity.ContentID{}
	}
	hostID := host.ContentID()
	if !hostID.Available() || w.Bytes(hostID[:]) != nil {
		return identity.ContentID{}
	}
	if link.project == nil || !contentDependencyDigest(&w, boundary, static, module) {
		return identity.ContentID{}
	}
	if w.Finish() != nil {
		return identity.ContentID{}
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// contentDependencyDigest records only Link's closed typed relation. The
// target BindingSpec used during admission is intentionally absent: it is a
// replay witness, while the resolved Operation is the semantic dependency.
// The rows are derived from child authorities for this call and never stored.
func contentDependencyDigest(w *framing.Writer, boundary *linkboundary.Component, static linkstatic.Cold, module linkmodule.Cold) bool {
	if w == nil || boundary == nil {
		return false
	}
	contract, ok := boundary.Target()
	if !ok || contract == nil {
		return false
	}
	rows, err := deriveDependencyRows(boundary, static, module)
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

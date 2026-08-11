// Package link owns immutable external source seeds and the project-sealed
// structural applications which relate already-sealed Programs to a target
// contract. It never copies Program or target correspondence rows.
package link

import (
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/target"
)

// Spec is consumed by Seal, including on failure.
type Spec struct {
	Modules []linkproject.Module
	Target  *target.Contract
	// EndpointRequests are Boundary-owned provider admissions. Host consumes
	// their issued Endpoint handles; it never mints a parallel endpoint family.
	EndpointRequests []linkboundary.EndpointRequest
	Host             linkhost.Spec
	Module           linkmodule.Spec
	consumed         bool
}

// Link stores canonical sealed authorities. Child components own their
// immutable rows; Link never retains a mixed dependency union or copies
// Program or Target correspondence.
type Link struct {
	// static is the sole immutable owner of Link's static-resolution relation.
	// It is derived before runtime Link geometry and has no runtime dependency.
	static   *linkstatic.Component
	project  *linkproject.Component
	boundary *linkboundary.Component
	// module is the sole actor/cache/init owner.  The remaining legacy root
	// fields below are being removed in the same atomic cut; no new root query
	// is permitted to consume them.
	module          *linkmodule.Component
	host            *linkhost.Component
	id              keyspace.ContentID
	semanticReceipt SemanticSourceReceipt
}

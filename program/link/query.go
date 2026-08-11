package link

import (
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

// Static returns Link's sole immutable static-resolution owner.
func (l *Link) Static() *linkstatic.Component {
	if l == nil {
		return nil
	}
	return l.static
}

// Project returns Link's sole immutable mount, exact-key, and Application
// authority. Construction-only Draft state is never observable here.
func (l *Link) Project() *linkproject.Component {
	if l == nil {
		return nil
	}
	return l.project
}

// Boundary returns Link's sole exact Program/Target topology owner. No root
// forwarding predicates or parallel Application x Operation surface remain.
func (l *Link) Boundary() *linkboundary.Component {
	if l == nil {
		return nil
	}
	return l.boundary
}

// Module returns Link's sole immutable actor-local module-cache owner.  The
// root deliberately forwards no Module handle or query: callers must retain
// the child authority which issued the handle.
func (l *Link) Module() *linkmodule.Component {
	if l == nil {
		return nil
	}
	return l.module
}

// Host is the sole provider/bootstrap/selector authority.
func (l *Link) Host() *linkhost.Component {
	if l == nil {
		return nil
	}
	return l.host
}

func (l *Link) ContentID() keyspace.ContentID {
	if l == nil {
		return keyspace.ContentID{}
	}
	return l.id
}

package link

import (
	"github.com/wippyai/go-lua/analysis/identity"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

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

// ContextDirectory returns Link's detached, frozen execution-context
// directory. Root IDs appear only on ingress rows; transitions and later
// consumers resolve Context IDs from this scalar result.
func (l *Link) ContextDirectory() executioncontext.Directory {
	if l == nil || !l.contextDirectory.Available() {
		return executioncontext.Directory{}
	}
	return l.contextDirectory
}

// Host is the sole provider/bootstrap/selector authority.
func (l *Link) Host() *linkhost.Component {
	if l == nil {
		return nil
	}
	return l.host
}

func (l *Link) ContentID() identity.ContentID {
	if l == nil {
		return identity.ContentID{}
	}
	return l.id
}

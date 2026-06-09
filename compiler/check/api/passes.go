package api

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
)

// ArtifactProjection projects post-solve public artifacts into FuncResult.Extras.
//
// It runs after canonical convergence while the public FuncResult read model is
// being built. It must not feed the canonical solve or publish semantic facts;
// analyses that need convergence must live in canonical transfer/summary
// carriers instead.
type ArtifactProjection interface {
	Name() string
	Project(graph *cfg.Graph, scopes map[cfg.Point]*scope.State) any
}

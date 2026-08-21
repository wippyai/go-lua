package analysis

import (
	"sync"

	analysisworkspace "github.com/wippyai/go-lua/analysis/internal/workspace"
	"github.com/wippyai/go-lua/domain/composite"
)

// Workspace is the explicit caller-owned lifetime for reusable immutable
// Program compiler products. Compile calls may run concurrently and share
// equal products only within this Workspace. Close admits no new work, waits
// for every compiled Plan to close, and then releases the product directory.
type Workspace struct {
	lifecycleMu   sync.Mutex
	lifecycleCond *sync.Cond
	artifacts     *analysisworkspace.Artifacts
	compilation   composite.Compilation
	compiles      uint64
	plans         uint64
	closing       bool
	closed        bool
	ephemeral     bool
}

// NewWorkspace opens an empty explicit analyzer Workspace.
func NewWorkspace() *Workspace {
	return newWorkspace(false)
}

func newWorkspace(ephemeral bool) *Workspace {
	compilation, compilationOK := composite.Build()
	workspace := &Workspace{
		artifacts:   analysisworkspace.NewArtifacts(),
		compilation: compilation,
		ephemeral:   ephemeral,
	}
	if !compilationOK {
		workspace.artifacts.Close()
		workspace.artifacts = nil
	}
	workspace.lifecycleCond = sync.NewCond(&workspace.lifecycleMu)
	return workspace
}

func (workspace *Workspace) beginCompile() (*analysisworkspace.Artifacts, bool) {
	if workspace == nil {
		return nil, false
	}
	workspace.lifecycleMu.Lock()
	defer workspace.lifecycleMu.Unlock()
	if workspace.lifecycleCond == nil || workspace.artifacts == nil || !workspace.compilation.Available() || workspace.closing || workspace.closed {
		return nil, false
	}
	workspace.compiles++
	return workspace.artifacts, true
}

func (workspace *Workspace) finishCompile(plan bool) {
	if workspace == nil {
		return
	}
	var artifacts *analysisworkspace.Artifacts
	workspace.lifecycleMu.Lock()
	if workspace.compiles > 0 {
		workspace.compiles--
	}
	if plan {
		workspace.plans++
	}
	// A convenience call owns a single private Workspace. It becomes terminal
	// before its Plan is published, so no unrelated call can ever enter it.
	if workspace.ephemeral {
		workspace.closing = true
		if workspace.compiles == 0 && workspace.plans == 0 && !workspace.closed {
			artifacts = workspace.artifacts
			workspace.artifacts = nil
			workspace.compilation = composite.Compilation{}
			workspace.closed = true
		}
	}
	workspace.lifecycleCond.Broadcast()
	workspace.lifecycleMu.Unlock()
	if artifacts != nil {
		artifacts.Close()
	}
}

func (workspace *Workspace) releasePlan() bool {
	if workspace == nil {
		return false
	}
	var artifacts *analysisworkspace.Artifacts
	workspace.lifecycleMu.Lock()
	if workspace.lifecycleCond == nil || workspace.plans == 0 {
		workspace.lifecycleMu.Unlock()
		return false
	}
	workspace.plans--
	if workspace.plans == 0 {
		workspace.lifecycleCond.Broadcast()
	}
	if workspace.ephemeral && workspace.closing && workspace.compiles == 0 && workspace.plans == 0 && !workspace.closed {
		artifacts = workspace.artifacts
		workspace.artifacts = nil
		workspace.compilation = composite.Compilation{}
		workspace.closed = true
	}
	workspace.lifecycleMu.Unlock()
	if artifacts != nil {
		artifacts.Close()
	}
	return true
}

// Close ends this Workspace. It waits for already-admitted Compile calls and
// live Plans because those Plans still read the immutable products this owner
// must release. Callers must close their Plans to complete this operation.
func (workspace *Workspace) Close() bool {
	if workspace == nil {
		return false
	}
	workspace.lifecycleMu.Lock()
	if workspace.lifecycleCond == nil || workspace.closing || workspace.closed {
		workspace.lifecycleMu.Unlock()
		return false
	}
	workspace.closing = true
	for workspace.compiles != 0 || workspace.plans != 0 {
		workspace.lifecycleCond.Wait()
	}
	artifacts := workspace.artifacts
	workspace.artifacts = nil
	workspace.compilation = composite.Compilation{}
	workspace.closed = true
	workspace.lifecycleMu.Unlock()
	if artifacts != nil {
		artifacts.Close()
	}
	return true
}

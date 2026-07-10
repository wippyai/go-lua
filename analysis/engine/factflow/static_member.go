package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
)

// PathStaticMemberWrite describes a structural/static-member proof write at a
// CFG point. It is intentionally distinct from PathAssignment, which describes
// ordinary member/path assignment value flow.
type PathStaticMemberWrite struct {
	targetPath path.Path
	source     ValueSource
}

// NewPathStaticMemberWrite creates a static-member proof write event.
func NewPathStaticMemberWrite(targetPath path.Path, source ValueSource) PathStaticMemberWrite {
	return PathStaticMemberWrite{
		targetPath: targetPath.Clone(),
		source:     source,
	}
}

// TargetPath returns the static member path written by this event.
func (w PathStaticMemberWrite) TargetPath() path.Path { return w.targetPath.Clone() }

// TargetPathRef returns the static member path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (w PathStaticMemberWrite) TargetPathRef() path.Path { return w.targetPath }

// Source returns the value evidence to write for TargetPath.
func (w PathStaticMemberWrite) Source() ValueSource { return w.source }

func (w PathStaticMemberWrite) copy() PathStaticMemberWrite {
	w.targetPath = w.targetPath.Clone()
	return w
}

package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootAssignment describes a root-symbol write at a CFG point.
type RootAssignment struct {
	targetSymbol symbol.ID
	targetPath   path.Path
	source       ValueSource
}

// NewRootAssignment creates a root-symbol assignment fact.
func NewRootAssignment(targetSymbol symbol.ID, targetPath path.Path, source ValueSource) RootAssignment {
	return RootAssignment{
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
		source:       source,
	}
}

// TargetSymbol returns the assignment target's symbol identity.
func (a RootAssignment) TargetSymbol() symbol.ID { return a.targetSymbol }

// TargetPath returns the assignment target's path identity.
func (a RootAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target.
func (a RootAssignment) Source() ValueSource { return a.source }

func (a RootAssignment) copy() RootAssignment {
	a.targetPath = copyPath(a.targetPath)
	return a
}

// PathAssignment describes a member/path refinement write at a CFG point.
type PathAssignment struct {
	targetPath path.Path
	source     ValueSource
}

// NewPathAssignment creates a member/path assignment fact.
func NewPathAssignment(targetPath path.Path, source ValueSource) PathAssignment {
	return PathAssignment{
		targetPath: copyPath(targetPath),
		source:     source,
	}
}

// TargetPath returns the assignment target's path identity.
func (a PathAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target path.
func (a PathAssignment) Source() ValueSource { return a.source }

func (a PathAssignment) copy() PathAssignment {
	a.targetPath = copyPath(a.targetPath)
	return a
}

// PathDescendantInvalidation describes a write through an unresolved descendant
// of a statically known container path.
type PathDescendantInvalidation struct {
	containerPath path.Path
}

// NewPathDescendantInvalidation creates a descendant-only invalidation fact for
// a statically known container path.
func NewPathDescendantInvalidation(containerPath path.Path) PathDescendantInvalidation {
	return PathDescendantInvalidation{containerPath: copyPath(containerPath)}
}

// ContainerPath returns the invalidated container's path identity.
func (i PathDescendantInvalidation) ContainerPath() path.Path {
	return copyPath(i.containerPath)
}

func (i PathDescendantInvalidation) copy() PathDescendantInvalidation {
	i.containerPath = copyPath(i.containerPath)
	return i
}

func copyRootAssignmentMap(in map[cfg.Point]RootAssignment) map[cfg.Point]RootAssignment {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]RootAssignment, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyPathDescendantInvalidationMap(in map[cfg.Point]PathDescendantInvalidation) map[cfg.Point]PathDescendantInvalidation {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]PathDescendantInvalidation, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

func copyPathAssignmentMap(in map[cfg.Point]PathAssignment) map[cfg.Point]PathAssignment {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]PathAssignment, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}

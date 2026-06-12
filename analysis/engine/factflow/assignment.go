package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootAssignmentKind preserves the source origin of a root-symbol write.
type RootAssignmentKind int

const (
	RootAssignmentUnknown RootAssignmentKind = iota
	RootAssignmentLocalDeclaration
	RootAssignmentOrdinaryRootWrite
)

// RootAssignment describes a root-symbol write at a CFG point.
type RootAssignment struct {
	kind         RootAssignmentKind
	targetSymbol symbol.ID
	targetPath   path.Path
	source       ValueSource

	declaredValue    product.Value
	hasDeclaredValue bool
}

// NewRootAssignment creates a root-symbol assignment fact.
func NewRootAssignment(kind RootAssignmentKind, targetSymbol symbol.ID, targetPath path.Path, source ValueSource) RootAssignment {
	return RootAssignment{
		kind:         kind,
		targetSymbol: targetSymbol,
		targetPath:   copyPath(targetPath),
		source:       source,
	}
}

// NewRootAssignmentWithDeclaredValue creates a root assignment fact that can
// write declared type evidence when its source has no value evidence.
func NewRootAssignmentWithDeclaredValue(kind RootAssignmentKind, targetSymbol symbol.ID, targetPath path.Path, source ValueSource, declaredValue product.Value) RootAssignment {
	fact := NewRootAssignment(kind, targetSymbol, targetPath, source)
	fact.declaredValue = declaredValue
	fact.hasDeclaredValue = true
	return fact
}

// Kind returns the source origin for this root assignment.
func (a RootAssignment) Kind() RootAssignmentKind { return a.kind }

// TargetSymbol returns the assignment target's symbol identity.
func (a RootAssignment) TargetSymbol() symbol.ID { return a.targetSymbol }

// TargetPath returns the assignment target's path identity.
func (a RootAssignment) TargetPath() path.Path { return copyPath(a.targetPath) }

// Source returns the value assigned to the target.
func (a RootAssignment) Source() ValueSource { return a.source }

// DeclaredValue returns conservative declared type evidence to write when
// Source has no value evidence.
func (a RootAssignment) DeclaredValue() (product.Value, bool) {
	return a.declaredValue, a.hasDeclaredValue
}

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

func mergePathDescendantInvalidationMap(a, b map[cfg.Point]PathDescendantInvalidation) map[cfg.Point]PathDescendantInvalidation {
	out := copyPathDescendantInvalidationMap(a)
	if len(out) == 0 {
		out = make(map[cfg.Point]PathDescendantInvalidation, len(b))
	}
	for point, fact := range b {
		if existing, ok := out[point]; ok && !existing.ContainerPath().Equal(fact.ContainerPath()) {
			continue
		}
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

package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// RootAssignmentKind preserves the source origin of a root-symbol write.
type RootAssignmentKind int

const (
	RootAssignmentLocalDeclaration RootAssignmentKind = iota + 1
	RootAssignmentOrdinaryRootWrite
)

// RootAssignment describes a root-symbol write at a CFG point.
type RootAssignment struct {
	kind          RootAssignmentKind
	targetSymbol  symbol.ID
	targetPath    path.Path
	source        ValueSource
	targetSpan    SourceSpan
	hasTargetSpan bool

	declaredValue          product.Value
	hasDeclaredValue       bool
	declaredValueContracts bool
	declaredValueOverlays  bool
	annotationValue        product.Value
	hasAnnotationValue     bool
}

// NewRootAssignment creates a root-symbol assignment fact.
func NewRootAssignment(kind RootAssignmentKind, targetSymbol symbol.ID, targetPath path.Path, source ValueSource) RootAssignment {
	return RootAssignment{
		kind:         kind,
		targetSymbol: targetSymbol,
		targetPath:   targetPath.Clone(),
		source:       source,
	}
}

func newRootAssignmentWithDeclaredValue(kind RootAssignmentKind, targetSymbol symbol.ID, targetPath path.Path, source ValueSource, declaredValue product.Value) RootAssignment {
	fact := NewRootAssignment(kind, targetSymbol, targetPath, source)
	fact.declaredValue = declaredValue
	fact.hasDeclaredValue = true
	fact.annotationValue = declaredValue
	fact.hasAnnotationValue = true
	return fact
}

// NewRootAssignmentWithDeclaredContractValue creates a root assignment fact
// whose declared type is the assignment's authoritative contract.
func NewRootAssignmentWithDeclaredContractValue(kind RootAssignmentKind, targetSymbol symbol.ID, targetPath path.Path, source ValueSource, declaredValue product.Value) RootAssignment {
	fact := newRootAssignmentWithDeclaredValue(kind, targetSymbol, targetPath, source, declaredValue)
	fact.declaredValueContracts = true
	return fact
}

// NewRootAssignmentWithDeclaredOverlayValue creates a root assignment fact whose
// source value remains precise but is overlaid with the target's declared
// contract before writing.
func NewRootAssignmentWithDeclaredOverlayValue(kind RootAssignmentKind, targetSymbol symbol.ID, targetPath path.Path, source ValueSource, declaredValue product.Value) RootAssignment {
	fact := newRootAssignmentWithDeclaredValue(kind, targetSymbol, targetPath, source, declaredValue)
	fact.declaredValueOverlays = true
	return fact
}

// Kind returns the source origin for this root assignment.
func (a RootAssignment) Kind() RootAssignmentKind { return a.kind }

// TargetSymbol returns the assignment target's symbol identity.
func (a RootAssignment) TargetSymbol() symbol.ID { return a.targetSymbol }

// TargetPath returns the assignment target's path identity.
func (a RootAssignment) TargetPath() path.Path { return a.targetPath.Clone() }

// TargetPathRef returns the assignment target path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (a RootAssignment) TargetPathRef() path.Path { return a.targetPath }

// Source returns the value assigned to the target.
func (a RootAssignment) Source() ValueSource { return a.source }

// TargetSpan returns the lowered source range for the assignment target.
func (a RootAssignment) TargetSpan() (SourceSpan, bool) {
	return a.targetSpan, a.hasTargetSpan
}

// WithTargetSpan returns a copy carrying target-location display metadata.
func (a RootAssignment) WithTargetSpan(span SourceSpan) RootAssignment {
	a.targetSpan = span
	a.hasTargetSpan = sourceSpanValid(span)
	return a
}

// WithDeclaredAnnotationValue returns a copy carrying target annotation
// evidence for diagnostics and declaration queries. Unlike DeclaredValue, this
// metadata does not instruct transfer to write or overlay the target value.
func (a RootAssignment) WithDeclaredAnnotationValue(declaredValue product.Value) RootAssignment {
	a.annotationValue = declaredValue
	a.hasAnnotationValue = true
	return a
}

// DeclaredValue returns conservative declared type evidence to write when
// Source has no value evidence.
func (a RootAssignment) DeclaredValue() (product.Value, bool) {
	return a.declaredValue, a.hasDeclaredValue
}

// DeclaredAnnotationValue returns inert annotation evidence for the target. It
// is present for annotated declarations even when source precision should win
// and no DeclaredValue transfer fallback is required.
func (a RootAssignment) DeclaredAnnotationValue() (product.Value, bool) {
	return a.annotationValue, a.hasAnnotationValue
}

// DeclaredValueContracts reports whether declared value evidence is an
// explicit contract that should take precedence over source precision.
func (a RootAssignment) DeclaredValueContracts() bool {
	return a.hasDeclaredValue && a.declaredValueContracts
}

// DeclaredValueOverlays reports whether declared value evidence should be
// merged into source precision before writing the target.
func (a RootAssignment) DeclaredValueOverlays() bool {
	return a.hasDeclaredValue && a.declaredValueOverlays
}

func (a RootAssignment) copy() RootAssignment {
	a.targetPath = a.targetPath.Clone()
	return a
}

// PathAssignment describes a member/path refinement write at a CFG point.
type PathAssignment struct {
	targetPath       path.Path
	source           ValueSource
	targetSpan       SourceSpan
	containerSpan    SourceSpan
	hasTargetSpan    bool
	hasContainerSpan bool
}

// NewPathAssignment creates a member/path assignment fact.
func NewPathAssignment(targetPath path.Path, source ValueSource) PathAssignment {
	return PathAssignment{
		targetPath: targetPath.Clone(),
		source:     source,
	}
}

// TargetPath returns the assignment target's path identity.
func (a PathAssignment) TargetPath() path.Path { return a.targetPath.Clone() }

// TargetPathRef returns the assignment target path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (a PathAssignment) TargetPathRef() path.Path { return a.targetPath }

// Source returns the value assigned to the target path.
func (a PathAssignment) Source() ValueSource { return a.source }

// TargetSpan returns the lowered source range for the assignment target.
func (a PathAssignment) TargetSpan() (SourceSpan, bool) {
	return a.targetSpan, a.hasTargetSpan
}

// WithTargetSpan returns a copy carrying target-location display metadata.
func (a PathAssignment) WithTargetSpan(span SourceSpan) PathAssignment {
	a.targetSpan = span
	a.hasTargetSpan = sourceSpanValid(span)
	return a
}

// ContainerSpan returns the lowered source range for the assignment container.
func (a PathAssignment) ContainerSpan() (SourceSpan, bool) {
	return a.containerSpan, a.hasContainerSpan
}

// WithContainerSpan returns a copy carrying container-location display metadata.
func (a PathAssignment) WithContainerSpan(span SourceSpan) PathAssignment {
	a.containerSpan = span
	a.hasContainerSpan = sourceSpanValid(span)
	return a
}

func (a PathAssignment) copy() PathAssignment {
	a.targetPath = a.targetPath.Clone()
	return a
}

func sourceSpanValid(span SourceSpan) bool {
	return span.StartLine > 0 && span.StartCol > 0 && span.EndLine > 0 && span.EndCol > 0
}

// PathDescendantInvalidation describes a write through an unresolved descendant
// of a statically known container path.
type PathDescendantInvalidation struct {
	containerPath path.Path
	dynamicTarget dynamicInvalidationTarget
}

// NewPathDescendantInvalidation creates a descendant-only invalidation fact for
// a statically known container path.
func NewPathDescendantInvalidation(containerPath path.Path) PathDescendantInvalidation {
	return PathDescendantInvalidation{containerPath: containerPath.Clone()}
}

type dynamicInvalidationTarget struct {
	tablePath path.Path
	keySource ValueSource
	suffix    []segment.Segment
	ok        bool
}

// WithDynamicTarget returns i with the unresolved dynamic-index target that
// caused the broad invalidation. When the key source later proves a literal
// member, fact application can additionally invalidate the concrete path.
func (i PathDescendantInvalidation) WithDynamicTarget(tablePath path.Path, keySource ValueSource, suffix []segment.Segment) PathDescendantInvalidation {
	i.dynamicTarget = dynamicInvalidationTarget{
		tablePath: tablePath.Clone(),
		keySource: keySource,
		suffix:    append([]segment.Segment(nil), suffix...),
		ok:        true,
	}
	return i
}

// ContainerPath returns the invalidated container's path identity.
func (i PathDescendantInvalidation) ContainerPath() path.Path {
	return i.containerPath.Clone()
}

// ContainerPathRef returns the invalidated container path for immediate
// read-only use. Callers must not mutate or retain the returned path.
func (i PathDescendantInvalidation) ContainerPathRef() path.Path {
	return i.containerPath
}

// DynamicTarget returns the dynamic-index table, key source, and post-index
// suffix when this broad invalidation came from a target such as t[k].field.
func (i PathDescendantInvalidation) DynamicTarget() (path.Path, ValueSource, []segment.Segment, bool) {
	if !i.dynamicTarget.ok {
		return path.Path{}, ValueSource{}, nil, false
	}
	return i.dynamicTarget.tablePath.Clone(), i.dynamicTarget.keySource, append([]segment.Segment(nil), i.dynamicTarget.suffix...), true
}

// DynamicTargetRef returns the dynamic target for immediate read-only use.
// Callers must not mutate or retain the returned suffix.
func (i PathDescendantInvalidation) DynamicTargetRef() (path.Path, ValueSource, []segment.Segment, bool) {
	if !i.dynamicTarget.ok {
		return path.Path{}, ValueSource{}, nil, false
	}
	return i.dynamicTarget.tablePath, i.dynamicTarget.keySource, i.dynamicTarget.suffix, true
}

func (i PathDescendantInvalidation) copy() PathDescendantInvalidation {
	i.containerPath = i.containerPath.Clone()
	i.dynamicTarget.tablePath = i.dynamicTarget.tablePath.Clone()
	i.dynamicTarget.suffix = append([]segment.Segment(nil), i.dynamicTarget.suffix...)
	return i
}

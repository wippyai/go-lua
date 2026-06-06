package place

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/access"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// Place is a semantic lvalue/read location inside a PointState.
//
// A Place is not a CFG point. A point is where Kildall stores a PointState;
// a Place is the location inside that state touched by a local transfer, such as
// users, users.byName, or subscribers[cid][pid].
//
// Place values are canonical IR with no AST/cfg-lowering dependencies.
type Place struct {
	Root     cfg.SymbolID
	RootName string
	Steps    []Step
}

// StepKind identifies the structural kind of a location segment.
type StepKind uint8

const (
	StepStaticMember StepKind = iota + 1
	StepDynamicIndex
)

// Step identifies a structural path segment under a Place root.
//
// Static steps store a resolved MemberKey. Dynamic steps keep an already-evaluated
// abstract key value for the eventual lvalue semantics consumed by product.
type Step struct {
	Kind   StepKind
	Member value.MemberKey
	Key    product.AbstractValue
}

// ValueUpdater rewrites the value currently stored at a Place.
type ValueUpdater func(product.AbstractValue) (product.AbstractValue, bool)

// FinalDynamicWriter writes a final dynamic index step. It lets callers choose
// exact lvalue semantics while Place owns traversal through the root value.
type FinalDynamicWriter func(product.AbstractValue, Step, product.AbstractValue) (product.AbstractValue, bool)

// FromStaticPath converts a fully-static constraint.Path into a Place.
// It is lossless for paths with dynamic-free segments, returning false for
// placeholder/static-member-incompatible segments.
func FromStaticPath(path constraint.Path) (Place, bool) {
	loc, ok := access.FromPath(path)
	if !ok {
		return Place{}, false
	}
	return FromLocation(loc)
}

// FromSymbolPath builds a symbol-rooted Place from static segments.
func FromSymbolPath(sym cfg.SymbolID, segments []constraint.Segment) (Place, bool) {
	if sym == 0 {
		return Place{}, false
	}
	path := constraint.NewPath(sym, "")
	path.Segments = append(path.Segments, segments...)
	return FromStaticPath(path)
}

// FromLocation converts the shared access normal form into the compiler-facing
// Place IR.
func FromLocation(loc access.Location) (Place, bool) {
	if loc.Root == 0 {
		return Place{}, false
	}
	steps := make([]Step, 0, len(loc.Steps))
	for _, step := range loc.Steps {
		placeStep, ok := placeStepFromAccessStep(step)
		if !ok {
			return Place{}, false
		}
		steps = append(steps, placeStep)
	}
	return Place{Root: loc.Root, RootName: loc.RootName, Steps: steps}, true
}

// StaticPath projects a fully-static Place to a constraint.Path.
// Dynamic steps fail instead of inventing an exact path.
func (p Place) StaticPath() (constraint.Path, bool) {
	loc, ok := p.Location()
	if !ok {
		return constraint.Path{}, false
	}
	return loc.StaticPath()
}

// StaticPrefixPath projects the largest static prefix of Place.
//
// It is the write invalidation footprint for dynamic places like x[k].field:
// stale facts under x can be killed without pretending k is a concrete segment.
func (p Place) StaticPrefixPath() (constraint.Path, bool) {
	loc, ok := p.Location()
	if !ok {
		return constraint.Path{}, false
	}
	return loc.StaticPrefixPath()
}

// StaticPathKey returns the constraint.Path key used by condition/function-ref
// style domains (`sym<ID>...`).
func (p Place) StaticPathKey() (constraint.PathKey, bool) {
	path, ok := p.StaticPath()
	if !ok || path.IsEmpty() {
		return "", false
	}
	return path.Key(), true
}

// Location projects Place to the shared access normal form.
func (p Place) Location() (access.Location, bool) {
	if p.Root == 0 {
		return access.Location{}, false
	}
	steps := make([]access.Step, 0, len(p.Steps))
	for _, step := range p.Steps {
		accessStep, ok := accessStepFromPlaceStep(step)
		if !ok {
			return access.Location{}, false
		}
		steps = append(steps, accessStep)
	}
	return access.Location{Root: p.Root, RootName: p.RootName, Steps: steps}, true
}

// WriteFootprint derives the shared access write footprint for this Place.
func (p Place) WriteFootprint(
	presentElementWrite bool,
	written product.AbstractValue,
) (access.WriteFootprint, bool) {
	loc, ok := p.Location()
	if !ok {
		return access.WriteFootprint{}, false
	}
	return loc.WriteFootprint(presentElementWrite, written)
}

// FinalDynamicIndexTargetPath projects the table path for writes whose final
// access step is dynamic, such as rows[id] = value.
func (p Place) FinalDynamicIndexTargetPath() (constraint.Path, bool) {
	loc, ok := p.Location()
	if !ok {
		return constraint.Path{}, false
	}
	return loc.FinalDynamicIndexTargetPath()
}

// UpdateRootValue applies update at p inside root and returns the rebuilt root.
func (p Place) UpdateRootValue(root product.AbstractValue, update ValueUpdater) (product.AbstractValue, bool) {
	if update == nil {
		return product.AbstractValue{}, false
	}
	return updateValueAtSteps(root, p.Steps, update)
}

// AssignRootValue writes val at p inside root and returns the rebuilt root.
func (p Place) AssignRootValue(
	root product.AbstractValue,
	val product.AbstractValue,
	finalDynamic FinalDynamicWriter,
) (product.AbstractValue, bool) {
	if val.IsZero() {
		return product.AbstractValue{}, false
	}
	return assignValueAtSteps(root, p.Steps, val, finalDynamic)
}

// String returns a deterministic location string representation.
func (p Place) String() string {
	root := p.RootName
	if root == "" && p.Root != 0 {
		root = "$sym" + strconv.FormatUint(uint64(p.Root), 10)
	}
	var b strings.Builder
	b.WriteString(root)
	for _, step := range p.Steps {
		switch step.Kind {
		case StepStaticMember:
			seg, ok := SegmentFromMemberKey(step.Member)
			if !ok {
				b.WriteString("[?]")
				continue
			}
			b.WriteString(constraint.FormatSegments([]constraint.Segment{seg}))
		case StepDynamicIndex:
			if seg, ok := SegmentFromStep(step); ok {
				b.WriteString(constraint.FormatSegments([]constraint.Segment{seg}))
			} else {
				b.WriteString("[?]")
			}
		default:
			b.WriteString("[?]")
		}
	}
	return b.String()
}

// SegmentFromStep converts a Place Step to a static constraint segment when
// possible. Dynamic indexes with concrete literal values are converted to their
// static segment form.
func SegmentFromStep(step Step) (constraint.Segment, bool) {
	accessStep, ok := accessStepFromPlaceStep(step)
	if !ok {
		return constraint.Segment{}, false
	}
	return access.SegmentFromStep(accessStep)
}

func accessStepFromPlaceStep(step Step) (access.Step, bool) {
	switch step.Kind {
	case StepStaticMember:
		return access.Step{Kind: access.StepStaticMember, Member: step.Member}, true
	case StepDynamicIndex:
		return access.Step{Kind: access.StepDynamicIndex, Key: step.Key}, true
	default:
		return access.Step{}, false
	}
}

func placeStepFromAccessStep(step access.Step) (Step, bool) {
	switch step.Kind {
	case access.StepStaticMember:
		return Step{Kind: StepStaticMember, Member: step.Member}, true
	case access.StepDynamicIndex:
		return Step{Kind: StepDynamicIndex, Key: step.Key}, true
	default:
		return Step{}, false
	}
}

func updateValueAtSteps(
	base product.AbstractValue,
	steps []Step,
	update ValueUpdater,
) (product.AbstractValue, bool) {
	if len(steps) == 0 {
		return update(base)
	}
	step := steps[0]
	child, ok := readStep(base, step)
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	} else {
		child = product.NarrowPresent(child)
	}
	updatedChild, ok := updateValueAtSteps(child, steps[1:], update)
	if !ok || updatedChild.IsZero() {
		return product.AbstractValue{}, false
	}
	return writeStep(base, step, updatedChild)
}

func assignValueAtSteps(
	base product.AbstractValue,
	steps []Step,
	val product.AbstractValue,
	finalDynamic FinalDynamicWriter,
) (product.AbstractValue, bool) {
	if len(steps) == 0 {
		return val, true
	}
	step := steps[0]
	if len(steps) == 1 {
		return writeFinalStep(base, step, val, finalDynamic)
	}
	child, ok := readStep(base, step)
	if !ok || child.IsZero() {
		child = product.FromType(typ.NewRecord().Build())
	} else {
		child = product.NarrowPresent(child)
	}
	updatedChild, ok := assignValueAtSteps(child, steps[1:], val, finalDynamic)
	if !ok || updatedChild.IsZero() {
		return product.AbstractValue{}, false
	}
	return writeStep(base, step, updatedChild)
}

func readStep(base product.AbstractValue, step Step) (product.AbstractValue, bool) {
	if base.IsZero() {
		return product.AbstractValue{}, false
	}
	switch step.Kind {
	case StepStaticMember:
		return product.MemberOf(base, step.Member)
	case StepDynamicIndex:
		if step.Key.IsZero() {
			return product.AbstractValue{}, false
		}
		return product.IndexOf(base, step.Key)
	default:
		return product.AbstractValue{}, false
	}
}

func writeStep(base product.AbstractValue, step Step, child product.AbstractValue) (product.AbstractValue, bool) {
	if base.IsZero() || child.IsZero() {
		return product.AbstractValue{}, false
	}
	switch step.Kind {
	case StepStaticMember:
		return product.WithMember(base, step.Member, child), true
	case StepDynamicIndex:
		if step.Key.IsZero() {
			return product.AbstractValue{}, false
		}
		return product.MutateIndex(base, step.Key, child), true
	default:
		return product.AbstractValue{}, false
	}
}

func writeFinalStep(
	base product.AbstractValue,
	step Step,
	val product.AbstractValue,
	finalDynamic FinalDynamicWriter,
) (product.AbstractValue, bool) {
	if base.IsZero() || val.IsZero() {
		return product.AbstractValue{}, false
	}
	switch step.Kind {
	case StepStaticMember:
		return product.WithMember(base, step.Member, val), true
	case StepDynamicIndex:
		if step.Key.IsZero() {
			return product.AbstractValue{}, false
		}
		if finalDynamic != nil {
			return finalDynamic(base, step, val)
		}
		return product.WriteIndexForeign(base, step.Key, val), true
	default:
		return product.AbstractValue{}, false
	}
}

// SegmentFromMemberKey converts a static member key to a constraint segment.
func SegmentFromMemberKey(key value.MemberKey) (constraint.Segment, bool) {
	return access.SegmentFromMemberKey(key)
}

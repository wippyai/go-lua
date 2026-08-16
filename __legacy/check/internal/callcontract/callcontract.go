// Package callcontract owns callable contract operations used by post-solve
// read models. It hides the Lua typecall implementation vocabulary from
// readmodel projection code.
package callcontract

import (
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/contract"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typelit "github.com/wippyai/go-lua/analysis/domain/type/literal"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typecall"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// ArgumentConstraintViolation describes an inferred generic argument that does
// not satisfy the declared type-parameter constraint.
type ArgumentConstraintViolation struct {
	Index      int
	Got        typ.Type
	Constraint typ.Type
}

// InferencePathKind identifies one step from a call argument to the nested type
// position that contributed to a generic type-parameter binding.
type InferencePathKind int

const (
	InferencePathField InferencePathKind = iota + 1
	InferencePathStaticString
	InferencePathStaticInt
	InferencePathTypeArgument
	InferencePathFunctionParam
	InferencePathFunctionReturn
)

// InferencePathStep records one readable segment in a generic inference path.
type InferencePathStep struct {
	Kind  InferencePathKind
	Name  string
	Index int
}

// InferenceContribution records one concrete type that helped infer a generic
// type parameter from a call argument.
type InferenceContribution struct {
	Param *typ.TypeParam
	Index int
	Type  typ.Type
	Path  []InferencePathStep
}

// GenericCallTrace carries generic inference provenance without exposing the
// typecall package to readmodel consumers.
type GenericCallTrace struct {
	Contributions []InferenceContribution
}

// GenericInferenceConflict records one call argument whose use of a generic
// type parameter produced incompatible concrete contributions.
type GenericInferenceConflict struct {
	Index         int
	ParamName     string
	Contributions []InferenceContribution
}

// MemberCallStatus describes whether a receiver member can be called.
type MemberCallStatus uint8

const (
	MemberCallOK MemberCallStatus = iota
	MemberCallMissing
	MemberCallNotCallable
)

// InstantiateGenericCallWithTrace infers type arguments for a generic function
// call and returns readmodel-owned trace records.
func InstantiateGenericCallWithTrace(fn *typ.Function, args []typ.Type) (*typ.Function, []ArgumentConstraintViolation, GenericCallTrace) {
	instantiated, violations, trace := typecall.InstantiateGenericCallWithTrace(fn, args)
	return instantiated, convertViolations(violations), convertTrace(trace)
}

// InstantiatedArgumentAssignable reports whether actual can be assigned to a
// generic-instantiated formal type.
func InstantiatedArgumentAssignable(actual typ.Type, formal typ.Type) bool {
	return typecall.InstantiatedArgumentAssignable(actual, formal)
}

// Callable reports whether t can be invoked.
func Callable(t typ.Type) (*typ.Function, bool) {
	return typecall.Callable(t)
}

// TypeCallable reports whether t is wholly callable for diagnostic planning.
// A union is callable only when every arm is callable.
func TypeCallable(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	if _, ok := Callable(t); ok {
		return true
	}
	if union, ok := t.(*typ.Union); ok && len(union.Members) != 0 {
		for _, member := range union.Members {
			if !TypeCallable(member) {
				return false
			}
		}
		return true
	}
	return false
}

// TypeCallableIgnoringNil applies the callable predicate after dropping the
// optional nil arm used by nilable receiver diagnostics.
func TypeCallableIgnoringNil(t typ.Type) bool {
	return TypeCallable(unwrap.Optional(t))
}

// ParamConsumesReceiver applies the receiver-consumption rule for one formal.
func ParamConsumesReceiver(receiverParam bool, param typ.Type, receiver typ.Type) bool {
	return typecall.ParamConsumesReceiver(receiverParam, param, receiver)
}

// ReceiverTypeUsable reports whether a receiver type is precise enough to drive
// receiver binding and member-call contract lookup.
func ReceiverTypeUsable(t typ.Type) bool {
	t = ReceiverContractType(t)
	return t != nil && !typ.IsAny(t) && !typ.IsUnknown(t)
}

// ReceiverContractType returns the structural receiver shape used for member
// lookup. A constrained type parameter is callable through its upper bound: the
// generic value remains symbolic for assignment/inference, but receiver
// nilability and member lookup are governed by the constraint.
func ReceiverContractType(t typ.Type) typ.Type {
	if param, ok := unwrap.Annotations(t).(*typ.TypeParam); ok && param.Constraint != nil {
		return param.Constraint
	}
	return t
}

// BindReceiver consumes the first callable parameter when call syntax supplies
// an implicit receiver. Syntax owns implicit-self consumption; receiver type
// precision is only needed for explicit receiver-consuming parameters.
func BindReceiver(callContract contract.Contract, receiver typ.Type, supplied bool) contract.Contract {
	if !supplied {
		return callContract
	}
	receiver = ReceiverContractType(receiver)
	params := callContract.Params()
	if len(params) == 0 {
		return callContract
	}
	first := params[0]
	if !first.ImplicitSelf && (!ReceiverTypeUsable(receiver) || !ParamConsumesReceiver(first.ImplicitSelf, first.Type, receiver)) {
		return callContract
	}
	if bound, ok := callContract.BindFirstParameter(); ok {
		return bound
	}
	return callContract
}

// MemberCallable resolves a receiver member for call syntax and returns its
// callable witness.
func MemberCallable(receiver typ.Type, name string) (*typ.Function, MemberCallStatus, bool) {
	receiver = ReceiverContractType(receiver)
	fn, status, ok := typecall.MemberCallable(receiver, name)
	return fn, convertMemberCallStatus(status), ok
}

// MemberCall resolves a receiver member for call syntax without requiring the
// member to be callable. It returns the member type when one exists and the
// call-status classification owned by the Lua call contract layer.
func MemberCall(receiver typ.Type, member segment.Segment) (typ.Type, MemberCallStatus, bool) {
	receiver = ReceiverContractType(receiver)
	switch member.Kind {
	case segment.SegmentField:
		t, status := typecall.MemberCall(receiver, member.Name)
		return t, convertMemberCallStatus(status), true
	case segment.SegmentIndexString:
		t, status := typecall.IndexedMemberCall(receiver, typ.LiteralString(member.Name))
		return t, convertMemberCallStatus(status), true
	case segment.SegmentIndexInt:
		t, status := typecall.IndexedMemberCall(receiver, typ.LiteralInt(int64(member.Index)))
		return t, convertMemberCallStatus(status), true
	default:
		return nil, MemberCallMissing, false
	}
}

// MemberType returns the type of a receiver member when the member exists,
// without requiring that the member itself is callable.
func MemberType(receiver typ.Type, member segment.Segment) (typ.Type, bool) {
	t, status, ok := MemberCall(unwrap.Optional(receiver), member)
	if !ok || status == MemberCallMissing {
		return nil, false
	}
	return t, true
}

// InferenceContributionKey returns the stable deduplication key for a generic
// inference contribution. The key belongs with the inference-path vocabulary so
// read models do not duplicate path-step encoding rules.
func InferenceContributionKey(contribution InferenceContribution) string {
	if contribution.Type == nil || len(contribution.Path) == 0 {
		return ""
	}
	return InferencePathKey(contribution.Path) + "\x00" + contribution.Type.String()
}

// InferencePathKey returns the stable readable key for an inference path.
func InferencePathKey(path []InferencePathStep) string {
	if len(path) == 0 {
		return ""
	}
	var b strings.Builder
	for _, step := range path {
		switch step.Kind {
		case InferencePathField:
			b.WriteString(".")
			b.WriteString(step.Name)
		case InferencePathStaticString:
			b.WriteString("[")
			b.WriteString(step.Name)
			b.WriteString("]")
		case InferencePathStaticInt:
			b.WriteString("[")
			b.WriteString(strconv.Itoa(step.Index))
			b.WriteString("]")
		case InferencePathTypeArgument:
			b.WriteString("<")
			b.WriteString(strconv.Itoa(step.Index))
			b.WriteString(">")
		case InferencePathFunctionParam:
			b.WriteString(" param ")
			if step.Name != "" {
				b.WriteString(step.Name)
			} else {
				b.WriteString(strconv.Itoa(step.Index))
			}
		case InferencePathFunctionReturn:
			b.WriteString(" return ")
			b.WriteString(strconv.Itoa(step.Index))
		}
	}
	return b.String()
}

// InferenceContributionMatchesSegments reports whether the contribution's
// value path exactly matches object/member segments.
func InferenceContributionMatchesSegments(contribution InferenceContribution, segments []segment.Segment) bool {
	valueSegments := InferenceContributionValueSegments(contribution)
	if len(valueSegments) != len(segments) {
		return false
	}
	for i, seg := range segments {
		if !InferencePathStepMatchesSegment(valueSegments[i], seg) {
			return false
		}
	}
	return len(segments) > 0
}

// InferenceContributionHasSegmentPrefix reports whether the contribution's
// value path starts with object/member segments.
func InferenceContributionHasSegmentPrefix(contribution InferenceContribution, segments []segment.Segment) bool {
	valueSegments := InferenceContributionValueSegments(contribution)
	if len(segments) == 0 {
		return false
	}
	if len(segments) > len(valueSegments) {
		return false
	}
	for i, seg := range segments {
		if !InferencePathStepMatchesSegment(valueSegments[i], seg) {
			return false
		}
	}
	return true
}

// InferenceContributionValueSegments filters an inference path down to the
// steps that correspond to value/member traversal.
func InferenceContributionValueSegments(contribution InferenceContribution) []InferencePathStep {
	var out []InferencePathStep
	for _, step := range contribution.Path {
		if InferencePathStepIsValueSegment(step) {
			out = append(out, step)
		}
	}
	return out
}

// InferencePathStepIsValueSegment reports whether step maps to a value/member
// segment rather than type-argument or function-slot structure.
func InferencePathStepIsValueSegment(step InferencePathStep) bool {
	switch step.Kind {
	case InferencePathField, InferencePathStaticString, InferencePathStaticInt:
		return true
	default:
		return false
	}
}

// InferencePathStepMatchesSegment applies the canonical bridge between generic
// inference path steps and object/member path segments.
func InferencePathStepMatchesSegment(step InferencePathStep, seg segment.Segment) bool {
	switch step.Kind {
	case InferencePathField:
		return (seg.Kind == segment.SegmentField || seg.Kind == segment.SegmentIndexString) && step.Name == seg.Name
	case InferencePathStaticString:
		return (seg.Kind == segment.SegmentIndexString || seg.Kind == segment.SegmentField) && step.Name == seg.Name
	case InferencePathStaticInt:
		return seg.Kind == segment.SegmentIndexInt && step.Index == seg.Index
	default:
		return false
	}
}

// InferenceParamSetContains reports whether params already contains param by
// type-parameter identity/equality.
func InferenceParamSetContains(params []*typ.TypeParam, param *typ.TypeParam) bool {
	for _, candidate := range params {
		if SameInferenceParam(candidate, param) {
			return true
		}
	}
	return false
}

// SameInferenceParam reports whether two inference parameters describe the same
// generic type parameter.
func SameInferenceParam(left, right *typ.TypeParam) bool {
	return left == right || (left != nil && right != nil && left.Equals(right))
}

// InferenceParamName returns the stable display name for a generic inference
// parameter.
func InferenceParamName(param *typ.TypeParam) string {
	if param != nil && param.Name != "" {
		return param.Name
	}
	return "type parameter"
}

// InferenceTypesConflict reports whether two concrete contributions for one
// generic type parameter are incompatible enough to render an inference
// conflict. Literal members of the same scalar family remain compatible because
// they can widen to the family base.
func InferenceTypesConflict(left, right typ.Type) bool {
	if left == nil || right == nil || typ.SameNodeOrAcyclicEqual(left, right) {
		return false
	}
	if inferenceLiteralFamiliesCompatible(left, right) {
		return false
	}
	return !InstantiatedArgumentAssignable(left, right) &&
		!InstantiatedArgumentAssignable(right, left)
}

// InferenceTypeSetHasConflict reports whether any pair of inferred concrete
// types conflicts for one generic type parameter.
func InferenceTypeSetHasConflict(types []typ.Type) bool {
	if len(types) < 2 {
		return false
	}
	for i, left := range types {
		for _, right := range types[i+1:] {
			if InferenceTypesConflict(left, right) {
				return true
			}
		}
	}
	return false
}

// PlanGenericInferenceConflicts groups generic inference contributions by call
// argument and type parameter, removes duplicate path/type evidence, and
// returns the first conflicting parameter for each argument. This keeps the
// generic-inference conflict policy with the call-contract vocabulary instead
// of duplicating it in readmodel adapters.
func PlanGenericInferenceConflicts(trace GenericCallTrace) []GenericInferenceConflict {
	if len(trace.Contributions) < 2 {
		return nil
	}
	paramsByIndex := make(map[int][]*typ.TypeParam)
	for _, contribution := range trace.Contributions {
		if contribution.Param == nil || contribution.Type == nil {
			continue
		}
		if InferenceParamSetContains(paramsByIndex[contribution.Index], contribution.Param) {
			continue
		}
		paramsByIndex[contribution.Index] = append(paramsByIndex[contribution.Index], contribution.Param)
	}
	var out []GenericInferenceConflict
	indices := make([]int, 0, len(paramsByIndex))
	for index := range paramsByIndex {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		for _, param := range paramsByIndex[index] {
			contributions := inferenceConflictContributions(index, param, trace)
			if !InferenceTypeSetHasConflict(inferenceContributionTypes(contributions)) {
				continue
			}
			out = append(out, GenericInferenceConflict{
				Index:         index,
				ParamName:     InferenceParamName(param),
				Contributions: contributions,
			})
			break
		}
	}
	return out
}

func inferenceConflictContributions(index int, param *typ.TypeParam, trace GenericCallTrace) []InferenceContribution {
	seen := map[string]struct{}{}
	var out []InferenceContribution
	for _, contribution := range trace.Contributions {
		if contribution.Index != index || contribution.Param == nil || contribution.Type == nil {
			continue
		}
		if !SameInferenceParam(param, contribution.Param) {
			continue
		}
		key := InferenceContributionKey(contribution)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, contribution)
	}
	return out
}

func inferenceContributionTypes(contributions []InferenceContribution) []typ.Type {
	if len(contributions) == 0 {
		return nil
	}
	out := make([]typ.Type, 0, len(contributions))
	for _, contribution := range contributions {
		out = append(out, contribution.Type)
	}
	return out
}

func inferenceLiteralFamiliesCompatible(left, right typ.Type) bool {
	leftBase, leftOK := typelit.FamilyBase(left)
	rightBase, rightOK := typelit.FamilyBase(right)
	if !leftOK || !rightOK {
		return false
	}
	_, ok := typelit.MergeFamilyBases(leftBase, rightBase)
	return ok
}

func convertViolations(items []typecall.ArgumentConstraintViolation) []ArgumentConstraintViolation {
	if len(items) == 0 {
		return nil
	}
	out := make([]ArgumentConstraintViolation, 0, len(items))
	for _, item := range items {
		out = append(out, ArgumentConstraintViolation{
			Index:      item.Index,
			Got:        item.Got,
			Constraint: item.Constraint,
		})
	}
	return out
}

func convertTrace(trace typecall.GenericCallTrace) GenericCallTrace {
	if len(trace.Contributions) == 0 {
		return GenericCallTrace{}
	}
	out := make([]InferenceContribution, 0, len(trace.Contributions))
	for _, item := range trace.Contributions {
		out = append(out, InferenceContribution{
			Param: item.Param,
			Index: item.Index,
			Type:  item.Type,
			Path:  convertInferencePath(item.Path),
		})
	}
	return GenericCallTrace{Contributions: out}
}

func convertInferencePath(path []typecall.InferencePathStep) []InferencePathStep {
	if len(path) == 0 {
		return nil
	}
	out := make([]InferencePathStep, 0, len(path))
	for _, step := range path {
		out = append(out, InferencePathStep{
			Kind:  convertInferencePathKind(step.Kind),
			Name:  step.Name,
			Index: step.Index,
		})
	}
	return out
}

func convertInferencePathKind(kind typecall.InferencePathKind) InferencePathKind {
	switch kind {
	case typecall.InferencePathField:
		return InferencePathField
	case typecall.InferencePathStaticString:
		return InferencePathStaticString
	case typecall.InferencePathStaticInt:
		return InferencePathStaticInt
	case typecall.InferencePathTypeArgument:
		return InferencePathTypeArgument
	case typecall.InferencePathFunctionParam:
		return InferencePathFunctionParam
	case typecall.InferencePathFunctionReturn:
		return InferencePathFunctionReturn
	default:
		return 0
	}
}

func convertMemberCallStatus(status typecall.MemberCallStatus) MemberCallStatus {
	switch status {
	case typecall.MemberCallOK:
		return MemberCallOK
	case typecall.MemberCallMissing:
		return MemberCallMissing
	case typecall.MemberCallNotCallable:
		return MemberCallNotCallable
	default:
		return MemberCallMissing
	}
}

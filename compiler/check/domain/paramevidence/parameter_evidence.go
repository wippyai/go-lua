package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type JoinFn func(prev, next typ.Type) typ.Type

// MergeIntoSignature merges parameter evidence into a synthesized signature.
// Hard annotations stay authoritative. Soft structural annotations keep their
// container shape while evidence refines their element/value domain.
func MergeIntoSignature(fn *ast.FunctionExpr, evidence []typ.Type, sig *typ.Function) *typ.Function {
	if sig == nil || fn == nil || fn.ParList == nil {
		return sig
	}
	modified := false
	for i, p := range sig.Params {
		paramType, optional := mergeSignatureParam(fn, sig, evidence, i, p)
		if !typ.TypeEquals(p.Type, paramType) || p.Optional != optional {
			modified = true
		}
	}
	if !modified {
		return sig
	}

	builder := typ.Func()
	for i, p := range sig.Params {
		paramType, optional := mergeSignatureParam(fn, sig, evidence, i, p)
		if optional {
			builder = builder.OptParam(p.Name, paramType)
		} else {
			builder = builder.Param(p.Name, paramType)
		}
	}
	if sig.Variadic != nil {
		builder = builder.Variadic(sig.Variadic)
	}
	if len(sig.Returns) > 0 {
		builder = builder.Returns(sig.Returns...)
	}
	if sig.Effects != nil {
		builder = builder.Effects(sig.Effects)
	}
	if sig.Spec != nil {
		builder = builder.Spec(sig.Spec)
	}
	if sig.Refinement != nil {
		builder = builder.WithRefinement(sig.Refinement)
	}
	return builder.Build()
}

func mergeSignatureParam(fn *ast.FunctionExpr, sig *typ.Function, evidence []typ.Type, idx int, param typ.Param) (typ.Type, bool) {
	if idx >= len(evidence) || evidence[idx] == nil {
		return param.Type, param.Optional
	}
	srcIdx, hasSource := signatureSourceParamIndex(fn, sig, idx)
	if hasSource && srcIdx < len(fn.ParList.Types) && fn.ParList.Types[srcIdx] != nil {
		return RefineAnnotationWithEvidence(param.Type, evidence[idx]), param.Optional
	}
	if AnyLikeParam(param.Type) && PassiveOptionalRecordEvidence(evidence[idx]) {
		return param.Type, param.Optional
	}
	return MergeUnannotatedParam(param, evidence[idx])
}

// MergeUnannotatedParam merges public call-boundary evidence into an
// unannotated parameter. Literal values widen to their base domains because the
// resulting signature is a caller contract, not a body interpreter state.
func MergeUnannotatedParam(param typ.Param, evidence typ.Type) (typ.Type, bool) {
	return mergeUnannotatedParam(param, evidence, Join, false)
}

// MergeBodyUnannotatedParam merges body-effective evidence into an unannotated
// parameter. Structural literals are preserved so the abstract interpreter can
// use them as discriminants while checking the function body.
func MergeBodyUnannotatedParam(param typ.Param, evidence typ.Type) (typ.Type, bool) {
	return mergeUnannotatedParam(param, evidence, JoinBody, true)
}

// Concrete synthesized demands dominate stale nilable seeds: nil remains a valid
// call-boundary arity concern, but it must not poison the specialized body type
// once all observed/demanded uses require a non-nil value.
func mergeUnannotatedParam(param typ.Param, evidence typ.Type, join JoinFn, preserveEvidenceSubtype bool) (typ.Type, bool) {
	if evidence == nil {
		return param.Type, param.Optional
	}
	if join == nil {
		join = Join
	}
	if concreteParamTypeDominatesNilableEvidence(param.Type, evidence) {
		return param.Type, false
	}
	paramType := evidence
	optional := param.Optional
	if hasConcreteParamType(param.Type) {
		if preserveEvidenceSubtype {
			switch {
			case subtype.IsSubtype(evidence, param.Type):
				paramType = evidence
				if !unwrap.IsOptionalLike(paramType) {
					optional = false
				}
				return paramType, optional
			case subtype.IsSubtype(param.Type, evidence):
				return param.Type, false
			}
		}
		paramType = join(param.Type, evidence)
		if concreteParamTypeDominatesNilableEvidence(param.Type, paramType) {
			return param.Type, false
		}
	}
	if !unwrap.IsOptionalLike(paramType) {
		optional = false
	}
	return paramType, optional
}

func concreteParamTypeDominatesNilableEvidence(paramType, evidence typ.Type) bool {
	if !hasConcreteParamType(paramType) || evidence == nil {
		return false
	}
	inner, nilable := typ.SplitNilableFieldType(evidence)
	if !nilable || inner == nil {
		return false
	}
	return typ.TypeEquals(paramType, inner) || subtype.IsSubtype(paramType, inner)
}

func hasConcreteParamType(t typ.Type) bool {
	return t != nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!t.Kind().IsPlaceholder() &&
		!unwrap.IsOptionalLike(t)
}

// PassiveOptionalRecordEvidence is field-read/default evidence that is useful
// inside the function body but is not a hard public precondition by itself.
func PassiveOptionalRecordEvidence(t typ.Type) bool {
	inner := unwrap.Optional(t)
	rec := unwrap.Record(inner)
	if rec == nil || len(rec.Fields) == 0 || rec.HasMapComponent() {
		return false
	}
	for _, field := range rec.Fields {
		if !field.Optional {
			return false
		}
	}
	return true
}

// AnyLikeParam reports whether a parameter slot is the unannotated gradual top,
// including the arity nilability wrapper used for Lua's optional arguments.
func AnyLikeParam(t typ.Type) bool {
	if typ.IsAny(t) {
		return true
	}
	inner := unwrap.Optional(t)
	return inner != t && typ.IsAny(inner)
}

// RefineAnnotationWithEvidence returns the function-body type produced when a
// soft structural annotation receives harder evidence. Hard annotations and top
// annotations (`any`, `unknown`) remain authoritative.
func RefineAnnotationWithEvidence(annotation, evidence typ.Type) typ.Type {
	if annotation == nil || evidence == nil || !typ.IsRefinableAnnotation(annotation) {
		return annotation
	}
	evidence = NormalizeType(evidence)
	if !IsInformative(evidence) {
		return annotation
	}
	if refined, changed := value.RefineStructuralAnnotation(annotation, evidence, Join); changed {
		return refined
	}
	return annotation
}

func signatureSourceParamIndex(fn *ast.FunctionExpr, sig *typ.Function, paramIdx int) (int, bool) {
	if fn == nil || fn.ParList == nil || sig == nil || paramIdx < 0 || paramIdx >= len(sig.Params) {
		return 0, false
	}
	if signatureHasImplicitSelf(fn, sig) {
		if paramIdx == 0 {
			return 0, false
		}
		srcIdx := paramIdx - 1
		return srcIdx, srcIdx >= 0 && srcIdx < len(fn.ParList.Names)
	}
	return paramIdx, paramIdx < len(fn.ParList.Names)
}

func signatureHasImplicitSelf(fn *ast.FunctionExpr, sig *typ.Function) bool {
	if fn == nil || fn.ParList == nil || sig == nil || len(sig.Params) == 0 {
		return false
	}
	if sig.Params[0].Name != "self" {
		return false
	}
	if len(fn.ParList.Names) > 0 && fn.ParList.Names[0] == "self" {
		return false
	}
	return len(sig.Params) == len(fn.ParList.Names)+1
}

func WidenType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Literal:
		switch v.Base {
		case kind.Boolean:
			return typ.Boolean
		case kind.Integer:
			return typ.Integer
		case kind.Number:
			return typ.Number
		case kind.String:
			return typ.String
		}
	case *typ.Optional:
		inner := WidenType(v.Inner)
		if inner != v.Inner && inner != nil {
			return typ.NewOptional(inner)
		}
	case *typ.Alias:
		if v.Target != nil {
			return WidenType(v.Target)
		}
	case *typ.Union:
		changed := false
		members := make([]typ.Type, 0, len(v.Members))
		for _, m := range v.Members {
			wm := WidenType(m)
			if wm != m {
				changed = true
			}
			members = append(members, wm)
		}
		if changed {
			return typ.NewUnion(members...)
		}
	case *typ.Array:
		elem := WidenType(v.Element)
		if elem != v.Element {
			return typ.NewArray(elem)
		}
	case *typ.Tuple:
		changed := false
		elements := make([]typ.Type, len(v.Elements))
		for i, elem := range v.Elements {
			we := WidenType(elem)
			if we != elem {
				changed = true
			}
			elements[i] = we
		}
		if changed {
			return typ.NewTuple(elements...)
		}
	case *typ.Map:
		key := WidenType(v.Key)
		elem := WidenType(v.Value)
		if key != v.Key || elem != v.Value {
			return typ.NewMap(key, elem)
		}
	case *typ.ReadonlyMap:
		key := WidenType(v.Key)
		elem := WidenType(v.Value)
		if key != v.Key || elem != v.Value {
			return typ.NewReadonlyMap(key, elem)
		}
	case *typ.Record:
		builder := typ.NewRecord()
		changed := false
		if v.Open {
			builder.SetOpen(true)
		}
		for _, f := range v.Fields {
			ft := WidenType(f.Type)
			if ft != f.Type {
				changed = true
			}
			if f.Optional {
				builder.OptField(f.Name, ft)
			} else {
				builder.Field(f.Name, ft)
			}
		}
		if v.MapKey != nil && v.MapValue != nil {
			k := WidenType(v.MapKey)
			val := WidenType(v.MapValue)
			if k != v.MapKey || val != v.MapValue {
				changed = true
			}
			builder.MapComponent(k, val)
		}
		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if changed {
			return builder.Build()
		}
	}
	return t
}

type normalizer func(typ.Type) typ.Type

// NormalizeType applies public call-boundary canonicalization. Literal values
// widen to their base types so exported function facts describe contracts
// rather than the current finite set of call-site constants.
func NormalizeType(t typ.Type) typ.Type {
	return normalizeType(t, WidenType, Join)
}

// NormalizeBodyType applies body-effective canonicalization. Structural
// literals are retained because the abstract interpreter uses them as
// discriminants for path-sensitive branch proof inside the callee body.
func NormalizeBodyType(t typ.Type) typ.Type {
	return normalizeType(t, WidenBodyType, JoinBody)
}

func WidenBodyType(t typ.Type) typ.Type {
	return value.AdmitObservation(t)
}

func normalizeType(t typ.Type, widen normalizer, join JoinFn) typ.Type {
	if widen != nil {
		t = widen(t)
	}
	t = value.CollapseSequenceUnion(t, join)
	t = value.CollapseStructuralUnionShape(t, join)
	return value.CollapseTableTopEvidence(typ.PruneSoftUnionMembers(t))
}

// EnsureCapacity grows evidence vector to at least size.
func EnsureCapacity(evidence []typ.Type, size int) []typ.Type {
	if size <= len(evidence) {
		return evidence
	}
	expanded := make([]typ.Type, size)
	copy(expanded, evidence)
	return expanded
}

// MergeAt normalizes and joins one observation into vector slot idx.
func MergeAt(vec []typ.Type, idx int, observed typ.Type, join JoinFn) ([]typ.Type, bool) {
	return mergeAt(vec, idx, observed, join, NormalizeType)
}

// MergeBodyAt merges one observation into body-effective parameter evidence.
func MergeBodyAt(vec []typ.Type, idx int, observed typ.Type, join JoinFn) ([]typ.Type, bool) {
	return mergeAt(vec, idx, observed, join, NormalizeBodyType)
}

func mergeAt(vec []typ.Type, idx int, observed typ.Type, join JoinFn, normalize normalizer) ([]typ.Type, bool) {
	if idx < 0 {
		return vec, false
	}
	if normalize == nil {
		normalize = NormalizeType
	}
	observed = normalize(observed)
	if !IsInformative(observed) {
		return vec, false
	}
	vec = EnsureCapacity(vec, idx+1)

	joinFn := join
	if joinFn == nil {
		joinFn = typ.JoinPreferNonSoft
	}
	prev := vec[idx]
	merged := joinFn(prev, observed)
	merged = normalize(merged)
	if typ.TypeEquals(prev, merged) {
		return vec, false
	}
	vec[idx] = merged
	return vec, true
}

// MergeCallArgAt merges a call-argument observation into a parameter evidence
// slot. Unlike MergeAt, unresolved/top-like argument observations are
// preserved as uncertainty evidence so later literal calls cannot over-specialize
// unannotated parameters.
func MergeCallArgAt(evidence []typ.Type, idx int, argType typ.Type, join JoinFn, unknownOnNil bool) ([]typ.Type, bool) {
	return mergeCallArgAt(evidence, idx, argType, unknownOnNil, NormalizeType, Join, JoinCall, join)
}

// MergeBodyCallArgAt merges a call-argument observation into body-effective
// parameter evidence. Unlike public call-boundary evidence, structural literal
// discriminants remain available to the callee's abstract interpreter.
func MergeBodyCallArgAt(evidence []typ.Type, idx int, argType typ.Type, join JoinFn, unknownOnNil bool) ([]typ.Type, bool) {
	return mergeCallArgAt(evidence, idx, argType, unknownOnNil, NormalizeBodyType, JoinBody, JoinBody, JoinBody)
}

func mergeCallArgAt(
	evidence []typ.Type,
	idx int,
	argType typ.Type,
	unknownOnNil bool,
	normalize normalizer,
	recursiveJoin JoinFn,
	topJoin JoinFn,
	slotJoin JoinFn,
) ([]typ.Type, bool) {
	if idx < 0 {
		return evidence, false
	}
	if normalize == nil {
		normalize = NormalizeType
	}
	if recursiveJoin == nil {
		recursiveJoin = Join
	}
	if topJoin == nil {
		topJoin = JoinCall
	}
	if slotJoin == nil {
		slotJoin = typ.JoinPreferNonSoft
	}
	argType = normalize(argType)
	if argType == nil {
		if !unknownOnNil {
			return evidence, false
		}
		argType = typ.Unknown
	}
	evidence = EnsureCapacity(evidence, idx+1)

	prev := normalize(evidence[idx])
	if prev == nil {
		prev = evidence[idx]
	}

	topLikeArg := typ.IsAny(argType) || typ.IsUnknown(argType)
	nilArg := unwrap.IsNilType(argType)
	if !topLikeArg && !nilArg && !IsInformative(argType) {
		return evidence, false
	}

	var merged typ.Type
	switch {
	case nilArg && prev != nil && !unwrap.IsNilType(prev):
		merged = typ.NewOptional(prev)
	case unwrap.IsNilType(prev) && !nilArg:
		merged = typ.NewOptional(argType)
	default:
		merged = topJoin(prev, argType)
	}
	if !topLikeArg && !nilArg && !typ.IsUnknown(prev) && !typ.IsAny(prev) && !unwrap.IsNilType(prev) {
		if seq, ok := value.JoinSequenceShape(prev, argType, recursiveJoin); ok {
			merged = seq
		} else if joined, ok := value.JoinRecordShape(prev, argType, recursiveJoin); ok {
			merged = joined
		} else if joined, ok := value.JoinMapRecordShape(prev, argType, recursiveJoin); ok {
			merged = joined
		} else if joined, ok := value.JoinStructuralUnionShape(prev, argType, recursiveJoin); ok {
			merged = joined
		} else {
			merged = slotJoin(prev, argType)
		}
	}
	merged = normalize(merged)
	if typ.TypeEquals(evidence[idx], merged) {
		return evidence, false
	}
	evidence[idx] = merged
	return evidence, true
}

// IsInformative reports whether a type carries useful call-site
// information for parameter evidence propagation.
//
// It intentionally rejects top-like and empty placeholder shapes that tend to
// poison evidence, while preserving structured evidence such as maps/arrays with
// partial information (for example `{[string]: any[]}`).
func IsInformative(t typ.Type) bool {
	return isInformativeEvidenceType(t, typ.NewGuard())
}

func isInformativeEvidenceType(t typ.Type, guard internal.RecursionGuard) bool {
	if t == nil {
		return false
	}
	next, ok := guard.Enter(t)
	if !ok {
		return false
	}

	if t.Kind().IsDeferred() {
		return false
	}

	k := t.Kind()
	if k.IsPlaceholder() || k == kind.Nil || k == kind.Never {
		return false
	}

	switch v := t.(type) {
	case *typ.Optional:
		return isInformativeEvidenceType(v.Inner, next)
	case *typ.Union:
		for _, m := range v.Members {
			if isInformativeEvidenceType(m, next) {
				return true
			}
		}
		return false
	case *typ.Alias:
		if v.Target == nil {
			return false
		}
		return isInformativeEvidenceType(v.Target, next)
	}

	if r, ok := t.(*typ.Record); ok {
		if len(r.Fields) == 0 && !r.HasMapComponent() && !r.Open {
			return false
		}
	}

	return true
}

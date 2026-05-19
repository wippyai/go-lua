package paramevidence

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/internal"
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
		if i >= len(evidence) || evidence[i] == nil {
			continue
		}
		paramType := p.Type
		optional := p.Optional
		if srcIdx, hasSource := signatureSourceParamIndex(fn, sig, i); hasSource && srcIdx < len(fn.ParList.Types) && fn.ParList.Types[srcIdx] != nil {
			paramType = RefineAnnotationWithEvidence(p.Type, evidence[i])
		} else {
			paramType = evidence[i]
			if !unwrap.IsOptionalLike(evidence[i]) {
				optional = false
			}
		}
		if !typ.TypeEquals(p.Type, paramType) || p.Optional != optional {
			modified = true
		}
	}
	if !modified {
		return sig
	}

	builder := typ.Func()
	for i, p := range sig.Params {
		paramType := p.Type
		optional := p.Optional
		if i < len(evidence) && evidence[i] != nil {
			srcIdx, hasSource := signatureSourceParamIndex(fn, sig, i)
			annotated := hasSource && srcIdx < len(fn.ParList.Types) && fn.ParList.Types[srcIdx] != nil
			if annotated {
				paramType = RefineAnnotationWithEvidence(p.Type, evidence[i])
			} else {
				paramType = evidence[i]
				if !unwrap.IsOptionalLike(evidence[i]) {
					optional = false
				}
			}
		}
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
	if refined, changed := refineAnnotationShape(annotation, evidence, typ.NewGuard()); changed {
		return refined
	}
	return annotation
}

func refineAnnotationShape(annotation, evidence typ.Type, guard internal.RecursionGuard) (typ.Type, bool) {
	if annotation == nil || evidence == nil {
		return annotation, false
	}
	next, ok := guard.Enter(annotation)
	if !ok {
		return annotation, false
	}
	switch a := unwrap.Alias(annotation).(type) {
	case *typ.Optional:
		inner, changed := refineAnnotationShape(a.Inner, evidence, next)
		if !changed {
			return annotation, false
		}
		return typ.NewOptional(inner), true
	case *typ.Array:
		elem := arrayElementEvidence(evidence, next)
		if elem == nil {
			return annotation, false
		}
		refined := Join(a.Element, elem)
		if typ.TypeEquals(refined, a.Element) {
			return annotation, false
		}
		return typ.NewArray(refined), true
	case *typ.Map:
		key, value, ok := mapEvidence(evidence, a.Key, next)
		if !ok {
			return annotation, false
		}
		refinedKey := a.Key
		if typ.IsAny(a.Key) || typ.IsUnknown(a.Key) {
			refinedKey = Join(a.Key, key)
		}
		refinedValue := Join(a.Value, value)
		if typ.TypeEquals(refinedKey, a.Key) && typ.TypeEquals(refinedValue, a.Value) {
			return annotation, false
		}
		return typ.NewMap(refinedKey, refinedValue), true
	case *typ.Record:
		return refineRecordAnnotation(a, evidence, next)
	case *typ.Union:
		members := make([]typ.Type, len(a.Members))
		changed := false
		for i, member := range a.Members {
			refined, memberChanged := refineAnnotationShape(member, evidence, next)
			members[i] = refined
			changed = changed || memberChanged
		}
		if !changed {
			return annotation, false
		}
		return typ.NewUnion(members...), true
	default:
		return annotation, false
	}
}

func refineRecordAnnotation(annotation *typ.Record, evidence typ.Type, guard internal.RecursionGuard) (typ.Type, bool) {
	if annotation == nil {
		return nil, false
	}
	if annotation.Open && len(annotation.Fields) == 0 && !annotation.HasMapComponent() && annotation.Metatable == nil {
		return evidence, !typ.TypeEquals(annotation, evidence)
	}
	if !annotation.HasMapComponent() {
		return annotation, false
	}
	key, value, ok := mapEvidence(evidence, annotation.MapKey, guard)
	if !ok {
		return annotation, false
	}
	refinedKey := annotation.MapKey
	if typ.IsAny(annotation.MapKey) || typ.IsUnknown(annotation.MapKey) {
		refinedKey = Join(annotation.MapKey, key)
	}
	refinedValue := Join(annotation.MapValue, value)
	if typ.TypeEquals(refinedKey, annotation.MapKey) && typ.TypeEquals(refinedValue, annotation.MapValue) {
		return annotation, false
	}
	builder := typ.NewRecord().SetOpen(annotation.Open)
	if annotation.Metatable != nil {
		builder.Metatable(annotation.Metatable)
	}
	builder.MapComponent(refinedKey, refinedValue)
	for _, field := range annotation.Fields {
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
			builder.Field(field.Name, field.Type)
		}
	}
	return builder.Build(), true
}

func arrayElementEvidence(evidence typ.Type, guard internal.RecursionGuard) typ.Type {
	if evidence == nil {
		return nil
	}
	next, ok := guard.Enter(evidence)
	if !ok {
		return nil
	}
	switch e := unwrap.Alias(evidence).(type) {
	case *typ.Array:
		return e.Element
	case *typ.Tuple:
		var elem typ.Type
		for _, slot := range e.Elements {
			elem = Join(elem, slot)
		}
		return elem
	case *typ.Union:
		var elem typ.Type
		for _, member := range e.Members {
			memberElem := arrayElementEvidence(member, next)
			if memberElem == nil {
				continue
			}
			elem = Join(elem, memberElem)
		}
		return elem
	case *typ.Optional:
		return arrayElementEvidence(e.Inner, next)
	default:
		return nil
	}
}

func mapEvidence(evidence, expectedKey typ.Type, guard internal.RecursionGuard) (typ.Type, typ.Type, bool) {
	if evidence == nil {
		return nil, nil, false
	}
	next, ok := guard.Enter(evidence)
	if !ok {
		return nil, nil, false
	}
	switch e := unwrap.Alias(evidence).(type) {
	case *typ.Map:
		if expectedKey != nil && !keyEvidenceCompatible(e.Key, expectedKey) {
			return nil, nil, false
		}
		return e.Key, e.Value, true
	case *typ.Record:
		if e.HasMapComponent() {
			if expectedKey != nil && !keyEvidenceCompatible(e.MapKey, expectedKey) {
				return nil, nil, false
			}
			return e.MapKey, e.MapValue, true
		}
		if len(e.Fields) == 0 {
			return nil, nil, false
		}
		var value typ.Type
		for _, field := range e.Fields {
			if expectedKey != nil && !keyEvidenceCompatible(typ.LiteralString(field.Name), expectedKey) {
				return nil, nil, false
			}
			value = Join(value, field.Type)
		}
		return typ.String, value, true
	case *typ.Union:
		var key typ.Type
		var value typ.Type
		seen := false
		for _, member := range e.Members {
			memberKey, memberValue, ok := mapEvidence(member, expectedKey, next)
			if !ok {
				continue
			}
			key = Join(key, memberKey)
			value = Join(value, memberValue)
			seen = true
		}
		return key, value, seen
	case *typ.Optional:
		return mapEvidence(e.Inner, expectedKey, next)
	default:
		return nil, nil, false
	}
}

func keyEvidenceCompatible(candidate, expected typ.Type) bool {
	if candidate == nil || expected == nil {
		return false
	}
	return subtype.IsSubtype(candidate, expected) || typ.IsAny(expected) || typ.IsUnknown(expected)
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

// NormalizeType applies canonical widening and soft-member pruning.
func NormalizeType(t typ.Type) typ.Type {
	return collapseTableTopEvidence(typ.PruneSoftUnionMembers(WidenType(t)))
}

func collapseTableTopEvidence(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Alias:
		target := collapseTableTopEvidence(v.Target)
		if target != nil && !typ.TypeEquals(target, v.Target) {
			return typ.NewAlias(v.Name, target)
		}
		return t
	case *typ.Optional:
		inner := collapseTableTopEvidence(v.Inner)
		if inner != nil && !typ.TypeEquals(inner, v.Inner) {
			return typ.NewOptional(inner)
		}
		return t
	case *typ.Union:
		return collapseTableTopUnion(v)
	default:
		return t
	}
}

func collapseTableTopUnion(u *typ.Union) typ.Type {
	if u == nil {
		return nil
	}
	tableTop := firstTableTopMember(u.Members)
	members := make([]typ.Type, 0, len(u.Members))
	changed := false

	if tableTop == nil {
		for _, member := range u.Members {
			collapsed := collapseTableTopEvidence(member)
			if !typ.TypeEquals(collapsed, member) {
				changed = true
			}
			members = append(members, collapsed)
		}
		if !changed {
			return u
		}
		return typ.NewUnion(members...)
	}

	tableAdded := false
	for _, member := range u.Members {
		if member == nil {
			continue
		}
		if typ.UnwrapAnnotated(member).Kind() == kind.Nil {
			members = append(members, member)
			continue
		}
		collapsed := collapseTableTopEvidence(member)
		if tableTopCoversEvidenceMember(collapsed) {
			if !tableAdded {
				members = append(members, tableTop)
				tableAdded = true
			}
			if !typ.TypeEquals(member, tableTop) {
				changed = true
			}
			continue
		}
		if !typ.TypeEquals(collapsed, member) {
			changed = true
		}
		members = append(members, collapsed)
	}
	if !changed {
		return u
	}
	return typ.NewUnion(members...)
}

func firstTableTopMember(members []typ.Type) typ.Type {
	for _, member := range members {
		if isBuiltinTableTopEvidence(member) {
			return member
		}
	}
	return nil
}

func isBuiltinTableTopEvidence(t typ.Type) bool {
	return unwrap.IsBuiltinTableTop(typ.UnwrapAnnotated(t))
}

func tableTopCoversEvidenceMember(t typ.Type) bool {
	if t == nil {
		return false
	}
	if isBuiltinTableTopEvidence(t) {
		return true
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return tableTopCoversEvidenceMember(v.UnaliasedTarget())
	case *typ.Recursive:
		return v.Body != nil && v.Body != v && tableTopCoversEvidenceMember(v.Body)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if member == nil || typ.UnwrapAnnotated(member).Kind() == kind.Nil || !tableTopCoversEvidenceMember(member) {
				return false
			}
		}
		return true
	case *typ.Record, *typ.Map, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Intersection:
		return true
	default:
		return false
	}
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
	if idx < 0 {
		return vec, false
	}
	observed = NormalizeType(observed)
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
	if idx < 0 {
		return evidence, false
	}
	argType = NormalizeType(argType)
	if argType == nil {
		if !unknownOnNil {
			return evidence, false
		}
		argType = typ.Unknown
	}
	evidence = EnsureCapacity(evidence, idx+1)

	joinFn := join
	if joinFn == nil {
		joinFn = typ.JoinPreferNonSoft
	}

	prev := NormalizeType(evidence[idx])
	if prev == nil {
		prev = evidence[idx]
	}

	mergeTopAware := func(a, b typ.Type) typ.Type {
		if a == nil {
			return b
		}
		if b == nil {
			return a
		}
		if typ.IsAny(a) || typ.IsAny(b) {
			return typ.Any
		}
		if typ.IsUnknown(a) {
			return b
		}
		if typ.IsUnknown(b) {
			return a
		}
		return joinFn(a, b)
	}

	topLikeArg := typ.IsAny(argType) || typ.IsUnknown(argType)
	nilArg := unwrap.IsNilType(argType)
	if !topLikeArg && !nilArg && !IsInformative(argType) {
		return evidence, false
	}

	merged := mergeTopAware(prev, argType)
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

// BuildSignatureMap builds a function-expression keyed parameter evidence
// map for this graph from canonical FunctionFacts.
func BuildSignatureMap(
	store api.StoreReader,
	graph *cfg.Graph,
	parent *scope.State,
	stdlib *scope.State,
) map[*ast.FunctionExpr][]typ.Type {
	if store == nil || graph == nil || parent == nil {
		return nil
	}

	functionFacts := store.GetFunctionFactsSnapshot(graph, parent)

	out := make(map[*ast.FunctionExpr][]typ.Type)

	if len(functionFacts) > 0 {
		for _, sym := range cfg.SortedSymbolIDs(functionFacts) {
			vec := functionFacts.Params(sym)
			if len(vec) == 0 {
				continue
			}
			hasEvidence := false
			for _, observed := range vec {
				if observed != nil {
					hasEvidence = true
					break
				}
			}
			if !hasEvidence {
				continue
			}
			fn := store.FuncForSymbol(sym)
			if fn == nil {
				continue
			}
			if _, exists := out[fn]; !exists {
				out[fn] = vec
			}
		}
	}

	// If this graph is a nested function, pull parameter evidence from the
	// parent graph and apply it to the current function signature.
	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			defaultScope := (*scope.State)(nil)
			if _, isNestedParent := store.NestedMetaFor(parentGraph.ID()); !isNestedParent {
				defaultScope = stdlib
			}
			parentScope := api.ParentScopeForGraph(store, parentGraph.ID(), defaultScope)
			if parentScope != nil {
				parentFacts := store.GetFunctionFactsSnapshot(parentGraph, parentScope)
				if len(parentFacts) > 0 {
					fn := store.FuncForGraph(graph)
					if fn == nil {
						fn = graph.Func()
					}
					if fn != nil {
						if sym, ok := store.SymbolForFunc(fn); ok {
							if evidence := parentFacts.Params(sym); len(evidence) > 0 {
								out[fn] = evidence
							}
						}
					}
				}
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

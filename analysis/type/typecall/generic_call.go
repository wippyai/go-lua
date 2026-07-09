package typecall

import (
	typelit "github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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

// GenericCallTrace carries diagnostic-only generic inference provenance.
type GenericCallTrace struct {
	Contributions []InferenceContribution
}

// TypeParamBinding records one inferred type argument for a function type
// parameter. The Param pointer is the canonical binder from the original
// generic function signature, so callers can substitute type-bearing sidecars
// without matching by name.
type TypeParamBinding struct {
	Param *typ.TypeParam
	Type  typ.Type
	Index int
}

// InstantiateGenericCall infers type arguments for a generic function call from
// concrete argument types, validates type-parameter constraints, and returns the
// callable signature with inferred type parameters substituted.
func InstantiateGenericCall(fn *typ.Function, args []typ.Type) (*typ.Function, []ArgumentConstraintViolation) {
	instantiated, violations, _, _ := instantiateGenericCall(fn, args, false)
	return instantiated, violations
}

// InstantiateGenericCallWithBindings is the boundary-facing variant used when
// non-type signature payloads, such as operational effects, must be substituted
// with the exact same inferred type arguments as the callable type.
func InstantiateGenericCallWithBindings(fn *typ.Function, args []typ.Type) (*typ.Function, []ArgumentConstraintViolation, []TypeParamBinding) {
	instantiated, violations, _, bindings := instantiateGenericCall(fn, args, false)
	return instantiated, violations, typeParamBindings(fn, bindings)
}

// InstantiateGenericCallWithTrace is the diagnostics-facing variant of
// InstantiateGenericCall. It returns the same instantiated signature and
// violations, plus the concrete argument locations that contributed to each
// inferred type parameter.
func InstantiateGenericCallWithTrace(fn *typ.Function, args []typ.Type) (*typ.Function, []ArgumentConstraintViolation, GenericCallTrace) {
	instantiated, violations, trace, _ := instantiateGenericCall(fn, args, true)
	return instantiated, violations, trace
}

func instantiateGenericCall(fn *typ.Function, args []typ.Type, trace bool) (*typ.Function, []ArgumentConstraintViolation, GenericCallTrace, map[*typ.TypeParam]inferredArg) {
	if fn == nil || len(fn.TypeParams) == 0 {
		return fn, nil, GenericCallTrace{}, nil
	}
	bindings := make(map[*typ.TypeParam]inferredArg, len(fn.TypeParams))
	for i, arg := range args {
		if arg == nil {
			continue
		}
		formal, ok := callParamType(fn, i)
		if !ok || formal == nil {
			continue
		}
		inferTypeParamBindings(formal, arg, i, bindings, nil, trace, 0)
	}

	params := make([]*typ.TypeParam, 0, len(fn.TypeParams))
	argsToSubstitute := make([]typ.Type, 0, len(fn.TypeParams))
	for _, param := range fn.TypeParams {
		if param == nil {
			continue
		}
		if binding, ok := bindings[param]; ok {
			params = append(params, param)
			argsToSubstitute = append(argsToSubstitute, binding.typ)
		}
	}

	var violations []ArgumentConstraintViolation
	for _, param := range fn.TypeParams {
		if param == nil || param.Constraint == nil {
			continue
		}
		binding, ok := bindings[param]
		if !ok || binding.typ == nil {
			continue
		}
		if !subtype.IsSubtype(binding.typ, param.Constraint) {
			violations = append(violations, ArgumentConstraintViolation{
				Index:      binding.index,
				Got:        binding.typ,
				Constraint: param.Constraint,
			})
		}
	}

	instantiated, ok := subst.Params(fn, params, argsToSubstitute).(*typ.Function)
	if !ok {
		return fn, violations, genericCallTraceFromBindings(fn.TypeParams, bindings), bindings
	}
	violations = append(violations, validateInstantiatedArguments(instantiated, args, params)...)
	return instantiated, violations, genericCallTraceFromBindings(fn.TypeParams, bindings), bindings
}

type inferredArg struct {
	typ           typ.Type
	index         int
	contributions []InferenceContribution
}

func callParamType(fn *typ.Function, index int) (typ.Type, bool) {
	if fn == nil || index < 0 {
		return nil, false
	}
	if index < len(fn.Params) {
		return fn.Params[index].Type, true
	}
	if fn.Variadic != nil {
		return fn.Variadic, true
	}
	return nil, false
}

func validateInstantiatedArguments(fn *typ.Function, args []typ.Type, substitutedParams []*typ.TypeParam) []ArgumentConstraintViolation {
	if fn == nil || len(args) == 0 {
		return nil
	}
	var violations []ArgumentConstraintViolation
	for i, actual := range args {
		if actual == nil {
			continue
		}
		if refinement.ContainsFreeTypeParam(actual) {
			continue
		}
		formal, ok := callParamType(fn, i)
		if !ok || formal == nil {
			continue
		}
		if containsUnsubstitutedTypeParam(formal, substitutedParams) {
			violations = append(violations, ArgumentConstraintViolation{
				Index:      i,
				Got:        actual,
				Constraint: formal,
			})
			continue
		}
		if refinement.ContainsFreeTypeParam(formal) {
			continue
		}
		if instantiatedArgumentAssignable(actual, formal, 0) {
			continue
		}
		violations = append(violations, ArgumentConstraintViolation{
			Index:      i,
			Got:        actual,
			Constraint: formal,
		})
	}
	return violations
}

func containsUnsubstitutedTypeParam(t typ.Type, params []*typ.TypeParam) bool {
	for _, param := range params {
		if param != nil && containsTypeParamOutsideShadow(t, param, false, nil) {
			return true
		}
	}
	return false
}

func containsTypeParamOutsideShadow(t typ.Type, target *typ.TypeParam, shadowed bool, active map[typ.Type]bool) bool {
	if t == nil || target == nil {
		return false
	}
	if param, ok := t.(*typ.TypeParam); ok {
		return !shadowed && param == target
	}
	if active == nil {
		active = make(map[typ.Type]bool)
	}
	if active[t] {
		return false
	}
	active[t] = true
	defer delete(active, t)

	switch node := t.(type) {
	case *typ.Instantiated:
		for _, arg := range node.TypeArgs {
			if containsTypeParamOutsideShadow(arg, target, shadowed, active) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, binder := range node.TypeParams {
			if binder != nil && (binder == target || binder.Name == target.Name) {
				shadowed = true
				break
			}
		}
	case *typ.Generic:
		for _, binder := range node.TypeParams {
			if binder != nil && (binder == target || binder.Name == target.Name) {
				shadowed = true
				break
			}
		}
	}
	return typ.WalkChildren(t, func(child typ.Type) bool {
		return containsTypeParamOutsideShadow(child, target, shadowed, active)
	})
}

// InstantiatedArgumentAssignable reports whether an actual argument type
// satisfies an already-instantiated formal parameter type using the same
// precision rules as generic-call validation.
func InstantiatedArgumentAssignable(actual typ.Type, formal typ.Type) bool {
	return instantiatedArgumentAssignable(actual, formal, 0)
}

func instantiatedArgumentAssignable(actual typ.Type, formal typ.Type, depth int) bool {
	if actual == nil || formal == nil {
		return true
	}
	if depth > typ.DefaultRecursionDepth {
		return false
	}
	actual = unwrap.Annotated(actual)
	formal = unwrap.Annotated(formal)
	if formal == nil || refinement.ContainsFreeTypeParam(formal) {
		return true
	}

	if actualInst, ok := actual.(*typ.Instantiated); ok {
		if formalInst, ok := formal.(*typ.Instantiated); ok && sameGeneric(actualInst.Generic, formalInst.Generic) {
			return instantiatedArgsAssignable(actualInst, formalInst, depth+1)
		}
	}

	switch f := formal.(type) {
	case *typ.Alias:
		return instantiatedArgumentAssignable(actual, f.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		if a, ok := actual.(*typ.Optional); ok {
			return instantiatedArgumentAssignable(a.Inner, f.Inner, depth+1)
		}
		return instantiatedArgumentAssignable(actual, f.Inner, depth+1)
	case *typ.Instantiated:
		expanded := expandFormalInstantiatedForInference(f)
		if expanded != nil && expanded != formal {
			return instantiatedArgumentAssignable(actual, expanded, depth+1)
		}
	case *typ.Record:
		if actualRecord, ok := actualRecordForValidation(actual, depth+1); ok {
			return providedRecordFieldsAssignable(actualRecord, f, depth+1)
		}
	}
	if underSpecifiedFunctionLiteral(actual, formal) {
		return true
	}

	return subtype.IsFreshAssignable(actual, formal)
}

func underSpecifiedFunctionLiteral(actual typ.Type, formal typ.Type) bool {
	actualFn, actualOK := actual.(*typ.Function)
	formalFn, formalOK := formal.(*typ.Function)
	if !actualOK || !formalOK || actualFn == nil || formalFn == nil {
		return false
	}
	return len(actualFn.Returns) == 0 && len(formalFn.Returns) != 0
}

func instantiatedArgsAssignable(actual *typ.Instantiated, formal *typ.Instantiated, depth int) bool {
	if actual == nil || formal == nil || len(actual.TypeArgs) != len(formal.TypeArgs) {
		return false
	}
	for i, actualArg := range actual.TypeArgs {
		formalArg := formal.TypeArgs[i]
		if subtype.IsSubtype(actualArg, formalArg) && subtype.IsSubtype(formalArg, actualArg) {
			continue
		}
		if freshPrecisionArgumentAssignable(actualArg, formalArg, depth+1) {
			continue
		}
		return false
	}
	return true
}

func freshPrecisionArgumentAssignable(actual typ.Type, formal typ.Type, depth int) bool {
	if actual == nil || formal == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	if !subtype.IsFreshAssignable(actual, formal) {
		return false
	}
	return hasFreshPrecisionShape(actual, formal, depth+1)
}

func hasFreshPrecisionShape(actual typ.Type, formal typ.Type, depth int) bool {
	if actual == nil || formal == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	actual = unwrap.Annotated(actual)
	formal = unwrap.Annotated(formal)
	if alias, ok := actual.(*typ.Alias); ok {
		return hasFreshPrecisionShape(alias.UnaliasedTarget(), formal, depth+1)
	}
	if alias, ok := formal.(*typ.Alias); ok {
		return hasFreshPrecisionShape(actual, alias.UnaliasedTarget(), depth+1)
	}
	if subtype.IsSubtype(actual, formal) && subtype.IsSubtype(formal, actual) {
		return false
	}
	if _, ok := actual.(*typ.Literal); ok {
		return true
	}
	if opt, ok := actual.(*typ.Optional); ok {
		if formalOpt, ok := formal.(*typ.Optional); ok {
			return hasFreshPrecisionShape(opt.Inner, formalOpt.Inner, depth+1)
		}
		return hasFreshPrecisionShape(opt.Inner, formal, depth+1)
	}
	if opt, ok := formal.(*typ.Optional); ok {
		return hasFreshPrecisionShape(actual, opt.Inner, depth+1)
	}
	if union, ok := actual.(*typ.Union); ok {
		for _, member := range union.Members {
			if hasFreshPrecisionShape(member, formal, depth+1) {
				return true
			}
		}
		return false
	}
	if union, ok := formal.(*typ.Union); ok {
		for _, member := range union.Members {
			if subtype.IsFreshAssignable(actual, member) && hasFreshPrecisionShape(actual, member, depth+1) {
				return true
			}
		}
		return false
	}
	if actualInst, ok := actual.(*typ.Instantiated); ok {
		if formalInst, ok := formal.(*typ.Instantiated); ok && sameGeneric(actualInst.Generic, formalInst.Generic) {
			for i, actualArg := range actualInst.TypeArgs {
				if i >= len(formalInst.TypeArgs) {
					return false
				}
				if hasFreshPrecisionShape(actualArg, formalInst.TypeArgs[i], depth+1) {
					return true
				}
			}
			return false
		}
	}
	actualRecord, actualOK := actualRecordForValidation(actual, depth+1)
	formalRecord, formalOK := actualRecordForValidation(formal, depth+1)
	if actualOK && formalOK {
		for _, field := range actualRecord.Fields {
			formalField := formalRecord.GetField(field.Name)
			if formalField == nil || formalField.Type == nil || field.Type == nil {
				continue
			}
			if hasFreshPrecisionShape(field.Type, formalField.Type, depth+1) {
				return true
			}
		}
		for _, member := range actualRecord.StaticMembers {
			formalMember := formalRecord.GetStaticMember(member.Kind, member.Name, member.Index)
			if formalMember == nil || formalMember.Type == nil || member.Type == nil {
				continue
			}
			if hasFreshPrecisionShape(member.Type, formalMember.Type, depth+1) {
				return true
			}
		}
		if actualRecord.HasMapComponent() && formalRecord.HasMapComponent() {
			return hasFreshPrecisionShape(actualRecord.MapKey, formalRecord.MapKey, depth+1) ||
				hasFreshPrecisionShape(actualRecord.MapValue, formalRecord.MapValue, depth+1)
		}
		return false
	}
	actualFn, actualFnOK := actual.(*typ.Function)
	formalFn, formalFnOK := formal.(*typ.Function)
	if actualFnOK && formalFnOK {
		for i, ret := range actualFn.Returns {
			if i >= len(formalFn.Returns) {
				break
			}
			if hasFreshPrecisionShape(ret, formalFn.Returns[i], depth+1) {
				return true
			}
		}
		return false
	}
	return false
}

func actualRecordForValidation(actual typ.Type, depth int) (*typ.Record, bool) {
	if actual == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	actual = unwrap.Annotated(actual)
	switch a := actual.(type) {
	case *typ.Alias:
		return actualRecordForValidation(a.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(a)
		if expanded != nil && expanded != actual {
			return actualRecordForValidation(expanded, depth+1)
		}
	case *typ.Record:
		return a, true
	}
	return nil, false
}

func providedRecordFieldsAssignable(actual *typ.Record, formal *typ.Record, depth int) bool {
	if actual == nil || formal == nil {
		return true
	}
	if depth > typ.DefaultRecursionDepth {
		return false
	}
	for _, field := range actual.Fields {
		formalField := formal.GetField(field.Name)
		if formalField == nil || formalField.Type == nil || field.Type == nil {
			continue
		}
		if !instantiatedArgumentAssignable(field.Type, formalField.Type, depth+1) {
			return false
		}
	}
	for _, member := range actual.StaticMembers {
		formalMember := formal.GetStaticMember(member.Kind, member.Name, member.Index)
		if formalMember == nil || formalMember.Type == nil || member.Type == nil {
			continue
		}
		if !instantiatedArgumentAssignable(member.Type, formalMember.Type, depth+1) {
			return false
		}
	}
	if actual.HasMapComponent() && formal.HasMapComponent() {
		return instantiatedArgumentAssignable(actual.MapKey, formal.MapKey, depth+1) &&
			instantiatedArgumentAssignable(actual.MapValue, formal.MapValue, depth+1)
	}
	return true
}

func inferTypeParamBindings(formal typ.Type, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, path []InferencePathStep, trace bool, depth int) {
	if formal == nil || actual == nil || depth > typ.DefaultRecursionDepth {
		return
	}

	formal = unwrap.Annotated(formal)
	actual = unwrap.Annotated(actual)

	if param, ok := formal.(*typ.TypeParam); ok {
		bindTypeParam(param, actual, index, bindings, path, trace)
		return
	}

	switch f := formal.(type) {
	case *typ.Alias:
		inferTypeParamBindings(f.UnaliasedTarget(), actual, index, bindings, path, trace, depth+1)
	case *typ.Optional:
		if a, ok := actual.(*typ.Optional); ok {
			inferTypeParamBindings(f.Inner, a.Inner, index, bindings, path, trace, depth+1)
			return
		}
		inferTypeParamBindings(f.Inner, actual, index, bindings, path, trace, depth+1)
	case *typ.Array:
		switch a := actual.(type) {
		case *typ.Array:
			inferTypeParamBindings(f.Element, a.Element, index, bindings, path, trace, depth+1)
		case *typ.Tuple:
			for _, elem := range a.Elements {
				inferTypeParamBindings(f.Element, elem, index, bindings, path, trace, depth+1)
			}
		}
	case *typ.Map:
		if a, ok := actual.(*typ.Map); ok {
			inferTypeParamBindings(f.Key, a.Key, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "key"}), trace, depth+1)
			inferTypeParamBindings(f.Value, a.Value, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "value"}), trace, depth+1)
		}
	case *typ.ReadonlyMap:
		switch a := actual.(type) {
		case *typ.ReadonlyMap:
			inferTypeParamBindings(f.Key, a.Key, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "key"}), trace, depth+1)
			inferTypeParamBindings(f.Value, a.Value, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "value"}), trace, depth+1)
		case *typ.Map:
			inferTypeParamBindings(f.Key, a.Key, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "key"}), trace, depth+1)
			inferTypeParamBindings(f.Value, a.Value, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "value"}), trace, depth+1)
		}
	case *typ.Tuple:
		if a, ok := actual.(*typ.Tuple); ok {
			for i, elem := range f.Elements {
				if i >= len(a.Elements) {
					break
				}
				inferTypeParamBindings(elem, a.Elements[i], index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticInt, Index: i + 1}), trace, depth+1)
			}
		}
	case *typ.Function:
		inferFunctionBindings(f, actual, index, bindings, path, trace, depth+1)
	case *typ.Record:
		inferRecordBindings(f, actual, index, bindings, path, trace, depth+1)
	case *typ.Instantiated:
		if a, ok := actual.(*typ.Instantiated); ok && sameGeneric(f.Generic, a.Generic) {
			for i, arg := range f.TypeArgs {
				if i >= len(a.TypeArgs) {
					break
				}
				inferTypeParamBindings(arg, a.TypeArgs[i], index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathTypeArgument, Index: i + 1}), trace, depth+1)
			}
			return
		}
		expanded := expandFormalInstantiatedForInference(f)
		if expanded != nil && expanded != formal {
			inferTypeParamBindings(expanded, actual, index, bindings, path, trace, depth+1)
		}
	case *typ.Union:
		for _, member := range f.Members {
			inferTypeParamBindings(member, actual, index, bindings, path, trace, depth+1)
		}
	case *typ.Intersection:
		for _, member := range f.Members {
			inferTypeParamBindings(member, actual, index, bindings, path, trace, depth+1)
		}
	}
}

func expandFormalInstantiatedForInference(inst *typ.Instantiated) typ.Type {
	if inst == nil || inst.Generic == nil ||
		inst.Generic.Body == nil ||
		len(inst.TypeArgs) != len(inst.Generic.TypeParams) {
		return inst
	}
	body := subst.Params(inst.Generic.Body, inst.Generic.TypeParams, inst.TypeArgs)
	return subst.Self(body, inst)
}

func inferFunctionBindings(formal *typ.Function, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, path []InferencePathStep, trace bool, depth int) {
	if formal == nil || depth > typ.DefaultRecursionDepth {
		return
	}
	actual = unwrap.Annotated(actual)
	if alias, ok := actual.(*typ.Alias); ok {
		actual = alias.UnaliasedTarget()
	}
	if inst, ok := actual.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != actual {
			actual = expanded
		}
	}
	actualFn, ok := actual.(*typ.Function)
	if !ok || actualFn == nil {
		return
	}
	for i, param := range formal.Params {
		if i >= len(actualFn.Params) {
			break
		}
		inferTypeParamBindings(param.Type, actualFn.Params[i].Type, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathFunctionParam, Name: param.Name, Index: i + 1}), trace, depth+1)
	}
	if formal.Variadic != nil {
		if actualFn.Variadic != nil {
			inferTypeParamBindings(formal.Variadic, actualFn.Variadic, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathFunctionParam, Index: len(formal.Params) + 1}), trace, depth+1)
		} else {
			for i := len(formal.Params); i < len(actualFn.Params); i++ {
				inferTypeParamBindings(formal.Variadic, actualFn.Params[i].Type, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathFunctionParam, Index: i + 1}), trace, depth+1)
			}
		}
	}
	for i, ret := range formal.Returns {
		if i >= len(actualFn.Returns) {
			break
		}
		inferTypeParamBindings(ret, actualFn.Returns[i], index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathFunctionReturn, Index: i + 1}), trace, depth+1)
	}
}

func inferRecordBindings(formal *typ.Record, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, path []InferencePathStep, trace bool, depth int) {
	if formal == nil {
		return
	}
	actual = unwrap.Annotated(actual)
	if alias, ok := actual.(*typ.Alias); ok {
		actual = alias.UnaliasedTarget()
	}
	if inst, ok := actual.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != actual {
			actual = expanded
		}
	}
	if union, ok := actual.(*typ.Union); ok {
		for _, member := range union.Members {
			inferRecordBindings(formal, member, index, bindings, path, trace, depth+1)
		}
		return
	}
	record, ok := actual.(*typ.Record)
	if !ok || record == nil {
		return
	}
	for _, field := range formal.Fields {
		actualField := record.GetField(field.Name)
		if actualField == nil {
			continue
		}
		inferTypeParamBindings(field.Type, actualField.Type, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathField, Name: field.Name}), trace, depth+1)
	}
	if formal.HasMapComponent() && record.HasMapComponent() {
		inferTypeParamBindings(formal.MapKey, record.MapKey, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "key"}), trace, depth+1)
		inferTypeParamBindings(formal.MapValue, record.MapValue, index, bindings, appendInferencePath(path, InferencePathStep{Kind: InferencePathStaticString, Name: "value"}), trace, depth+1)
	}
	for _, member := range formal.StaticMembers {
		var actualMember *typ.StaticMember
		var step InferencePathStep
		switch member.Kind {
		case typ.StaticMemberStringIndex:
			actualMember = record.GetStaticStringIndex(member.Name)
			step = InferencePathStep{Kind: InferencePathStaticString, Name: member.Name}
		case typ.StaticMemberIntIndex:
			actualMember = record.GetStaticIntIndex(member.Index)
			step = InferencePathStep{Kind: InferencePathStaticInt, Index: int(member.Index)}
		}
		if actualMember != nil {
			inferTypeParamBindings(member.Type, actualMember.Type, index, bindings, appendInferencePath(path, step), trace, depth+1)
		}
	}
}

func sameGeneric(left *typ.Generic, right *typ.Generic) bool {
	if left == nil || right == nil {
		return false
	}
	return left == right || typ.SameNodeOrAcyclicEqual(left, right)
}

func bindTypeParam(param *typ.TypeParam, actual typ.Type, index int, bindings map[*typ.TypeParam]inferredArg, path []InferencePathStep, trace bool) {
	if param == nil || actual == nil {
		return
	}
	contribution := inferenceContribution(param, actual, index, path, trace)
	if existing, ok := bindings[param]; ok {
		bindings[param] = inferredArg{typ: mergeInferredTypes(existing.typ, actual), index: existing.index, contributions: appendInferenceContribution(existing.contributions, contribution)}
		return
	}
	for known, existing := range bindings {
		if known != nil && known.Equals(param) {
			contribution.Param = known
			bindings[known] = inferredArg{typ: mergeInferredTypes(existing.typ, actual), index: existing.index, contributions: appendInferenceContribution(existing.contributions, contribution)}
			return
		}
	}
	bindings[param] = inferredArg{typ: actual, index: index, contributions: appendInferenceContribution(nil, contribution)}
}

func inferenceContribution(param *typ.TypeParam, actual typ.Type, index int, path []InferencePathStep, trace bool) InferenceContribution {
	if !trace {
		return InferenceContribution{}
	}
	return InferenceContribution{
		Param: param,
		Index: index,
		Type:  actual,
		Path:  cloneInferencePath(path),
	}
}

func appendInferenceContribution(existing []InferenceContribution, contribution InferenceContribution) []InferenceContribution {
	if contribution.Param == nil || contribution.Type == nil {
		return existing
	}
	if len(existing) >= 8 {
		return existing
	}
	return append(existing, contribution)
}

func appendInferencePath(path []InferencePathStep, step InferencePathStep) []InferencePathStep {
	out := make([]InferencePathStep, 0, len(path)+1)
	out = append(out, path...)
	out = append(out, step)
	return out
}

func cloneInferencePath(path []InferencePathStep) []InferencePathStep {
	if len(path) == 0 {
		return nil
	}
	out := make([]InferencePathStep, len(path))
	copy(out, path)
	return out
}

func genericCallTraceFromBindings(params []*typ.TypeParam, bindings map[*typ.TypeParam]inferredArg) GenericCallTrace {
	if len(bindings) == 0 {
		return GenericCallTrace{}
	}
	var out GenericCallTrace
	for _, param := range params {
		binding, ok := inferredBindingForParam(bindings, param)
		if !ok {
			continue
		}
		out.Contributions = append(out.Contributions, binding.contributions...)
	}
	return out
}

func typeParamBindings(fn *typ.Function, bindings map[*typ.TypeParam]inferredArg) []TypeParamBinding {
	if fn == nil || len(bindings) == 0 {
		return nil
	}
	out := make([]TypeParamBinding, 0, len(bindings))
	for _, param := range fn.TypeParams {
		binding, ok := inferredBindingForParam(bindings, param)
		if !ok || binding.typ == nil {
			continue
		}
		out = append(out, TypeParamBinding{
			Param: param,
			Type:  binding.typ,
			Index: binding.index,
		})
	}
	return out
}

func inferredBindingForParam(bindings map[*typ.TypeParam]inferredArg, param *typ.TypeParam) (inferredArg, bool) {
	if binding, ok := bindings[param]; ok {
		return binding, true
	}
	for known, binding := range bindings {
		if known != nil && param != nil && known.Equals(param) {
			return binding, true
		}
	}
	return inferredArg{}, false
}

func mergeInferredTypes(left typ.Type, right typ.Type) typ.Type {
	if left == nil {
		return right
	}
	if right == nil || typ.SameNodeOrAcyclicEqual(left, right) {
		return left
	}
	if subtype.IsSubtype(left, right) {
		return right
	}
	if subtype.IsSubtype(right, left) {
		return left
	}
	if merged, ok := mergeInferredLiteralFamilies(left, right); ok {
		return merged
	}
	return normalize.UnionForEvidence(left, right)
}

func mergeInferredLiteralFamilies(left typ.Type, right typ.Type) (typ.Type, bool) {
	leftBase, leftOK := typelit.FamilyBase(left)
	rightBase, rightOK := typelit.FamilyBase(right)
	if !leftOK || !rightOK {
		return nil, false
	}
	return typelit.MergeFamilyBases(leftBase, rightBase)
}

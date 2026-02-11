package transform

import (
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ApplyEffectTransform applies return type effects to compute the actual return type.
// If the function has a contract.Spec with a Return effect, the transform is applied
// to derive the concrete return type from the argument types.
func ApplyEffectTransform(fn *typ.Function, args []typ.Type, returnIdx int, baseReturn typ.Type) typ.Type {
	if fn == nil || fn.Spec == nil {
		return baseReturn
	}

	spec, ok := fn.Spec.(*contract.Spec)
	if !ok || spec == nil {
		return baseReturn
	}

	// Error-return pattern: value is optional when an error return is present.
	if er := spec.Effects.GetErrorReturn(returnIdx); er != nil {
		if !unwrap.IsOptionalLike(baseReturn) {
			return typ.NewOptional(baseReturn)
		}
	}

	ret := spec.Effects.GetReturn(returnIdx)
	if ret == nil || ret.Transform == nil {
		return baseReturn
	}

	switch transform := ret.Transform.(type) {
	case effect.SelectResultOfCases:
		result := buildSelectResultUnion(args, transform)
		if result != nil {
			return result
		}
		return baseReturn
	default:
		return baseReturn
	}
}

// buildSelectResultUnion builds a union of result records from SelectCase elements.
func buildSelectResultUnion(args []typ.Type, transform effect.SelectResultOfCases) typ.Type {
	casesIdx, ok := effect.ResolveParamIndex(transform.Cases, len(args))
	if !ok {
		return nil
	}

	casesArg := args[casesIdx]
	if casesArg == nil {
		return nil
	}

	caseTypes := extractSelectCaseElements(casesArg)
	if len(caseTypes) == 0 {
		return nil
	}

	addDefault := transform.Default.Index >= 0 && transform.Default.Index < len(args)

	var resultTypes []typ.Type
	seen := make(map[uint64]bool)

	for _, caseType := range caseTypes {
		channelType, valueType := extractSelectCaseParts(caseType)
		if channelType == nil {
			channelType = typ.Any
			valueType = typ.Any
		}

		builder := typ.NewRecord().
			Field("channel", channelType).
			Field("ok", typ.Boolean).
			Field("value", valueType)

		if addDefault {
			builder = builder.OptField("default", typ.Boolean)
		}

		resultRecord := builder.Build()

		h := resultRecord.Hash()
		if !seen[h] {
			seen[h] = true
			resultTypes = append(resultTypes, resultRecord)
		}
	}

	if len(resultTypes) == 0 {
		return nil
	}

	if len(resultTypes) == 1 {
		return resultTypes[0]
	}

	return typ.NewUnion(resultTypes...)
}

// extractSelectCaseElements extracts individual SelectCase types from a cases argument.
func extractSelectCaseElements(casesArg typ.Type) []typ.Type {
	if casesArg == nil {
		return nil
	}

	switch v := casesArg.(type) {
	case *typ.Tuple:
		result := make([]typ.Type, len(v.Elements))
		copy(result, v.Elements)
		return result
	case *typ.Record:
		if len(v.Fields) == 0 {
			return nil
		}
		result := make([]typ.Type, 0, len(v.Fields))
		for _, f := range v.Fields {
			if f.Type != nil {
				result = append(result, f.Type)
			}
		}
		return result
	case *typ.Array:
		if v.Element != nil {
			if u, ok := v.Element.(*typ.Union); ok {
				result := make([]typ.Type, len(u.Members))
				copy(result, u.Members)
				return result
			}
			return []typ.Type{v.Element}
		}
	case *typ.Union:
		result := make([]typ.Type, len(v.Members))
		copy(result, v.Members)
		return result
	}

	return []typ.Type{casesArg}
}

// extractSelectCaseParts extracts the channel and value types from a SelectCase<Ch, T>.
func extractSelectCaseParts(caseType typ.Type) (channel typ.Type, value typ.Type) {
	if caseType == nil {
		return nil, typ.Unknown
	}

	inst, ok := caseType.(*typ.Instantiated)
	if !ok || inst.Generic == nil {
		return nil, typ.Unknown
	}

	if len(inst.TypeArgs) < 2 {
		return nil, typ.Unknown
	}

	return inst.TypeArgs[0], inst.TypeArgs[1]
}

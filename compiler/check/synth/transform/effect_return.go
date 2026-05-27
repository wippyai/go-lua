package transform

import (
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ApplyEffectTransform applies return type effects to compute the actual return type.
// If the function has a contract.Spec with a Return effect, the transform is applied
// to derive the concrete return type from the argument types.
func ApplyEffectTransform(fn *typ.Function, args []typ.Type, returnIdx int, baseReturn typ.Type) typ.Type {
	if fn == nil {
		return baseReturn
	}

	// Error-return pattern: value is optional when an error return is present.
	if er := contract.ErrorReturnForValue(fn, returnIdx); er != nil {
		if !unwrap.IsOptionalLike(baseReturn) {
			return typ.NewOptional(baseReturn)
		}
	}

	row := effectiveReturnEffectRow(fn)
	ret := row.GetReturn(returnIdx)
	if ret == nil || ret.Transform == nil {
		return applyFlowIntoTransforms(row, args, returnIdx, baseReturn)
	}

	transformedReturn := baseReturn
	switch transform := ret.Transform.(type) {
	case effect.SameAs:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			transformedReturn = resolved
			break
		}
	case effect.ElementOf:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			if elem := querycore.ElementType(resolved); elem != nil {
				transformedReturn = elem
				break
			}
		}
	case effect.OptionalElementOf:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			if elem := querycore.ElementType(resolved); elem != nil {
				transformedReturn = typ.NewOptional(elem)
				break
			}
		}
	case effect.DeepElementOf:
		if resolved := resolveParamType(args, transform.Source); resolved != nil {
			if elem := deepElementType(resolved); elem != nil {
				transformedReturn = elem
				break
			}
		}
	case effect.StringUnpackValue:
		if resolved := resolveParamType(args, transform.Format); resolved != nil {
			if unpacked := unpackFirstValueType(resolved); unpacked != nil {
				transformedReturn = unpacked
				break
			}
		}
	case effect.CallbackReturn:
		if resolved := resolveParamType(args, transform.CallbackParam); resolved != nil {
			if cbRet := callbackReturnType(resolved); cbRet != nil {
				transformedReturn = cbRet
				break
			}
		}
	case effect.ArrayOfCallbackReturn:
		if resolved := resolveParamType(args, transform.CallbackParam); resolved != nil {
			if cbRet := callbackReturnType(resolved); cbRet != nil {
				transformedReturn = typ.NewArray(cbRet)
				break
			}
		}
	case effect.SelectResultOfCases:
		result := buildSelectResultUnion(args, transform)
		if result != nil {
			transformedReturn = result
			break
		}
	default:
	}
	return applyFlowIntoTransforms(row, args, returnIdx, transformedReturn)
}

func effectiveReturnEffectRow(fn *typ.Function) effect.Row {
	if fn == nil {
		return effect.Empty
	}
	var row effect.Row
	if spec, ok := fn.Spec.(*contract.Spec); ok && spec != nil {
		row = effect.Union(row, spec.Effects)
	}
	if r, ok := fn.Effects.(effect.Row); ok {
		row = effect.Union(row, r)
	}
	if refinement, ok := fn.Refinement.(*constraint.FunctionRefinement); ok && refinement != nil {
		if r, ok := refinement.Row.(effect.Row); ok {
			row = effect.Union(row, r)
		}
	}
	return row
}

func resolveParamType(args []typ.Type, ref effect.ParamRef) typ.Type {
	idx, ok := effect.ResolveParamIndex(ref, len(args))
	if !ok || idx < 0 || idx >= len(args) {
		return nil
	}
	return args[idx]
}

func applyFlowIntoTransforms(row effect.Row, args []typ.Type, returnIdx int, baseReturn typ.Type) typ.Type {
	flows := row.FlowIntoReturns(returnIdx)
	if len(flows) == 0 {
		return baseReturn
	}
	out := baseReturn
	for _, flow := range flows {
		source, ok := resolveFlowSourceType(args, flow)
		if !ok {
			continue
		}
		source = narrow.ToTruthy(source)
		if typ.IsNever(source) {
			source = nil
		}
		projected := mergeFlowRemainder(source, flow.Remainder)
		if projected == nil {
			continue
		}
		out = setReturnPathType(out, splitEffectPath(flow.TargetPath), projected)
	}
	return out
}

// resolveFlowSourceType reads the flow's source path on the call argument. The
// boolean reports whether the flow applies at all: a missing argument or an
// unresolvable base cannot be projected and skips the flow. A field that is
// provably absent on a concrete argument resolves to nil, so an `or`-default
// flow still contributes its Remainder rather than being dropped.
func resolveFlowSourceType(args []typ.Type, flow effect.FlowInto) (typ.Type, bool) {
	if flow.ParamIndex < 0 || flow.ParamIndex >= len(args) {
		return nil, false
	}
	current := args[flow.ParamIndex]
	for _, field := range splitEffectPath(flow.SourcePath) {
		if current == nil {
			return nil, false
		}
		next, ok := querycore.Field(current, field)
		if !ok {
			if absentFieldResolvesToNil(current) {
				return typ.Nil, true
			}
			return nil, false
		}
		current = next
	}
	return current, true
}

// absentFieldResolvesToNil reports whether a failed field read on t proves the
// field is absent (value nil) rather than merely unresolved. A concrete table
// shape that does not declare the field reads nil at that key.
func absentFieldResolvesToNil(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Record:
		return !v.Open && !v.HasMapComponent()
	case *typ.Optional:
		return absentFieldResolvesToNil(v.Inner)
	default:
		return false
	}
}

func mergeFlowRemainder(source, remainder typ.Type) typ.Type {
	switch {
	case source == nil:
		return remainder
	case remainder == nil:
		return source
	default:
		return typjoin.Types(source, remainder)
	}
}

func splitEffectPath(path string) []string {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func setReturnPathType(t typ.Type, path []string, value typ.Type) typ.Type {
	if value == nil {
		return t
	}
	if len(path) == 0 {
		return refineFlowTargetType(t, value)
	}
	if t == nil || typ.IsUnknown(t) || typ.IsAny(t) || typ.IsNever(t) {
		return buildRecordPath(path, value)
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		inner := setReturnPathType(v.Inner, path, value)
		if typ.TypeEquals(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		for i, member := range v.Members {
			members[i] = member
			if member == nil || !pathMayExist(member, path) {
				continue
			}
			next := setReturnPathType(member, path, value)
			if !typ.TypeEquals(next, member) {
				members[i] = next
				changed = true
			}
		}
		if !changed {
			return t
		}
		return typ.NewUnion(members...)
	case *typ.Record:
		return setRecordPathType(v, path, value)
	default:
		return t
	}
}

func pathMayExist(t typ.Type, path []string) bool {
	if len(path) == 0 {
		return true
	}
	switch v := unwrap.Alias(t).(type) {
	case *typ.Optional:
		return pathMayExist(v.Inner, path)
	case *typ.Union:
		for _, member := range v.Members {
			if pathMayExist(member, path) {
				return true
			}
		}
		return false
	case *typ.Record:
		field := v.GetField(path[0])
		return field != nil && pathMayExist(field.Type, path[1:])
	default:
		return typ.IsUnknown(t) || typ.IsAny(t)
	}
}

func setRecordPathType(rec *typ.Record, path []string, value typ.Type) typ.Type {
	if rec == nil || len(path) == 0 {
		return rec
	}
	fieldName := path[0]
	fields := make([]typ.Field, len(rec.Fields))
	copy(fields, rec.Fields)
	found := false
	changed := false
	for i := range fields {
		if fields[i].Name != fieldName {
			continue
		}
		found = true
		next := fields[i].Type
		if len(path) == 1 {
			next = refineFlowTargetType(fields[i].Type, value)
		} else {
			next = setReturnPathType(fields[i].Type, path[1:], value)
		}
		if !typ.TypeEquals(next, fields[i].Type) {
			fields[i].Type = next
			changed = true
		}
		break
	}
	if !found {
		fields = append(fields, typ.Field{Name: fieldName, Type: buildRecordPath(path[1:], value)})
		changed = true
	}
	if !changed {
		return rec
	}
	builder := typ.NewRecord().SetOpen(rec.Open)
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	for _, field := range fields {
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
	return builder.Build()
}

func buildRecordPath(path []string, value typ.Type) typ.Type {
	if len(path) == 0 {
		return value
	}
	return typ.NewRecord().Field(path[0], buildRecordPath(path[1:], value)).Build()
}

func refineFlowTargetType(existing, candidate typ.Type) typ.Type {
	if candidate == nil {
		return existing
	}
	if existing == nil || typ.IsUnknown(existing) || typ.IsAny(existing) || typ.IsNever(existing) {
		return candidate
	}
	if subtype.IsSubtype(candidate, existing) {
		return candidate
	}
	if subtype.IsSubtype(existing, candidate) {
		return existing
	}
	return typjoin.Types(existing, candidate)
}

func callbackReturnType(t typ.Type) typ.Type {
	t = unwrap.Alias(t)
	if t == nil {
		return nil
	}
	switch v := t.(type) {
	case *typ.Function:
		if len(v.Returns) == 0 || v.Returns[0] == nil {
			return nil
		}
		return v.Returns[0]
	case *typ.Optional:
		return callbackReturnType(v.Inner)
	case *typ.Union:
		var members []typ.Type
		for _, m := range v.Members {
			if rt := callbackReturnType(m); rt != nil {
				members = append(members, rt)
			}
		}
		if len(members) == 0 {
			return nil
		}
		return typ.NewUnion(members...)
	default:
		if typ.IsAny(t) {
			return typ.Any
		}
		if typ.IsUnknown(t) {
			return typ.Unknown
		}
		return nil
	}
}

func deepElementType(t typ.Type) typ.Type {
	current := t
	last := querycore.ElementType(current)
	for last != nil {
		current = last
		next := querycore.ElementType(current)
		if next == nil {
			return current
		}
		last = next
	}
	return nil
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

	addDefault := selectDefaultPossible(args, casesArg, transform)

	var resultTypes []typ.Type
	seen := make(map[uint64]bool)

	for caseIdx, caseType := range caseTypes {
		channelType, valueType := extractSelectCaseParts(caseType)
		if channelType == nil {
			// Keep unknown/any case elements conservative; skip concrete non-case fields.
			if !typ.IsAny(caseType) && !typ.IsUnknown(caseType) {
				continue
			}
			channelType = typ.Any
			valueType = typ.Any
		}

		builder := typ.NewRecord().
			Field(effect.SelectResultChannelField, channelType).
			Field("ok", typ.Boolean).
			Field(effect.SelectResultValueField, valueType).
			// Preserve case multiplicity even when channel/value types are equal.
			// This keeps identity-sensitive narrowing sound for `result.channel ~= ch`.
			Field(effect.SelectResultCaseIDField, typ.LiteralInt(int64(caseIdx)))

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

func selectDefaultPossible(args []typ.Type, casesArg typ.Type, transform effect.SelectResultOfCases) bool {
	if idx, ok := effect.ResolveParamIndex(transform.Default, len(args)); ok {
		if defaultArgMayEnableSelectDefault(args[idx]) {
			return true
		}
	}
	return casesArgHasDefaultFlag(casesArg)
}

func defaultArgMayEnableSelectDefault(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}

	switch v := t.(type) {
	case *typ.Optional:
		return defaultArgMayEnableSelectDefault(v.Inner)
	case *typ.Union:
		for _, m := range v.Members {
			if defaultArgMayEnableSelectDefault(m) {
				return true
			}
		}
		return false
	case *typ.Literal:
		if v.Base == kind.Boolean {
			b, _ := v.Value.(bool)
			return b
		}
		return false
	default:
		return t.Kind() == kind.Boolean || typ.IsAny(t) || typ.IsUnknown(t)
	}
}

func casesArgHasDefaultFlag(casesArg typ.Type) bool {
	casesArg = unwrap.Alias(casesArg)
	if casesArg == nil {
		return false
	}

	switch v := casesArg.(type) {
	case *typ.Optional:
		return casesArgHasDefaultFlag(v.Inner)
	case *typ.Union:
		for _, m := range v.Members {
			if casesArgHasDefaultFlag(m) {
				return true
			}
		}
		return false
	case *typ.Record:
		if field := v.GetField("default"); field != nil {
			return defaultArgMayEnableSelectDefault(field.Type)
		}
		// Conservative map-component handling: {[string]: boolean}-style cases tables
		// can carry a "default" flag even when not modeled as a named field.
		if v.MapKey == nil || v.MapValue == nil {
			return false
		}
		return typ.TypeMatchesLiteral(v.MapKey, typ.LiteralString("default")) &&
			defaultArgMayEnableSelectDefault(v.MapValue)
	default:
		return false
	}
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
			if f.Name == "default" {
				continue
			}
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

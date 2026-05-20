package paramevidence

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

type paramUse struct {
	whole  bool
	fields map[string]struct{}
}

// ProjectToParameterUse trims structured call-site evidence to the surface the
// function body actually reads from each unannotated parameter. It is evidence
// for analyzing a helper, not a promise that every unused field on the
// first argument shape is part of that helper's public contract.
func ProjectToParameterUse(slots []cfg.ParamSlot, evidence []api.ParameterUseEvidence, vec []typ.Type) []typ.Type {
	if len(slots) == 0 || len(vec) == 0 {
		return vec
	}

	uses := parameterUseMap(evidence)
	if len(uses) == 0 {
		return vec
	}

	var out []typ.Type
	for idx, slot := range slots {
		if slot.Symbol == 0 || idx < 0 || idx >= len(vec) {
			continue
		}
		observed := vec[idx]
		if observed == nil {
			continue
		}
		projected := projectEvidenceToUse(observed, uses[slot.Symbol])
		if typ.TypeEquals(observed, projected) {
			continue
		}
		if out == nil {
			out = make([]typ.Type, len(vec))
			copy(out, vec)
		}
		out[idx] = projected
	}

	if out == nil {
		return vec
	}
	return out
}

// ProjectSignatureToParamUse completes a function signature's parameter slots
// against the fields the function body reads. Unlike ProjectToParameterUse it
// does not trim unused fields: a function fact is already a canonical signature
// observation, and same-body analysis only needs to ensure demanded fields are
// present even when the parameter is also used as a whole value.
func ProjectSignatureToParamUse(slots []cfg.ParamSlot, evidence []api.ParameterUseEvidence, sig *typ.Function) *typ.Function {
	if sig == nil || len(sig.Params) == 0 {
		return sig
	}
	uses := parameterUseMap(evidence)
	if len(uses) == 0 {
		return sig
	}
	projected := make([]typ.Type, len(sig.Params))
	changed := false
	for idx, slot := range slots {
		if idx < 0 || idx >= len(sig.Params) || slot.Symbol == 0 {
			continue
		}
		use := uses[slot.Symbol]
		if len(use.fields) == 0 {
			continue
		}
		completed, ok := completeTypeWithFields(sig.Params[idx].Type, use.fields)
		if !ok || completed == nil {
			continue
		}
		projected[idx] = completed
		if !typ.TypeEquals(sig.Params[idx].Type, completed) {
			changed = true
		}
	}
	if !changed {
		return sig
	}

	builder := typ.Func().ReserveParams(len(sig.Params))
	for _, tp := range sig.TypeParams {
		builder = builder.TypeParam(tp.Name, tp.Constraint)
	}
	for i, p := range sig.Params {
		paramType := p.Type
		if i < len(projected) && projected[i] != nil {
			paramType = projected[i]
		}
		if p.Optional {
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

func completeTypeWithFields(t typ.Type, fields map[string]struct{}) (typ.Type, bool) {
	if t == nil || len(fields) == 0 {
		return t, false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		completed, ok := completeTypeWithFields(v.Target, fields)
		if !ok {
			return t, false
		}
		return completed, true
	case *typ.Optional:
		inner, ok := completeTypeWithFields(v.Inner, fields)
		if !ok {
			return t, false
		}
		return typ.NewOptional(inner), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			completed, ok := completeTypeWithFields(member, fields)
			if !ok {
				members = append(members, member)
				continue
			}
			if !typ.TypeEquals(member, completed) {
				changed = true
			}
			members = append(members, completed)
		}
		if !changed {
			return t, false
		}
		return typ.NewUnion(members...), true
	case *typ.Record:
		return completeRecordWithFields(v, fields), true
	default:
		return t, false
	}
}

func completeRecordWithFields(r *typ.Record, fields map[string]struct{}) typ.Type {
	builder := typ.NewRecord()
	if r.Open {
		builder.SetOpen(true)
	}
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	for _, field := range r.Fields {
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
	if r.HasMapComponent() {
		builder.MapComponent(r.MapKey, r.MapValue)
	}

	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if r.GetField(name) != nil {
			continue
		}
		if r.HasMapComponent() && subtype.IsSubtype(typ.LiteralString(name), r.MapKey) {
			mapValue := r.MapValue
			if mapValue == nil {
				mapValue = typ.Unknown
			}
			builder.OptField(name, mapValue)
			continue
		}
		if !r.Open {
			builder.Field(name, typ.Nil)
		}
	}
	return builder.Build()
}

// UnobservedParameterMask reports parameter slots whose values are not demanded
// by the function body. Nil means every slot is observed or no parameter-use
// information is available.
func UnobservedParameterMask(slots []cfg.ParamSlot, evidence []api.ParameterUseEvidence) []bool {
	if len(slots) == 0 {
		return nil
	}
	uses := parameterUseMap(evidence)
	var mask []bool
	for i, slot := range slots {
		if slot.Symbol == 0 {
			continue
		}
		use, observed := uses[slot.Symbol]
		if observed && (use.whole || len(use.fields) > 0) {
			continue
		}
		if mask == nil {
			mask = make([]bool, len(slots))
		}
		mask[i] = true
	}
	return mask
}

func parameterUseMap(evidence []api.ParameterUseEvidence) map[cfg.SymbolID]paramUse {
	if len(evidence) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]paramUse, len(evidence))
	for _, ev := range evidence {
		if ev.Symbol == 0 {
			continue
		}
		use := out[ev.Symbol]
		if ev.Whole {
			use.whole = true
		}
		if len(ev.Fields) > 0 {
			if use.fields == nil {
				use.fields = make(map[string]struct{}, len(ev.Fields))
			}
			for _, field := range ev.Fields {
				if field != "" {
					use.fields[field] = struct{}{}
				}
			}
		}
		out[ev.Symbol] = use
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func projectEvidenceToUse(observed typ.Type, use paramUse) typ.Type {
	if observed == nil {
		return observed
	}
	if len(use.fields) == 0 {
		if use.whole {
			return observed
		}
		return nil
	}
	if use.whole {
		completed, ok := completeTypeWithFields(observed, use.fields)
		if !ok {
			return observed
		}
		return completed
	}
	projected, ok := projectTypeToFields(observed, use.fields)
	if !ok {
		return observed
	}
	return projected
}

func projectTypeToFields(t typ.Type, fields map[string]struct{}) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	switch v := typ.UnwrapAnnotated(t).(type) {
	case *typ.Alias:
		return projectTypeToFields(v.Target, fields)
	case *typ.Optional:
		inner, ok := projectTypeToFields(v.Inner, fields)
		if !ok {
			return t, false
		}
		return typ.NewOptional(inner), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			projected, ok := projectTypeToFields(member, fields)
			if !ok {
				return t, false
			}
			members = append(members, projected)
		}
		return typ.NewUnion(members...), true
	case *typ.Record:
		return projectRecordToFields(v, fields), true
	default:
		return t, false
	}
}

func projectRecordToFields(r *typ.Record, fields map[string]struct{}) typ.Type {
	builder := typ.NewRecord().SetOpen(true)
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		field := r.GetField(name)
		if field == nil {
			if r.HasMapComponent() && subtype.IsSubtype(typ.LiteralString(name), r.MapKey) {
				mapValue := r.MapValue
				if mapValue == nil {
					mapValue = typ.Unknown
				}
				builder.OptField(name, mapValue)
			} else if !r.Open {
				builder.Field(name, typ.Nil)
			}
			continue
		}
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

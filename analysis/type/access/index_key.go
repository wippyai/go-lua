package access

import (
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func indexByKeyVariants(key typ.Type, depth int, mode indexMode, missingNil bool, project func(typ.Type) fieldResult) fieldResult {
	return descendAccessWrappers(key, depth, func(key typ.Type, depth int) fieldResult {
		switch v := unwrap.Annotated(key).(type) {
		case *typ.Union:
			return indexKeyUnion(v, depth+1, mode, missingNil, project)
		default:
			res := project(key)
			if !res.ok && mode == indexRuntime && missingNil {
				return fieldResult{t: typ.Nil, ok: true}
			}
			return res
		}
	}, func(res fieldResult) fieldResult {
		if res.ok {
			res.nilable = true
		}
		return res
	})
}

func indexKeyUnion(u *typ.Union, depth int, mode indexMode, missingNil bool, project func(typ.Type) fieldResult) fieldResult {
	if u == nil || len(u.Members) == 0 {
		return fieldResult{}
	}
	out := make([]typ.Type, 0, len(u.Members))
	nilable := false
	for _, member := range u.Members {
		res := indexByKeyVariants(member, depth+1, mode, missingNil, project)
		if !res.ok {
			if mode == indexRuntime && missingNil {
				nilable = true
				continue
			}
			return fieldResult{}
		}
		if res.nilable {
			nilable = true
		}
		if res.t != nil {
			out = append(out, res.t)
		}
	}
	if len(out) == 0 {
		if nilable {
			return fieldResult{t: typ.Nil, ok: true}
		}
		return fieldResult{}
	}
	return fieldResult{t: normalize.UnionForEvidence(out...), ok: true, nilable: nilable}
}

func literalStringKey(t typ.Type) (string, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

func literalIntKey(t typ.Type) (int64, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Integer {
		return 0, false
	}
	index, ok := lit.Value.(int64)
	return index, ok
}

package callshape

import (
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// MethodConsumesReceiver reports whether a method call consumes receiver as
// runtime argument 0 for parameter-indexed checking/effects.
func MethodConsumesReceiver(
	ctx *db.QueryContext,
	query querycore.TypeOps,
	fn *typ.Function,
	receiver typ.Type,
	isMethod bool,
	forceMethodReceiver bool,
) bool {
	if !isMethod || receiver == nil || fn == nil {
		return false
	}
	if forceMethodReceiver {
		return true
	}
	return HasExplicitSelf(ctx, query, fn, receiver)
}

// MethodConsumesReceiverSimple is the non-query variant used during generic
// inference before a query context is available.
func MethodConsumesReceiverSimple(fn *typ.Function, receiver typ.Type, isMethod bool, forceMethodReceiver bool) bool {
	if !isMethod || receiver == nil || fn == nil {
		return false
	}
	if forceMethodReceiver {
		return true
	}
	return HasExplicitSelfSimple(fn, receiver)
}

// RuntimeArgsForEffects returns the runtime argument vector seen by
// parameter-indexed return/effect transforms.
func RuntimeArgsForEffects(
	ctx *db.QueryContext,
	query querycore.TypeOps,
	fn *typ.Function,
	args []typ.Type,
	receiver typ.Type,
	isMethod bool,
	forceMethodReceiver bool,
) []typ.Type {
	if !MethodConsumesReceiver(ctx, query, fn, receiver, isMethod, forceMethodReceiver) {
		return args
	}
	out := make([]typ.Type, 0, len(args)+1)
	out = append(out, receiver)
	out = append(out, args...)
	return out
}

// HasExplicitSelf checks whether fn declares receiver as its first parameter.
func HasExplicitSelf(ctx *db.QueryContext, query querycore.TypeOps, fn *typ.Function, receiver typ.Type) bool {
	if len(fn.Params) == 0 {
		return false
	}
	if firstParamDeclaresSelf(fn) {
		return true
	}
	receiverMatch := normalizeReceiverForSelfCheck(ctx, query, receiver)
	return hasExplicitSelfCommon(fn, receiver, receiverMatch, func(sub, super typ.Type) bool {
		return isSubtypeCheck(ctx, query, sub, super)
	})
}

// HasExplicitSelfSimple is the non-memoized form of HasExplicitSelf.
func HasExplicitSelfSimple(fn *typ.Function, receiver typ.Type) bool {
	if len(fn.Params) == 0 {
		return false
	}
	if firstParamDeclaresSelf(fn) {
		return true
	}
	receiverMatch := normalizeReceiverForSelfCheck(nil, nil, receiver)
	return hasExplicitSelfCommon(fn, receiver, receiverMatch, subtype.IsSubtype)
}

func firstParamDeclaresSelf(fn *typ.Function) bool {
	if fn == nil || len(fn.Params) == 0 {
		return false
	}
	if name := fn.Params[0].Name; name == "self" || name == "Self" {
		return true
	}
	firstParam := fn.Params[0].Type
	return firstParam != nil && firstParam.Kind() == kind.Self
}

func hasExplicitSelfCommon(
	fn *typ.Function,
	receiver typ.Type,
	receiverMatch typ.Type,
	isSubtype func(sub, super typ.Type) bool,
) bool {
	if fn == nil || len(fn.Params) == 0 || isSubtype == nil {
		return false
	}

	if name := fn.Params[0].Name; name == "self" || name == "Self" {
		return true
	}

	firstParam := fn.Params[0].Type
	if firstParam == nil {
		return false
	}
	if firstParam.Kind() == kind.Self {
		return true
	}
	if tp, ok := firstParam.(*typ.TypeParam); ok {
		if tp.Constraint != nil && receiverMatch != nil &&
			isExplicitSelfSubtypeCandidate(receiverMatch) &&
			isExplicitSelfSubtypeCandidate(tp.Constraint) &&
			(isSubtype(receiverMatch, tp.Constraint) && isSubtype(tp.Constraint, receiverMatch)) {
			return true
		}
		return false
	}
	if receiverMatch != nil &&
		isExplicitSelfSubtypeCandidate(receiverMatch) &&
		isExplicitSelfSubtypeCandidate(firstParam) &&
		(isSubtype(receiverMatch, firstParam) && isSubtype(firstParam, receiverMatch)) {
		return true
	}

	return receiver != nil && isLocalRefMatch(firstParam, receiver)
}

func isLocalRefMatch(param typ.Type, receiver typ.Type) bool {
	ref, ok := param.(*typ.Ref)
	if !ok || ref.Module != "" {
		return false
	}
	name, ok := receiverAliasName(receiver)
	return ok && ref.Name == name
}

func receiverAliasName(t typ.Type) (string, bool) {
	switch v := t.(type) {
	case *typ.Alias:
		return v.Name, true
	case *typ.Optional:
		return receiverAliasName(v.Inner)
	default:
		return "", false
	}
}

func normalizeReceiverForSelfCheck(ctx *db.QueryContext, query querycore.TypeOps, receiver typ.Type) typ.Type {
	if receiver == nil {
		return nil
	}
	if typ.ContainsRecursive(receiver) {
		return receiver
	}
	if query != nil {
		if widened := query.Widen(ctx, receiver); widened != nil {
			return widened
		}
	}
	return subtype.Widen(receiver)
}

func isSubtypeCheck(ctx *db.QueryContext, query querycore.TypeOps, sub, super typ.Type) bool {
	if query != nil {
		return query.IsSubtype(ctx, sub, super)
	}
	return subtype.IsSubtype(sub, super)
}

func isExplicitSelfSubtypeCandidate(t typ.Type) bool {
	return t != nil && !typ.IsSoft(t, typ.SoftAnnotationPolicy)
}

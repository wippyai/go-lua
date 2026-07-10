package pass

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
)

func normalizeJudgmentSubjects(ctx Context, items []judgment.Judgment) {
	if len(items) == 0 {
		return
	}
	ordinals := make(map[string]int, len(items))
	for i := range items {
		anchor := judgmentSubjectAnchor(ctx, items[i])
		base := anchor
		base.Ordinal = 0
		baseKey := base.StableKey()
		anchor.Ordinal = ordinals[baseKey]
		ordinals[baseKey]++
		items[i].Subject = items[i].Subject.WithAnchor(anchor)
		for j := range items[i].Evidence {
			if !items[i].Evidence[j].Detail.Cause.IsZero() &&
				items[i].Evidence[j].Detail.Cause.Origin == (judgment.OriginRef{}) {
				items[i].Evidence[j].Detail.Cause.Origin = items[i].Evidence[j].Origin
			}
		}
	}
}

func judgmentSubjectAnchor(ctx Context, item judgment.Judgment) judgment.SubjectAnchor {
	if !item.Subject.Anchor.IsZero() {
		anchor := item.Subject.Anchor
		if anchor.ModulePath == "" {
			anchor.ModulePath = subjectAnchorModule(ctx, item)
		}
		if anchor.FunctionKey == "" {
			anchor.FunctionKey = subjectAnchorFunction(ctx, item)
		}
		if anchor.Kind == judgment.SubjectUnknown {
			anchor.Kind = item.Subject.Kind
		}
		return anchor
	}
	anchor := judgment.SubjectAnchor{
		ModulePath:  subjectAnchorModule(ctx, item),
		FunctionKey: subjectAnchorFunction(ctx, item),
		Kind:        item.Subject.Kind,
		Role:        string(item.Code),
	}
	key := item.Subject.Key
	switch item.Code {
	case judgment.CodeCallArgType:
		anchor.Role, anchor.BindingKey = callSubjectRoleAndBinding(key)
	case judgment.CodeSendIsolation:
		anchor.Role, anchor.BindingKey = callSubjectRoleAndBinding(key)
	case judgment.CodeCallArity:
		anchor.Role = "call.arity"
	case judgment.CodeCallCallee:
		anchor.Role = "call.callee"
	case judgment.CodeAssignment:
		anchor.Role = "assignment.value"
		anchor.BindingKey = stripLegacyPointKey(key, "assignment")
	case judgment.CodeAssignmentTarget:
		anchor.Role = "assignment.optional_target"
		anchor.BindingKey = stripLegacyPointKey(key, "assignment")
	case judgment.CodeReturn:
		anchor.Role = returnSubjectRole(key)
	case judgment.CodeDeadAssignment:
		anchor.Role = "assignment.dead"
		anchor.BindingKey = stripLegacyPointKey(key, "dead-assignment")
	case judgment.CodeFrozenTable:
		anchor.Role = "effect.freeze.mutation"
		anchor.BindingKey = stripLegacyPointKey(key, "frozen-table")
	case judgment.CodeLifecycle:
		anchor.Role = "effect.lifecycle"
		anchor.BindingKey = strings.TrimPrefix(key, "lifecycle:")
	case judgment.CodeTypestateInvalidTransition:
		anchor.Role = "typestate.invalid_transition"
		anchor.BindingKey = strings.TrimPrefix(key, "typestate-transition:")
	case judgment.CodeUnusedLocal:
		anchor.Role = "lint.unused_local"
		anchor.BindingKey = stripLegacyPointKey(key, "unused-local")
	case judgment.CodeUnresolvedValue:
		anchor.Role = "value.unresolved"
		anchor.BindingKey = stripLegacyPointKey(key, "unresolved-value")
	case judgment.CodeUnresolvedType:
		anchor.Role = "type.unresolved"
		anchor.BindingKey = stripLegacyPointKey(key, "unresolved-type")
	case judgment.CodeMemberRead:
		anchor.Role = "member.read"
		anchor.BindingKey = stripLegacyPointKey(key, "member-read")
	case judgment.CodeConcatOperand:
		anchor.Role = "operator.concat.operand"
		anchor.BindingKey = stripLegacyPointKey(key, "concat")
	case judgment.CodeNonNilAssertion:
		anchor.Role = "assertion.nonnil"
		anchor.BindingKey = stripLegacyPointKey(key, "nonnil")
	case judgment.CodeNumericForOperand:
		anchor.Role = "for.numeric.operand"
		anchor.BindingKey = stripLegacyPointKey(key, "numeric-for")
	case judgment.CodeChannelSelect:
		anchor.Role = "channel.select"
		anchor.BindingKey = stripLegacyPointKey(key, "channel-select")
	case judgment.CodeDiscriminatedUnion:
		anchor.Role = "union.discriminated"
		anchor.BindingKey = stripLegacyPointKey(key, "discriminated-union")
	case judgment.CodeOptional:
		anchor.Role = "union.optional"
		anchor.BindingKey = stripLegacyPointKey(key, "optional")
	case judgment.CodeResultShape:
		anchor.Role = "union.result_shape"
		anchor.BindingKey = stripLegacyPointKey(key, "result-shape")
	case judgment.CodeRegistration:
		anchor.Role = "union.registration"
		anchor.BindingKey = stripLegacyPointKey(key, "registration")
	case judgment.CodeTableDispatch:
		anchor.Role = "union.table_dispatch"
		anchor.BindingKey = stripLegacyPointKey(key, "table-dispatch")
	case judgment.CodeRedundantCondition:
		anchor.Role = "condition.redundant"
	case judgment.CodeAdviceRedundantClaim:
		anchor.Role = "advice.redundant_claim"
	case judgment.CodeAdviceAlwaysTrueGuard:
		anchor.Role = "advice.always_true_guard"
	case judgment.CodeAdviceInvariantLoopRead:
		anchor.Role = "advice.invariant_loop_read"
		anchor.BindingKey = stripLegacyPointKey(key, "advice-loop-read")
	case judgment.CodeAdviceSplitBirthDiscriminant:
		anchor.Role = "advice.split_birth_discriminant"
		anchor.BindingKey = stripLegacyPointKey(key, "advice-split-birth")
	}
	if item.Subject.Label != "" {
		anchor.BindingName = item.Subject.Label
	}
	if anchor.BindingKey != "" {
		rawBindingKey := anchor.BindingKey
		anchor.BindingKey = stableBindingKey(rawBindingKey, item.Subject.Label)
		anchor.BindingKind = subjectBindingKind(rawBindingKey)
	}
	return anchor
}

func stableBindingKey(key, label string) string {
	parts := strings.Split(key, ":")
	if len(parts) == 0 {
		return key
	}
	switch parts[0] {
	case "sym":
		if label != "" {
			return "symbol:" + label
		}
		return ""
	case "local":
		if len(parts) > 1 && parts[1] != "" {
			return "local:" + parts[1]
		}
	case "value":
		if len(parts) > 1 && parts[1] != "" {
			return "value:" + parts[1]
		}
	case "type":
		if len(parts) > 1 && parts[1] != "" {
			return "type:" + parts[1]
		}
	case "ordinary":
		if label != "" {
			return "ordinary:" + label
		}
		return ""
	case "path":
		return key
	}
	if hasTrailingSourceCoordinates(parts) && label != "" {
		return "label:" + label
	}
	return key
}

func hasTrailingSourceCoordinates(parts []string) bool {
	if len(parts) < 3 {
		return false
	}
	trailingInts := 0
	for i := len(parts) - 1; i >= 0; i-- {
		if _, err := strconv.Atoi(parts[i]); err != nil {
			break
		}
		trailingInts++
	}
	return trailingInts >= 2
}

func subjectAnchorModule(ctx Context, item judgment.Judgment) string {
	if ctx.SourceFile != "" {
		return ctx.SourceFile
	}
	for _, span := range item.Spans {
		if span.File != "" {
			return span.File
		}
	}
	return item.Subject.FunctionKey
}

func subjectAnchorFunction(ctx Context, item judgment.Judgment) string {
	if item.Subject.FunctionKey != "" {
		return item.Subject.FunctionKey
	}
	if ctx.FunctionKey != "" {
		return ctx.FunctionKey
	}
	return "body"
}

func callSubjectRoleAndBinding(key string) (string, string) {
	parts := strings.Split(key, ":")
	if len(parts) < 3 || parts[0] != "call" {
		return "call", ""
	}
	switch parts[2] {
	case "arg":
		if len(parts) < 4 {
			return "call.arg", ""
		}
		role := "call.arg:" + parts[3]
		binding := ""
		if len(parts) > 4 && parts[4] == "send" {
			role += ":send"
			if len(parts) > 5 {
				binding = strings.Join(parts[5:], ":")
			}
		} else if len(parts) > 4 {
			role += ":" + strings.Join(parts[4:], ":")
		}
		return role, binding
	case "arity":
		return "call.arity", ""
	case "callee":
		return "call.callee", ""
	default:
		return "call." + parts[2], ""
	}
}

func returnSubjectRole(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) == 3 && parts[0] == "return" {
		return "return.value:" + parts[2]
	}
	return "return.value"
}

func stripLegacyPointKey(key string, prefix string) string {
	prefix += ":"
	if !strings.HasPrefix(key, prefix) {
		return key
	}
	rest := strings.TrimPrefix(key, prefix)
	head, tail, ok := strings.Cut(rest, ":")
	if !ok {
		return ""
	}
	if _, err := strconv.Atoi(head); err == nil {
		return tail
	}
	return rest
}

func subjectBindingKind(key string) string {
	switch {
	case strings.HasPrefix(key, "sym:"):
		return "symbol"
	case strings.HasPrefix(key, "path:"):
		return "path"
	case strings.HasPrefix(key, "local:"):
		return "local"
	case strings.HasPrefix(key, "ordinary:"):
		return "ordinary"
	case strings.HasPrefix(key, "type:"):
		return "type"
	default:
		return "value"
	}
}

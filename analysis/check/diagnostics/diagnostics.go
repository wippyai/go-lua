// Package diagnostics produces checker diagnostics from completed analysis
// results. It is intentionally post-solve: diagnostics may observe facts, but
// they do not publish facts back into the fixed point.
package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	CodeAssignmentType             diagnostic.Code = "type.assignment"
	CodeMissingMember              diagnostic.Code = "type.member.missing"
	CodeOptionalMethodCall         diagnostic.Code = "type.call.optional_receiver"
	CodeNotCallable                diagnostic.Code = "type.call.not_callable"
	CodeDirectCallNotCallable      diagnostic.Code = "type.call.direct.not_callable"
	CodeDirectCallTooFewArgs       diagnostic.Code = "type.call.direct.too_few_args"
	CodeDirectCallTooManyArgs      diagnostic.Code = "type.call.direct.too_many_args"
	CodeDirectCallArgType          diagnostic.Code = "type.call.direct.argument_type"
	CodeReturnContractType         diagnostic.Code = "type.return.contract"
	CodeDirectCallResultAssignment diagnostic.Code = "type.call.direct.result_assignment"
	CodeNumericForOperand          diagnostic.Code = "type.for.numeric_operand"
	CodeChannelSelectExhaustive    diagnostic.Code = "channel.select.exhaustiveness"
	CodeUnresolvedTypeReference    diagnostic.Code = "type.reference.unresolved"
	CodeUnresolvedValueReference   diagnostic.Code = "value.reference.unresolved"
	CodeUnusedLocal                diagnostic.Code = "lint.unused.local"
	CodeDeadAssignment             diagnostic.Code = "lint.dead.assignment"
	CodeRedundantCondition         diagnostic.Code = "lint.condition.redundant"
)

type producerContext struct {
	resolver typeannotation.Resolver
}

// Config controls post-solve diagnostic production.
type Config struct {
	Policy diagnostic.Policy
}

func Produce(result *body.Result) []diagnostic.Diagnostic {
	return ProduceWithConfig(result, Config{})
}

func ProduceWithConfig(result *body.Result, config Config) []diagnostic.Diagnostic {
	return config.Policy.Apply(produceWithResolver(result, nil, nil, config))
}

func produceWithResolver(result *body.Result, parent typeannotation.Resolver, inheritedDefs map[symbol.ID]*ast.FunctionExpr, config Config) []diagnostic.Diagnostic {
	resolver := newResultResolver(result, parent)
	defer releaseGuardEnvironments(result)
	context := producerContext{resolver: resolver}
	defs := directCallDefinitions(result, inheritedDefs)
	var out []diagnostic.Diagnostic
	out = append(out, unresolvedTypeReferences(context).Produce(result)...)
	out = append(out, unresolvedValueReferences(context).Produce(result)...)
	out = append(out, annotationAssignability(context).Produce(result)...)
	out = append(out, produceReturnContract(result, context, defs)...)
	out = append(out, produceDirectCallContract(result, context, defs)...)
	out = append(out, produceCallParamObligations(result, context, defs)...)
	out = append(out, produceDirectCallResultAssignment(result, context, defs)...)
	out = append(out, numericForOperands(context).Produce(result)...)
	out = append(out, memberCall(context).Produce(result)...)
	out = append(out, memberRead(context).Produce(result)...)
	out = append(out, channelSelectExhaustiveness(context).Produce(result)...)
	if config.Policy.Enabled(CodeUnusedLocal, false) {
		out = append(out, unusedLocals(context).Produce(result)...)
	}
	if config.Policy.Enabled(CodeDeadAssignment, false) {
		out = append(out, deadAssignments(context).Produce(result)...)
	}
	if config.Policy.Enabled(CodeRedundantCondition, false) {
		out = append(out, redundantConditions(context).Produce(result)...)
	}
	for _, fn := range result.FunctionResults() {
		out = append(out, produceWithResolver(fn, resolver, defs, config)...)
	}
	return out
}

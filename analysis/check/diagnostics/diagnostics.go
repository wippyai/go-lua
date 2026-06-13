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
	CodeNotCallable                diagnostic.Code = "type.call.not_callable"
	CodeDirectCallNotCallable      diagnostic.Code = "type.call.direct.not_callable"
	CodeDirectCallTooFewArgs       diagnostic.Code = "type.call.direct.too_few_args"
	CodeDirectCallArgType          diagnostic.Code = "type.call.direct.argument_type"
	CodeReturnContractType         diagnostic.Code = "type.return.contract"
	CodeDirectCallResultAssignment diagnostic.Code = "type.call.direct.result_assignment"
	CodeNumericForOperand          diagnostic.Code = "type.for.numeric_operand"
	CodeChannelSelectExhaustive    diagnostic.Code = "channel.select.exhaustiveness"
	CodeUnresolvedTypeReference    diagnostic.Code = "type.reference.unresolved"
	CodeUnresolvedValueReference   diagnostic.Code = "value.reference.unresolved"
)

type producerContext struct {
	resolver typeannotation.Resolver
}

func Produce(result *body.Result) []diagnostic.Diagnostic {
	return produceWithResolver(result, nil, nil)
}

func produceWithResolver(result *body.Result, parent typeannotation.Resolver, inheritedDefs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	resolver := newResultResolver(result, parent)
	context := producerContext{resolver: resolver}
	defs := directCallDefinitions(result, inheritedDefs)
	var out []diagnostic.Diagnostic
	out = append(out, unresolvedTypeReferences(context).Produce(result)...)
	out = append(out, unresolvedValueReferences(context).Produce(result)...)
	out = append(out, annotationAssignability(context).Produce(result)...)
	out = append(out, produceReturnContract(result, context, defs)...)
	out = append(out, produceDirectCallContract(result, context, defs)...)
	out = append(out, produceDirectCallResultAssignment(result, context, defs)...)
	out = append(out, numericForOperands(context).Produce(result)...)
	out = append(out, memberCall(context).Produce(result)...)
	out = append(out, channelSelectExhaustiveness(context).Produce(result)...)
	for _, fn := range result.FunctionResults() {
		out = append(out, produceWithResolver(fn, resolver, defs)...)
	}
	return out
}

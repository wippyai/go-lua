// Package diagnostics produces checker diagnostics from completed analysis
// results. It is intentionally post-solve: diagnostics may observe facts, but
// they do not publish facts back into the fixed point.
package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
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
)

type Config struct {
	Registry *axis.Registry
	Resolver typeannotation.Resolver
}

type Producer interface {
	Produce(*check.Result) []diagnostic.Diagnostic
}

func Produce(result *check.Result, config Config) []diagnostic.Diagnostic {
	return produceWithResolver(result, config, nil)
}

func produceWithResolver(result *check.Result, config Config, parent typeannotation.Resolver) []diagnostic.Diagnostic {
	resolver := newResultResolver(result, config.Resolver, parent)
	config.Resolver = resolver
	var out []diagnostic.Diagnostic
	out = append(out, AnnotationAssignability(config).Produce(result)...)
	out = append(out, ReturnContract(config).Produce(result)...)
	out = append(out, DirectCallContract(config).Produce(result)...)
	out = append(out, DirectCallResultAssignment(config).Produce(result)...)
	out = append(out, MemberCall(config).Produce(result)...)
	for _, fn := range result.FunctionResults() {
		out = append(out, produceWithResolver(fn, config, resolver)...)
	}
	return out
}

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
	CodeAssignmentType               diagnostic.Code = "type.assignment"
	CodeMissingMember                diagnostic.Code = "type.member.missing"
	CodeOptionalMethodCall           diagnostic.Code = "type.call.optional_receiver"
	CodeNotCallable                  diagnostic.Code = "type.call.not_callable"
	CodeDirectCallNotCallable        diagnostic.Code = "type.call.direct.not_callable"
	CodeDirectCallTooFewArgs         diagnostic.Code = "type.call.direct.too_few_args"
	CodeDirectCallTooManyArgs        diagnostic.Code = "type.call.direct.too_many_args"
	CodeDirectCallArgType            diagnostic.Code = "type.call.direct.argument_type"
	CodeReturnContractType           diagnostic.Code = "type.return.contract"
	CodeDirectCallResultAssignment   diagnostic.Code = "type.call.direct.result_assignment"
	CodeOptionalAssignmentTarget     diagnostic.Code = "type.assignment.optional_target"
	CodeConcatOperand                diagnostic.Code = "type.operator.concat_operand"
	CodeNonNilAssertAlwaysNil        diagnostic.Code = "type.assert.nonnil_always_nil"
	CodeNumericForOperand            diagnostic.Code = "type.for.numeric_operand"
	CodeChannelSelectExhaustive      diagnostic.Code = "channel.select.exhaustiveness"
	CodeUnresolvedTypeReference      diagnostic.Code = "type.reference.unresolved"
	CodeUnresolvedValueReference     diagnostic.Code = "value.reference.unresolved"
	CodeUnusedLocal                  diagnostic.Code = "lint.unused.local"
	CodeDeadAssignment               diagnostic.Code = "lint.dead.assignment"
	CodeRedundantCondition           diagnostic.Code = "lint.condition.redundant"
	CodeDiscriminatedUnionExhaustive diagnostic.Code = "lint.union.exhaustiveness"
	CodeFrozenTableMutation          diagnostic.Code = "effect.freeze.mutation"
	CodeResourceUnreleased           diagnostic.Code = "effect.lifecycle.unreleased"
)

type producerContext struct {
	resolver typeannotation.Resolver
	flow     *diagnosticFlowCache
}

type diagnosticProducer struct {
	codes          []diagnostic.Code
	defaultEnabled bool
	produce        func(result *body.Result, context producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic
}

func (p diagnosticProducer) shouldRun(policy diagnostic.Policy) bool {
	if len(p.codes) == 0 {
		return true
	}
	for _, code := range p.codes {
		if policy.Enabled(code, p.defaultEnabled) {
			return true
		}
	}
	return false
}

func diagnosticProducers(context producerContext) []diagnosticProducer {
	return []diagnosticProducer{
		{
			codes:          []diagnostic.Code{CodeUnresolvedTypeReference},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return unresolvedTypeReferences(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeUnresolvedValueReference},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return unresolvedValueReferences(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeAssignmentType, CodeOptionalAssignmentTarget},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return annotationAssignability(context).Produce(result, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeReturnContractType},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceReturnContract(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDirectCallNotCallable, CodeDirectCallTooFewArgs, CodeDirectCallTooManyArgs, CodeDirectCallArgType},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceDirectCallContract(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDirectCallArgType},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceCallParamObligations(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDirectCallResultAssignment},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceDirectCallResultAssignment(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeConcatOperand},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return concatOperands(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeNumericForOperand},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return numericForOperands(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeNonNilAssertAlwaysNil},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return nonNilAssertions(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeOptionalMethodCall, CodeMissingMember, CodeNotCallable},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return memberCall(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeMissingMember},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return memberRead(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeChannelSelectExhaustive},
			defaultEnabled: true,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return channelSelectExhaustiveness(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeUnusedLocal},
			defaultEnabled: false,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return unusedLocals{}.Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDeadAssignment},
			defaultEnabled: false,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return deadAssignments{}.Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeRedundantCondition},
			defaultEnabled: false,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return redundantConditions(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDiscriminatedUnionExhaustive},
			defaultEnabled: false,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return discriminatedUnionExhaustiveness(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeFrozenTableMutation},
			defaultEnabled: false,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return frozenTableMutations(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeResourceUnreleased},
			defaultEnabled: false,
			produce: func(result *body.Result, _ producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return lifecycleObligations(context).Produce(result)
			},
		},
	}
}

// Config controls post-solve diagnostic production.
type Config struct {
	Policy diagnostic.Policy
}

func Produce(result *body.Result) []diagnostic.Diagnostic {
	return ProduceWithConfig(result, Config{})
}

func ProduceWithConfig(result *body.Result, config Config) []diagnostic.Diagnostic {
	out := config.Policy.Apply(produceWithResolver(result, nil, nil, config))
	diagnostic.Sort(out)
	return out
}

func produceWithResolver(result *body.Result, parent typeannotation.Resolver, inheritedDefs map[symbol.ID]*ast.FunctionExpr, config Config) []diagnostic.Diagnostic {
	resolver := newResultResolver(result, parent)
	defer releaseGuardEnvironments(result)
	context := producerContext{resolver: resolver, flow: newDiagnosticFlowCache(result)}
	defs := directCallDefinitions(result, inheritedDefs)
	var out []diagnostic.Diagnostic
	for _, producer := range diagnosticProducers(context) {
		if !producer.shouldRun(config.Policy) {
			continue
		}
		out = append(out, producer.produce(result, context, defs)...)
	}
	for _, fn := range result.FunctionResults() {
		out = append(out, produceWithResolver(fn, resolver, defs, config)...)
	}
	return out
}

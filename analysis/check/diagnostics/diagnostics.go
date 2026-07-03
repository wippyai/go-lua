// Package diagnostics produces checker diagnostics from completed analysis
// results. It is intentionally post-solve: diagnostics may observe facts, but
// they do not publish facts back into the fixed point.
package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
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
	parent  *body.Result
	parents []*body.Result

	judgmentPolicy     judgment.Policy
	judgmentStrictness judgment.StrictnessMode
}

type diagnosticProducer struct {
	codes          []diagnostic.Code
	defaultEnabled bool
	produce        func(result *body.Result, context producerContext) []diagnostic.Diagnostic
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

func diagnosticProducers() []diagnosticProducer {
	return []diagnosticProducer{
		{
			codes:          []diagnostic.Code{CodeUnresolvedTypeReference},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceUnresolvedTypeJudgmentDiagnosticsWithPolicy(result, context.parent, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeUnresolvedValueReference},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceUnresolvedValueJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeAssignmentType, CodeOptionalAssignmentTarget, CodeDirectCallResultAssignment},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceAssignmentJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeReturnContractType},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceReturnJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes: []diagnostic.Code{
				CodeDirectCallNotCallable,
				CodeDirectCallTooFewArgs,
				CodeDirectCallTooManyArgs,
				CodeDirectCallArgType,
				CodeOptionalMethodCall,
				CodeMissingMember,
				CodeNotCallable,
			},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				if result == nil {
					return nil
				}
				return produceDirectCallContractJudgmentDiagnosticsWithPolicy(
					result,
					"",
					context.judgmentPolicy,
					context.judgmentStrictness,
				)
			},
		},
		{
			codes:          []diagnostic.Code{CodeConcatOperand},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceConcatOperandJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeNumericForOperand},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceNumericForJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeNonNilAssertAlwaysNil},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceNonNilAssertionJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeMissingMember},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceMemberReadJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeChannelSelectExhaustive},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceChannelSelectJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeUnusedLocal},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceUnusedLocalJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDeadAssignment},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceDeadAssignmentJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeRedundantCondition},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				if result == nil {
					return nil
				}
				query := newDiagnosticQuery(result, context.parents...)
				items := pass.New(pass.RedundantConditions{}).Run(pass.Context{
					Reader:      query.reader,
					FunctionKey: "body",
					SourceFile:  "",
				})
				return renderJudgmentDiagnostics(items, context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDiscriminatedUnionExhaustive},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				if result == nil {
					return nil
				}
				query := newDiagnosticQuery(result, context.parents...)
				items := pass.New(
					pass.DiscriminatedUnions{},
					pass.Optionals{},
					pass.ResultShapes{},
					pass.Registrations{},
					pass.TableDispatches{},
				).Run(pass.Context{
					Reader:      query.reader,
					FunctionKey: "body",
					SourceFile:  "",
				})
				out := renderJudgmentDiagnostics(items, context.judgmentPolicy, context.judgmentStrictness)
				return out
			},
		},
		{
			codes:          []diagnostic.Code{CodeFrozenTableMutation},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceFrozenTableJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
		{
			codes:          []diagnostic.Code{CodeResourceUnreleased},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
				return produceLifecycleJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
			},
		},
	}
}

// Config controls post-solve diagnostic production.
type Config struct {
	Policy diagnostic.Policy

	// JudgmentPolicy maps post-solve semantic judgment verdicts to diagnostic
	// levels. The zero value uses judgment.DefaultPolicy.
	JudgmentPolicy judgment.Policy

	// JudgmentStrictness selects the judgment-policy mode for unknown
	// obligations. The zero value is judgment.StrictnessDefault.
	JudgmentStrictness judgment.StrictnessMode
}

func Produce(result *body.Result) []diagnostic.Diagnostic {
	return ProduceWithConfig(result, Config{})
}

func ProduceWithConfig(result *body.Result, config Config) []diagnostic.Diagnostic {
	out := config.Policy.Apply(produceWithParents(result, config, nil))
	out = diagnostic.Deduplicate(out)
	diagnostic.Sort(out)
	out = applyDiagnosticPrecedence(out, defaultDiagnosticPrecedenceRules())
	out = diagnostic.CoalesceSamePrimary(out)
	return out
}

func produceWithParents(
	result *body.Result,
	config Config,
	parentResult *body.Result,
	parentResults ...*body.Result,
) []diagnostic.Diagnostic {
	context := producerContext{
		parent:  parentResult,
		parents: append([]*body.Result(nil), parentResults...),

		judgmentPolicy:     normalizedJudgmentPolicy(config.JudgmentPolicy),
		judgmentStrictness: config.JudgmentStrictness,
	}
	var out []diagnostic.Diagnostic
	for _, producer := range diagnosticProducers() {
		if !producer.shouldRun(config.Policy) {
			continue
		}
		out = append(out, producer.produce(result, context)...)
	}
	for _, fn := range diagnosticFunctionResults(result) {
		childParents := append(append([]*body.Result(nil), parentResults...), result)
		out = append(out, produceWithParents(fn, config, result, childParents...)...)
	}
	return out
}

func diagnosticFunctionResults(result *body.Result) []*body.Result {
	functions := result.FunctionResults()
	if len(functions) == 0 {
		return nil
	}
	hasContext := make(map[*ast.FunctionExpr]struct{})
	for _, fn := range functions {
		if fn == nil || !fn.IsCallContextResult() {
			continue
		}
		if expr := fn.Function(); expr != nil {
			hasContext[expr] = struct{}{}
		}
	}
	if len(hasContext) == 0 {
		return functions
	}
	out := make([]*body.Result, 0, len(functions))
	for _, fn := range functions {
		if fn == nil {
			continue
		}
		if !fn.IsCallContextResult() {
			if expr := fn.Function(); expr != nil {
				if _, ok := hasContext[expr]; ok && !hasImplicitSelfEntrySurface(fn) && !hasExplicitValidationResultSurface(fn) {
					continue
				}
			}
		}
		out = append(out, fn)
	}
	return out
}

func hasExplicitValidationResultSurface(result *body.Result) bool {
	if result == nil {
		return false
	}
	fn := result.Function()
	if fn == nil {
		return false
	}
	if len(fn.TypeParams) != 0 {
		return false
	}
	if len(fn.ReturnTypes) != 0 {
		return true
	}
	for _, slot := range result.FunctionParamSlots(fn) {
		if !slot.ImplicitSelf && slot.Type != nil {
			return true
		}
	}
	if fn.ParList == nil {
		return false
	}
	for _, expr := range fn.ParList.Types {
		if expr != nil {
			return true
		}
	}
	return fn.ParList.VarargType != nil
}

func hasImplicitSelfEntrySurface(result *body.Result) bool {
	if result == nil {
		return false
	}
	fn := result.Function()
	if fn == nil {
		return false
	}
	self := symbol.ID(0)
	for _, slot := range result.FunctionParamSlots(fn) {
		if slot.ImplicitSelf && slot.Symbol != 0 {
			self = slot.Symbol
			break
		}
	}
	if self == 0 {
		return false
	}
	entry, ok := result.EntryState()
	if !ok {
		return false
	}
	ks := result.KeySpace()
	if ks == nil {
		return false
	}
	found := false
	entry.ForEachPathStaticMember(func(memberKey keyspace.Key, _ product.Value) bool {
		if memberKey.Sym != self {
			return true
		}
		switch memberKey.Kind {
		case keyspace.KindResolverSym, keyspace.KindStableSym:
			if len(ks.Segments(memberKey)) != 0 {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

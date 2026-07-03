// Package diagnostics produces checker diagnostics from completed analysis
// results. It is intentionally post-solve: diagnostics may observe facts, but
// they do not publish facts back into the fixed point.
package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
	resolver          typeannotation.Resolver
	flow              *diagnosticFlowCache
	guards            *diagnosticGuardCache
	directDefinitions *directCallDefinitionCache
	root              *body.Result
	parent            *body.Result
	rootFlow          *diagnosticFlowCache
	dispatchTables    map[pathdom.PathKey]dispatchTableSummary
	callContextResult bool

	judgmentPolicy     judgment.Policy
	judgmentStrictness judgment.StrictnessMode

	useAssignmentJudgments bool
}

func (c producerContext) guardEnvironments(result *body.Result) map[cfg.Point]guardEnv {
	return c.guards.environments(result)
}

func (c producerContext) guardEnv(result *body.Result, point cfg.Point) guardEnv {
	if envs := c.guardEnvironments(result); envs != nil {
		return envs[point]
	}
	return guardEnv{}
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

func diagnosticProducers() []diagnosticProducer {
	return []diagnosticProducer{
		{
			codes:          []diagnostic.Code{CodeUnresolvedTypeReference},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return unresolvedTypeReferences(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeUnresolvedValueReference},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return unresolvedValueReferences(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeAssignmentType, CodeOptionalAssignmentTarget},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				if context.useAssignmentJudgments {
					return produceAssignmentJudgmentDiagnosticsWithPolicy(result, "", context.judgmentPolicy, context.judgmentStrictness)
				}
				return annotationAssignability(context).Produce(result, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeReturnContractType},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceReturnContract(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDirectCallNotCallable, CodeDirectCallTooFewArgs, CodeDirectCallTooManyArgs, CodeDirectCallArgType},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceDirectCallContract(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDirectCallResultAssignment},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, defs map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return produceDirectCallResultAssignment(result, context, defs)
			},
		},
		{
			codes:          []diagnostic.Code{CodeConcatOperand},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return concatOperands(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeNumericForOperand},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return numericForOperands(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeNonNilAssertAlwaysNil},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return nonNilAssertions(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeOptionalMethodCall, CodeMissingMember, CodeNotCallable},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return memberCall(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeMissingMember},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return memberRead(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeChannelSelectExhaustive},
			defaultEnabled: true,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return channelSelectExhaustiveness(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeUnusedLocal},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return unusedLocals(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDeadAssignment},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return deadAssignments(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeRedundantCondition},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return redundantConditions(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeDiscriminatedUnionExhaustive},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return discriminatedUnionExhaustiveness(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeFrozenTableMutation},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return frozenTableMutations(context).Produce(result)
			},
		},
		{
			codes:          []diagnostic.Code{CodeResourceUnreleased},
			defaultEnabled: false,
			produce: func(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
				return lifecycleObligations(context).Produce(result)
			},
		},
	}
}

// Config controls post-solve diagnostic production.
type Config struct {
	Policy diagnostic.Policy

	// UseAssignmentJudgments routes annotated local assignment diagnostics
	// through post-solve judgments. The assignment producer is flipped to
	// judgments by default; this field is retained only for migration callers
	// that already pass it explicitly.
	UseAssignmentJudgments bool

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
	rootFlow := newDiagnosticFlowCache(result)
	guards := newDiagnosticGuardCache()
	directDefinitions := newDirectCallDefinitionCache()
	out := config.Policy.Apply(produceWithResolver(result, nil, nil, nil, config, result, nil, rootFlow, guards, directDefinitions))
	out = diagnostic.Deduplicate(out)
	diagnostic.Sort(out)
	out = applyDiagnosticPrecedence(out, defaultDiagnosticPrecedenceRules())
	out = diagnostic.CoalesceSamePrimary(out)
	return out
}

func produceWithResolver(
	result *body.Result,
	parent typeannotation.Resolver,
	inheritedDefs map[symbol.ID]*ast.FunctionExpr,
	inheritedDispatchTables map[pathdom.PathKey]dispatchTableSummary,
	config Config,
	root *body.Result,
	parentResult *body.Result,
	rootFlow *diagnosticFlowCache,
	guards *diagnosticGuardCache,
	directDefinitions *directCallDefinitionCache,
) []diagnostic.Diagnostic {
	resolver := newResultResolver(result, parent)
	flow := newDiagnosticFlowCache(result)
	if guards == nil {
		guards = newDiagnosticGuardCache()
	}
	if directDefinitions == nil {
		directDefinitions = newDirectCallDefinitionCache()
	}
	if root == nil {
		root = result
	}
	if rootFlow == nil {
		rootFlow = newDiagnosticFlowCache(root)
	}
	childDispatchTables := collectDispatchTableSummaries(result, inheritedDispatchTables)
	context := producerContext{
		resolver:           resolver,
		flow:               flow,
		guards:             guards,
		directDefinitions:  directDefinitions,
		root:               root,
		parent:             parentResult,
		rootFlow:           rootFlow,
		dispatchTables:     cloneDispatchTableSummaries(inheritedDispatchTables),
		callContextResult:  result.IsCallContextResult(),
		judgmentPolicy:     normalizedJudgmentPolicy(config.JudgmentPolicy),
		judgmentStrictness: config.JudgmentStrictness,

		useAssignmentJudgments: true,
	}
	defs := directCallDefinitions(result, context, inheritedDefs)
	var out []diagnostic.Diagnostic
	for _, producer := range diagnosticProducers() {
		if !producer.shouldRun(config.Policy) {
			continue
		}
		out = append(out, producer.produce(result, context, defs)...)
	}
	for _, fn := range diagnosticFunctionResults(result) {
		out = append(out, produceWithResolver(fn, resolver, defs, childDispatchTables, config, root, result, rootFlow, guards, directDefinitions)...)
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

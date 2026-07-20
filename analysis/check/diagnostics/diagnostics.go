// Package diagnostics produces checker diagnostics from completed analysis
// results. It is intentionally post-solve: diagnostics may observe facts, but
// they do not publish facts back into the fixed point.
package diagnostics

import (
	"context"
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

const (
	CodeAssignmentType               diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeAssignmentType)
	CodeMissingMember                diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeMissingMember)
	CodeOptionalMethodCall           diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeOptionalMethodCall)
	CodeNilUnsafeUse                 diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeNilUnsafeUse)
	CodeNotCallable                  diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeNotCallable)
	CodeDirectCallNotCallable        diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDirectCallNotCallable)
	CodeDirectCallTooFewArgs         diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDirectCallTooFewArgs)
	CodeDirectCallTooManyArgs        diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDirectCallTooManyArgs)
	CodeDirectCallArgType            diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDirectCallArgType)
	CodeReturnContractType           diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeReturnContractType)
	CodeDirectCallResultAssignment   diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDirectCallResultAssignment)
	CodeOptionalAssignmentTarget     diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeOptionalAssignmentTarget)
	CodeConcatOperand                diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeConcatOperand)
	CodeNonNilAssertAlwaysNil        diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeNonNilAssertAlwaysNil)
	CodeNumericForOperand            diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeNumericForOperand)
	CodeChannelSelectExhaustive      diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeChannelSelectExhaustive)
	CodeChannelSendClosed            diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeChannelSendClosed)
	CodeChannelDoubleClose           diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeChannelDoubleClose)
	CodeUnresolvedTypeReference      diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeUnresolvedTypeReference)
	CodeUnresolvedValueReference     diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeUnresolvedValueReference)
	CodeUnusedLocal                  diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeUnusedLocal)
	CodeDeadAssignment               diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDeadAssignment)
	CodeRedundantCondition           diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeRedundantCondition)
	CodeDiscriminatedUnionExhaustive diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeDiscriminatedUnionExhaustive)
	CodeFrozenTableMutation          diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeFrozenTableMutation)
	CodeResourceUnreleased           diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeResourceUnreleased)
	CodeTypestateInvalidTransition   diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeTypestateInvalidTransition)
	CodeTypestateInvalidRequirement  diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeTypestateInvalidRequirement)
	CodeTypestateUnprovenRequirement diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeTypestateUnprovenRequirement)
	CodeSendIsolation                diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeSendIsolation)
	CodeAdviceRedundantClaim         diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeAdviceRedundantClaim)
	CodeAdviceAlwaysTrueGuard        diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeAdviceAlwaysTrueGuard)
	CodeAdviceInvariantLoopRead      diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeAdviceInvariantLoopRead)
	CodeAdviceSplitBirthDiscriminant diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeAdviceSplitBirthDiscriminant)
	CodeAdviceShapePolymorphic       diagnostic.Code = diagnostic.Code(judgment.DiagnosticCodeAdviceShapePolymorphic)
)

type producerContext struct {
	parent      *body.Result
	parents     []*body.Result
	sourceFile  string
	displayFile string

	judgmentPolicy judgment.PolicyConfig

	canceled func() bool
}

type diagnosticProducer struct {
	codes          []diagnostic.Code
	defaultEnabled bool
	judgments      func(result *body.Result, context producerContext) []judgment.Judgment
	produce        func(result *body.Result, context producerContext) []diagnostic.Diagnostic
}

func judgmentProducer(judgmentCodes []judgment.Code, producers ...pass.Producer) diagnosticProducer {
	return diagnosticProducerForJudgments(judgmentCodes, func(result *body.Result, context producerContext) []judgment.Judgment {
		return context.judgments(result, producers...)
	})
}

func diagnosticProducerForJudgments(
	judgmentCodes []judgment.Code,
	produce func(result *body.Result, context producerContext) []judgment.Judgment,
) diagnosticProducer {
	out := diagnosticProducer{
		codes:          diagnosticCodesForJudgments(judgmentCodes...),
		defaultEnabled: defaultDiagnosticEnabledForJudgments(judgmentCodes...),
		judgments:      produce,
	}
	out.produce = func(result *body.Result, context producerContext) []diagnostic.Diagnostic {
		return renderJudgmentDiagnostics(produce(result, context), context.judgmentPolicy.Policy, context.judgmentPolicy.Strictness)
	}
	return out
}

func diagnosticCodesForJudgments(judgmentCodes ...judgment.Code) []diagnostic.Code {
	var out []diagnostic.Code
	seen := make(map[diagnostic.Code]struct{})
	for _, judgmentCode := range judgmentCodes {
		spec, ok := judgment.DefaultRegistry().Lookup(judgmentCode)
		if !ok {
			panic("diagnostics: unknown judgment code " + string(judgmentCode))
		}
		for _, code := range spec.DiagnosticCodes {
			diagnosticCode := diagnostic.Code(code)
			if _, ok := seen[diagnosticCode]; ok {
				continue
			}
			seen[diagnosticCode] = struct{}{}
			out = append(out, diagnosticCode)
		}
	}
	return out
}

func defaultDiagnosticEnabledForJudgments(judgmentCodes ...judgment.Code) bool {
	var (
		out bool
		set bool
	)
	for _, judgmentCode := range judgmentCodes {
		spec, ok := judgment.DefaultRegistry().Lookup(judgmentCode)
		if !ok {
			panic("diagnostics: unknown judgment code " + string(judgmentCode))
		}
		if !set {
			out = spec.DiagnosticDefault == judgment.DiagnosticDefaultEnabled
			set = true
			continue
		}
		if out != (spec.DiagnosticDefault == judgment.DiagnosticDefaultEnabled) {
			panic("diagnostics: mixed default diagnostic policy for judgment producer")
		}
	}
	if !set {
		return true
	}
	return out
}

func parentJudgmentProducer(judgmentCode judgment.Code, producers ...pass.Producer) diagnosticProducer {
	return diagnosticProducerForJudgments([]judgment.Code{judgmentCode}, func(result *body.Result, context producerContext) []judgment.Judgment {
		return context.judgmentsWithParent(result, context.parent, producers...)
	})
}

func reachableJudgmentProducer(judgmentCode judgment.Code, producers ...pass.Producer) diagnosticProducer {
	return diagnosticProducerForJudgments([]judgment.Code{judgmentCode}, func(result *body.Result, context producerContext) []judgment.Judgment {
		return context.reachableJudgments(result, producers...)
	})
}

func parentStackJudgmentProducer(judgmentCodes []judgment.Code, producers ...pass.Producer) diagnosticProducer {
	return diagnosticProducerForJudgments(judgmentCodes, func(result *body.Result, context producerContext) []judgment.Judgment {
		if result == nil {
			return nil
		}
		return context.judgmentsWithParents(result, context.parents, producers...)
	})
}

func directCallContractJudgmentProducer() diagnosticProducer {
	return diagnosticProducerForJudgments([]judgment.Code{
		judgment.CodeCallCallee,
		judgment.CodeCallArity,
		judgment.CodeCallArgType,
	}, func(result *body.Result, context producerContext) []judgment.Judgment {
		if result == nil {
			return nil
		}
		return context.directCallContractJudgments(result)
	})
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
		parentJudgmentProducer(judgment.CodeUnresolvedType, pass.UnresolvedTypes{}),
		judgmentProducer([]judgment.Code{judgment.CodeUnresolvedValue}, pass.UnresolvedValues{}),
		judgmentProducer(
			[]judgment.Code{
				judgment.CodeAssignment,
				judgment.CodeAssignmentTarget,
				judgment.CodeReturn,
				judgment.CodeConcatOperand,
			},
			pass.Assignments{},
			pass.Returns{},
			pass.ConcatOperands{},
		),
		directCallContractJudgmentProducer(),
		judgmentProducer([]judgment.Code{judgment.CodeNumericForOperand}, pass.NumericForOperands{}),
		judgmentProducer([]judgment.Code{judgment.CodeNonNilAssertion}, pass.NonNilAssertions{}),
		reachableJudgmentProducer(judgment.CodeMemberRead, pass.MemberReads{}),
		judgmentProducer([]judgment.Code{judgment.CodeChannelSelect}, pass.ChannelSelects{}),
		judgmentProducer(
			[]judgment.Code{judgment.CodeChannelSendClosed, judgment.CodeChannelDoubleClose},
			pass.ChannelLifecycles{},
		),
		judgmentProducer([]judgment.Code{judgment.CodeUnusedLocal}, pass.UnusedLocals{}),
		judgmentProducer([]judgment.Code{judgment.CodeDeadAssignment}, pass.DeadAssignments{}),
		parentStackJudgmentProducer([]judgment.Code{judgment.CodeRedundantCondition}, pass.RedundantConditions{}),
		parentStackJudgmentProducer(
			[]judgment.Code{
				judgment.CodeDiscriminatedUnion,
				judgment.CodeOptional,
				judgment.CodeResultShape,
				judgment.CodeRegistration,
				judgment.CodeTableDispatch,
			},
			pass.DiscriminatedUnions{},
			pass.Optionals{},
			pass.ResultShapes{},
			pass.Registrations{},
			pass.TableDispatches{},
		),
		judgmentProducer([]judgment.Code{judgment.CodeFrozenTable}, pass.FrozenTableMutations{}),
		judgmentProducer([]judgment.Code{judgment.CodeTypestateInvalidTransition}, pass.TypestateInvalidTransitions{}),
		judgmentProducer([]judgment.Code{judgment.CodeTypestateInvalidRequirement, judgment.CodeTypestateUnprovenRequirement}, pass.TypestateRequirements{}),
		judgmentProducer([]judgment.Code{judgment.CodeLifecycle}, pass.LifecycleObligations{}),
		judgmentProducer([]judgment.Code{judgment.CodeSendIsolation}, pass.SendSafety{}),
		judgmentProducer(
			[]judgment.Code{
				judgment.CodeAdviceRedundantClaim,
				judgment.CodeAdviceAlwaysTrueGuard,
				judgment.CodeAdviceInvariantLoopRead,
				judgment.CodeAdviceSplitBirthDiscriminant,
				judgment.CodeAdviceShapePolymorphic,
			},
			pass.AdviceRedundantClaims{},
			pass.AdviceAlwaysTrueGuards{},
			pass.AdviceInvariantLoopReads{},
			pass.AdviceSplitBirthDiscriminants{},
			pass.AdviceShapePolymorphic{},
		),
	}
}

// Config controls post-solve diagnostic production.
type Config struct {
	Policy diagnostic.Policy
	// SourceFile is the display label carried by source-backed judgment and
	// evidence spans. It does not participate in semantic identity.
	SourceFile string

	// Judgment maps post-solve semantic judgment verdicts to diagnostic levels.
	// The zero value uses judgment.DefaultPolicy in StrictnessDefault mode.
	Judgment judgment.PolicyConfig
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

// ProduceJudgments runs the canonical obligation producer set over result and
// its reportable nested bodies. The returned semantic records have stable
// subject anchors and per-body ResultVersion values and have not been filtered
// by diagnostic policy.
func ProduceJudgments(result *body.Result, sourceFile string) []judgment.Judgment {
	return produceJudgmentsWithParents(result, sourceFile, nil)
}

// ProduceJudgmentsContext is the cancelable counterpart of ProduceJudgments.
// Diagnostics are post-solve work, but they remain part of one analysis
// request and must not outlive its solve deadline.
func ProduceJudgmentsContext(ctx context.Context, result *body.Result, sourceFile string) ([]judgment.Judgment, error) {
	if ctx == nil {
		return ProduceJudgments(result, sourceFile), nil
	}
	items := produceJudgmentsWithContext(ctx, result, sourceFile, nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func produceJudgmentsWithParents(
	result *body.Result,
	sourceFile string,
	parentResult *body.Result,
	parentResults ...*body.Result,
) []judgment.Judgment {
	context := producerContext{
		parent:     parentResult,
		parents:    append([]*body.Result(nil), parentResults...),
		sourceFile: sourceFile,
	}
	var out []judgment.Judgment
	for _, producer := range diagnosticProducers() {
		if producer.judgments == nil {
			continue
		}
		out = append(out, producer.judgments(result, context)...)
	}
	if result == nil {
		return out
	}
	for _, fn := range result.FunctionResults() {
		childParents := append(append([]*body.Result(nil), parentResults...), result)
		out = append(out, produceJudgmentsWithParents(fn, sourceFile, result, childParents...)...)
	}
	return out
}

func produceJudgmentsWithContext(ctx context.Context, result *body.Result, sourceFile string, parentResult *body.Result, parentResults ...*body.Result) []judgment.Judgment {
	if ctx != nil && ctx.Err() != nil {
		return nil
	}
	context := producerContext{
		parent: parentResult, parents: append([]*body.Result(nil), parentResults...), sourceFile: sourceFile,
		canceled: func() bool { return ctx != nil && ctx.Err() != nil },
	}
	var out []judgment.Judgment
	for _, producer := range diagnosticProducers() {
		if context.canceled() || producer.judgments == nil {
			break
		}
		out = append(out, producer.judgments(result, context)...)
	}
	if result == nil || context.canceled() {
		return out
	}
	for _, fn := range result.FunctionResults() {
		childParents := append(append([]*body.Result(nil), parentResults...), result)
		out = append(out, produceJudgmentsWithContext(ctx, fn, sourceFile, result, childParents...)...)
		if context.canceled() {
			break
		}
	}
	return out
}

func produceWithParents(
	result *body.Result,
	config Config,
	parentResult *body.Result,
	parentResults ...*body.Result,
) []diagnostic.Diagnostic {
	context := producerContext{
		parent:      parentResult,
		parents:     append([]*body.Result(nil), parentResults...),
		displayFile: config.SourceFile,

		judgmentPolicy: config.Judgment.Normalized(),
	}
	out := produceOne(result, config, context)
	if result == nil {
		return out
	}
	for _, fn := range result.FunctionResults() {
		childParents := append(append([]*body.Result(nil), parentResults...), result)
		out = append(out, produceWithParents(fn, config, result, childParents...)...)
	}
	return out
}

func produceOne(result *body.Result, config Config, context producerContext) []diagnostic.Diagnostic {
	var out []diagnostic.Diagnostic
	for _, producer := range diagnosticProducers() {
		if !producer.shouldRun(config.Policy) {
			continue
		}
		out = append(out, producer.produce(result, context)...)
	}
	return out
}

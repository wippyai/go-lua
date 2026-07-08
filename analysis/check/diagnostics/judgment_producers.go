package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	internalreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceJudgmentsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(judgmentContext(result, sourceFile)), policy, mode)
}

func produceReachableCallJudgmentsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(reachableCallJudgmentContext(result, sourceFile)), policy, mode)
}

func judgmentContext(result *body.Result, sourceFile string) pass.Context {
	return judgmentContextWithParents(result, nil, sourceFile)
}

func judgmentContextWithParents(result *body.Result, parents []*body.Result, sourceFile string) pass.Context {
	return pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      internalreadmodel.NewWithParents(result, parents...),
	}
}

func reachableJudgmentContext(result *body.Result, sourceFile string) pass.Context {
	ctx := judgmentContext(result, sourceFile)
	if result != nil {
		ctx.PointReachable = result.PointNormallyReachable
	}
	return ctx
}

func reachableCallJudgmentContext(result *body.Result, sourceFile string) pass.Context {
	ctx := reachableJudgmentContext(result, sourceFile)
	if result != nil {
		ctx.SuppressCallerOwnedParameters = result.IsCallContextResult()
	}
	return ctx
}

func (c producerContext) produceJudgments(result *body.Result, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(judgmentContext(result, "")), c.judgmentPolicy, c.judgmentStrictness)
}

func (c producerContext) produceJudgmentsWithParent(result, parent *body.Result, producers ...pass.Producer) []diagnostic.Diagnostic {
	items := pass.New(producers...).Run(pass.Context{
		Reader: internalreadmodel.NewWithParents(result, parent),
	})
	return renderJudgmentDiagnostics(items, c.judgmentPolicy, c.judgmentStrictness)
}

func (c producerContext) produceJudgmentsWithParents(result *body.Result, parents []*body.Result, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(judgmentContextWithParents(result, parents, "")), c.judgmentPolicy, c.judgmentStrictness)
}

func (c producerContext) produceReachableJudgments(result *body.Result, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(reachableJudgmentContext(result, "")), c.judgmentPolicy, c.judgmentStrictness)
}

func (c producerContext) produceDirectCallContractJudgments(result *body.Result) []diagnostic.Diagnostic {
	ctx := reachableCallJudgmentContext(result, "")
	items := firstDirectCallContractJudgmentPerCall(
		pass.New(pass.CallCallee{}).Run(ctx),
		pass.New(pass.CallArity{}).Run(ctx),
		pass.New(pass.CallArguments{}).Run(ctx),
	)
	return renderJudgmentDiagnostics(items, c.judgmentPolicy, c.judgmentStrictness)
}

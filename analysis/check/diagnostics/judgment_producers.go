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
		FunctionKey:   sourceFile,
		SourceFile:    sourceFile,
		ResultVersion: resultVersion(result),
		Reader:        internalreadmodel.NewWithParents(result, parents...),
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

func (c producerContext) judgments(result *body.Result, producers ...pass.Producer) []judgment.Judgment {
	return pass.New(producers...).Run(judgmentContext(result, c.sourceFile))
}

func (c producerContext) judgmentsWithParent(result, parent *body.Result, producers ...pass.Producer) []judgment.Judgment {
	return pass.New(producers...).Run(pass.Context{
		FunctionKey:   c.sourceFile,
		SourceFile:    c.sourceFile,
		ResultVersion: resultVersion(result),
		Reader:        internalreadmodel.NewWithParents(result, parent),
	})
}

func (c producerContext) judgmentsWithParents(result *body.Result, parents []*body.Result, producers ...pass.Producer) []judgment.Judgment {
	return pass.New(producers...).Run(judgmentContextWithParents(result, parents, c.sourceFile))
}

func (c producerContext) reachableJudgments(result *body.Result, producers ...pass.Producer) []judgment.Judgment {
	return pass.New(producers...).Run(reachableJudgmentContext(result, c.sourceFile))
}

func (c producerContext) directCallContractJudgments(result *body.Result) []judgment.Judgment {
	ctx := reachableCallJudgmentContext(result, c.sourceFile)
	return firstDirectCallContractJudgmentPerCall(
		pass.New(pass.CallCallee{}).Run(ctx),
		pass.New(pass.CallArity{}).Run(ctx),
		pass.New(pass.CallArguments{}).Run(ctx),
	)
}

func resultVersion(result *body.Result) uint64 {
	if result == nil {
		return 0
	}
	return result.ResultVersion()
}

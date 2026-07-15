package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	internalreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/embedding"
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
		FunctionKey:     sourceFile,
		SourceFile:      sourceFile,
		BodyInputDigest: bodyInputDigest(result),
		Reader:          internalreadmodel.NewWithParents(result, parents...),
	}
}

func judgmentContextWithCancellation(result *body.Result, parents []*body.Result, sourceFile string, canceled func() bool) pass.Context {
	ctx := judgmentContextWithParents(result, parents, sourceFile)
	ctx.Canceled = canceled
	return ctx
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
	return pass.New(producers...).Run(c.judgmentContext(result, nil))
}

func (c producerContext) judgmentsWithParent(result, parent *body.Result, producers ...pass.Producer) []judgment.Judgment {
	// This is the positive owner for the one-parent diagnostic projection. Keep
	// construction here so diagnostics cannot bypass the read-model boundary.
	ctx := pass.Context{
		FunctionKey:     c.sourceFile,
		SourceFile:      c.sourceDisplayFile(),
		BodyInputDigest: bodyInputDigest(result),
		Reader:          internalreadmodel.NewWithParents(result, parent),
		Canceled:        c.canceled,
	}
	return pass.New(producers...).Run(ctx)
}

func (c producerContext) judgmentsWithParents(result *body.Result, parents []*body.Result, producers ...pass.Producer) []judgment.Judgment {
	return pass.New(producers...).Run(c.judgmentContext(result, parents))
}

func (c producerContext) reachableJudgments(result *body.Result, producers ...pass.Producer) []judgment.Judgment {
	ctx := c.judgmentContext(result, nil)
	if result != nil {
		ctx.PointReachable = result.PointNormallyReachable
	}
	ctx.Canceled = c.canceled
	return pass.New(producers...).Run(ctx)
}

func (c producerContext) directCallContractJudgments(result *body.Result) []judgment.Judgment {
	ctx := c.judgmentContext(result, nil)
	if result != nil {
		ctx.PointReachable = result.PointNormallyReachable
		ctx.SuppressCallerOwnedParameters = result.IsCallContextResult()
	}
	ctx.Canceled = c.canceled
	return firstDirectCallContractJudgmentPerCall(
		pass.New(pass.CallCallee{}).Run(ctx),
		pass.New(pass.CallArity{}).Run(ctx),
		pass.New(pass.CallArguments{}).Run(ctx),
	)
}

func (c producerContext) sourceDisplayFile() string {
	if c.displayFile != "" {
		return c.displayFile
	}
	return c.sourceFile
}

func (c producerContext) judgmentContext(result *body.Result, parents []*body.Result) pass.Context {
	ctx := judgmentContextWithCancellation(result, parents, c.sourceFile, c.canceled)
	ctx.SourceFile = c.sourceDisplayFile()
	return ctx
}

func bodyInputDigest(result *body.Result) embedding.BodyInputDigest {
	if result == nil {
		return 0
	}
	return embedding.BodyInputDigest(result.ResultVersion())
}

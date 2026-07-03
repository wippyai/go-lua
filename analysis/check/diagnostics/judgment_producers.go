package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	internalreadmodel "github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func produceAssignmentJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	return produceAssignmentJudgmentDiagnosticsWithPolicy(result, sourceFile, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func produceAssignmentJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.Assignments{})
}

func produceReturnJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	return produceReturnJudgmentDiagnosticsWithPolicy(result, sourceFile, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func produceReturnJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.Returns{})
}

func produceDirectCallArgumentJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	return produceDirectCallArgumentJudgmentDiagnosticsWithPolicy(result, sourceFile, judgment.DefaultPolicy(), judgment.StrictnessDefault)
}

func produceDirectCallArgumentJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceReachableCallJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.CallArguments{})
}

func produceDirectCallContractJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	ctx := reachableCallJudgmentContext(result, sourceFile)
	items := firstDirectCallContractJudgmentPerCall(
		pass.New(pass.CallCallee{}).Run(ctx),
		pass.New(pass.CallArity{}).Run(ctx),
		pass.New(pass.CallArguments{}).Run(ctx),
	)
	return renderJudgmentDiagnostics(items, policy, mode)
}

func produceCallArityJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallArity{})
}

func produceCallCalleeJudgmentDiagnostics(result *body.Result, sourceFile string) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, judgment.DefaultPolicy(), judgment.StrictnessDefault, pass.CallCallee{})
}

func produceConcatOperandJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.ConcatOperands{})
}

func produceNumericForJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.NumericForOperands{})
}

func produceNonNilAssertionJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.NonNilAssertions{})
}

func produceMemberReadJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceReachableJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.MemberReads{})
}

func produceChannelSelectJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.ChannelSelects{})
}

func produceUnusedLocalJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.UnusedLocals{})
}

func produceDeadAssignmentJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.DeadAssignments{})
}

func produceFrozenTableJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.FrozenTableMutations{})
}

func produceLifecycleJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.LifecycleObligations{})
}

func produceUnresolvedValueJudgmentDiagnosticsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	return produceJudgmentsWithPolicy(result, sourceFile, policy, mode, pass.UnresolvedValues{})
}

func produceUnresolvedTypeJudgmentDiagnosticsWithPolicy(result, parent *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode) []diagnostic.Diagnostic {
	items := pass.New(pass.UnresolvedTypes{}).Run(pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      internalreadmodel.NewWithParent(result, parent),
	})
	return renderJudgmentDiagnostics(items, policy, mode)
}

func produceJudgmentsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(judgmentContext(result, sourceFile)), policy, mode)
}

func produceReachableJudgmentsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(reachableJudgmentContext(result, sourceFile)), policy, mode)
}

func produceReachableCallJudgmentsWithPolicy(result *body.Result, sourceFile string, policy judgment.Policy, mode judgment.StrictnessMode, producers ...pass.Producer) []diagnostic.Diagnostic {
	return renderJudgmentDiagnostics(pass.New(producers...).Run(reachableCallJudgmentContext(result, sourceFile)), policy, mode)
}

func judgmentContext(result *body.Result, sourceFile string) pass.Context {
	query := newDiagnosticQuery(result)
	return pass.Context{
		FunctionKey: sourceFile,
		SourceFile:  sourceFile,
		Reader:      query.reader,
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

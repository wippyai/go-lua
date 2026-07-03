package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// direct-call diagnostics are rendered from post-solve judgments. The legacy
// AST/guard-env direct-call checker was removed after the shadow oracle matched.
func produceDirectCallContract(result *body.Result, context producerContext, _ map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	envs := context.guardEnvironments(result)
	return produceDirectCallContractJudgmentDiagnosticsFiltered(
		result,
		"",
		context.judgmentPolicy,
		context.judgmentStrictness,
		func(point cfg.Point) bool {
			return guardEnvReachableAt(envs, point)
		},
	)
}

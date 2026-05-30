// call_check.go implements function call argument validation for the type checker.
//
// This pass validates that function calls have correct argument counts and types.
// It runs after flow analysis using narrowed types for both callees and arguments.
//
// The pass handles several call patterns:
//   - Direct calls: fn(args) - validates args against fn's parameter types
//   - Method calls: obj:method(args) - resolves method, validates receiver and args
//   - Generic calls: infers type arguments and validates against instantiated params
//   - Type constructors: TypeName(x) - special handling for callable type effects
//
// For each call, the shared ops.CallPipeline handles the two-phase synthesis
// process, allowing contextual typing for callback arguments.
//
// Errors are mapped from ops.CallError to diag.Diagnostic with appropriate
// source positions (pointing to the problematic argument when possible).
package hooks

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/synth/callarg"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/diag"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// CheckCalls validates function call arguments against parameter types.
func CheckCalls(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	observer observation.Projector,
	ctx *db.QueryContext,
	query core.TypeOps,
	results map[*ast.FunctionExpr]*api.FuncResult,
	sourceName string,
) []diag.Diagnostic {
	if graph == nil || query == nil {
		return nil
	}

	var diags []diag.Diagnostic
	bindings := graph.Bindings()
	for _, call := range evidence.Calls {
		if call.Origin == api.CallOriginExpression {
			continue
		}
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		callDiags := checkSingleCall(p, info, observer, ctx, query, sourceName, graph, evidence, bindings, results)
		diags = append(diags, callDiags...)
	}

	return diags
}

func checkSingleCall(
	p cfg.Point,
	info *cfg.CallInfo,
	observer observation.Projector,
	ctx *db.QueryContext,
	query core.TypeOps,
	sourceName string,
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	bindings *bind.BindingTable,
	results map[*ast.FunctionExpr]*api.FuncResult,
) []diag.Diagnostic {
	if info.Method == "" && info.Callee != nil {
		if observer.HasCallableTypeEffect(info.Callee, p) {
			return nil
		}
	}

	if callsite.IsMethodCallInfo(info) {
		if observer.HasTypeValueMethodEffect(info.Receiver, p, info.Method) {
			return nil
		}
	}

	args := make([]typ.Type, len(info.Args))
	for i, arg := range info.Args {
		args[i] = observer.TypeOf(arg, p)
	}

	def := ops.CallDef{
		Args:  args,
		Query: query,
	}

	if callsite.IsMethodCallInfo(info) {
		def.IsMethod = true
		def.MethodName = info.Method
		def.Receiver = observer.TypeOf(info.Receiver, p)
		def.ForceMethodReceiver = callsite.ForceMethodReceiver(bindings, graph, evidence, info)
	} else if info.Callee != nil {
		def.Callee = observer.TypeOf(info.Callee, p)
		if projection, ok := functionfact.ProjectCall(functionfact.CallProjectionInput{
			Store:    api.StoreFrom(ctx),
			Info:     info,
			Graph:    graph,
			Evidence: evidence,
			Bindings: bindings,
			Results:  results,
			Args:     args,
			Current:  def.Callee,
		}); ok {
			def.Callee = projection.Callee
			def.AllowExtraArgs = projection.AllowExtraArgs
		}
	}

	full := callarg.Full(
		func(arg ast.Expr, pt cfg.Point, expected typ.Type) typ.Type {
			return observer.TypeOfWithExpected(arg, pt, expected)
		},
		func(table *ast.TableExpr, expected typ.Type, pt cfg.Point) bool {
			return observer.TableCompatible(table, pt, expected)
		},
		p,
	)
	// An argument observed as the gradual top `any` is consistent with a concrete
	// expected parameter type (gradual consistency), mirroring the assignment and
	// return boundaries. The synthesis-side re-synth declines to refine a gradual
	// `any` toward an expected type, so admit it here at the diagnostic boundary
	// against the resolved expected parameter; a narrowed/dominated `any` is no
	// longer the gradual top and is untouched.
	resynth := func(idx int, arg ast.Expr, expected typ.Type) typ.Type {
		return observer.AdmitGradualArgument(full(idx, arg, expected), arg, p, expected)
	}
	pipeline := ops.NewCallPipeline(ctx, def, len(info.Args)).
		WithReSynth(callarg.ForArgs(info.Args, resynth))
	result := pipeline.Run()
	return callErrorsToDiags(result.Errors, info, sourceName)
}

func getCallPosition(info *cfg.CallInfo, sourceName string) diag.Position {
	pos := diag.Position{File: sourceName}
	if info.Receiver != nil {
		pos.Line = info.Receiver.Line()
		pos.Column = info.Receiver.Column()
	} else if info.Callee != nil {
		pos.Line = info.Callee.Line()
		pos.Column = info.Callee.Column()
	}
	return pos
}

func callErrorsToDiags(errors []ops.CallError, info *cfg.CallInfo, sourceName string) []diag.Diagnostic {
	if len(errors) == 0 || info == nil {
		return nil
	}

	var diags []diag.Diagnostic
	callPos := getCallPosition(info, sourceName)

	for _, err := range errors {
		message := err.DiagnosticMessage()
		pos := callPos
		span := ast.SpanOf(info.Callee)
		if !span.Valid() && info.Receiver != nil {
			span = ast.SpanOf(info.Receiver)
		}
		if err.ArgIdx > 0 && err.ArgIdx <= len(info.Args) {
			arg := info.Args[err.ArgIdx-1]
			pos = diag.Position{File: sourceName, Line: arg.Line(), Column: arg.Column()}
			span = ast.SpanOf(arg)
			if pos.Line == 0 {
				pos = callPos
			}
		}

		code := diag.ErrTypeMismatch
		switch err.Kind {
		case ops.ErrWrongArity:
			code = diag.ErrWrongArity
		case ops.ErrNotCallable:
			if strings.HasPrefix(message, "no method") {
				code = diag.ErrNoMethod
			} else {
				code = diag.ErrNotCallable
			}
		case ops.ErrOptionalCall:
			code = diag.ErrOptionalCall
		}

		_, help := diag.ContextualHelp(code, message, "")
		diags = append(diags, diag.Diagnostic{
			Severity: diag.SeverityError,
			Code:     code,
			Position: pos,
			Span:     span,
			Message:  message,
			Help:     help,
		})
	}

	return diags
}

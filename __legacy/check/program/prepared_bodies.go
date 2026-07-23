package program

import (
	"fmt"
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// preparedBodies is the immutable lexical forest consumed by the sole
// RelationProgram transaction.
type preparedBodies struct {
	root              *body.Static
	functions         map[*ast.FunctionExpr]*body.Static
	bindings          *bind.Result
	callTopology      operationplan.CallTopology
	memberUseClosures map[symbol.ID]frozenMemberUseClosure
}

func (p preparedBodies) function(fn *ast.FunctionExpr) *body.Static {
	return p.functions[fn]
}

func (p preparedBodies) directLexicalOwner(static *body.Static) (*ast.FunctionExpr, bool) {
	if static == nil || p.bindings == nil {
		return nil, false
	}
	if static == p.root {
		return nil, true
	}
	for function, candidate := range p.functions {
		if candidate == static {
			return function, true
		}
	}
	return nil, false
}

func prepareBoundChunkBodies(stmts []ast.Stmt, bindings *bind.Result, config body.Config, keys programKeys) (preparedBodies, error) {
	forest, err := body.PrepareBoundChunkForest(stmts, bindings, staticPrepareConfig(config, keys))
	if err != nil {
		return preparedBodies{}, err
	}
	functions := bindings.Functions()
	topology := forest.CallTopology()
	if !topology.Complete() {
		return preparedBodies{}, fmt.Errorf("prepare program bodies: lexical forest has no call topology")
	}
	prepared := preparedBodies{root: forest.Root(), functions: make(map[*ast.FunctionExpr]*body.Static, len(functions)), bindings: bindings, callTopology: topology}
	for _, fn := range functions {
		static := forest.Function(fn)
		if static == nil {
			return preparedBodies{}, fmt.Errorf("prepare program bodies: lexical function is absent from source forest")
		}
		prepared.functions[fn] = static
	}
	prepared.memberUseClosures = freezeMemberUseClosures(prepared, keys.metatableProof)
	return prepared, nil
}

func prepareBoundFunctionBodies(rootFn *ast.FunctionExpr, bindings *bind.Result, config body.Config, keys programKeys) (preparedBodies, error) {
	forest, err := body.PrepareBoundFunctionForest(rootFn, bindings, staticPrepareConfig(config, keys))
	if err != nil {
		return preparedBodies{}, err
	}
	functions := bindings.Functions()
	topology := forest.CallTopology()
	if !topology.Complete() {
		return preparedBodies{}, fmt.Errorf("prepare program bodies: lexical forest has no call topology")
	}
	prepared := preparedBodies{functions: make(map[*ast.FunctionExpr]*body.Static, len(functions)), bindings: bindings, callTopology: topology}
	if forest.Function(rootFn) == nil {
		return preparedBodies{}, fmt.Errorf("prepare program bodies: root function is absent from source forest")
	}
	for _, fn := range functions {
		static := forest.Function(fn)
		if static == nil {
			return preparedBodies{}, fmt.Errorf("prepare program bodies: lexical function is absent from source forest")
		}
		prepared.functions[fn] = static
	}
	prepared.memberUseClosures = freezeMemberUseClosures(prepared, keys.metatableProof)
	return prepared, nil
}

func staticPrepareConfig(config body.Config, keys programKeys) body.Config {
	out := cloneCheckConfig(config)
	out.MethodReceiverTypes = keys.metatableMethodReceivers
	return out
}

func cloneCheckConfig(config body.Config) body.Config {
	config.Globals = slices.Clone(config.Globals)
	config.ExpressionValues = maps.Clone(config.ExpressionValues)
	config.MethodReceiverTypes = maps.Clone(config.MethodReceiverTypes)
	config.StateLanes = state.CloneLanes(config.StateLanes)
	config.ClosedDynamicAllValues = slices.Clone(config.ClosedDynamicAllValues)
	config.Signatures.Manifests = slices.Clone(config.Signatures.Manifests)
	config.ModuleExports.Manifests = slices.Clone(config.ModuleExports.Manifests)
	config.ModuleTypes.Manifests = slices.Clone(config.ModuleTypes.Manifests)
	return config
}

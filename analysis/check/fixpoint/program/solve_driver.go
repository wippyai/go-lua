package program

import (
	"maps"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type keyedFunction struct {
	funcExpr      *ast.FunctionExpr
	key           summary.SummaryKey
	entryState    state.State
	entryKeys     *keyspace.KeySpace
	hasEntryState bool
}

type preparedBodies struct {
	root      *body.Static
	functions map[*ast.FunctionExpr]*body.Static
}

func (p preparedBodies) function(fn *ast.FunctionExpr) *body.Static {
	if p.functions == nil {
		return nil
	}
	return p.functions[fn]
}

func prepareBoundChunkBodies(stmts []ast.Stmt, bindings *bind.Result, config body.Config, keys programKeys) (preparedBodies, error) {
	root, err := body.PrepareBoundChunk(stmts, bindings, staticPrepareConfig(config, keys))
	if err != nil {
		return preparedBodies{}, err
	}
	prepared := preparedBodies{
		root:      root,
		functions: make(map[*ast.FunctionExpr]*body.Static, len(keys.functions)),
	}
	if err := prepareFunctionStatics(prepared.functions, keys.functions, bindings, config, keys); err != nil {
		return preparedBodies{}, err
	}
	return prepared, nil
}

func prepareBoundFunctionBodies(rootFn *ast.FunctionExpr, bindings *bind.Result, config body.Config, keys programKeys) (preparedBodies, error) {
	prepared := preparedBodies{
		functions: make(map[*ast.FunctionExpr]*body.Static, 1+len(keys.functions)),
	}
	if err := prepareFunctionStatic(prepared.functions, rootFn, bindings, config, keys); err != nil {
		return preparedBodies{}, err
	}
	if err := prepareFunctionStatics(prepared.functions, keys.functions, bindings, config, keys); err != nil {
		return preparedBodies{}, err
	}
	return prepared, nil
}

func prepareFunctionStatics(out map[*ast.FunctionExpr]*body.Static, functions []keyedFunction, bindings *bind.Result, config body.Config, keys programKeys) error {
	for _, fn := range functions {
		if err := prepareFunctionStatic(out, fn.funcExpr, bindings, config, keys); err != nil {
			return err
		}
	}
	return nil
}

func prepareFunctionStatic(out map[*ast.FunctionExpr]*body.Static, fn *ast.FunctionExpr, bindings *bind.Result, config body.Config, keys programKeys) error {
	if fn == nil {
		return nil
	}
	if _, ok := out[fn]; ok {
		return nil
	}
	prepared, err := body.PrepareBoundFunction(fn, bindings, staticPrepareConfig(config, keys))
	if err != nil {
		return err
	}
	out[fn] = prepared
	return nil
}

func staticPrepareConfig(config body.Config, keys programKeys) body.Config {
	out := cloneCheckConfig(config)
	out.MethodReceiverTypes = keys.metatableMethodReceivers
	return out
}

func solvePrepared(prepared *body.Static, config body.Config) (*body.Result, error) {
	return body.SolvePrepared(prepared, config.SolveConfig())
}

func solvePreparedCounted(prepared *body.Static, config body.Config, counter *int) (*body.Result, error) {
	if counter != nil {
		(*counter)++
	}
	return solvePrepared(prepared, config)
}

func summaryIndexBase(keys programKeys) *callresult.SummaryIndexBase {
	return callresult.NewSummaryIndexBase(callresult.SummaryIndexBaseConfig{
		FunctionKeys:  keys.functionKeys,
		FunctionIDs:   keys.functionIDs,
		PathKeys:      keys.pathKeys,
		PathMultiKeys: keys.pathMultiKeys,
		FunctionTypes: keys.functionTypes,
	})
}

func summaryIndexForOwner(base *callresult.SummaryIndexBase, keys programKeys, owner summary.SummaryKey) *callresult.SummaryIndex {
	return base.WithFunctionExpressionKeys(keys.contexts.FunctionExpressionKeysForOwner(owner))
}

func chunkFunction(
	key summary.SummaryKey,
	prepared *body.Static,
	config body.Config,
	stats *Stats,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	index *callresult.SummaryIndex,
	metatableProof metatableMethodProof,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, contextKeyFor, keyFor, index, metatableProof)
			result, err := solvePreparedCounted(prepared, config, summaryCounter(stats))
			if err != nil {
				return summary.Summary{}, err
			}
			return summaryprojection.FromResult(result), nil
		},
	}
}

func boundFunction(
	origin keyedFunction,
	prepared *body.Static,
	config body.Config,
	stats *Stats,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	index *callresult.SummaryIndex,
	metatableProof metatableMethodProof,
) query.Function {
	captured := cloneCheckConfig(config)
	return query.Function{
		Key: origin.key,
		Body: func(ctx query.Context) (summary.Summary, error) {
			config := checkConfigWithSummaries(captured, ctx.Summaries, contextKeyFor, keyFor, index, metatableProof)
			if origin.hasEntryState {
				config.EntryState = origin.entryState.Snapshot().RekeyPathEvidence(origin.entryKeys, prepared.KeySpace())
			}
			result, err := solvePreparedCounted(prepared, config, summaryCounter(stats))
			if err != nil {
				return summary.Summary{}, err
			}
			return summaryprojection.FromResult(result), nil
		},
	}
}

func contextKeyFunc(keys programKeys, owner summary.SummaryKey) callresult.KeyFunc {
	return func(_ transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool) {
		if calleeKey := site.CalleePathKey(); calleeKey.Valid() && len(keys.pathMultiKeys[calleeKey]) > 1 {
			return summary.SummaryKey{}, false
		}
		if expr, ok := site.Expr(); ok && expr != 0 {
			if key, ok := keys.contexts.CallContextKey(owner, expr); ok {
				return key, true
			}
		}
		return summary.SummaryKey{}, false
	}
}

func directKeyFunc(keys programKeys) callresult.KeyFunc {
	direct := callresult.ByCalleeIdentity(keys.targetKeys)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool) {
		if calleeKey := site.CalleePathKey(); calleeKey.Valid() && len(keys.pathMultiKeys[calleeKey]) > 1 {
			return summary.SummaryKey{}, false
		}
		return direct(ctx, site)
	}
}

func checkConfigWithSummaries(
	config body.Config,
	summaries summary.Reader,
	contextKeyFor callresult.KeyFunc,
	keyFor callresult.KeyFunc,
	index *callresult.SummaryIndex,
	metatableProof metatableMethodProof,
) body.Config {
	out := cloneCheckConfig(config)
	baseFactory := out.CallOutcomeFactory
	baseSignatureArgumentType := out.SignatureArgumentType
	baseSignatureArgumentTypeFactory := out.SignatureArgumentTypeFactory
	out.CallOutcomeFactory = func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
		providerConfig := callresult.ProviderConfig{
			Summaries:               summaries,
			ContextKeyFor:           contextKeyFor,
			KeyFor:                  keyFor,
			CalleeValue:             callresult.CalleeValueFunc(ctx.CalleeValue),
			ReceiverCallable:        callresult.ReceiverCallableFunc(ctx.ReceiverCallable),
			Facts:                   ctx.Facts,
			Index:                   index,
			Sources:                 ctx.Sources,
			ReturnPresenceRelations: callresult.ReturnPresenceRelationsForPathFunc(ctx.ReturnPresenceRelationsPath),
			KeySpace:                ctx.KeySpace,
			TypeValues:              ctx.TypeValues,
		}
		primary := callresult.OutcomeProvider(callresult.ProviderConfig{
			Summaries:               providerConfig.Summaries,
			ContextKeyFor:           providerConfig.ContextKeyFor,
			KeyFor:                  providerConfig.KeyFor,
			CalleeValue:             providerConfig.CalleeValue,
			ReceiverCallable:        providerConfig.ReceiverCallable,
			Facts:                   providerConfig.Facts,
			Index:                   providerConfig.Index,
			Sources:                 providerConfig.Sources,
			ReturnPresenceRelations: providerConfig.ReturnPresenceRelations,
			KeySpace:                providerConfig.KeySpace,
			TypeValues:              providerConfig.TypeValues,
		})
		if baseFactory == nil {
			return primary
		}
		return calloutcome.ComposeSupplemental(primary, baseFactory(ctx))
	}
	out.SignatureArgumentTypeFactory = func(ctx body.CallOutcomeContext) body.SignatureArgumentTypeFunc {
		provider := body.SignatureArgumentTypeFunc(callresult.SummaryArgumentTypeProvider(callresult.ProviderConfig{
			Summaries:  summaries,
			Facts:      ctx.Facts,
			Index:      index,
			Sources:    ctx.Sources,
			TypeValues: ctx.TypeValues,
		}))
		baseFactoryProvider := body.SignatureArgumentTypeFunc(nil)
		if baseSignatureArgumentTypeFactory != nil {
			baseFactoryProvider = baseSignatureArgumentTypeFactory(ctx)
		}
		return func(node transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
			if t, ok := provider(node, source, in, read); ok {
				if surfaced, surfacedOK := metatableSignatureArgumentType(ctx, metatableProof, node, source, t); surfacedOK {
					return surfaced, true
				}
				return t, true
			}
			if baseFactoryProvider != nil {
				if t, ok := baseFactoryProvider(node, source, in, read); ok {
					return t, true
				}
			}
			if baseSignatureArgumentType != nil {
				return baseSignatureArgumentType(node, source, in, read)
			}
			return nil, false
		}
	}
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

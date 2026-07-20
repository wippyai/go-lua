package program

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

// newRelationProgramExecutionFactories creates exactly one execution authority
// per lexical body. The authorities prepare concrete entry/publication edges;
// they do not own a body solver or an application context.
func newRelationProgramExecutionFactories(ctx context.Context, prepared preparedBodies, config body.Config) (context.Context, relationProgramExecutionFactories, error) {
	ctx, session := cancellation.Attach(ctx)
	inferred := inferRelationClosedDynamicInvariants(config.Registry, prepared)
	statics := make([]*body.Static, 0, 1+len(prepared.functions))
	if prepared.root != nil {
		statics = append(statics, prepared.root)
	}
	for _, static := range prepared.functions {
		statics = append(statics, static)
	}
	factories := make(relationProgramExecutionFactories, len(statics))
	for _, static := range statics {
		if static == nil {
			return ctx, nil, fmt.Errorf("program: formal execution has a nil prepared body")
		}
		bodyID := static.StableLexicalBodyID()
		if bodyID == (lexicalidentity.StableLexicalBodyID{}) {
			return ctx, nil, fmt.Errorf("program: formal execution has a zero lexical body identity")
		}
		if previous, exists := factories[bodyID]; exists {
			if previous.Graph() != static.Graph() {
				return ctx, nil, fmt.Errorf("program: lexical body %s has conflicting execution authorities", bodyID)
			}
			continue
		}
		closedDynamic := mergeRelationClosedDynamicInvariants(config.ClosedDynamicAllValues, inferred[bodyID])
		factory, err := static.NewExecutionFactory(body.ExecutionFactoryConfig{
			Context: ctx, Session: session, StateLanes: state.CloneLanes(config.StateLanes),
			ClosedDynamicAllValues: closedDynamic, TypeValues: config.TypeValues,
			SignatureArgumentType:        config.SignatureArgumentType,
			SignatureArgumentTypeFactory: config.SignatureArgumentTypeFactory,
		})
		if err != nil {
			return ctx, nil, fmt.Errorf("program: execution factory for lexical body %s: %w", bodyID, err)
		}
		factories[bodyID] = factory
	}
	if len(factories) == 0 {
		return ctx, nil, fmt.Errorf("program: formal execution has no lexical bodies")
	}
	return ctx, factories, nil
}

// runPreparedRelationProgram is the production transaction: one forest
// freeze, one formal WTO solve (including post-WTO Apply outcomes), and one
// route-free lexical publication. There is no application enumeration,
// context prepass, fallback solver, or second materialization pass.
func runPreparedRelationProgram(
	ctx context.Context,
	prepared preparedBodies,
	rootStatic *body.Static,
	config body.Config,
	keys programKeys,
	stats *Stats,
) (formalLexicalPublishedProgram, error) {
	if rootStatic == nil || config.Registry == nil {
		return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal transaction has no root or registry")
	}
	ctx, factories, err := newRelationProgramExecutionFactories(ctx, prepared, config)
	if err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	units, err := relationProgramInput(prepared, factories, config.Initial)
	if err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	program, err := transformer.FreezeRelationProgram(units, prepared.callTopology)
	if err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	rootBody := rootStatic.StableLexicalBodyID()
	if rootBody == (lexicalidentity.StableLexicalBodyID{}) {
		return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal root has no stable lexical identity")
	}
	view, err := program.Solve(ctx, rootBody, config.EntryState)
	if err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	if err := recordFunctionalSummaryStats(stats, prepared, view); err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	bodyKeys, err := relationProgramBodyKeys(prepared, rootStatic, keys)
	if err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	return publishFormalLexicalProgram(ctx, view.LexicalBodies(), factories, rootBody, config.EntryState, config.Initial, config, bodyKeys, prepared, keys)
}

func relationProgramBodyKeys(prepared preparedBodies, rootStatic *body.Static, keys programKeys) (map[lexicalidentity.StableLexicalBodyID]summary.SummaryKey, error) {
	if rootStatic == nil {
		return nil, fmt.Errorf("program: formal key map has no root")
	}
	out := make(map[lexicalidentity.StableLexicalBodyID]summary.SummaryKey, 1+len(keys.functions))
	rootBody := rootStatic.StableLexicalBodyID()
	out[rootBody] = keys.rootKey
	for _, function := range keys.functions {
		static := prepared.function(function.funcExpr)
		if static == nil {
			return nil, fmt.Errorf("program: formal public key map has no prepared function")
		}
		bodyID := static.StableLexicalBodyID()
		if prior, exists := out[bodyID]; exists && prior != function.key {
			return nil, fmt.Errorf("program: lexical body %s has conflicting public summary keys", bodyID)
		}
		out[bodyID] = function.key
	}
	if bodyIDs := factoriesBodyIDs(prepared); len(out) != len(bodyIDs) {
		return nil, fmt.Errorf("program: formal public key map covers %d bodies, want %d", len(out), len(bodyIDs))
	}
	return out, nil
}

func factoriesBodyIDs(prepared preparedBodies) map[lexicalidentity.StableLexicalBodyID]struct{} {
	out := make(map[lexicalidentity.StableLexicalBodyID]struct{}, 1+len(prepared.functions))
	if prepared.root != nil {
		out[prepared.root.StableLexicalBodyID()] = struct{}{}
	}
	for _, static := range prepared.functions {
		if static != nil {
			out[static.StableLexicalBodyID()] = struct{}{}
		}
	}
	return out
}

func relationResultSolveConfig(config body.Config) body.SolveConfig {
	solve := config.SolveConfig()
	solve.WidenAt = nil
	solve.WidenDelay = nil
	solve.SummaryInputDigests = nil
	solve.SummaryInputs = nil
	solve.SummaryInputsComplete = false
	solve.EntryState = state.State{}
	solve.Initial = nil
	return solve
}

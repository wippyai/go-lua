package program

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/projection"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/compiler/ast"
)

// formalLexicalPublishedProgram is the route-free public image of one formal
// forest solve. Each lexical identity owns exactly one Result and one Summary.
type formalLexicalPublishedProgram struct {
	root     *body.Result
	results  map[lexicalidentity.StableLexicalBodyID]*body.Result
	snapshot summary.Snapshot
}

// publishFormalLexicalProgram consumes the one-record-per-body output of the
// formal WTO and delegates to the existing Result and Summary publishers. It
// is intentionally not wired into the production driver until the atomic
// RelationProgram.Solve cut.
func publishFormalLexicalProgram(
	ctx context.Context,
	coordinates []transformer.FormalLexicalBodyCoordinates,
	factories relationProgramExecutionFactories,
	rootBody lexicalidentity.StableLexicalBodyID,
	rawRootEntry state.State,
	rootInitial transfer.InitialState,
	config body.Config,
	bodyKeys map[lexicalidentity.StableLexicalBodyID]summary.SummaryKey,
	prepared preparedBodies,
	keys programKeys,
) (formalLexicalPublishedProgram, error) {
	if ctx == nil || rootBody == (lexicalidentity.StableLexicalBodyID{}) || len(coordinates) == 0 {
		return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical publication is unowned")
	}
	if len(coordinates) != len(factories) || len(bodyKeys) != len(factories) {
		return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical publication covers %d coordinates, %d factories, and %d keys", len(coordinates), len(factories), len(bodyKeys))
	}
	versions := make(map[lexicalidentity.StableLexicalBodyID]uint64, len(coordinates))
	for _, lexical := range coordinates {
		if lexical.Body == (lexicalidentity.StableLexicalBodyID{}) {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical publication has a zero body identity")
		}
		if _, duplicate := versions[lexical.Body]; duplicate {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical body %s was published more than once", lexical.Body)
		}
		factory := factories[lexical.Body]
		if factory == nil {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical body %s has no execution authority", lexical.Body)
		}
		version, err := stabilizedCoordinateSemanticVersion(ctx, factory, "formal lexical body "+lexical.Body.String(), stabilizedCoordinateFingerprint{
			pointInputs: lexical.PointInputs, pointOutputs: lexical.PlannedNodeOutputs,
			pointReachable: lexical.PointReachable, outputReachable: lexical.NodeOutputReachable,
			edgeNormal: lexical.EdgeNormal, callOutcomes: lexical.CallOutcomes,
			diagnostics: lexical.DiagnosticOutput,
		})
		if err != nil {
			return formalLexicalPublishedProgram{}, err
		}
		versions[lexical.Body] = version
	}
	results := make(map[lexicalidentity.StableLexicalBodyID]*body.Result, len(coordinates))
	summaries := make(map[summary.SummaryKey]summary.Summary, len(coordinates))
	var rootResult *body.Result
	for _, lexical := range coordinates {
		if err := contextErr(ctx); err != nil {
			return formalLexicalPublishedProgram{}, err
		}
		factory := factories[lexical.Body]
		entry, exactEntry := lexical.PointInputs[factory.Graph().Entry()]
		if !exactEntry {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical body %s has no entry coordinate", lexical.Body)
		}
		publishedCoordinates := body.StabilizedResultCoordinates{
			PointInputs: lexical.PointInputs, PlannedNodeOutputs: lexical.PlannedNodeOutputs,
			PointReachable: lexical.PointReachable, NodeOutputReachable: lexical.NodeOutputReachable,
			EdgeNormal:   make(map[body.ResultEdge]bool, len(lexical.EdgeNormal)),
			CallOutcomes: lexical.CallOutcomes, DiagnosticOutput: lexical.DiagnosticOutput,
		}
		for edge, normal := range lexical.EdgeNormal {
			publishedCoordinates.EdgeNormal[body.ResultEdge{From: edge.From, To: edge.To}] = normal
		}

		seededEntry, initial := entry, transfer.InitialState(nil)
		if lexical.Body == rootBody {
			seededEntry, initial = factory.SeedEntry(rawRootEntry, rootInitial)
			if !factory.Domain().Equal(state.NormalizeForDomain(factory.Domain(), seededEntry), entry) {
				return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal root lexical entry differs from its canonical seed transaction")
			}
		}
		dependencies := make([]body.ApplicationDependency, len(lexical.Calls))
		for index, call := range lexical.Calls {
			version, present := versions[call.Target]
			if !present {
				return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical body %s call %d/%d has no target %s coordinate", lexical.Body, call.Point, call.Occurrence, call.Target)
			}
			dependencies[index] = body.ApplicationDependency{
				Target: call.Target, CallPoint: call.Point, CallOccurrence: call.Occurrence, SemanticVersion: version,
			}
		}
		result, err := factory.PublishResult(body.ResultPublicationConfig{
			Coordinates:             publishedCoordinates,
			Solve:                   relationResultSolveConfig(config),
			SeededEntry:             seededEntry,
			Initial:                 initial,
			ApplicationDependencies: dependencies,
		})
		if err != nil {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: publish formal lexical body %s: %w", lexical.Body, err)
		}
		projected, err := summaryprojection.FromResultContext(ctx, result)
		if err != nil {
			return formalLexicalPublishedProgram{}, err
		}
		body.WithBodyOwnedParamObligations(result, relationSummaryHasUsefulParamObligation(config.Registry, projected))
		key, present := bodyKeys[lexical.Body]
		if !present {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical body %s has no public summary identity", lexical.Body)
		}
		if _, duplicate := summaries[key]; duplicate {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical bodies share public summary identity %#v", key)
		}
		results[lexical.Body] = result
		summaries[key] = summary.Normalize(config.Registry, projected)
		if lexical.Body == rootBody {
			rootResult = result
		}
	}
	if rootResult == nil {
		return formalLexicalPublishedProgram{}, fmt.Errorf("program: formal lexical publication has no root result")
	}
	entries := make([]summary.EntrySummary, 0, len(summaries))
	for key, projected := range summaries {
		entries = append(entries, summary.EntrySummary{Key: key, Summary: projected})
	}
	snapshot := summary.NewSnapshotOwnedNormalized(config.Registry, entries...)
	if err := attachFormalLexicalFunctionResults(rootResult, results, prepared); err != nil {
		return formalLexicalPublishedProgram{}, err
	}
	functionTypes := functionValueTypesFromSummaryRoots(config.Registry, snapshot, keys, config.ModuleTypes)
	for _, result := range results {
		body.WithOwnedFunctionValueTypes(result, functionTypes)
	}
	return formalLexicalPublishedProgram{root: rootResult, results: results, snapshot: snapshot}, nil
}

// attachFormalLexicalFunctionResults mirrors lexical nesting exactly. There is
// no context/route filtering because there cannot be more than one Result for
// a lexical body.
func attachFormalLexicalFunctionResults(
	root *body.Result,
	results map[lexicalidentity.StableLexicalBodyID]*body.Result,
	prepared preparedBodies,
) error {
	if root == nil || prepared.bindings == nil {
		return fmt.Errorf("program: formal lexical result tree has no root or binder")
	}
	attached := make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(results))
	var attach func(*body.Result, *ast.FunctionExpr) error
	attach = func(parent *body.Result, owner *ast.FunctionExpr) error {
		children := make([]*body.Result, 0)
		for _, childFunction := range prepared.bindings.NestedFunctions(owner) {
			static := prepared.function(childFunction)
			if static == nil {
				return fmt.Errorf("program: formal nested function has no prepared body")
			}
			bodyID := static.StableLexicalBodyID()
			child := results[bodyID]
			if child == nil {
				return fmt.Errorf("program: formal nested lexical body %s has no result", bodyID)
			}
			if _, duplicate := attached[bodyID]; duplicate {
				return fmt.Errorf("program: formal nested lexical body %s was attached more than once", bodyID)
			}
			attached[bodyID] = struct{}{}
			if err := attach(child, childFunction); err != nil {
				return err
			}
			children = append(children, child)
		}
		body.WithFunctionResults(parent, children)
		return nil
	}
	if err := attach(root, root.Function()); err != nil {
		return err
	}
	if len(attached)+1 != len(results) {
		return fmt.Errorf("program: formal lexical result tree attached %d of %d bodies", len(attached)+1, len(results))
	}
	return nil
}

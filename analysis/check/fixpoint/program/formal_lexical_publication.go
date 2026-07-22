package program

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/projection"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
		version, err := formalArtifactSemanticVersion(ctx, factory, lexical)
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
			EdgeNormal: make(map[body.ResultEdge]bool, len(lexical.EdgeNormal)), ReturnSlots: lexical.ReturnSlots,
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
			FormalPathValue:         lexical.PathValue,
			Solve:                   relationResultSolveConfig(config),
			SeededEntry:             seededEntry,
			Initial:                 initial,
			ApplicationDependencies: dependencies,
		})
		if err != nil {
			return formalLexicalPublishedProgram{}, fmt.Errorf("program: publish formal lexical body %s: %w", lexical.Body, err)
		}
		projected, err := summaryprojection.FromFormalArtifactsContext(ctx, formalSummaryResult{Result: result, lexical: lexical})
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

// formalSummaryResult carries only the two non-retained summary observations
// published directly by the formal solve. Result remains the read-model for
// the five explicitly retained relation-completion classes.
type formalSummaryResult struct {
	*body.Result
	lexical transformer.FormalLexicalBodyCoordinates
}

func (r formalSummaryResult) FormalNormalReturnParameters() ([]product.Value, []product.Value, bool) {
	return append([]product.Value(nil), r.lexical.NormalReturnParameters.Entry...),
		append([]product.Value(nil), r.lexical.NormalReturnParameters.Exit...),
		r.lexical.NormalReturnParameters.HasNormalExit
}

func (r formalSummaryResult) formalNormalReturnReachability(point cfg.Point) (bool, bool) {
	reachable, ok := r.lexical.NormalReturnReachability[point]
	return reachable, ok
}

// formalArtifactSemanticVersion derives lineage from the immutable formal
// observations, never from a correlation-forgotten State reconstruction.
// Every field consumed by summary construction or lexical Apply lineage is
// included in a canonical order.
func formalArtifactSemanticVersion(
	ctx context.Context,
	factory *body.ExecutionFactory,
	lexical transformer.FormalLexicalBodyCoordinates,
) (uint64, error) {
	if ctx == nil || factory == nil || factory.Registry() == nil || factory.KeySpace() == nil {
		return 0, fmt.Errorf("program: formal lexical body %s has no artifact identity authority", lexical.Body)
	}
	h := fnv.New64a()
	var scratch [8]byte
	write := func(value uint64) { binary.LittleEndian.PutUint64(scratch[:], value); _, _ = h.Write(scratch[:]) }
	writeValue := func(value product.Value) {
		write(uint64(summary.NormalizedPayloadDigest(factory.Registry(), summary.Summary{Returns: []product.Value{value}})))
	}
	for _, value := range lexical.NormalReturnParameters.Entry {
		writeValue(value)
	}
	write(uint64(len(lexical.NormalReturnParameters.Entry)))
	for _, value := range lexical.NormalReturnParameters.Exit {
		writeValue(value)
	}
	write(uint64(len(lexical.NormalReturnParameters.Exit)))
	if lexical.NormalReturnParameters.HasNormalExit {
		write(1)
	} else {
		write(0)
	}
	points := make([]int, 0, len(lexical.NormalReturnReachability))
	for point := range lexical.NormalReturnReachability {
		points = append(points, int(point))
	}
	sort.Ints(points)
	write(uint64(len(points)))
	for _, raw := range points {
		write(uint64(raw))
		if lexical.NormalReturnReachability[cfg.Point(raw)] {
			write(1)
		} else {
			write(0)
		}
	}
	returns := make([]int, 0, len(lexical.ReturnSlots))
	for slot := range lexical.ReturnSlots {
		returns = append(returns, slot)
	}
	sort.Ints(returns)
	write(uint64(len(returns)))
	for _, slot := range returns {
		write(uint64(slot))
		writeValue(lexical.ReturnSlots[slot])
	}
	callPoints := make([]int, 0, len(lexical.CallOutcomes))
	for point := range lexical.CallOutcomes {
		callPoints = append(callPoints, int(point))
	}
	sort.Ints(callPoints)
	write(uint64(len(callPoints)))
	for _, raw := range callPoints {
		write(uint64(raw))
		digest, err := summary.CanonicalCallOutcomeDigestContext(ctx, factory.Registry(), factory.KeySpace(), lexical.CallOutcomes[cfg.Point(raw)])
		if err != nil {
			return 0, err
		}
		write(uint64(digest))
	}
	writeUint32 := func(value uint32) { write(uint64(value)) }
	for _, call := range lexical.Calls {
		write(uint64(call.Point))
		writeUint32(call.Occurrence)
		for _, value := range call.Target {
			write(uint64(value))
		}
	}
	write(uint64(lexical.DiagnosticOutput.Fingerprint(factory.Registry())))
	version := h.Sum64()
	if version == 0 {
		return 0, fmt.Errorf("program: formal lexical body %s has zero artifact identity", lexical.Body)
	}
	return version, nil
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

package program

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// evaluatedProgramBindings is the caller-owned lexical boundary valuation.
// Values and paths use the PreparedPlanCompiler Shape packing order. The
// transaction borrows them only while specializing its immutable roots.
type evaluatedProgramBindings struct {
	values []product.Value
	paths  []pathdom.Path
	order  []symbol.ID
}

func evaluatedBindingOrderMatchesPlan(plan *operationplan.Plan, binding evaluatedProgramBindings) bool {
	if plan == nil {
		return false
	}
	want := make([]symbol.ID, 0, len(plan.BoundaryParams())+len(plan.BoundaryCaptures())+len(plan.BoundaryGlobals()))
	want = append(want, plan.BoundaryParams()...)
	want = append(want, plan.BoundaryCaptures()...)
	want = append(want, plan.BoundaryGlobals()...)
	return slices.Equal(binding.order, want)
}

// evaluatedProgram is the compact transaction-local product of one complete
// relation evaluation. entryBody + observer are its production-shaped program
// publication: exactly one chunk entry owns the reachable diagnostic closure,
// while observer.Uncalled owns the separate declared-contract policy.
//
// shadowBodies/shadowRoots remain prototype internals for differential and
// per-body summary validation. They are not a claim that every lexical body is
// an independently invoked production diagnostic root. The product retains no
// body.Result, concrete State, solver checkpoint, term Arena, or callback.
type evaluatedProgram struct {
	entryBody    lexicalidentity.StableLexicalBodyID
	observer     lexicalObserverForest
	program      observerProgramArtifact
	shadowBodies []lexicalidentity.StableLexicalBodyID
	shadowRoots  map[lexicalidentity.StableLexicalBodyID]evaluated.RootArtifact
}

// Root is the package-internal shadow-root accessor retained for differential
// tests. Production-shaped consumers use Entry and never select an arbitrary
// body as an invocation root.
func (p evaluatedProgram) Root(ctx context.Context, reg *axis.Registry, body lexicalidentity.StableLexicalBodyID) (evaluated.Root, bool, error) {
	artifact, ok := p.shadowRoots[body]
	if !ok {
		return evaluated.Root{}, false, nil
	}
	root, err := artifact.Materialize(ctx, reg)
	if err != nil {
		return evaluated.Root{}, false, err
	}
	return root, true, nil
}

// Entry materializes the unique chunk-entry shadow root. Observer attachment
// is structural in this tranche; this method does not claim diagnostic parity.
func (p evaluatedProgram) Entry(ctx context.Context, reg *axis.Registry) (evaluated.Root, bool, error) {
	if p.entryBody == (lexicalidentity.StableLexicalBodyID{}) || p.observer.Root.Body != p.entryBody {
		return evaluated.Root{}, false, nil
	}
	return p.Root(ctx, reg, p.entryBody)
}

func (p evaluatedProgram) EntryBody() (lexicalidentity.StableLexicalBodyID, bool) {
	return p.entryBody, p.entryBody != (lexicalidentity.StableLexicalBodyID{}) && p.observer.Root.Body == p.entryBody
}

// Bodies returns shadow/prototype root identities, not independent production
// diagnostic invocations.
func (p evaluatedProgram) Bodies() []lexicalidentity.StableLexicalBodyID {
	return append([]lexicalidentity.StableLexicalBodyID(nil), p.shadowBodies...)
}

func (p evaluatedProgram) ObserverWork() (lexicalObserverWork, bool) {
	return p.observer.Work, p.observer.Root != (lexicalObserverTemplateRef{})
}

func (p evaluatedProgram) UncalledValidationTemplates() []lexicalObserverTemplateRef {
	return append([]lexicalObserverTemplateRef(nil), p.observer.Uncalled...)
}

// solveEvaluatedProgram is the explicit shadow/oracle helper. A catalog already
// owns one PreparedPlanCompiler and one complete direct-call surface per
// lexical body. This helper solves the complete relation transaction and
// projects every body root for differential validation. The production-shaped
// observer transaction below projects only its unique chunk entry.
//
// It deliberately has no legacy fallback. An unsupported equation, contextual
// relation, incomplete observation projection, identity mismatch, or
// cancellation returns the zero artifact. The current service does not call
// this seam until its consumers have moved from body.Result to evaluated.Root.
func solveEvaluatedProgram(
	ctx context.Context,
	catalog relationRunCatalog,
	bindings map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings,
	stats *Stats,
) (evaluatedProgram, error) {
	return solveEvaluatedProgramTransaction(ctx, catalog, bindings, nil, stats)
}

// solveEvaluatedObserverProgram is the activation seam used by the non-default
// total evaluated runner. The forest must have been built once from this exact
// sealed catalog before relation solving starts.
func solveEvaluatedObserverProgram(
	ctx context.Context,
	catalog relationRunCatalog,
	bindings map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings,
	forest lexicalObserverForest,
	stats *Stats,
) (evaluatedProgram, error) {
	if ctx == nil {
		return evaluatedProgram{}, fmt.Errorf("evaluated observer program: context is required")
	}
	if err := ctx.Err(); err != nil {
		return evaluatedProgram{}, err
	}
	if err := forest.validatePublication(catalog); err != nil {
		return evaluatedProgram{}, err
	}
	if stats != nil {
		stats.EvaluatedObserverCatalogTemplates += forest.Work.CatalogTemplates
		stats.EvaluatedObserverCallSitesScanned += forest.Work.CallSitesScanned
		stats.EvaluatedObserverDiagnosticNodes += forest.Work.DiagnosticNodes
		stats.EvaluatedObserverDiagnosticEdges += forest.Work.DiagnosticEdges
		stats.EvaluatedObserverUncalledTemplates += len(forest.Uncalled)
	}
	return solveEvaluatedProgramTransaction(ctx, catalog, bindings, &forest, stats)
}

func solveEvaluatedProgramTransaction(
	ctx context.Context,
	catalog relationRunCatalog,
	bindings map[lexicalidentity.StableLexicalBodyID]evaluatedProgramBindings,
	forest *lexicalObserverForest,
	stats *Stats,
) (evaluatedProgram, error) {
	if ctx == nil {
		return evaluatedProgram{}, fmt.Errorf("evaluated program: context is required")
	}
	if err := ctx.Err(); err != nil {
		return evaluatedProgram{}, err
	}
	if catalog.generation == nil || len(catalog.entries) == 0 {
		return evaluatedProgram{}, fmt.Errorf("evaluated program: empty or unowned lexical catalog")
	}

	entries := append([]relationCatalogEntry(nil), catalog.entries...)
	slices.SortFunc(entries, func(left, right relationCatalogEntry) int {
		leftBody, rightBody := lexicalBodyForEvaluatedEntry(left), lexicalBodyForEvaluatedEntry(right)
		if order := bytes.Compare(leftBody[:], rightBody[:]); order != 0 {
			return order
		}
		if left.identity.Cell.Function < right.identity.Cell.Function {
			return -1
		}
		if left.identity.Cell.Function > right.identity.Cell.Function {
			return 1
		}
		if left.identity.Cell.Slot < right.identity.Cell.Slot {
			return -1
		}
		if left.identity.Cell.Slot > right.identity.Cell.Slot {
			return 1
		}
		return 0
	})

	cells := make([]transformer.RelationCell, 0, len(entries))
	orderedBodies := make([]lexicalidentity.StableLexicalBodyID, 0, len(entries))
	seenBodies := make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(entries))
	seenCells := make(map[transformer.CellRef]struct{}, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return evaluatedProgram{}, err
		}
		bodyID := lexicalBodyForEvaluatedEntry(entry)
		if entry.identity.Prepared == nil || entry.compiler == nil {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: incomplete lexical producer %v", entry.identity.Cell)
		}
		plan := entry.identity.Prepared.OperationPlan()
		if plan == nil {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: incomplete lexical producer %v", entry.identity.Cell)
		}
		shape := transformer.Shape{
			Params: uint32(len(plan.BoundaryParams())), Captures: uint32(len(plan.BoundaryCaptures())), Globals: uint32(len(plan.BoundaryGlobals())),
		}
		catalogIndex, cataloged := catalog.byKey[entry.identity.Summary]
		if bodyID == (lexicalidentity.StableLexicalBodyID{}) || entry.identity.Generation != catalog.generation ||
			!entry.equationIdentityMatches(catalog.generation) ||
			entry.identity.Summary.Ref.IsZero() ||
			entry.identity.BodyDigest == 0 || entry.identity.BodyDigest != entry.identity.Prepared.IdentityDigest() ||
			plan.ObservationBody() != bodyID || catalog.registry == nil ||
			!entry.compiler.MatchesPreparation(catalog.registry, entry.identity.Prepared.Graph(), plan, shape) ||
			!cataloged || catalogIndex < 0 || catalogIndex >= len(catalog.entries) || catalog.entries[catalogIndex].identity != entry.identity {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: incomplete lexical producer %v", entry.identity.Cell)
		}
		if _, duplicate := seenBodies[bodyID]; duplicate {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: duplicate lexical body %x", bodyID)
		}
		if _, duplicate := seenCells[entry.identity.Cell]; duplicate {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: duplicate relation cell %v", entry.identity.Cell)
		}
		seenBodies[bodyID] = struct{}{}
		seenCells[entry.identity.Cell] = struct{}{}

		var (
			equation *transformer.PreparedEquation
			err      error
		)
		if len(entry.direct.Cells()) == 0 {
			equation, err = entry.compiler.Equation(entry.identity.Cell)
		} else {
			equation, err = entry.compiler.DirectEquation(entry.identity.Cell, entry.direct)
		}
		if err != nil {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: prepare %x: %w", bodyID, err)
		}
		cell, err := equation.Cell()
		if err != nil {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: cell %x: %w", bodyID, err)
		}
		if stats != nil {
			evaluate := cell.Equation
			cell.Equation = func(ctx context.Context, view transformer.RelationView) (transformer.Relation, error) {
				stats.EvaluatedRelationEquationApplications++
				return evaluate(ctx, view)
			}
		}
		cells = append(cells, cell)
		orderedBodies = append(orderedBodies, bodyID)
	}

	relations, err := transformer.SolveRelationCells(ctx, cells, transformer.RelationSolveOptions{})
	if err != nil {
		return evaluatedProgram{}, err
	}
	if err := ctx.Err(); err != nil {
		return evaluatedProgram{}, err
	}
	var observerPlan observerProgramTemplatePlan
	if forest != nil {
		observerPlan, err = matchObserverCallTemplates(ctx, *forest, catalog, relations)
		if err != nil {
			return evaluatedProgram{}, err
		}
		if stats != nil {
			stats.EvaluatedObserverCallTemplates += observerPlan.templates
		}
	}

	// The shadow/oracle transaction projects every lexical body once. The
	// observer transaction instead projects structurally shared boundary
	// instances into one program artifact and exposes only its chunk entry.
	roots := make(map[lexicalidentity.StableLexicalBodyID]evaluated.RootArtifact)
	projectedBodies := make([]lexicalidentity.StableLexicalBodyID, 0, len(entries))
	var programArtifact observerProgramArtifact
	project := func(entry relationCatalogEntry, relation transformer.Relation, binding evaluatedProgramBindings) (evaluated.RootArtifact, error) {
		return projectEvaluatedRelationRoot(ctx, catalog, entry, relation, binding, stats)
	}
	if forest != nil {
		rootBinding, bound := bindings[forest.Root.Body]
		if !bound {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: observer root has no boundary binding")
		}
		programArtifact, err = buildObserverProgramArtifact(ctx, catalog.registry, observerPlan, catalog, relations, rootBinding, project, stats)
		if err != nil {
			return evaluatedProgram{}, err
		}
		if programArtifact.entry == 0 || int(programArtifact.entry) > len(programArtifact.instances) {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: observer artifact has no unique entry instance")
		}
		entryArtifact := programArtifact.instances[int(programArtifact.entry)-1]
		if entryArtifact.template != forest.Root {
			return evaluatedProgram{}, fmt.Errorf("evaluated program: observer artifact entry identity drifted")
		}
		roots[forest.Root.Body] = entryArtifact.local
		projectedBodies = append(projectedBodies, forest.Root.Body)
	} else {
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return evaluatedProgram{}, err
			}
			bodyID := lexicalBodyForEvaluatedEntry(entry)
			relation, ok := relations.Lookup(entry.identity.Cell)
			if !ok {
				return evaluatedProgram{}, fmt.Errorf("evaluated program: relation %x is missing", bodyID)
			}
			binding, ok := bindings[bodyID]
			if !ok {
				return evaluatedProgram{}, fmt.Errorf("evaluated program: body %x has no boundary binding", bodyID)
			}
			artifact, err := project(entry, relation, binding)
			if err != nil {
				return evaluatedProgram{}, err
			}
			roots[bodyID] = artifact
			projectedBodies = append(projectedBodies, bodyID)
		}
	}
	if err := ctx.Err(); err != nil {
		return evaluatedProgram{}, err
	}

	if stats != nil {
		if stats.PrebuiltSemanticLexicalEvaluationsByBody == nil {
			stats.PrebuiltSemanticLexicalEvaluationsByBody = make(map[lexicalidentity.StableLexicalBodyID]int, len(orderedBodies))
		}
		for _, bodyID := range orderedBodies {
			stats.PrebuiltSemanticLexicalEvaluations++
			stats.PrebuiltSemanticLexicalEvaluationsByBody[bodyID]++
		}
		stats.EvaluatedShadowRootsProduced += len(roots)
		if forest != nil {
			stats.EvaluatedObserverProgramPublications++
		}
	}
	result := evaluatedProgram{shadowBodies: projectedBodies, shadowRoots: roots}
	if forest != nil {
		result.entryBody = forest.Root.Body
		result.observer = *forest
		result.program = programArtifact
	}
	return result, nil
}

func projectEvaluatedRelationRoot(
	ctx context.Context,
	catalog relationRunCatalog,
	entry relationCatalogEntry,
	relation transformer.Relation,
	binding evaluatedProgramBindings,
	stats *Stats,
) (evaluated.RootArtifact, error) {
	bodyID := lexicalBodyForEvaluatedEntry(entry)
	if relation.ContextualReason() != "" || relation.Widened() {
		reason := relation.ContextualReason()
		if reason == "" {
			reason = "widened relation"
		}
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: relation %x rejected: %s", bodyID, reason)
	}
	if entry.identity.Prepared == nil {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x has no preparation", bodyID)
	}
	plan := entry.identity.Prepared.OperationPlan()
	if plan == nil {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x has no operation plan", bodyID)
	}
	requirements, requirementsOK := plan.ObservationRequirements()
	surface, surfaceOK := plan.CallSurface()
	if !requirementsOK || !surfaceOK || !surface.Complete() || surface.Owner() != bodyID {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x has incomplete projection authority", bodyID)
	}
	if !evaluatedBindingOrderMatchesPlan(plan, binding) {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x boundary binding order differs from sealed plan", bodyID)
	}
	cursor, err := transformer.NewBindingCursor(relation.Shape(), binding.values, binding.paths)
	if err != nil {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x binding: %w", bodyID, err)
	}
	view, err := evaluated.SealProjectionView(requirements, false)
	if err != nil {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x view: %w", bodyID, err)
	}
	unavailable := evaluated.AuthorityDigest{Status: evaluated.AuthorityUnavailable}
	identity := evaluated.Identity{
		Body: bodyID, Relation: unavailable, Entry: unavailable, Lineage: unavailable, Registry: unavailable,
		CallSurface: surface.Digest(), Schema: requirements.SchemaID(), Inventory: requirements.ConsumerInventoryID(),
		View: view, PointCount: uint32(entry.identity.Prepared.Graph().Size()),
	}
	if !relation.ObservationCoverageComplete() {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x cell %v has incomplete relation observation coverage", bodyID, entry.identity.Cell)
	}
	if stats != nil {
		stats.EvaluatedRootProjections++
	}
	root, err := relation.EvaluateSparseRoot(ctx, transformer.EvaluatedRootRequest{
		Identity: identity, ExpectedIdentity: identity, Requirements: requirements, CallSurface: surface,
	}, cursor, transformer.SpecializationContext{})
	if err != nil {
		if specialized, exact := relation.Specialize(cursor, nil, nil); exact {
			if _, canonicalErr := summary.EncodeCanonical(ctx, entry.identity.Prepared.Registry(), specialized); canonicalErr != nil {
				return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x projection: %w (canonical summary: %v)", bodyID, err, canonicalErr)
			}
		}
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x projection: %w", bodyID, err)
	}
	if !root.Coverage().Complete() {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x projection is incomplete", bodyID)
	}
	artifact, err := evaluated.SealRoot(ctx, catalog.registry, root)
	if err != nil {
		return evaluated.RootArtifact{}, fmt.Errorf("evaluated program: body %x artifact seal: %w", bodyID, err)
	}
	return artifact, nil
}

func lexicalBodyForEvaluatedEntry(entry relationCatalogEntry) lexicalidentity.StableLexicalBodyID {
	if entry.identity.Prepared == nil {
		return lexicalidentity.StableLexicalBodyID{}
	}
	return entry.identity.Prepared.StableLexicalBodyID()
}

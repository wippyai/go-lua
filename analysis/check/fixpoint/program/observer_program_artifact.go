package program

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/evaluated"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type observerProgramInstanceID uint32
type observerProgramProofID uint32

type observerProgramBoundaryArtifact struct {
	values []product.CanonicalArtifact
	paths  []pathdom.Path
}

type observerProgramExpressionArtifact struct {
	ID       evaluated.ExpressionID
	Op       evaluated.ExpressionOp
	RootKind evaluated.RootKind
	Root     uint32
	Args     []evaluated.ExpressionID
	Constant product.CanonicalArtifact
	Scalar   evaluated.Scalar
}

type observerProgramProofArtifact struct {
	expressions []observerProgramExpressionArtifact
	predicates  []evaluated.Predicate
	decisions   []evaluated.Decision
}

type observerProgramParentArtifact struct {
	parent     observerProgramInstanceID
	proof      observerProgramProofID
	worlds     evaluated.WorldSet
	point      cfg.Point
	occurrence observation.Occurrence
	backedge   bool
	mu         lexicalObserverMuRef
}

type observerProgramInstanceArtifact struct {
	id       observerProgramInstanceID
	template lexicalObserverTemplateRef
	boundary observerProgramBoundaryArtifact
	parents  []observerProgramParentArtifact
	local    evaluated.RootArtifact
}

// observerProgramArtifact is one ownership-isolated program publication. It
// contains canonical product bytes and structural DTOs only. Local roots are
// instance-owned evidence, not independently addressable production roots.
type observerProgramArtifact struct {
	entry     observerProgramInstanceID
	instances []observerProgramInstanceArtifact
	proofs    []observerProgramProofArtifact
}

type observerTransientBinding struct {
	values []product.Value
	paths  []pathdom.Path
}

type observerTransientInstance struct {
	ref     lexicalObserverTemplateRef
	binding observerTransientBinding
}

type observerLocalRootProjector func(
	entry relationCatalogEntry,
	relation transformer.Relation,
	binding evaluatedProgramBindings,
) (evaluated.RootArtifact, error)

func sealObserverBoundary(
	ctx context.Context,
	reg *axis.Registry,
	values []product.Value,
	paths []pathdom.Path,
) (observerProgramBoundaryArtifact, string, error) {
	if ctx == nil || reg == nil || len(paths) != len(values) {
		return observerProgramBoundaryArtifact{}, "", fmt.Errorf("observer program: incomplete boundary artifact input")
	}
	out := observerProgramBoundaryArtifact{
		values: make([]product.CanonicalArtifact, len(values)),
		paths:  make([]pathdom.Path, len(paths)),
	}
	key := make([]byte, 0, len(values)*24)
	var raw [8]byte
	for index, value := range values {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return observerProgramBoundaryArtifact{}, "", err
			}
		}
		artifact, err := product.SealCanonical(ctx, reg, value)
		if err != nil {
			return observerProgramBoundaryArtifact{}, "", fmt.Errorf("observer program: boundary value %d is not canonical: %w", index, err)
		}
		out.values[index] = artifact
		encoded := artifact.Bytes()
		binary.BigEndian.PutUint64(raw[:], uint64(len(encoded)))
		key = append(key, raw[:]...)
		key = append(key, encoded...)
		out.paths[index] = paths[index].Clone()
		pathKey := []byte(paths[index].Key())
		binary.BigEndian.PutUint64(raw[:], uint64(len(pathKey)))
		key = append(key, raw[:]...)
		key = append(key, pathKey...)
	}
	return out, string(key), nil
}

func sealObserverProof(ctx context.Context, reg *axis.Registry, proof evaluated.WorldProof) (observerProgramProofArtifact, error) {
	if ctx == nil || reg == nil {
		return observerProgramProofArtifact{}, fmt.Errorf("observer program: proof context and registry are required")
	}
	out := observerProgramProofArtifact{
		expressions: make([]observerProgramExpressionArtifact, len(proof.Expressions)),
		predicates:  append([]evaluated.Predicate(nil), proof.Predicates...),
		decisions:   append([]evaluated.Decision(nil), proof.Decisions...),
	}
	for index, expression := range proof.Expressions {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return observerProgramProofArtifact{}, err
			}
		}
		constant, err := product.SealCanonical(ctx, reg, expression.Constant)
		if err != nil {
			return observerProgramProofArtifact{}, fmt.Errorf("observer program: proof expression %d constant is not canonical: %w", expression.ID, err)
		}
		out.expressions[index] = observerProgramExpressionArtifact{
			ID: expression.ID, Op: expression.Op, RootKind: expression.RootKind, Root: expression.Root,
			Args: append([]evaluated.ExpressionID(nil), expression.Args...), Constant: constant, Scalar: expression.Scalar,
		}
	}
	for index, decision := range out.decisions {
		if decision.ID != evaluated.DecisionID(index+2) || decision.Low >= decision.ID || decision.High >= decision.ID {
			return observerProgramProofArtifact{}, fmt.Errorf("observer program: malformed world decision %d", decision.ID)
		}
	}
	return out, nil
}

func observerBindingOrder(entry relationCatalogEntry) []symbol.ID {
	if entry.identity.Prepared == nil || entry.identity.Prepared.OperationPlan() == nil {
		return nil
	}
	plan := entry.identity.Prepared.OperationPlan()
	out := make([]symbol.ID, 0, len(plan.BoundaryParams())+len(plan.BoundaryCaptures())+len(plan.BoundaryGlobals()))
	out = append(out, plan.BoundaryParams()...)
	out = append(out, plan.BoundaryCaptures()...)
	out = append(out, plan.BoundaryGlobals()...)
	return out
}

func normalizeObserverPaths(values []product.Value, paths []pathdom.Path) ([]pathdom.Path, error) {
	if len(paths) == 0 {
		return make([]pathdom.Path, len(values)), nil
	}
	if len(paths) != len(values) {
		return nil, fmt.Errorf("observer program: boundary path width %d differs from value width %d", len(paths), len(values))
	}
	out := make([]pathdom.Path, len(paths))
	for index := range paths {
		out[index] = paths[index].Clone()
	}
	return out, nil
}

// buildObserverProgramArtifact expands only acyclic, concretely specialized
// call applications. Recursive topology was already sealed by the lexical
// forest; until recursive boundary environments are exact this function fails
// closed before publishing an artifact.
func buildObserverProgramArtifact(
	ctx context.Context,
	reg *axis.Registry,
	plan observerProgramTemplatePlan,
	catalog relationRunCatalog,
	relations transformer.RelationSnapshot,
	root evaluatedProgramBindings,
	project observerLocalRootProjector,
	stats *Stats,
) (observerProgramArtifact, error) {
	if ctx == nil || reg == nil || project == nil || plan.root == (lexicalObserverTemplateRef{}) {
		return observerProgramArtifact{}, fmt.Errorf("observer program: complete build authority is required")
	}
	if plan.recursive {
		return observerProgramArtifact{}, fmt.Errorf("observer program: recursive boundary environments are structurally sealed but not evaluable")
	}
	entries := make(map[lexicalObserverTemplateRef]relationCatalogEntry, len(catalog.entries))
	bodyPlans := make(map[lexicalObserverTemplateRef]observerBodyTemplatePlan, len(plan.bodies))
	for _, entry := range catalog.entries {
		if entry.identity.Prepared == nil {
			return observerProgramArtifact{}, fmt.Errorf("observer program: catalog entry has no prepared body")
		}
		ref := lexicalObserverTemplateRef{Body: entry.identity.Prepared.StableLexicalBodyID(), Cell: entry.identity.Cell}
		entries[ref] = entry
	}
	for _, body := range plan.bodies {
		bodyPlans[body.ref] = body
	}
	rootEntry, rooted := entries[plan.root]
	_, planned := bodyPlans[plan.root]
	if !rooted || !planned || rootEntry.compiler == nil {
		return observerProgramArtifact{}, fmt.Errorf("observer program: root template is incomplete")
	}
	rootPaths, err := normalizeObserverPaths(root.values, root.paths)
	if err != nil {
		return observerProgramArtifact{}, err
	}

	artifact := observerProgramArtifact{entry: 1}
	transient := make([]observerTransientInstance, 0, len(plan.bodies))
	byBoundary := make(map[lexicalObserverTemplateRef]map[string]observerProgramInstanceID)
	addInstance := func(ref lexicalObserverTemplateRef, values []product.Value, paths []pathdom.Path, parent *observerProgramParentArtifact) (observerProgramInstanceID, error) {
		sealed, key, err := sealObserverBoundary(ctx, reg, values, paths)
		if err != nil {
			return 0, err
		}
		bucket := byBoundary[ref]
		if bucket == nil {
			bucket = make(map[string]observerProgramInstanceID)
			byBoundary[ref] = bucket
		}
		if id, exists := bucket[key]; exists {
			if parent != nil {
				artifact.instances[int(id)-1].parents = append(artifact.instances[int(id)-1].parents, *parent)
			}
			return id, nil
		}
		id := observerProgramInstanceID(len(artifact.instances) + 1)
		bucket[key] = id
		instance := observerProgramInstanceArtifact{id: id, template: ref, boundary: sealed}
		if parent != nil {
			instance.parents = append(instance.parents, *parent)
		}
		artifact.instances = append(artifact.instances, instance)
		evaluationPaths := cloneObserverProgramPaths(paths)
		// The batch boundary reports caller-address provenance. An empty path
		// therefore remains empty in the durable artifact. The params-only child
		// relation nevertheless owns addressable lexical parameter roots; use the
		// same local placeholders as evaluatedCatalogBoundaryBindings solely for
		// child-local term evaluation.
		for index := range evaluationPaths {
			if evaluationPaths[index].IsEmpty() {
				evaluationPaths[index] = pathdom.NewPlaceholder(index)
			}
		}
		transient = append(transient, observerTransientInstance{
			ref: ref, binding: observerTransientBinding{
				values: append([]product.Value(nil), values...), paths: evaluationPaths,
			},
		})
		return id, nil
	}
	if _, err := addInstance(plan.root, root.values, rootPaths, nil); err != nil {
		return observerProgramArtifact{}, err
	}

	for cursor := 0; cursor < len(transient); cursor++ {
		if err := ctx.Err(); err != nil {
			return observerProgramArtifact{}, err
		}
		pending := transient[cursor]
		entry, owned := entries[pending.ref]
		bodyPlan, bodyPlanned := bodyPlans[pending.ref]
		relation, solved := relations.Lookup(pending.ref.Cell)
		if !owned || !bodyPlanned || !solved || entry.compiler == nil {
			return observerProgramArtifact{}, fmt.Errorf("observer program: instance %v has no exact template relation", pending.ref.Cell)
		}
		binding := evaluatedProgramBindings{values: pending.binding.values, paths: pending.binding.paths, order: observerBindingOrder(entry)}
		local, err := project(entry, relation, binding)
		if err != nil {
			return observerProgramArtifact{}, err
		}
		artifact.instances[cursor].local = local
		if stats != nil {
			stats.EvaluatedObserverInstanceProjections++
			if cursor == 0 {
				stats.EvaluatedObserverEntryProjections++
			}
		}

		cursorBinding, err := transformer.NewBindingCursor(relation.Shape(), pending.binding.values, pending.binding.paths)
		if err != nil {
			return observerProgramArtifact{}, err
		}
		projection, err := relation.SpecializeObserverCalls(ctx, cursorBinding, transformer.SpecializationContext{})
		if err != nil {
			return observerProgramArtifact{}, fmt.Errorf("observer program: specialize calls for %v: %w", pending.ref.Cell, err)
		}
		if stats != nil {
			stats.EvaluatedObserverTermApplications += int(projection.TermApplications())
		}
		items := projection.Items()
		if len(items) == 0 {
			continue
		}
		proof, err := sealObserverProof(ctx, reg, projection.Proof())
		if err != nil {
			return observerProgramArtifact{}, err
		}
		artifact.proofs = append(artifact.proofs, proof)
		proofID := observerProgramProofID(len(artifact.proofs))
		for _, item := range items {
			call, targetRef, matched := matchObserverCallInstance(bodyPlan, item)
			if !matched {
				return observerProgramArtifact{}, fmt.Errorf("observer program: specialized call %v point %d has no matched template", pending.ref.Cell, item.Point)
			}
			if item.Worlds.Root > evaluated.DecisionID(len(proof.decisions)+1) || item.Worlds.Root == evaluated.DecisionFalse {
				return observerProgramArtifact{}, fmt.Errorf("observer program: specialized call %v point %d has an invalid world", pending.ref.Cell, item.Point)
			}
			paths, err := normalizeObserverPaths(item.Values, item.Paths)
			if err != nil {
				return observerProgramArtifact{}, err
			}
			parent := observerProgramParentArtifact{
				parent: observerProgramInstanceID(cursor + 1), proof: proofID, worlds: item.Worlds,
				point: item.Point, occurrence: item.Occurrence,
			}
			if call.edge.Target.Kind == lexicalObserverMuTarget {
				parent.backedge, parent.mu = true, call.edge.Target.Mu
				return observerProgramArtifact{}, fmt.Errorf("observer program: recursive call %v point %d reached after acyclic admission", pending.ref.Cell, item.Point)
			}
			if _, err := addInstance(targetRef, item.Values, paths, &parent); err != nil {
				return observerProgramArtifact{}, err
			}
		}
	}

	for index := range artifact.instances {
		sort.SliceStable(artifact.instances[index].parents, func(i, j int) bool {
			left, right := artifact.instances[index].parents[i], artifact.instances[index].parents[j]
			if left.parent != right.parent {
				return left.parent < right.parent
			}
			if left.point != right.point {
				return left.point < right.point
			}
			if left.occurrence != right.occurrence {
				return left.occurrence.Less(right.occurrence)
			}
			if left.proof != right.proof {
				return left.proof < right.proof
			}
			return left.worlds.Root < right.worlds.Root
		})
	}
	return artifact, nil
}

func matchObserverCallInstance(body observerBodyTemplatePlan, item transformer.ObserverCallInstance) (observerCallTemplatePlan, lexicalObserverTemplateRef, bool) {
	for _, call := range body.calls {
		target, ok := observerEdgeTargetRef(call.edge)
		if ok && item.Owner == body.ref.Body && item.Point == call.edge.Point && item.Occurrence == call.edge.Occurrence &&
			item.Target.Cell == target.Cell && item.Target.Shape == call.templates[0].Target().Shape {
			return call, target, true
		}
	}
	return observerCallTemplatePlan{}, lexicalObserverTemplateRef{}, false
}

func cloneObserverProgramPaths(in []pathdom.Path) []pathdom.Path {
	out := make([]pathdom.Path, len(in))
	for index := range in {
		out[index] = in[index].Clone()
	}
	return out
}

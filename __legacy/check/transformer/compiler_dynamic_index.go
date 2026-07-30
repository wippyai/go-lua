package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// dynamicIndexPlanHandler is one atomic semantic owner registered under both
// N3 PathDescendantInvalidation and N4 DynamicIndexWrite. N3 validates the
// barrier but publishes nothing; N4 appends the single ordered EffectTerm.
type dynamicIndexPlanHandler struct{ kind operationplan.Kind }

type dynamicIndexDependencyPlanHandler struct{ kind operationplan.Kind }

func (h dynamicIndexDependencyPlanHandler) Kind() operationplan.Kind                    { return h.kind }
func (dynamicIndexDependencyPlanHandler) Preflight(planCompileContext, cfg.Point) error { return nil }
func (dynamicIndexDependencyPlanHandler) Lower(planCompileContext, cfg.Point, *[]Operation) error {
	return nil
}

func (h dynamicIndexPlanHandler) Kind() operationplan.Kind { return h.kind }

func (dynamicIndexPlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	_, err := buildBoundaryDynamicIndexEffect(ctx, point)
	return err
}

func (h dynamicIndexPlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	if h.kind == operationplan.PathDescendantInvalidation {
		return nil
	}
	if h.kind != operationplan.DynamicIndexWrite || ctx.rowSteps == nil {
		return fmt.Errorf("dynamic index: ordered row effect sink missing")
	}
	effect, err := buildBoundaryDynamicIndexEffect(ctx, point)
	if err != nil {
		return err
	}
	*ctx.rowSteps = append(*ctx.rowSteps, localEffectStep(effect))
	return nil
}

func buildBoundaryDynamicIndexEffect(ctx planCompileContext, point cfg.Point) (EffectTerm, error) {
	catalog := DefaultEffectCatalog()
	admission, admitted, err := catalog.AdmitPoint([]operationplan.Kind{
		operationplan.PathDescendantInvalidation,
		operationplan.DynamicIndexWrite,
	})
	if err != nil || !admitted || admission.Kind != EffectIndexMutation {
		return 0, fmt.Errorf("dynamic index: atomic N3/N4 catalog admission failed")
	}
	invalidation, hasInvalidation := ctx.facts.PathDescendantInvalidation(point)
	write, hasWrite := ctx.facts.DynamicIndexWrite(point)
	if !hasInvalidation || !hasWrite {
		return 0, fmt.Errorf("dynamic index: same-point invalidation/write pair required")
	}
	writeTarget := write.TargetRef()
	invalidationTarget, precise := invalidation.DynamicTargetContract()
	if !precise || !writeTarget.Equal(invalidationTarget) {
		return 0, fmt.Errorf("dynamic index: invalidation/write targets differ")
	}
	tablePath := writeTarget.TablePathRef()
	if tablePath.IsEmpty() || !invalidation.ContainerPathRef().Equal(tablePath) {
		return 0, fmt.Errorf("dynamic index: invalidation/write table paths differ")
	}
	tableTerm, err := boundaryLexicalPathTerm(ctx, tablePath)
	if err != nil {
		return 0, fmt.Errorf("dynamic index: table: %w", err)
	}
	keyTerm, keyPath, err := boundaryLexicalSourceTerms(ctx, write.KeySource())
	if err != nil {
		return 0, fmt.Errorf("dynamic index: key: %w", err)
	}
	valueTerm, valuePath, err := boundaryLexicalSourceTerms(ctx, write.Source())
	if err != nil {
		return 0, fmt.Errorf("dynamic index: value: %w", err)
	}
	if write.Admission() == dynamicindex.AdmissionBottom ||
		!write.ReadbackIntent().ReadsValue() {
		return 0, fmt.Errorf("dynamic index: admission/readback is outside exact boundary slice")
	}
	if point <= 0 {
		return 0, fmt.Errorf("dynamic index: lexical point provenance missing")
	}
	return ctx.builder.EffectArena().IndexMutation(IndexMutationConfig{
		Invalidation: InvalidatePathConfig{
			Target: PathEffectTarget(tableTerm), Scope: InvalidationScopeDescendants,
			PreserveStructuralWitness:       true,
			PreserveDynamicValueMemberships: true,
		},
		Table: PathEffectTarget(tableTerm), Key: keyTerm, Value: valueTerm,
		KeyPath: keyPath, ValuePath: valuePath,
		Admission: write.Admission(), Readback: write.ReadbackIntent(),
		Site: EffectSite{Owner: uint64(tablePath.Symbol), Ordinal: uint32(point)},
	})
}

func boundaryLexicalPathTerm(ctx planCompileContext, path pathdom.Path) (PathTerm, error) {
	if path.Symbol == 0 || path.Version != 0 {
		return 0, fmt.Errorf("path is not canonical")
	}
	binding, err := exactBoundaryPathBinding(ctx, path)
	if err != nil {
		return 0, err
	}
	term := ctx.builder.Arena().AppendPath(binding.Base, path.Segments...)
	if term == 0 {
		return 0, fmt.Errorf("lexical path term construction failed")
	}
	return term, nil
}

func boundaryLexicalSourceTerms(ctx planCompileContext, source factflow.ValueSource) (ValueTerm, PathTerm, error) {
	// Keep one scalar-admission rule for every symbolic consumer. In
	// particular, a selected coordinate of an expanded call is exact even
	// though its producer retains value-list metadata.
	valueTerm, err := exactCompilerScalarSourceTerm(ctx, source)
	if err != nil {
		return 0, 0, err
	}
	var path pathdom.Path
	hasPath := false
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if source.HasExpr {
			path, hasPath = ctx.facts.ExpressionPathRef(source.ExprRef)
		}
	case factflow.ValueSourcePath:
		path, hasPath = compilerResolverPath(source.PathKey)
		if !hasPath || path.Version != 0 {
			return 0, 0, fmt.Errorf("source path is not canonical")
		}
	}
	if !hasPath {
		return valueTerm, 0, nil
	}
	binding, err := exactBoundaryPathBinding(ctx, path)
	if err != nil {
		return 0, 0, err
	}
	_, pathTerm, lowerErr := ctx.builder.Arena().LowerBoundaryPathValue(path, binding)
	if lowerErr != nil {
		return 0, 0, lowerErr
	}
	return valueTerm, pathTerm, nil
}

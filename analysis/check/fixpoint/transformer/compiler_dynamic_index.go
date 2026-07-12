package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
	if h.kind != operationplan.DynamicIndexWrite || ctx.rowEffects == nil {
		return fmt.Errorf("dynamic index: ordered row effect sink missing")
	}
	effect, err := buildBoundaryDynamicIndexEffect(ctx, point)
	if err != nil {
		return err
	}
	*ctx.rowEffects = append(*ctx.rowEffects, effect)
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
	tablePath := write.TablePathRef()
	if tablePath.IsEmpty() || !invalidation.ContainerPathRef().Equal(tablePath) {
		return 0, fmt.Errorf("dynamic index: invalidation/write table paths differ")
	}
	tableRoot, tableTerm, err := boundaryParamPathTerm(ctx, tablePath)
	if err != nil {
		return 0, fmt.Errorf("dynamic index: table: %w", err)
	}
	keyRoot, keyTerm, keyPath, err := boundaryParamSourceTerms(ctx, write.KeySource())
	if err != nil {
		return 0, fmt.Errorf("dynamic index: key: %w", err)
	}
	valueRoot, valueTerm, valuePath, err := boundaryParamSourceTerms(ctx, write.Source())
	if err != nil {
		return 0, fmt.Errorf("dynamic index: value: %w", err)
	}
	if tableRoot == keyRoot || tableRoot == valueRoot || keyRoot == valueRoot {
		return 0, fmt.Errorf("dynamic index: table, key, and value require distinct boundary parameters")
	}
	if write.Admission() == dynamicindex.AdmissionBottom ||
		write.ReadbackIntent() != factflow.DynamicIndexReadbackKeyAndValue {
		return 0, fmt.Errorf("dynamic index: admission/readback is outside exact boundary slice")
	}
	if dynamicTable, dynamicKey, suffix, precise := invalidation.DynamicTargetRef(); precise {
		if !dynamicTable.Equal(tablePath) || dynamicKey != write.KeySource() || len(suffix) != 0 {
			return 0, fmt.Errorf("dynamic index: precise target does not match direct table/key write")
		}
	}
	if point <= 0 {
		return 0, fmt.Errorf("dynamic index: lexical point provenance missing")
	}
	return ctx.builder.EffectArena().IndexMutation(IndexMutationConfig{
		Invalidation: InvalidatePathConfig{
			Target: tableTerm, Scope: InvalidationScopeDescendants,
			PreserveStructuralWitness:       true,
			PreserveDynamicValueMemberships: true,
		},
		Table: tableTerm, Key: keyTerm, Value: valueTerm,
		KeyPath: keyPath, ValuePath: valuePath,
		Admission: write.Admission(), Readback: write.ReadbackIntent(),
		Site: EffectSite{Owner: uint64(tablePath.Symbol), Ordinal: uint32(point)},
	})
}

func boundaryParamPathTerm(ctx planCompileContext, path pathdom.Path) (Root, PathTerm, error) {
	if path.Symbol == 0 || path.Version != 0 || len(path.Segments) != 0 {
		return Root{}, 0, fmt.Errorf("path is not a canonical root")
	}
	index, ok := ctx.plan.BoundaryParamIndex(path.Symbol)
	if !ok {
		return Root{}, 0, fmt.Errorf("symbol %d is not a boundary parameter", path.Symbol)
	}
	root := Root{Kind: RootParam, Index: uint32(index)}
	if ctx.locals[path.Symbol] != ctx.builder.Arena().Root(root) {
		return Root{}, 0, fmt.Errorf("boundary parameter %d was overwritten", path.Symbol)
	}
	term := ctx.builder.Arena().Path(root)
	if term == 0 {
		return Root{}, 0, fmt.Errorf("boundary path term construction failed")
	}
	return root, term, nil
}

func boundaryParamSourceTerms(ctx planCompileContext, source factflow.ValueSource) (Root, ValueTerm, PathTerm, error) {
	if !source.Valid() || source.Expanded || source.Adjusted || source.OpenTail {
		return Root{}, 0, 0, fmt.Errorf("source is not one exact scalar")
	}
	var path pathdom.Path
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return Root{}, 0, 0, fmt.Errorf("expression identity missing")
		}
		var ok bool
		path, ok = ctx.facts.ExpressionPathRef(source.ExprRef)
		if !ok {
			return Root{}, 0, 0, fmt.Errorf("expression is not a boundary path")
		}
	case factflow.ValueSourcePath:
		sym, version, suffix, ok := pathaddr.ParseResolverPath(source.PathKey)
		if !ok || sym == 0 || version != 0 || suffix != "" {
			return Root{}, 0, 0, fmt.Errorf("source path is not canonical")
		}
		path = pathdom.NewPath(sym, "")
	default:
		return Root{}, 0, 0, fmt.Errorf("source kind %d is not a boundary parameter", source.Kind)
	}
	root, pathTerm, err := boundaryParamPathTerm(ctx, path)
	if err != nil {
		return Root{}, 0, 0, err
	}
	valueTerm := ctx.builder.Arena().Root(root)
	if valueTerm == 0 {
		return Root{}, 0, 0, fmt.Errorf("boundary value term construction failed")
	}
	return root, valueTerm, pathTerm, nil
}

func dynamicIndexEffectCapability(kind operationplan.Kind, lane state.LaneID) (Capability, bool) {
	if kind != operationplan.PathDescendantInvalidation && kind != operationplan.DynamicIndexWrite {
		return CapabilityUnsupported, false
	}
	descriptor, ok := DefaultEffectCatalog().Descriptor(EffectIndexMutation)
	if !ok {
		return CapabilityUnsupported, false
	}
	switch descriptor.LaneUse(lane) {
	case LaneUseUnaffected:
		return CapabilityUnaffected, true
	case LaneUseRead, LaneUseWrite, LaneUseReadWrite:
		return CapabilitySupported, true
	default:
		return CapabilityUnsupported, true
	}
}

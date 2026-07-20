package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// pathStorePlanHandler registers both N4 source families under the single
// factapply.PathStoreTransaction owner. The assignment handler emits the
// complete transaction when present; a static-only point is emitted by its
// static handler. Independent targets and values remain independent payloads.
type pathStorePlanHandler struct{ kind operationplan.Kind }

func (h pathStorePlanHandler) Kind() operationplan.Kind { return h.kind }
func (pathStorePlanHandler) Preflight(ctx planCompileContext, point cfg.Point) error {
	_, err := buildBoundaryPathStoreEffect(ctx, point)
	return err
}
func (h pathStorePlanHandler) Lower(ctx planCompileContext, point cfg.Point, _ *[]Operation) error {
	transaction, ok := factapply.PlanPathStoreTransaction(ctx.facts, point)
	if !ok {
		return fmt.Errorf("path store: transaction missing")
	}
	// The assignment owner emits the complete combined N4 transaction. A
	// static-only point is emitted by its static owner. This is an ownership
	// rule, not paired-write semantics.
	if h.kind == operationplan.PathStaticMemberWrite && transaction.HasPathAssignment() {
		return nil
	}
	if h.kind != operationplan.PathAssignment && h.kind != operationplan.PathStaticMemberWrite || ctx.rowSteps == nil {
		return fmt.Errorf("path store: ordered row effect sink missing")
	}
	effect, err := buildBoundaryPathStoreEffect(ctx, point)
	if err != nil {
		return err
	}
	*ctx.rowSteps = append(*ctx.rowSteps, localEffectStep(effect))
	return nil
}

func buildBoundaryPathStoreEffect(ctx planCompileContext, point cfg.Point) (EffectTerm, error) {
	transaction, ok := factapply.PlanPathStoreTransaction(ctx.facts, point)
	if !ok || !transaction.HasPathAssignment() && !transaction.HasStaticMemberWrite() {
		return 0, fmt.Errorf("path store: transaction has no write")
	}
	if transaction.HasCovariantProofPolicy() || transaction.HasAssignmentGroupPresenceStep() {
		return 0, fmt.Errorf("path store: call-presence/covariant descriptor is outside the current exact symbolic slice")
	}
	compileWrite := func(target pathdom.Path, source factflow.ValueSource) (PathStoreWriteConfig, error) {
		targetTerm, err := boundaryMemberPathTerm(ctx, target)
		if err != nil {
			return PathStoreWriteConfig{}, fmt.Errorf("target: %w", err)
		}
		value, err := exactCompilerSourceTerm(ctx, source)
		if err != nil {
			return PathStoreWriteConfig{}, fmt.Errorf("source: %w", err)
		}
		compiled := PathStoreWriteConfig{Target: targetTerm, Value: value, SuppressProof: transaction.SuppressesPathProof(source)}
		if sourcePath, ok := transaction.SourcePath(source); ok {
			compiled.SourcePath, err = boundaryMemberPathTerm(ctx, sourcePath)
			if err != nil {
				return PathStoreWriteConfig{}, fmt.Errorf("source path: %w", err)
			}
			compiled.HasSourcePath = true
		}
		return compiled, nil
	}
	config := PathStoreConfig{StaticHasAnnotation: transaction.StaticHasAnnotation(), Site: EffectSite{Ordinal: uint32(point)}}
	var err error
	var target pathdom.Path
	var source factflow.ValueSource
	if transaction.HasPathAssignment() {
		target, _ = transaction.AssignmentTarget()
		source, _ = transaction.AssignmentSource()
		config.Assignment, err = compileWrite(target, source)
		if err != nil {
			return 0, fmt.Errorf("path store: assignment %w", err)
		}
		config.HasAssignment = true
		config.Object, err = compileBoundaryPathStoreObject(ctx, transaction, target, source)
		if err != nil {
			return 0, err
		}
	}
	if transaction.HasStaticMemberWrite() {
		staticTarget, _ := transaction.StaticTarget()
		staticSource, _ := transaction.StaticSource()
		config.Static, err = compileWrite(staticTarget, staticSource)
		if err != nil {
			return 0, fmt.Errorf("path store: static %w", err)
		}
		config.HasStatic = true
		if target.IsEmpty() {
			target = staticTarget
		}
	}
	if point <= 0 || target.Symbol == 0 {
		return 0, fmt.Errorf("path store: lexical provenance missing")
	}
	config.Site.Owner = uint64(target.Symbol)
	return ctx.builder.EffectArena().PathStore(config)
}

type boundaryObjectLiteralLookup func(factflow.ValueSource) (factflow.ObjectLiteralView, bool)

func compileBoundaryObjectHeaps(ctx planCompileContext, roots []factflow.ValueSource, lookup boundaryObjectLiteralLookup) ([]PathStoreHeapObjectConfig, error) {
	if lookup == nil {
		return nil, nil
	}
	active := make(map[factflow.ExprRef]bool)
	done := make(map[factflow.ExprRef]bool)
	heaps := make([]PathStoreHeapObjectConfig, 0)
	var compile func(factflow.ValueSource) error
	compile = func(source factflow.ValueSource) error {
		literal, ok := lookup(source)
		if !ok {
			return nil
		}
		if _, identified := literal.Identity(); !identified {
			return fmt.Errorf("object materialization: object %d has invalid allocation identity provenance", source.ExprRef)
		}
		if active[source.ExprRef] {
			return fmt.Errorf("object materialization: cyclic object literal %d", source.ExprRef)
		}
		if done[source.ExprRef] {
			return nil
		}
		active[source.ExprRef] = true
		defer delete(active, source.ExprRef)
		root, err := exactCompilerSourceTerm(ctx, source)
		if err != nil {
			return fmt.Errorf("object materialization: object %d root: %w", source.ExprRef, err)
		}
		_, hasListTail := literal.ListElementSource()
		heap := PathStoreHeapObjectConfig{
			Root: root, Members: make([]PathStoreHeapMemberConfig, 0, literal.EntryCount()),
			StableShape: literal.StaticStringKeysComplete() && !hasListTail,
		}
		var entryErr error
		literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			entrySource := entry.Source()
			if err := compile(entrySource); err != nil {
				entryErr = err
				return false
			}
			value, err := exactCompilerSourceTerm(ctx, entrySource)
			if err != nil {
				entryErr = fmt.Errorf("object materialization: object %d member source: %w", source.ExprRef, err)
				return false
			}
			member := PathStoreHeapMemberConfig{Suffix: entry.SuffixSegments(), Value: value}
			if expected, ok := entry.Expected(); ok {
				member.Expected = ctx.builder.Arena().Constant(expected)
				member.HasExpected = member.Expected != 0
				if !member.HasExpected {
					entryErr = fmt.Errorf("object materialization: object %d member expected value is not owned", source.ExprRef)
					return false
				}
			}
			heap.Members = append(heap.Members, member)
			return true
		})
		if entryErr != nil {
			return entryErr
		}
		// Post-order preserves the concrete heap materialization contract:
		// nested objects precede the owner that references them.
		heaps = append(heaps, heap)
		done[source.ExprRef] = true
		return nil
	}
	for _, root := range roots {
		if err := compile(root); err != nil {
			return nil, err
		}
	}
	return heaps, nil
}

func compileBoundaryObjectMaterialization(ctx planCompileContext, point cfg.Point, candidates []factflow.ValueSource) (EffectTerm, error) {
	sources := make([]factflow.ValueSource, 0, len(candidates))
	var owner factflow.ExprRef
	for _, source := range candidates {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			continue
		}
		if _, ok := ctx.facts.ObjectLiteralView(source.ExprRef); !ok {
			continue
		}
		if owner == 0 {
			owner = source.ExprRef
		}
		sources = append(sources, source)
	}
	if len(sources) == 0 {
		return 0, nil
	}
	heaps, err := compileBoundaryObjectHeaps(ctx, sources, func(source factflow.ValueSource) (factflow.ObjectLiteralView, bool) {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return factflow.ObjectLiteralView{}, false
		}
		return ctx.facts.ObjectLiteralView(source.ExprRef)
	})
	if err != nil {
		return 0, err
	}
	return ctx.builder.EffectArena().ObjectMaterialization(PathStoreObjectConfig{Heaps: heaps}, EffectSite{Owner: uint64(owner), Ordinal: uint32(point)})
}

func compileBoundaryCallObjectMaterialization(ctx planCompileContext, point cfg.Point, site factflow.CallSiteView) (EffectTerm, error) {
	sources := make([]factflow.ValueSource, 0, site.ArgumentSourceCount())
	for index := 0; index < site.ArgumentSourceCount(); index++ {
		source, ok := site.ArgumentSourceAt(index)
		if ok {
			sources = append(sources, source)
		}
	}
	return compileBoundaryObjectMaterialization(ctx, point, sources)
}

func compileBoundaryReturnObjectMaterialization(ctx planCompileContext, point cfg.Point, fact factflow.Return) (EffectTerm, error) {
	return compileBoundaryObjectMaterialization(ctx, point, fact.Sources())
}

func compileBoundaryPathStoreObject(ctx planCompileContext, transaction factapply.PathStoreTransaction, target pathdom.Path, rootSource factflow.ValueSource) (PathStoreObjectConfig, error) {
	rootLiteral, ok := transaction.ObjectLiteralForSource(rootSource)
	if !ok {
		return PathStoreObjectConfig{}, nil
	}
	object := PathStoreObjectConfig{ListFloor: factapply.ObjectLiteralListLengthFloor(rootLiteral)}
	heaps, err := compileBoundaryObjectHeaps(ctx, []factflow.ValueSource{rootSource}, transaction.ObjectLiteralForSource)
	if err != nil {
		return PathStoreObjectConfig{}, err
	}
	object.Heaps = heaps
	var entryErr error
	rootLiteral.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		entryTarget, ok := entry.AppendSuffixTo(target)
		if !ok {
			entryErr = fmt.Errorf("path store: object entry target is not structural")
			return false
		}
		targetTerm, err := boundaryMemberPathTerm(ctx, entryTarget)
		if err != nil {
			entryErr = fmt.Errorf("path store: object entry target: %w", err)
			return false
		}
		entrySource := entry.Source()
		value, err := exactCompilerSourceTerm(ctx, entrySource)
		if err != nil {
			entryErr = fmt.Errorf("path store: object entry source: %w", err)
			return false
		}
		compiled := PathStoreObjectEntryConfig{Target: targetTerm, Value: value, SuppressProof: transaction.SuppressesPathProof(entrySource)}
		if sourcePath, ok := transaction.SourcePath(entrySource); ok && !sourcePath.IsEmpty() && sourcePath.Symbol != 0 {
			compiled.SourcePath, err = boundaryMemberPathTerm(ctx, sourcePath)
			if err != nil {
				entryErr = fmt.Errorf("path store: object entry source path: %w", err)
				return false
			}
			compiled.HasSourcePath = true
		}
		if expected, ok := entry.Expected(); ok {
			compiled.Expected = ctx.builder.Arena().Constant(expected)
			compiled.HasExpected = compiled.Expected != 0
			if !compiled.HasExpected {
				entryErr = fmt.Errorf("path store: object entry expected value is not owned")
				return false
			}
		}
		object.Entries = append(object.Entries, compiled)
		return true
	})
	if entryErr != nil {
		return PathStoreObjectConfig{}, entryErr
	}
	return object, nil
}

func boundaryMemberPathTerm(ctx planCompileContext, path pathdom.Path) (PathTerm, error) {
	if path.Symbol == 0 || path.Version != 0 {
		return 0, fmt.Errorf("path is not canonical")
	}
	binding, err := exactBoundaryPathBinding(ctx, path)
	if err != nil {
		return 0, err
	}
	term := ctx.builder.Arena().AppendPath(binding.Base, path.Segments...)
	if term == 0 {
		return 0, fmt.Errorf("path term construction failed")
	}
	return term, nil
}

func pathStoreEffectCapability(kind operationplan.Kind, lane state.LaneID) (Capability, bool) {
	if kind != operationplan.PathAssignment && kind != operationplan.PathStaticMemberWrite {
		return CapabilityUnsupported, false
	}
	descriptor, ok := DefaultEffectCatalog().Descriptor(EffectPathStore)
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

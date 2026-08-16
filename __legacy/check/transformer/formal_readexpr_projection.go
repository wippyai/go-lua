package transformer

import (
	"context"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// formalReadexprFactorReader binds the same four factor roles used by the
// concrete resolved-path projector to one selected formal publication leaf.
// It deliberately has no State adapter: the leaf's Values, path evidence,
// dynamic-index evidence, and heap-table identity factors remain separate
// formal observations until the carrier-neutral projector reads them.
type formalReadexprFactorReader struct {
	domain  state.ProductDomain
	keys    *keyspace.KeySpace
	values  state.ValueLaneFactor
	path    state.LaneFactor
	dynamic state.LaneFactor
	heap    state.LaneFactor
}

func newFormalReadexprFactorReader(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	values state.ValueLaneFactor,
	factors []state.LaneFactor,
) (formalReadexprFactorReader, bool) {
	if !domain.Valid() || keys == nil || !keys.Valid() {
		return formalReadexprFactorReader{}, false
	}
	pathFamily, pathOK := domain.PathValueFamily()
	dynamicLane, dynamicOK := domain.ProductLane(state.LaneDynamicIndex)
	heapLane, heapOK := domain.ProductLane(state.LaneHeapTableIdentity)
	if !pathOK || !dynamicOK || !heapOK {
		return formalReadexprFactorReader{}, false
	}
	var pathFactor, dynamicFactor, heapFactor state.LaneFactor
	var hasPath, hasDynamic, hasHeap bool
	for _, factor := range factors {
		switch factor.Lane() {
		case pathFamily.Lane():
			pathFactor, hasPath = factor, true
		case dynamicLane:
			dynamicFactor, hasDynamic = factor, true
		case heapLane:
			heapFactor, hasHeap = factor, true
		}
	}
	if !hasPath || !hasDynamic || !hasHeap {
		return formalReadexprFactorReader{}, false
	}
	return formalReadexprFactorReader{
		domain: domain, keys: keys, values: values,
		path: pathFactor, dynamic: dynamicFactor, heap: heapFactor,
	}, true
}

func (r formalReadexprFactorReader) ReadRootValue(id symbol.ID) (product.Value, bool) {
	if id == 0 {
		return product.Value{}, false
	}
	if r.values.Top {
		return product.Top(), true
	}
	value, ok := r.values.Values[statekey.SymbolValue(id)]
	if !ok {
		value = product.Bottom(r.domain.Registry())
	}
	return value, true
}

func (r formalReadexprFactorReader) ReadLocalPathValue(path keyspace.Key) (product.Value, bool) {
	value, present, err := r.domain.ReadPathValueFactor(r.path, r.keys, path)
	return value, present && err == nil
}

func (r formalReadexprFactorReader) ReadDynamicIndexTable(table keyspace.Key) (state.DynamicIndexTableEvidence, bool) {
	evidence, err := r.domain.ObserveDynamicIndexTableFactor(r.dynamic, table)
	return evidence, err == nil
}

func (r formalReadexprFactorReader) ReadHeapObject(term identity.Term) (heapidentity.TableObject, bool) {
	object, err := r.domain.ReadHeapTableObjectTermFactor(r.heap, term)
	return object, err == nil
}

var _ factapply.ResolvedPathValueReader = formalReadexprFactorReader{}

// directFormalPathValue keeps the conservative path-factor observation that
// predates structural read projection. Joining this one factor across the
// published observation is the established proof boundary: if it has an
// entry for the requested syntax path, no weaker reconstruction may replace
// it with heap or Values structure.
func (v *FormalRelationPublicationView) directFormalPathValue(
	ctx context.Context,
	coordinates []formalPublishedCoordinate,
	p pathdom.Path,
) (product.Value, bool, error) {
	if v == nil || v.body == nil || v.body.keys == nil || v.execution == nil || v.execution.algebra == nil || p.IsEmpty() {
		return product.Value{}, false, nil
	}
	key, exact := v.body.keys.FromPathKey(p.Key())
	if !exact {
		return product.Value{}, false, nil
	}
	factor, factorPresent, err := v.joinPublishedPathFactor(ctx, coordinates)
	if err != nil || !factorPresent {
		return product.Value{}, false, err
	}
	value, present, err := v.body.productDomain.ReadPathValueFactor(factor, v.body.keys, key)
	if err != nil || !present {
		return product.Value{}, false, err
	}
	return value, true, nil
}

// projectFormalReadPath is the formal equivalent of readexpr.Project's
// concrete shape projection. Each formal leaf supplies the exact factor roles
// required to resolve the read expression, then only the resulting product
// value is joined. This keeps heap/member and path-evidence correlation intact
// and never falls back to a concrete State observation.
func (v *FormalRelationPublicationView) projectFormalReadPath(
	ctx context.Context,
	point cfg.Point,
	boundary bool,
	coordinates []formalPublishedCoordinate,
	p pathdom.Path,
) (product.Value, bool, error) {
	if v == nil || v.body == nil || v.body.keys == nil || v.execution == nil || v.execution.algebra == nil {
		return product.Value{}, false, errFormalComponentForeignOwner
	}
	if direct, proven, err := v.directFormalPathValue(ctx, coordinates, p); err != nil || proven {
		return direct, proven, err
	}
	// A root is not a structural projection. If its path factor did not record
	// a proof, Values may be intentionally broad (for example a formal call
	// parameter) and the ordinary read-model recovery remains authoritative.
	if len(p.Segments) == 0 {
		return product.Value{}, false, nil
	}
	if v.body.pathSemantics == nil || !v.body.pathSemantics.Valid() {
		return product.Value{}, false, errFormalComponentForeignOwner
	}
	key, exact := v.body.pathSemantics.VisibleInputLocalPathKey(point, p)
	if boundary {
		key, exact = v.body.pathSemantics.VisibleLocalPathKey(point, p)
	}
	if !exact {
		return product.Value{}, false, nil
	}
	structural, err := factapply.FreezeResolvedStructuralPath(v.body.keys, key, p.Symbol)
	if err != nil {
		return product.Value{}, false, nil
	}
	reg := v.execution.algebra.program.registry
	joined := product.Bottom(reg)
	present := false
	for _, coordinate := range coordinates {
		coordinate.view = v
		if coordinate.inverseErr != nil {
			return product.Value{}, false, coordinate.inverseErr
		}
		tuple, live := v.execution.values[coordinate.cell]
		if !live || tuple.bottom() {
			continue
		}
		ordinals, valuesProjection, projectionErr := v.publicationProductProjection(ctx, tuple)
		if projectionErr != nil {
			return product.Value{}, false, projectionErr
		}
		partitions, partitionErr := v.execution.algebra.partitionSparseLeafViewsUnderCare(
			[]formalSparseTupleProjection{{tuple: tuple, ordinals: ordinals}}, nil,
		)
		if partitionErr != nil {
			return product.Value{}, false, partitionErr
		}
		cache := formalPublicationProjectionCache{
			factors: make(map[formalPublicationProjectionFactorKey][]formalPublicationProjectionFactorEntry),
			values:  make(map[uint64][]formalPublicationProjectionValuesEntry),
		}
		for _, partition := range partitions {
			if err := ctx.Err(); err != nil {
				return product.Value{}, false, err
			}
			if len(partition.views) != 1 || partition.guard == decisionFalse {
				return product.Value{}, false, errDecisionMalformed
			}
			values, factors, factorErr := v.projectLeafFactorTuple(ctx, partition.views[0], coordinate.inverse, &cache, valuesProjection)
			if factorErr != nil {
				return product.Value{}, false, factorErr
			}
			reader, readerOK := newFormalReadexprFactorReader(v.body.productDomain, v.body.keys, values, factors)
			if !readerOK {
				return product.Value{}, false, errFormalComponentMalformed
			}
			root, rootOK := reader.ReadRootValue(p.Symbol)
			if !rootOK || formalReadRootOpaque(reg, root) {
				// A missing or top-like root cannot prove any descendant shape. Do
				// not discard this leaf and join only the more precise siblings:
				// that would turn a may-be-any path into a claimed record member.
				return product.Value{}, false, nil
			}
			value, valueOK := factapply.ResolveStructuralPathFactorValue(reg, reader, structural)
			if !valueOK || product.Equal(reg, value, product.Bottom(reg)) {
				// Every live correlated leaf must establish the same structural
				// observation. An absent factor is not a license to recover stale
				// heap/member evidence from a different leaf.
				return product.Value{}, false, nil
			}
			if present {
				joined = product.Join(reg, joined, value)
			} else {
				joined, present = value, true
			}
		}
	}
	return joined, present, nil
}

func formalReadRootOpaque(reg *axis.Registry, value product.Value) bool {
	if reg == nil || product.Equal(reg, value, product.Bottom(reg)) || product.Get(reg, value, assertion.Key).Has(assertion.AnyClaim) {
		return true
	}
	t, ok := typevalue.TypeOf(reg, value)
	return !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t)
}

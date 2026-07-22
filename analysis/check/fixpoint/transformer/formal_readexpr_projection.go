package transformer

import (
	"context"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
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
	return r.preferExactHeapRoot(value), true
}

// preferExactHeapRoot is the factor-native counterpart of the root refinement
// at the end of readexpr.Project. A symbol slot carries table identity while
// the selected heap factor carries the object's current literal shape.
func (r formalReadexprFactorReader) preferExactHeapRoot(current product.Value) product.Value {
	reg := r.domain.Registry()
	term, exact := identityvalue.ExactTerm(reg, current)
	if !exact {
		return current
	}
	object, present := r.ReadHeapObject(term)
	if !present {
		return current
	}
	root := object.Root()
	rootTerm, rootExact := identityvalue.ExactTerm(reg, root)
	if !rootExact || rootTerm != term || product.Equal(reg, root, product.Bottom(reg)) {
		return current
	}
	if product.LessOrEq(reg, root, current) {
		return root
	}
	currentType, currentOK := typevalue.TypeOf(reg, current)
	rootType, rootOK := typevalue.TypeOf(reg, root)
	if currentOK && rootOK && subtype.IsSubtype(rootType, currentType) && !subtype.IsSubtype(currentType, rootType) {
		return root
	}
	return current
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
			value, valueOK := factapply.ResolveStructuralPathFactorValue(reg, reader, structural)
			if !valueOK || product.Equal(reg, value, product.Bottom(reg)) {
				continue
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

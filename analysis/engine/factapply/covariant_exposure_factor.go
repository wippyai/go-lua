package factapply

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type CovariantFactorBindingKind uint8

const (
	CovariantFactorBindingInvalid CovariantFactorBindingKind = iota
	CovariantFactorBindingNoop
	CovariantFactorBindingValues
	CovariantFactorBindingStructural
)

// CovariantFactorBinding explicitly distinguishes a zero-source no-op, a
// Values-only exposure, and an exposure with structural invalidation authority.
type CovariantFactorBinding[K comparable] struct {
	Kind   CovariantFactorBindingKind
	Source K
	Root   keyspace.Key
}

type CovariantFactorTransaction[K comparable] struct {
	Transaction CovariantExposureTransaction
	Bindings    []CovariantFactorBinding[K]
	Values      state.ValueFactor[K]
	Factors     []state.LaneFactor
	Domain      state.ProductDomain
	Keys        *keyspace.KeySpace
	Topology    state.CovariantFactorTopology
	Token       *cancellation.Token
}

type CovariantFactorResult[K comparable] struct {
	Values  state.ValueFactor[K]
	Factors []state.LaneFactor
}

// ApplyCovariantExposureFactors is the sole N6 semantic law. Concrete and
// formal adapters supply only address vocabularies and the same sealed sparse
// factor cone. Cancellation/failure returns the exact pre-N6 factors.
func ApplyCovariantExposureFactors[K comparable](ctx context.Context, widen CovariantWiden, in CovariantFactorTransaction[K]) (CovariantFactorResult[K], error) {
	fail := func(err error) (CovariantFactorResult[K], error) {
		return CovariantFactorResult[K]{Values: in.Values, Factors: in.Factors}, err
	}
	if ctx == nil || widen == nil || !in.Transaction.Valid(in.Domain.Registry()) || !in.Domain.Valid() ||
		!in.Topology.ValidFor(in.Domain) ||
		len(in.Bindings) != in.Transaction.Len() || len(in.Factors) != in.Topology.Len() {
		return fail(fmt.Errorf("factapply: invalid factor-native covariant transaction"))
	}
	if err := covariantCancellation(ctx, in.Token); err != nil {
		return fail(err)
	}
	for index, factor := range in.Factors {
		lane, ok := in.Topology.Lane(index)
		if !ok || factor.Lane() != lane {
			return fail(fmt.Errorf("factapply: covariant factor %d has foreign ownership", index))
		}
	}
	values := state.ValueFactor[K]{Top: in.Values.Top}
	if !in.Values.Top && len(in.Values.Values) != 0 {
		values.Values = make(map[K]product.Value, len(in.Values.Values))
		for slot, value := range in.Values.Values {
			values.Values[slot] = value
		}
	}
	factors := append([]state.LaneFactor(nil), in.Factors...)
	reg := in.Domain.Registry()
	for index := 0; index < in.Transaction.Len(); index++ {
		if index&255 == 0 {
			if err := covariantCancellation(ctx, in.Token); err != nil {
				return fail(err)
			}
		}
		step, ok := in.Transaction.Step(index)
		binding := in.Bindings[index]
		if !ok || binding.Kind == CovariantFactorBindingInvalid {
			return fail(fmt.Errorf("factapply: covariant step %d has no frozen binding", index))
		}
		if binding.Kind == CovariantFactorBindingNoop {
			continue
		}
		if binding.Kind != CovariantFactorBindingValues && binding.Kind != CovariantFactorBindingStructural {
			return fail(fmt.Errorf("factapply: covariant step %d has invalid binding kind", index))
		}
		if binding.Kind == CovariantFactorBindingStructural &&
			(in.Keys == nil || !in.Keys.Valid() || binding.Root.Kind == keyspace.KindInvalid) {
			return fail(fmt.Errorf("factapply: covariant step %d has no structural binding", index))
		}
		exposure := step.Exposure()
		source := product.Bottom(reg)
		if values.Top {
			source = product.Top()
		} else if current, present := values.Values[binding.Source]; present {
			source = current
		}
		if product.Equal(reg, source, product.Bottom(reg)) {
			continue
		}
		wide := product.Get(reg, exposure.WideValue(), typewitness.Key)
		if wide.IsTop() || wide.IsBottom() {
			continue
		}
		if exposure.Kind() == factflow.CovariantExposureArray {
			if _, tracked := product.Get(reg, source, identity.Key).ID(); tracked {
				continue
			}
			if !typewitness.Equal(product.Get(reg, source, typewitness.Key), wide) && !values.Top {
				if values.Values == nil {
					values.Values = make(map[K]product.Value)
				}
				values.Values[binding.Source] = product.Set(reg, source, typewitness.Key, wide)
			}
			continue
		}
		sourceType, sourceOK := product.Get(reg, source, typewitness.Key).Type()
		contractType, contractOK := wide.Type()
		if !sourceOK || !contractOK {
			continue
		}
		widened, tops, widenedOK := widen(sourceType, contractType, exposure.SourcePath().Segments)
		if err := covariantCancellation(ctx, in.Token); err != nil {
			return fail(err)
		}
		if !widenedOK {
			continue
		}
		witness := typewitness.Of(widened)
		if witness.IsTop() || witness.IsBottom() {
			continue
		}
		if !values.Top {
			if values.Values == nil {
				values.Values = make(map[K]product.Value)
			}
			values.Values[binding.Source] = product.Set(reg, source, typewitness.Key, witness)
		}
		if binding.Kind != CovariantFactorBindingStructural || in.Topology.Len() == 0 {
			continue
		}
		for _, top := range tops {
			target := binding.Root
			for _, segment := range top {
				var appended bool
				target, appended = in.Keys.AppendSegment(target, segment)
				if !appended {
					return fail(fmt.Errorf("factapply: covariant step %d has invalid widened path", index))
				}
			}
			var err error
			factors, err = applyCovariantSubtreeMutation(in.Domain, in.Keys, target, in.Topology, factors)
			if err != nil {
				return fail(err)
			}
		}
	}
	return CovariantFactorResult[K]{Values: values, Factors: factors}, nil
}

func covariantCancellation(ctx context.Context, token *cancellation.Token) error {
	if token != nil {
		if err := token.Err(); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func applyCovariantSubtreeMutation(domain state.ProductDomain, keys *keyspace.KeySpace, target keyspace.Key, topology state.CovariantFactorTopology, factors []state.LaneFactor) ([]state.LaneFactor, error) {
	pathOwner, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		return nil, fmt.Errorf("factapply: covariant mutation has no path-evidence owner")
	}
	pathIndex := -1
	var pathSkeleton state.CoordinateFamilySkeleton
	var pathScalars []state.CoordinateScalarFactor
	for index, factor := range factors {
		lane, _ := topology.Lane(index)
		if lane != pathOwner.Lane() {
			continue
		}
		var err error
		pathSkeleton, pathScalars, err = domain.DecomposeCoordinateFamily(factor, pathOwner, keys)
		if err != nil {
			return nil, err
		}
		pathIndex = index
	}
	if pathIndex < 0 {
		return nil, fmt.Errorf("factapply: covariant topology omits path-evidence owner")
	}
	transaction, err := domain.PrepareCoordinatePathSubtreeMutation(
		pathSkeleton, pathScalars, keys.FormatReadOnly(target),
	)
	if err != nil {
		return nil, err
	}
	return applyPathSubtreeMutationFactorLanes(domain, keys, transaction, factors)
}

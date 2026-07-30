package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// ApplyReturnResultBindings applies the canonical N5 result-binding
// component. The returned seeds retain their source spelling; heap projection
// changes only the value written to the destination slot.
func ApplyReturnResultBindings[K comparable](
	authority *ReturnAuthority,
	domain state.ProductDomain,
	transaction ReturnTransaction,
	sources []product.Value,
	targets []ReturnFactorTarget[K],
	values state.ValueFactor[K],
	container state.CoordinateFamilyFactor,
) (state.ValueFactor[K], []product.Value, error) {
	boundTargets, err := bindReturnFactorTargets(transaction, targets)
	if err != nil {
		return values, nil, err
	}
	if authority == nil || !authority.Valid() || !domain.Valid() || len(sources) != transaction.SourceCount() {
		return values, nil, fmt.Errorf("factapply: return source schema is incomplete")
	}
	out := state.ValueFactor[K]{Top: values.Top}
	if !values.Top && len(values.Values) != 0 {
		out.Values = make(map[K]product.Value, len(values.Values)+len(boundTargets))
		for slot, value := range values.Values {
			out.Values[slot] = value
		}
	}
	seeds := make([]product.Value, 0, transaction.ResultBindingCount())
	for index := 0; index < transaction.ResultBindingCount(); index++ {
		source, target, ok := transaction.ResultBinding(index)
		if !ok || source < 0 || source >= len(sources) {
			return values, nil, fmt.Errorf("factapply: malformed return binding %d", index)
		}
		value := sources[source]
		seeds = append(seeds, value)
		projects, ok := transaction.ResultBindingProjectsHeap(index)
		if !ok {
			return values, nil, fmt.Errorf("factapply: malformed return projection %d", index)
		}
		if projects {
			value, err = ProjectReturnContainerFactor(authority, domain, container, value)
			if err != nil {
				return values, nil, err
			}
		}
		if !out.Top {
			if out.Values == nil {
				out.Values = make(map[K]product.Value, len(boundTargets))
			}
			out.Values[boundTargets[target].Slot] = value
		}
	}
	return out, seeds, nil
}

// ProjectReturnContainerFactor applies the canonical returned-container
// projection through the unique registered container family. Concrete and
// guarded callers supply the same factor-native spelling.
func ProjectReturnContainerFactor(
	authority *ReturnAuthority,
	domain state.ProductDomain,
	factor state.CoordinateFamilyFactor,
	value product.Value,
) (product.Value, error) {
	if authority == nil || !authority.Valid() || !domain.Valid() {
		return product.Value{}, fmt.Errorf("factapply: invalid return-container projection")
	}
	owner, ok := domain.ReturnIdentityContainerFamily()
	if !ok {
		return value, nil
	}
	if factor.Family() != owner {
		return product.Value{}, fmt.Errorf("factapply: return-container factor has foreign ownership")
	}
	sealed, err := domain.SealCoordinateFamilyFactor(factor.Skeleton(), factor.Scalars())
	if err != nil {
		return product.Value{}, err
	}
	reg := domain.Registry()
	root, exact := product.Get(reg, value, identity.Key).Term()
	if !exact || !root.Valid() {
		return value, nil
	}
	for _, scalar := range sealed.Scalars() {
		var container product.Value
		found := false
		if err := domain.VisitCoordinateReturnIdentityScalarObservations(scalar, func(observation state.CoordinateReturnIdentityObservation) bool {
			if observation.Role() == state.CoordinateReturnIdentityContainer && observation.Root() == root {
				container, found = observation.Value(), true
				return false
			}
			return true
		}); err != nil {
			return product.Value{}, err
		}
		if !found {
			continue
		}
		var visitErr error
		projected, projectedOK := authority.ProjectFactoredHeapContainer(reg, value, container, func(visit func(dynamicindex.Fact)) {
			_, visitErr = domain.VisitCoordinateReturnContainerFacts(sealed.Skeleton(), root, visit)
		})
		if visitErr != nil {
			return product.Value{}, visitErr
		}
		if projectedOK {
			return projected, nil
		}
	}
	return value, nil
}

// ApplyReturnPresenceFactor applies the canonical N5 result-row presence
// component to its sole registered coordinate family.
func ApplyReturnPresenceFactor[K comparable](
	authority *ReturnAuthority,
	keys *keyspace.KeySpace,
	transaction ReturnTransaction,
	targets []ReturnFactorTarget[K],
	values state.ValueFactor[K],
	domain state.ProductDomain,
	factor state.CoordinateFamilyFactor,
) (state.CoordinateFamilyFactor, error) {
	boundTargets, err := bindReturnFactorTargets(transaction, targets)
	if err != nil {
		return factor, err
	}
	if authority == nil || !authority.Valid() || keys == nil || !keys.Valid() ||
		!transaction.Valid() || !domain.Valid() || domain.Registry() == nil {
		return factor, fmt.Errorf("factapply: invalid return presence transaction")
	}
	if len(boundTargets) < 2 {
		return factor, nil
	}
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok || factor.Family() != family || factor.Skeleton().KeySpace() != keys {
		return factor, fmt.Errorf("factapply: return presence has no registered coordinate authority")
	}
	sealed, err := domain.SealCoordinateFamilyFactor(factor.Skeleton(), factor.Scalars())
	if err != nil {
		return factor, err
	}
	rows := make([]CallReturnPresenceRowTarget, 0, len(boundTargets))
	reg := domain.Registry()
	for index := 0; index < transaction.ResultTargetCount(); index++ {
		target, ok := transaction.ResultTarget(index)
		if !ok {
			return factor, fmt.Errorf("factapply: malformed return target schema")
		}
		value := product.Bottom(reg)
		if values.Top {
			value = product.Top()
		} else if found, present := values.Values[boundTargets[target].Slot]; present {
			value = found
		}
		rows = append(rows, CallReturnPresenceRowTarget{
			Index: target, Path: boundTargets[target].Path, Value: value,
		})
	}
	plan, err := authority.paths.PrepareCallReturnPresenceRowInKeySpace(reg, keys, transaction.Point(), rows)
	if err != nil {
		return factor, err
	}
	skeleton, scalars, err := plan.ApplyCoordinates(domain, sealed.Skeleton(), sealed.Scalars())
	if err != nil {
		return factor, err
	}
	return domain.SealCoordinateFamilyFactor(skeleton, scalars)
}

func bindReturnFactorTargets[K comparable](transaction ReturnTransaction, targets []ReturnFactorTarget[K]) (map[int]ReturnFactorTarget[K], error) {
	if !transaction.Valid() {
		return nil, fmt.Errorf("factapply: invalid return transaction")
	}
	out := make(map[int]ReturnFactorTarget[K], len(targets))
	for _, target := range targets {
		if target.Index < 0 || target.Path.Kind == keyspace.KindInvalid {
			return nil, fmt.Errorf("factapply: return target has invalid output identity")
		}
		if _, duplicate := out[target.Index]; duplicate {
			return nil, fmt.Errorf("factapply: duplicate return target %d", target.Index)
		}
		out[target.Index] = target
	}
	if len(out) != transaction.ResultTargetCount() {
		return nil, fmt.Errorf("factapply: incomplete return target schema")
	}
	for index := 0; index < transaction.ResultTargetCount(); index++ {
		target, ok := transaction.ResultTarget(index)
		if !ok {
			return nil, fmt.Errorf("factapply: malformed return target schema")
		}
		if _, present := out[target]; !present {
			return nil, fmt.Errorf("factapply: missing return target %d", target)
		}
	}
	return out, nil
}

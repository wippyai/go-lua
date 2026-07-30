package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// applyPathSubtreeMutationFactorLanes is the sole whole-lane edge adapter for
// factor-native subtree mutation. It transposes into the sealed exclusive
// tuple, invokes the ProductDomain law once, and patches only the represented
// families back into the caller-owned factors.
func applyPathSubtreeMutationFactorLanes(
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	transaction state.PathSubtreeMutation,
	factors []state.LaneFactor,
) ([]state.LaneFactor, error) {
	positions := make(map[state.LaneOrdinal]int, len(factors))
	for index, factor := range factors {
		lane := factor.Lane()
		if _, duplicate := positions[lane.Ordinal()]; duplicate {
			return nil, fmt.Errorf("factapply: duplicate path-subtree factor lane %s", lane.ID())
		}
		positions[lane.Ordinal()] = index
	}
	current, err := domain.BindPathSubtreeMutationFactors(keys, func(lane state.ProductLane) (state.LaneFactor, bool) {
		index, present := positions[lane.Ordinal()]
		if !present || factors[index].Lane() != lane {
			return state.LaneFactor{}, false
		}
		return factors[index], true
	})
	if err != nil {
		return nil, err
	}
	next, err := domain.ApplyPathSubtreeMutationFactors(transaction, current)
	if err != nil {
		return nil, err
	}
	out := append([]state.LaneFactor(nil), factors...)
	for _, factor := range next.LaneFactors() {
		index, present := positions[factor.Lane().Ordinal()]
		if !present || out[index].Lane() != factor.Lane() {
			return nil, fmt.Errorf("factapply: path-subtree lane result has no owner")
		}
		out[index] = factor
	}
	for _, factor := range next.CoordinateFactors() {
		index, present := positions[factor.Family().Lane().Ordinal()]
		if !present || out[index].Lane() != factor.Family().Lane() {
			return nil, fmt.Errorf("factapply: path-subtree coordinate result has no owner")
		}
		out[index], err = domain.ReplaceCoordinateFamily(out[index], factor.Skeleton(), factor.Scalars())
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

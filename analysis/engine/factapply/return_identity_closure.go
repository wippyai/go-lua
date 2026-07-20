package factapply

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// ReturnIdentityCondition associates one identity with the condition under
// which it participates in return-identity closure.
type ReturnIdentityCondition[C any] struct {
	Root      identity.Term
	Condition C
}

// ReturnIdentityEdgeCondition associates one directed identity edge with the
// condition under which it may carry reachability.
type ReturnIdentityEdgeCondition[C any] struct {
	From      identity.Term
	To        identity.Term
	Condition C
}

// ReturnBooleanAlgebra is the complete condition vocabulary required by
// return-identity closure. C must be a finite canonical Boolean algebra:
// Equal must decide semantic equality and And, Or, and Not must return the
// unique representative of their Boolean result. Concrete execution binds C
// to bool; guarded formal execution binds it to its canonical decision node.
type ReturnBooleanAlgebra[C any] struct {
	False C
	And   func(C, C) (C, error)
	Or    func(C, C) (C, error)
	Not   func(C) (C, error)
	Equal func(C, C) bool
}

func (a ReturnBooleanAlgebra[C]) valid() bool {
	return a.And != nil && a.Or != nil && a.Not != nil && a.Equal != nil
}

type sealedReturnIdentityEdge[C any] struct {
	from      identity.Term
	to        identity.Term
	condition C
}

// CloseReturnIdentities computes the least guarded reachability solution.
// Sources, admissions, and edges are independently sealed: structural keys
// are sorted and duplicate conditions are joined before evaluation. The
// worklist propagates only the newly reached condition, so cycles terminate by
// the Boolean algebra's own finite canonical representation rather than a
// depth, work, SCC, or time budget.
func CloseReturnIdentities[C any](
	ctx context.Context,
	algebra ReturnBooleanAlgebra[C],
	sources []ReturnIdentityCondition[C],
	admissions []ReturnIdentityCondition[C],
	edges []ReturnIdentityEdgeCondition[C],
) ([]ReturnIdentityCondition[C], error) {
	if ctx == nil || !algebra.valid() {
		return nil, fmt.Errorf("factapply: invalid return-identity closure")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sealedSources, err := sealReturnIdentityConditions(ctx, algebra, sources)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sealedAdmissions, err := sealReturnIdentityConditions(ctx, algebra, admissions)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sealedEdges, err := sealReturnIdentityEdges(ctx, algebra, edges)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	admissionByRoot := make(map[identity.Term]C, len(sealedAdmissions))
	for _, admission := range sealedAdmissions {
		admissionByRoot[admission.Root] = admission.Condition
	}
	total := make(map[identity.Term]C)
	pending := make(map[identity.Term]C)
	queue := make([]identity.Term, 0, len(sealedSources))
	queued := make(map[identity.Term]bool)
	for _, source := range sealedSources {
		admission, present := admissionByRoot[source.Root]
		if !present {
			continue
		}
		condition, andErr := algebra.And(source.Condition, admission)
		if andErr != nil {
			return nil, andErr
		}
		if algebra.Equal(condition, algebra.False) {
			continue
		}
		total[source.Root] = condition
		pending[source.Root] = condition
		queue = append(queue, source.Root)
		queued[source.Root] = true
	}

	firstEdge := make(map[identity.Term]int)
	for index, edge := range sealedEdges {
		if _, present := firstEdge[edge.from]; !present {
			firstEdge[edge.from] = index
		}
	}
	edgesVisited := 0
	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		from := queue[head]
		delta := pending[from]
		delete(pending, from)
		queued[from] = false
		start, present := firstEdge[from]
		if !present {
			continue
		}
		for index := start; index < len(sealedEdges) && sealedEdges[index].from == from; index++ {
			edgesVisited++
			if edgesVisited&255 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			edge := sealedEdges[index]
			candidate, andErr := algebra.And(delta, edge.condition)
			if andErr != nil {
				return nil, andErr
			}
			prior, reached := total[edge.to]
			if !reached {
				prior = algebra.False
			}
			notPrior, notErr := algebra.Not(prior)
			if notErr != nil {
				return nil, notErr
			}
			fresh, freshErr := algebra.And(candidate, notPrior)
			if freshErr != nil {
				return nil, freshErr
			}
			if algebra.Equal(fresh, algebra.False) {
				continue
			}
			joined, orErr := algebra.Or(prior, fresh)
			if orErr != nil {
				return nil, orErr
			}
			total[edge.to] = joined
			if oldPending, exists := pending[edge.to]; exists {
				fresh, orErr = algebra.Or(oldPending, fresh)
				if orErr != nil {
					return nil, orErr
				}
			}
			pending[edge.to] = fresh
			if !queued[edge.to] {
				queue = append(queue, edge.to)
				queued[edge.to] = true
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]ReturnIdentityCondition[C], 0, len(total))
	for root, condition := range total {
		if !algebra.Equal(condition, algebra.False) {
			result = append(result, ReturnIdentityCondition[C]{Root: root, Condition: condition})
		}
	}
	sort.Slice(result, func(i, j int) bool { return identity.Less(result[i].Root, result[j].Root) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func sealReturnIdentityConditions[C any](ctx context.Context, algebra ReturnBooleanAlgebra[C], input []ReturnIdentityCondition[C]) ([]ReturnIdentityCondition[C], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := append([]ReturnIdentityCondition[C](nil), input...)
	for index, condition := range out {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !condition.Root.Valid() {
			return nil, fmt.Errorf("factapply: return-identity condition has invalid root")
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return identity.Less(out[i].Root, out[j].Root) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sealed := out[:0]
	for index, condition := range out {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if len(sealed) == 0 || sealed[len(sealed)-1].Root != condition.Root {
			sealed = append(sealed, condition)
			continue
		}
		joined, err := algebra.Or(sealed[len(sealed)-1].Condition, condition.Condition)
		if err != nil {
			return nil, err
		}
		sealed[len(sealed)-1].Condition = joined
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return sealed, nil
}

func sealReturnIdentityEdges[C any](ctx context.Context, algebra ReturnBooleanAlgebra[C], input []ReturnIdentityEdgeCondition[C]) ([]sealedReturnIdentityEdge[C], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]sealedReturnIdentityEdge[C], len(input))
	for index, edge := range input {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !edge.From.Valid() || !edge.To.Valid() {
			return nil, fmt.Errorf("factapply: return-identity edge has invalid endpoint")
		}
		out[index] = sealedReturnIdentityEdge[C]{from: edge.From, to: edge.To, condition: edge.Condition}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].from != out[j].from {
			return identity.Less(out[i].from, out[j].from)
		}
		return identity.Less(out[i].to, out[j].to)
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sealed := out[:0]
	for index, edge := range out {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if len(sealed) == 0 || sealed[len(sealed)-1].from != edge.from || sealed[len(sealed)-1].to != edge.to {
			sealed = append(sealed, edge)
			continue
		}
		joined, err := algebra.Or(sealed[len(sealed)-1].condition, edge.condition)
		if err != nil {
			return nil, err
		}
		sealed[len(sealed)-1].condition = joined
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return sealed, nil
}

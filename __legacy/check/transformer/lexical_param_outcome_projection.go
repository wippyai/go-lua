package transformer

import (
	"sort"

	valueref "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// projectLexicalParamOutcomeFacts closes the two expression-facing DTO lanes
// that cannot be replaced by path fact application alone. The inputs are the
// same already-joined child BoundaryWorld/BoundaryRoots used for every other
// State-owned CallOutcome lane; no body or summary is replayed.
func projectLexicalParamOutcomeFacts(reg *axis.Registry, roots state.BoundaryRoots, paramCount int, facts callboundary.NormalReturnFacts) ([]callpayload.CallParamCondition, []callpayload.CallParamPathRelation) {
	conditions := make([]callpayload.CallParamCondition, 0, paramCount)
	for index := 0; index < paramCount && index < len(roots); index++ {
		truthy := valueref.CanBeTruthy(reg, roots[index].Value)
		falsy := valueref.CanBeFalsy(reg, roots[index].Value)
		if truthy == falsy {
			continue
		}
		conditions = append(conditions, callpayload.CallParamCondition{ParamIndex: index, Value: truthy})
	}

	type pair struct{ left, right int }
	set := make(map[pair]struct{})
	for _, proof := range facts.BranchProofs {
		if proof.Kind != pathevidence.BranchProofPathEqual || !proof.Path.IsPlaceholder() || !proof.Other.IsPlaceholder() ||
			len(proof.Path.Segments) != 0 || len(proof.Other.Segments) != 0 {
			continue
		}
		left, right := proof.Path.PlaceholderIndex(), proof.Other.PlaceholderIndex()
		if left < 0 || right < 0 || left >= paramCount || right >= paramCount || left == right {
			continue
		}
		if right < left {
			left, right = right, left
		}
		set[pair{left: left, right: right}] = struct{}{}
	}
	pairs := make([]pair, 0, len(set))
	for relation := range set {
		pairs = append(pairs, relation)
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].left < pairs[j].left || pairs[i].left == pairs[j].left && pairs[i].right < pairs[j].right
	})
	relations := make([]callpayload.CallParamPathRelation, 0, len(pairs))
	for _, relation := range pairs {
		relations = append(relations, callpayload.CallParamPathRelation{
			Kind: callpayload.CallPathRelationEqual,
			Left: pathdom.NewPlaceholder(relation.left), Right: pathdom.NewPlaceholder(relation.right),
		})
	}
	return conditions, relations
}

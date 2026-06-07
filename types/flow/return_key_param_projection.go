package flow

import "github.com/wippyai/go-lua/types/constraint"

// ReturnKeyParamProofQuery asks whether a returned value is a proven key of a
// caller-visible parameter path. The query owns the key-presence storage shape:
// callers supply source paths and boundary projection, not stable-address keys.
type ReturnKeyParamProofQuery struct {
	ReturnIndex int
	KeyPath     constraint.Path
	KeyPresence KeyPresenceFacts
	Boundary    BoundaryPathProjection
}

// ProjectReturnKeyParamProof projects point-local key-presence facts into the
// caller-visible return-relation domain.
func ProjectReturnKeyParamProof(q ReturnKeyParamProofQuery) ReturnRelations {
	if q.ReturnIndex < 0 || q.KeyPath.Symbol == 0 {
		return ReturnRelationsDomain.Top()
	}
	key, ok := StableAddressOfPath(q.KeyPath)
	if !ok {
		return ReturnRelationsDomain.Top()
	}
	var proven []ReturnKeyParamRelation
	for _, table := range q.KeyPresence.TablesWithKeyAddress(key) {
		for _, path := range q.Boundary.PathsFromAddress(table.Address) {
			if path.Kind != BoundaryPathParam || path.Index < 0 {
				continue
			}
			proven = append(proven, ReturnKeyParamRelation{
				ReturnIndex:   q.ReturnIndex,
				ParamIndex:    path.Index,
				ParamSegments: path.Segments,
			})
		}
	}
	return ReturnRelationsOfKeyParams(proven)
}

package transformer

import (
	"context"
	"fmt"
)

// FreezeAcyclicRelation publishes one already-built lexical relation through
// the same transactional snapshot boundary used by the relation SCC solver.
// It is the direct-call base case; callers cannot construct a snapshot that
// bypasses arena/shape ownership validation.
func FreezeAcyclicRelation(ctx context.Context, ref CellRef, relation Relation) (RelationSnapshot, error) {
	if relation.arena == nil {
		return RelationSnapshot{}, fmt.Errorf("transformer: cannot freeze relation without arena ownership")
	}
	return SolveRelationCells(ctx, []RelationCell{{
		Ref: ref, Arena: relation.arena, Shape: relation.shape,
		Equation: func(context.Context, RelationView) (Relation, error) { return relation, nil },
	}}, RelationSolveOptions{})
}

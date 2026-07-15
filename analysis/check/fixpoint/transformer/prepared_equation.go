package transformer

import (
	"context"
	"fmt"
)

// PreparedEquationEvaluator builds one cell relation in its persistent Builder
// arena from the transaction-local dependency view. It must not retain view.
type PreparedEquationEvaluator func(context.Context, RelationView, *Builder) (Relation, error)

// PreparedEquation owns the durable symbolic arena for one lexical function.
// SCC rounds re-evaluate equations, but hash-consed terms and descriptors stay
// attached to the cell instead of being rebuilt in throwaway arenas.
type PreparedEquation struct {
	ref          CellRef
	shape        Shape
	builder      *Builder
	dependencies []CellRef
	evaluate     PreparedEquationEvaluator
}

// NewPreparedEquation validates and snapshots a lexical equation. Dependency
// order and duplicates are canonicalized once, before the SCC transaction.
func NewPreparedEquation(ref CellRef, builder *Builder, dependencies []CellRef, evaluate PreparedEquationEvaluator) (*PreparedEquation, error) {
	if ref == (CellRef{}) {
		return nil, fmt.Errorf("transformer: prepared equation has zero cell identity")
	}
	if builder == nil || builder.arena == nil || builder.arena.reg == nil {
		return nil, fmt.Errorf("transformer: prepared equation has no registered builder")
	}
	if evaluate == nil {
		return nil, fmt.Errorf("transformer: prepared equation has no evaluator")
	}
	return &PreparedEquation{
		ref: ref, shape: builder.shape, builder: builder,
		dependencies: canonicalCellRefs(dependencies), evaluate: evaluate,
	}, nil
}

// Builder returns the persistent lexical builder. Callers may use it during
// preparation to intern boundary terms before publishing the RelationCell.
func (p *PreparedEquation) Builder() *Builder {
	if p == nil {
		return nil
	}
	return p.builder
}

// Cell publishes the immutable SCC equation descriptor. The closure verifies
// identity immediately, in addition to RelationCell's publication-time gate,
// so foreign arenas cannot enter later SCC rounds.
func (p *PreparedEquation) Cell() (RelationCell, error) {
	if p == nil || p.builder == nil || p.evaluate == nil {
		return RelationCell{}, fmt.Errorf("transformer: nil prepared equation")
	}
	cell := RelationCell{
		Ref: p.ref, Arena: p.builder.arena, Shape: p.shape, Bottom: p.builder.bottomRelation(),
		Dependencies: append([]CellRef(nil), p.dependencies...),
	}
	cell.Equation = func(ctx context.Context, view RelationView) (Relation, error) {
		relation, err := p.evaluate(ctx, view, p.builder)
		if err != nil {
			return Relation{}, err
		}
		if relation.arena != p.builder.arena || relation.shape != p.shape {
			return Relation{}, fmt.Errorf("prepared equation returned foreign relation identity")
		}
		return relation, nil
	}
	return cell, nil
}

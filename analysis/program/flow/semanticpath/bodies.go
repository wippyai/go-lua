package semanticpath

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func deriveBodyPaths(view authored.View, bodies *body.Result, resolver *structuralResolver) ([]identity.ContentID, error) {
	paths := make([]identity.ContentID, bodies.BodyCount())
	resolver.body = paths
	relations, children, rootsOfForest, err := indexBodyRelations(view, bodies)
	if err != nil {
		return nil, err
	}
	queue := append([]uint32(nil), rootsOfForest...)
	for head := 0; head < len(queue); head++ {
		ordinal := queue[head]
		if ordinal == 0 || int(ordinal) > len(paths) {
			return nil, errors.New("semanticpath: Body ordinal is invalid")
		}
		index := ordinal - 1
		if paths[index].Available() {
			continue
		}
		parent, hasParent := bodies.Parent(keyspace.MakeTerm(keyspace.FamilyBody, ordinal))
		if !hasParent {
			paths[index] = sha256.Sum256([]byte("wippy/program/flow/semantic-body-root-v2"))
		} else {
			parentOrdinal := keyspace.TermOrdinal(parent)
			if parentOrdinal == 0 || int(parentOrdinal) > len(paths) || !paths[parentOrdinal-1].Available() {
				return nil, errors.New("semanticpath: Body parent path is unavailable")
			}
			relation := relations[index]
			ownerPath, ok := resolver.resolve(relation.owner, parent)
			if !ok {
				return nil, fmt.Errorf("semanticpath: Body %d owner has no structural path", ordinal)
			}
			edge := digestPath3("semantic-body-edge-v2", ownerPath, relation.relation, relation.rank, uint32(keyspace.TermFamily(relation.owner)), source.Span{})
			paths[index] = digestBytes("semantic-body-child-v2", paths[parentOrdinal-1], edge)
		}
		queue = append(queue, children[index]...)
	}
	for i := range paths {
		if !paths[i].Available() {
			return nil, fmt.Errorf("semanticpath: Body %d cannot be ordered", i+1)
		}
	}
	return paths, nil
}

type bodyRelationRow struct {
	owner          keyspace.Term
	relation, rank uint32
}

func indexBodyRelations(view authored.View, bodies *body.Result) ([]bodyRelationRow, [][]uint32, []uint32, error) {
	n := bodies.BodyCount()
	rows := make([]bodyRelationRow, n)
	children := make([][]uint32, n)
	roots := make([]uint32, 0, n)
	set := func(child, owner keyspace.Term, relation, rank uint32) error {
		o := keyspace.TermOrdinal(child)
		if keyspace.TermFamily(child) != keyspace.FamilyBody || o == 0 || int(o) > n || rows[o-1].owner != 0 {
			return errors.New("semanticpath: duplicate or invalid Body owner")
		}
		rows[o-1] = bodyRelationRow{owner, relation, rank}
		return nil
	}
	functions := view.Functions()
	for i := 0; i < functions.Count(); i++ {
		term, ok := functions.At(i)
		if !ok {
			return nil, nil, nil, errors.New("semanticpath: Function row unavailable")
		}
		_, child, _, ok := functions.Get(term)
		if ok {
			if err := set(child, term, 2, 0); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	branches := view.Control().Branches()
	for i := 0; i < branches.Count(); i++ {
		term, ok := branches.At(i)
		if !ok {
			return nil, nil, nil, errors.New("semanticpath: Branch row unavailable")
		}
		_, _, yes, no, ok := branches.Get(term)
		if ok {
			if err := set(yes, term, 3, 1); err != nil {
				return nil, nil, nil, err
			}
			if err := set(no, term, 3, 2); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	loops := view.Control().Loops()
	for i := 0; i < loops.Count(); i++ {
		term, ok := loops.At(i)
		if !ok {
			return nil, nil, nil, errors.New("semanticpath: Loop row unavailable")
		}
		_, child, _, _, ok := loops.Get(term)
		if ok {
			if err := set(child, term, 4, 0); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	// A lexical do-block is represented directly as a Body root. Function,
	// Branch, and Loop children were claimed above by their typed owner; any
	// remaining Body root owns its direct child relation itself.
	for parentIndex := 0; parentIndex < n; parentIndex++ {
		parent := keyspace.MakeTerm(keyspace.FamilyBody, uint32(parentIndex+1))
		count, countOK := bodies.RootCount(parent)
		if !countOK || count < 0 {
			return nil, nil, nil, errors.New("semanticpath: direct Body root denominator unavailable")
		}
		for rootIndex := 0; rootIndex < count; rootIndex++ {
			child, childOK := bodies.RootAt(parent, rootIndex)
			if !childOK {
				return nil, nil, nil, errors.New("semanticpath: direct Body root unavailable")
			}
			if keyspace.TermFamily(child) != keyspace.FamilyBody {
				continue
			}
			ordinal := keyspace.TermOrdinal(child)
			actualParent, parentOK := bodies.Parent(child)
			if ordinal == 0 || int(ordinal) > n || !parentOK || actualParent != parent || rows[ordinal-1].owner != 0 {
				return nil, nil, nil, errors.New("semanticpath: malformed direct Body relation")
			}
			if err := set(child, child, 1, 0); err != nil {
				return nil, nil, nil, err
			}
		}
	}
	for i := 0; i < n; i++ {
		child := keyspace.MakeTerm(keyspace.FamilyBody, uint32(i+1))
		parent, has := bodies.Parent(child)
		if !has {
			roots = append(roots, uint32(i+1))
			continue
		}
		po := keyspace.TermOrdinal(parent)
		if keyspace.TermFamily(parent) != keyspace.FamilyBody || po == 0 || int(po) > n || rows[i].owner == 0 {
			return nil, nil, nil, fmt.Errorf("semanticpath: malformed Body parent relation body=%d parent-family=%d parent-ordinal=%d owner-family=%d", i+1, keyspace.TermFamily(parent), po, keyspace.TermFamily(rows[i].owner))
		}
		children[po-1] = append(children[po-1], uint32(i+1))
	}
	return rows, children, roots, nil
}

package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

// contextBindRanges indexes authored Binds by Body in one dense pass. The
// linked ordinals avoid a Body-by-Bind scan while retaining source order
// semantics at each Bind's own Position.
func contextBindRanges(view authored.View, bodyCount int) ([]int, []int, error) {
	binds := view.Storage().Binds()
	first := make([]int, bodyCount+1)
	next := make([]int, binds.Count()+1)
	for ordinal := 1; ordinal <= binds.Count(); ordinal++ {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, uint32(ordinal))
		owner, _, ok := binds.Get(bind)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 || int(keyspace.TermOrdinal(owner)) > bodyCount {
			return nil, nil, errors.New("program/flow/staticcheck: Bind owner is unavailable")
		}
		ownerOrdinal := int(keyspace.TermOrdinal(owner))
		next[ordinal] = first[ownerOrdinal]
		first[ownerOrdinal] = ordinal
	}
	return first, next, nil
}

// contextParamRanges indexes TypeParams by Function in one dense pass. This
// makes header construction proportional to the parameters it owns.
func contextParamRanges(view static.View, functionCount int) ([]int, []int, error) {
	params := view.Declarations().TypeParams()
	first := make([]int, functionCount+1)
	next := make([]int, params.Count()+1)
	for ordinal := 1; ordinal <= params.Count(); ordinal++ {
		param := keyspace.MakeTerm(keyspace.FamilyTypeParam, uint32(ordinal))
		owner, _, _, ok := params.Get(param)
		if !ok || keyspace.TermOrdinal(owner) == 0 {
			return nil, nil, errors.New("program/flow/staticcheck: TypeParam owner is unavailable")
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyFunction {
			if keyspace.TermFamily(owner) != keyspace.FamilyTypeAlias && keyspace.TermFamily(owner) != keyspace.FamilyTypeFunction {
				return nil, nil, errors.New("program/flow/staticcheck: TypeParam owner is unavailable")
			}
			continue
		}
		if int(keyspace.TermOrdinal(owner)) > functionCount {
			return nil, nil, errors.New("program/flow/staticcheck: TypeParam owner is unavailable")
		}
		ownerOrdinal := int(keyspace.TermOrdinal(owner))
		next[ordinal] = first[ownerOrdinal]
		first[ownerOrdinal] = ordinal
	}
	return first, next, nil
}

// contextIntervals assigns Euler intervals to the point tree with an explicit
// stack walk. Every lexical visibility query then reduces to one
// interval containment check, including deep and wide nested Bodies.
func contextIntervals(tree *contextTree) error {
	if tree == nil || len(tree.points) == 0 {
		return errors.New("program/flow/staticcheck: lexical point tree is unavailable")
	}
	firstChild := make([]int, len(tree.points))
	nextSibling := make([]int, len(tree.points))
	for point := 1; point < len(tree.points); point++ {
		outer := tree.points[point].outer
		if outer < 0 || outer >= len(tree.points) {
			return errors.New("program/flow/staticcheck: lexical point parent is unavailable")
		}
		nextSibling[point] = firstChild[outer]
		firstChild[outer] = point
	}
	tree.tin = make([]int, len(tree.points))
	tree.tout = make([]int, len(tree.points))
	next := append([]int(nil), firstChild...)
	stack := make([]int, 0)
	stack = append(stack, 0)
	clock := 0
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		if tree.tin[current] == 0 {
			clock++
			tree.tin[current] = clock
		}
		child := next[current]
		if child != 0 {
			next[current] = nextSibling[child]
			stack = append(stack, child)
			continue
		}
		tree.tout[current] = clock
		stack = stack[:len(stack)-1]
	}
	if clock != len(tree.points) {
		return errors.New("program/flow/staticcheck: lexical point tree is disconnected")
	}
	return nil
}

// contextValidateFunctionGenerics checks each executable Function's complete
// TypeParam chain once, after lexical intervals exist. Scope rows can then
// test the cached dense range in O(1) without rewalking shared parameters.
func contextValidateFunctionGenerics(view authored.View, tree *contextTree) error {
	if tree == nil {
		return errors.New("program/flow/staticcheck: lexical generic context is unavailable")
	}
	functions := view.Functions()
	for index := 0; index < functions.Count(); index++ {
		function, functionOK := functions.At(index)
		owner, functionBody, _, rowOK := functions.Get(function)
		if !functionOK || !rowOK || keyspace.TermFamily(owner) != keyspace.FamilyBody ||
			keyspace.TermFamily(functionBody) != keyspace.FamilyBody {
			return errors.New("program/flow/staticcheck: Function generic owner is unavailable")
		}
		functionOrdinal := int(keyspace.TermOrdinal(function))
		bodyOrdinal := int(keyspace.TermOrdinal(functionBody))
		if functionOrdinal <= 0 || functionOrdinal >= len(tree.paramFirst) || bodyOrdinal <= 0 || bodyOrdinal >= len(tree.bodies) {
			return errors.New("program/flow/staticcheck: Function generic range is unavailable")
		}
		genericPoint := tree.bodies[bodyOrdinal].base
		if genericPoint == 0 {
			return errors.New("program/flow/staticcheck: Function generic point is unavailable")
		}
		for ordinal := tree.paramFirst[functionOrdinal]; ordinal != 0; ordinal = tree.paramNext[ordinal] {
			param := keyspace.MakeTerm(keyspace.FamilyTypeParam, uint32(ordinal))
			if !tree.paramVisible(genericPoint, param) {
				return errors.New("program/flow/staticcheck: Function generic is not visible at creation")
			}
		}
	}
	return nil
}

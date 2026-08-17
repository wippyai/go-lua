package semanticpath

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// edgeDescriptor is a semantic relation role. It has no Term ordinal; the
// ordinal is used only to locate the row while deriving the plane and is not
// part of a published identity.
type edgeDescriptor struct {
	kind uint32
	rank uint32
}

// mergeContainmentRoles transfers only semantic relation labels already
// issued by the canonical containment owner. Flow-authored edge roles remain
// authoritative where present; a second, disagreeing role is malformed.
func mergeContainmentRoles(sourceView source.View, forest *containment.Result, paths *[keyspace.FamilyCount][]edgeDescriptor) error {
	if forest == nil || paths == nil {
		return errors.New("semanticpath: containment edge roles are unavailable")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for index := range paths[family] {
			term := keyspace.MakeTerm(family, uint32(index+1))
			role, rank, ok := forest.StructuralRole(term)
			if !ok {
				continue
			}
			row := &paths[family][index]
			if row.kind != 0 && (row.kind != role || row.rank != rank) {
				return fmt.Errorf("semanticpath: conflicting owner-issued containment role for %v", term)
			}
			row.kind, row.rank = role, rank
		}
	}
	return nil
}

func makePlane(view source.View) [keyspace.FamilyCount][]edgeDescriptor {
	var plane [keyspace.FamilyCount][]edgeDescriptor
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if count := view.Identity().FamilyCount(family); count > 0 {
			plane[family] = make([]edgeDescriptor, count)
		}
	}
	return plane
}

func deriveEdges(sourceView source.View, view authored.View, forest *containment.Result) ([keyspace.FamilyCount][]edgeDescriptor, error) {
	paths := makePlane(sourceView)
	set := func(parent, child keyspace.Term, relation, rank uint32) error {
		family, ordinal := keyspace.TermFamily(child), keyspace.TermOrdinal(child)
		if child == 0 || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(paths[family])) {
			return nil
		}
		got, ok := forest.Parent(child)
		if !ok || got != parent {
			return nil
		}
		row := &paths[family][ordinal-1]
		if row.kind != 0 && (row.kind != relation || row.rank != rank) {
			return fmt.Errorf("semanticpath: conflicting containment edge for %v", child)
		}
		row.kind, row.rank = relation, rank
		return nil
	}

	values := view.Values()
	for i := 0; i < values.Count(); i++ {
		parent, ok := values.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Values denominator unavailable")
		}
		width, ok := values.Len(parent)
		if !ok {
			return paths, errors.New("semanticpath: Values row unavailable")
		}
		for j := 0; j < width; j++ {
			child, ok := values.Member(parent, j)
			if ok {
				if err := set(parent, child, 1, uint32(j+1)); err != nil {
					return paths, err
				}
			}
		}
		if _, tail, ok := values.Get(parent); ok && tail != 0 {
			if err := set(parent, tail, 1, uint32(width+1)); err != nil {
				return paths, err
			}
		}
	}
	exact := view.Access().Exact()
	for i := 0; i < exact.Count(); i++ {
		parent, ok := exact.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Exact denominator unavailable")
		}
		_, base, sourceTerm, _, rowOK := exact.Get(parent)
		if rowOK {
			if err := set(parent, base, 2, 1); err != nil {
				return paths, err
			}
			if err := set(parent, sourceTerm, 2, 2); err != nil {
				return paths, err
			}
		}
	}
	dynamic := view.Access().Dynamic()
	for i := 0; i < dynamic.Count(); i++ {
		parent, ok := dynamic.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Dynamic denominator unavailable")
		}
		_, base, key, rowOK := dynamic.Get(parent)
		if rowOK {
			if err := set(parent, base, 3, 1); err != nil {
				return paths, err
			}
			if err := set(parent, key, 3, 2); err != nil {
				return paths, err
			}
		}
	}
	reads := view.Storage().Reads()
	for i := 0; i < reads.Count(); i++ {
		parent, ok := reads.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Read denominator unavailable")
		}
		_, sourceTerm, _, rowOK := reads.Get(parent)
		if rowOK {
			if err := set(parent, sourceTerm, 4, 1); err != nil {
				return paths, err
			}
		}
	}
	varargs := view.Storage().Varargs()
	for i := 0; i < varargs.Count(); i++ {
		parent, ok := varargs.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Vararg denominator unavailable")
		}
		_, cell, rowOK := varargs.Get(parent)
		if rowOK {
			if err := set(parent, cell, 5, 1); err != nil {
				return paths, err
			}
		}
	}
	binds := view.Storage().Binds()
	for i := 0; i < binds.Count(); i++ {
		parent, ok := binds.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Bind denominator unavailable")
		}
		_, child, rowOK := binds.Get(parent)
		if rowOK {
			if err := set(parent, child, 6, 1); err != nil {
				return paths, err
			}
		}
	}
	assigns := view.Storage().Assigns()
	for i := 0; i < assigns.Count(); i++ {
		parent, ok := assigns.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Assign denominator unavailable")
		}
		_, child, rowOK := assigns.Get(parent)
		if rowOK {
			if err := set(parent, child, 7, 1); err != nil {
				return paths, err
			}
			for j := 0; ; j++ {
				write, ok := assigns.WriteAt(parent, j)
				if !ok {
					break
				}
				if err := set(parent, write, 8, uint32(j+1)); err != nil {
					return paths, err
				}
			}
		}
	}
	writes := view.Storage().Writes()
	for i := 0; i < writes.Count(); i++ {
		parent, ok := writes.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Write denominator unavailable")
		}
		_, child, rowOK := writes.Get(parent)
		if rowOK {
			if err := set(parent, child, 9, 1); err != nil {
				return paths, err
			}
		}
	}
	calls := view.Calls()
	for i := 0; i < calls.Count(); i++ {
		parent, ok := calls.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Call denominator unavailable")
		}
		_, callee, receiver, actuals, rowOK := calls.Get(parent)
		if rowOK {
			if err := set(parent, callee, 10, 1); err != nil {
				return paths, err
			}
			if receiver != 0 {
				if err := set(parent, receiver, 10, 2); err != nil {
					return paths, err
				}
			}
			if err := set(parent, actuals, 10, 3); err != nil {
				return paths, err
			}
		}
	}
	unary := view.Operators().Unaries()
	for i := 0; i < unary.Count(); i++ {
		parent, ok := unary.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Unary denominator unavailable")
		}
		_, _, child, rowOK := unary.Get(parent)
		if rowOK {
			if err := set(parent, child, 11, 1); err != nil {
				return paths, err
			}
		}
	}
	binary := view.Operators().Binaries()
	for i := 0; i < binary.Count(); i++ {
		parent, ok := binary.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Binary denominator unavailable")
		}
		_, _, left, right, rowOK := binary.Get(parent)
		if rowOK {
			if err := set(parent, left, 12, 1); err != nil {
				return paths, err
			}
			if err := set(parent, right, 12, 2); err != nil {
				return paths, err
			}
		}
	}
	selects := view.Operators().Selects()
	for i := 0; i < selects.Count(); i++ {
		parent, ok := selects.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Select denominator unavailable")
		}
		_, _, left, right, rowOK := selects.Get(parent)
		if rowOK {
			if err := set(parent, left, 13, 1); err != nil {
				return paths, err
			}
			if err := set(parent, right, 13, 2); err != nil {
				return paths, err
			}
		}
	}
	claims := view.Claims()
	for i := 0; i < claims.Count(); i++ {
		parent, ok := claims.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Claim denominator unavailable")
		}
		_, child, _, rowOK := claims.Get(parent)
		if rowOK {
			if err := set(parent, child, 14, 1); err != nil {
				return paths, err
			}
		}
	}
	returns := view.Control().Returns()
	for i := 0; i < returns.Count(); i++ {
		parent, ok := returns.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Return denominator unavailable")
		}
		_, child, rowOK := returns.Get(parent)
		if rowOK {
			if err := set(parent, child, 15, 1); err != nil {
				return paths, err
			}
		}
	}
	branches := view.Control().Branches()
	for i := 0; i < branches.Count(); i++ {
		parent, ok := branches.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Branch denominator unavailable")
		}
		_, condition, _, _, rowOK := branches.Get(parent)
		if rowOK {
			if err := set(parent, condition, 16, 1); err != nil {
				return paths, err
			}
		}
	}
	loops := view.Control().Loops()
	for i := 0; i < loops.Count(); i++ {
		parent, ok := loops.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Loop denominator unavailable")
		}
		_, _, _, control, rowOK := loops.Get(parent)
		if rowOK {
			if err := set(parent, control, 17, 1); err != nil {
				return paths, err
			}
		}
	}
	tables, fields := view.Tables(), view.Fields()
	for i := 0; i < tables.Count(); i++ {
		table, ok := tables.At(i)
		if !ok {
			return paths, errors.New("semanticpath: Table denominator unavailable")
		}
		n, ok := tables.FieldCount(table)
		if !ok {
			return paths, errors.New("semanticpath: Table fields unavailable")
		}
		for j := 0; j < n; j++ {
			field, ok := tables.FieldAt(table, j)
			if !ok {
				return paths, errors.New("semanticpath: Table field unavailable")
			}
			got, key, child, _, rowOK := fields.Get(field)
			if !rowOK || got != table {
				return paths, errors.New("semanticpath: Table field relation unavailable")
			}
			if err := set(table, field, 18, uint32(j+1)); err != nil {
				return paths, err
			}
			if err := set(field, key, 19, 1); err != nil {
				return paths, err
			}
			if err := set(field, child, 19, 2); err != nil {
				return paths, err
			}
		}
	}
	return paths, nil
}

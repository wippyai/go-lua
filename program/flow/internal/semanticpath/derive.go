package semanticpath

// This file is deliberately the only constructor for the semantic-path
// planes.  The public certificate is a consumer of the result; it must not be
// able to accept caller supplied rows.  All rows below are derived from the
// four owner-fenced views and the two immutable structural results.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// edgeDescriptor is a semantic relation role.  It has no Term ordinal; the
// ordinal is used only to locate the row while deriving the plane and is not
// part of a published identity.
type edgeDescriptor struct {
	kind uint32
	rank uint32
}

// derivedPlanes is intentionally private.  certificate.go is in this package
// and may consume the planes, while no downstream owner can manufacture one.
type derivedPlanes struct {
	edges           [keyspace.FamilyCount][]edgeDescriptor
	rootDescriptors [keyspace.FamilyCount][]keyspace.ContentID
	body            []keyspace.ContentID
	roots           [keyspace.FamilyCount][]keyspace.ContentID
	terms           [keyspace.FamilyCount][]keyspace.ContentID
}

// derive builds every body-qualified structural plane in one owner-fenced
// operation.  The source and authored views provide the canonical dense
// denominators; body, containment, and outcome results provide the exact
// sealed relations.  No identity plane is accepted from the caller.
func derive(sourceView source.View, cellRoles source.CellRoleCatalog, authoredView authored.View, bodies *body.Result, bindings binding.Result, forest *containment.Result, outcomes *outcome.Result, sourceID, flowID, staticID, moduleID keyspace.ContentID) (derivedPlanes, error) {
	var out derivedPlanes
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() ||
		sourceView.Identity().ContentID() != sourceID || authoredView.Cold().ContentID() != flowID {
		return out, errors.New("semanticpath: owner identities are unavailable or disagree")
	}
	if bodies == nil || !body.Matches(bodies, sourceID, flowID) {
		return out, errors.New("semanticpath: Body result does not match Source/Flow")
	}
	if forest == nil || !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return out, errors.New("semanticpath: containment result does not match owner quartet")
	}
	if outcomes == nil || !outcome.Matches(outcomes, sourceID, flowID, staticID, moduleID) {
		return out, errors.New("semanticpath: Outcome result does not match owner quartet")
	}
	if bodies.BodyCount() != sourceView.Identity().FamilyCount(keyspace.FamilyBody) {
		return out, errors.New("semanticpath: Body denominator disagrees with Source")
	}
	if !cellRoles.Matches(sourceView) || cellRoles.CellCount() != authoredView.Storage().Cells().Count() || !binding.Matches(&bindings, sourceID, flowID) || bindings.CellCount() != cellRoles.CellCount() {
		return out, errors.New("semanticpath: Cell role receipt or Binding disagrees with exact owners")
	}

	edges, err := deriveEdges(sourceView, authoredView, forest)
	if err != nil {
		return out, err
	}
	if err := mergeContainmentRoles(sourceView, forest, &edges); err != nil {
		return out, err
	}
	rootDescriptors, err := deriveRootDescriptors(sourceView, authoredView, bodies)
	if err != nil {
		return out, err
	}
	resolver := structuralResolver{source: sourceView, forest: forest, edges: edges, descriptors: rootDescriptors, memo: make(map[keyspace.Term]keyspace.ContentID), visiting: make(map[keyspace.Term]bool)}
	bodyPaths, err := deriveBodyPaths(sourceView, authoredView, bodies, forest, edges, rootDescriptors, &resolver)
	if err != nil {
		return out, err
	}
	rootPaths, err := deriveRootPaths(sourceView, bodies, bodyPaths, rootDescriptors)
	if err != nil {
		return out, err
	}
	resolver.body = bodyPaths
	termPaths, err := deriveTermPaths(sourceView, cellRoles, authoredView, bindings, bodies, forest, outcomes, edges, bodyPaths, rootDescriptors, rootPaths, &resolver)
	if err != nil {
		return out, err
	}
	out.edges, out.rootDescriptors, out.body, out.roots, out.terms = edges, rootDescriptors, bodyPaths, rootPaths, termPaths
	return out, nil
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

func deriveRootDescriptors(sourceView source.View, view authored.View, bodies *body.Result) ([keyspace.FamilyCount][]keyspace.ContentID, error) {
	var descriptors [keyspace.FamilyCount][]keyspace.ContentID
	identity, index := sourceView.Identity(), sourceView.Index()
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if n := identity.FamilyCount(family); n > 0 {
			descriptors[family] = make([]keyspace.ContentID, n)
		}
	}
	for i := 0; i < bodies.BodyCount(); i++ {
		bodyTerm, ok := bodies.BodyAt(i)
		if !ok {
			return descriptors, errors.New("semanticpath: Body denominator unavailable")
		}
		n, ok := index.BodyRootLen(bodyTerm)
		if !ok {
			return descriptors, errors.New("semanticpath: Body root denominator unavailable")
		}
		ranks := make(map[keyspace.ContentID]uint32)
		for j := 0; j < n; j++ {
			root, ok := index.BodyRootAt(bodyTerm, j)
			if !ok {
				return descriptors, errors.New("semanticpath: Body root row unavailable")
			}
			family, ordinal := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
			if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(descriptors[family])) {
				return descriptors, errors.New("semanticpath: root term is invalid")
			}
			class := rootClass(view, root)
			ranks[class]++
			id := digestPath("semantic-root-descriptor-v2", class, uint32(family), ranks[class], source.Span{})
			if descriptors[family][ordinal-1].Available() {
				return descriptors, errors.New("semanticpath: duplicate root descriptor")
			}
			descriptors[family][ordinal-1] = id
		}
	}
	// Body.Roots intentionally contains executable statement roots only. Source
	// nevertheless gives certain direct metadata terms (notably Label) their
	// own exact direct position/root so the rest of Flow can address them. Seed
	// those roots from that committed Source coordinate here; otherwise a valid
	// Label has no descriptor for the structural resolver merely because it is
	// not an executable Body root. The source offset is the exact local ordering
	// witness, while the Body path is added when the descriptor is consumed.
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := 1; ordinal <= len(descriptors[family]); ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			root, rootOK := index.Root(term)
			if !rootOK || root != term || descriptors[family][ordinal-1].Available() {
				continue
			}
			bodyTerm, offset, cursor, positionOK := index.Position(term)
			if !positionOK || keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || keyspace.TermOrdinal(bodyTerm) == 0 ||
				uint64(keyspace.TermOrdinal(bodyTerm)) > uint64(bodies.BodyCount()) || offset < 0 || cursor < 0 ||
				uint64(offset) >= uint64(^uint32(0)) || uint64(cursor) >= uint64(^uint32(0)) {
				return descriptors, errors.New("semanticpath: direct metadata root position is invalid")
			}
			descriptors[family][ordinal-1] = digestPath3(
				"semantic-direct-source-descriptor-v2",
				rootClass(view, term),
				uint32(family), uint32(offset+1), uint32(cursor+1), source.Span{},
			)
		}
	}
	return descriptors, nil
}

func rootClass(view authored.View, root keyspace.Term) keyspace.ContentID {
	family := keyspace.TermFamily(root)
	base := digestPath("semantic-root-class-v2", keyspace.ContentID{}, uint32(family), 0, source.Span{})
	childFamily := func(term keyspace.Term) uint32 {
		if term == 0 {
			return 0
		}
		return uint32(keyspace.TermFamily(term))
	}
	switch family {
	case keyspace.FamilyBind:
		_, values, ok := view.Storage().Binds().Get(root)
		if ok {
			member, _ := view.Values().Member(values, 0)
			tail, _, tailOK := view.Values().Get(values)
			aux := uint32(0)
			if tailOK && tail != 0 {
				aux = 1
			}
			return digestPath3("semantic-root-class-bind", base, childFamily(member), childFamily(tail), aux, source.Span{})
		}
	case keyspace.FamilyReturn:
		_, values, ok := view.Control().Returns().Get(root)
		if ok {
			member, _ := view.Values().Member(values, 0)
			tail, _, tailOK := view.Values().Get(values)
			aux := uint32(0)
			if tailOK && tail != 0 {
				aux = 1
			}
			return digestPath3("semantic-root-class-return", base, childFamily(member), aux, 0, source.Span{})
		}
	case keyspace.FamilyCall:
		_, callee, receiver, actuals, ok := view.Calls().Get(root)
		if ok {
			aux := uint32(0)
			if receiver != 0 {
				aux = 1
			}
			return digestPath3("semantic-root-class-call", base, childFamily(callee), childFamily(actuals), aux, source.Span{})
		}
	case keyspace.FamilyFunction:
		_, child, vararg, ok := view.Functions().Get(root)
		if ok {
			return digestPath3("semantic-root-class-function", base, childFamily(child), childFamily(vararg), 0, source.Span{})
		}
	case keyspace.FamilyBranch:
		_, _, yes, no, ok := view.Control().Branches().Get(root)
		if ok {
			return digestPath3("semantic-root-class-branch", base, childFamily(yes), childFamily(no), 0, source.Span{})
		}
	case keyspace.FamilyLoop:
		_, child, loopKind, control, ok := view.Control().Loops().Get(root)
		if ok {
			return digestPath3("semantic-root-class-loop", base, childFamily(child), childFamily(control), uint32(loopKind), source.Span{})
		}
	}
	return base
}

func deriveBodyPaths(sourceView source.View, view authored.View, bodies *body.Result, forest *containment.Result, edges [keyspace.FamilyCount][]edgeDescriptor, roots [keyspace.FamilyCount][]keyspace.ContentID, resolver *structuralResolver) ([]keyspace.ContentID, error) {
	paths := make([]keyspace.ContentID, bodies.BodyCount())
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

func deriveRootPaths(sourceView source.View, bodies *body.Result, bodyPaths []keyspace.ContentID, descriptors [keyspace.FamilyCount][]keyspace.ContentID) ([keyspace.FamilyCount][]keyspace.ContentID, error) {
	var paths [keyspace.FamilyCount][]keyspace.ContentID
	identity, index := sourceView.Identity(), sourceView.Index()
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if n := identity.FamilyCount(family); n > 0 {
			paths[family] = make([]keyspace.ContentID, n)
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := 1; ordinal <= len(paths[family]); ordinal++ {
			root := keyspace.MakeTerm(family, uint32(ordinal))
			bodyTerm, _, _, ok := index.Position(root)
			if !ok {
				continue
			}
			bo := keyspace.TermOrdinal(bodyTerm)
			if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || bo == 0 || uint64(bo) > uint64(len(bodyPaths)) {
				return paths, errors.New("semanticpath: root owner Body is invalid")
			}
			descriptor := descriptors[family][ordinal-1]
			if !descriptor.Available() {
				continue
			}
			paths[family][ordinal-1] = digestBytes("semantic-root-occurrence-v2", bodyPaths[bo-1], descriptor)
		}
	}
	return paths, nil
}

func deriveTermPaths(sourceView source.View, cellRoles source.CellRoleCatalog, view authored.View, bindings binding.Result, bodies *body.Result, forest *containment.Result, outcomes *outcome.Result, edges [keyspace.FamilyCount][]edgeDescriptor, bodyPaths []keyspace.ContentID, descriptors [keyspace.FamilyCount][]keyspace.ContentID, roots [keyspace.FamilyCount][]keyspace.ContentID, resolver *structuralResolver) ([keyspace.FamilyCount][]keyspace.ContentID, error) {
	var paths [keyspace.FamilyCount][]keyspace.ContentID
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		// Every family owns its ordinal-zero sentinel, including an empty
		// denominator.  Certificate.Seal validates the same dense n+1 plane;
		// leaving an empty family nil would make a valid Program fail issuance
		// before any term-path derivation runs.
		paths[family] = make([]keyspace.ContentID, sourceView.Identity().FamilyCount(family)+1)
	}
	// Body has no enclosing containment parent in Source's term index.  Its
	// canonical BodyPath is therefore the only lawful body-qualified TermPath
	// and is used for Body endpoint/Site identity without a synthetic root.
	for ordinal := 1; ordinal < len(paths[keyspace.FamilyBody]); ordinal++ {
		if ordinal > len(bodyPaths) || !bodyPaths[ordinal-1].Available() {
			return paths, errors.New("semanticpath: Body term path is unavailable")
		}
		paths[keyspace.FamilyBody][ordinal] = bodyPaths[ordinal-1]
	}
	resolveFamily := func(family keyspace.Family) error {
		for ordinal := 1; ordinal <= len(paths[family])-1; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			bodyTerm, _, _, positioned := sourceView.Index().Position(term)
			if !positioned {
				return fmt.Errorf("semanticpath: term %v has no Source position", term)
			}
			path, ok := resolver.resolve(term, bodyTerm)
			if !ok {
				root, _ := sourceView.Index().Root(term)
				parent, _ := forest.Parent(term)
				missing, missingParent := resolver.missingStructuralEdge(term, bodyTerm)
				return fmt.Errorf("semanticpath: term %v family=%d ordinal=%d root=%v parent=%v missing-child=%v missing-parent=%v has no structural path", term, family, ordinal, root, parent, missing, missingParent)
			}
			paths[family][ordinal] = path
		}
		return nil
	}
	// Definition hosts all precede Static families in the stable keyspace.
	// Resolve those hosts first, then issue Cell identities from their exact
	// Binding roles. Static descendants whose canonical parent is a Cell must
	// start from that Cell identity rather than invent a generic Cell edge.
	for family := keyspace.Family(1); family < keyspace.FamilyTypeAlias; family++ {
		if family == keyspace.FamilyBody || family == keyspace.FamilyCell || family == keyspace.FamilyOutcome {
			continue
		}
		if err := resolveFamily(family); err != nil {
			return paths, err
		}
	}
	if err := deriveCellTermPaths(sourceView, cellRoles, view, bindings, forest, bodyPaths, &paths); err != nil {
		return paths, err
	}
	for ordinal := 1; ordinal < len(paths[keyspace.FamilyCell]); ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		path := paths[keyspace.FamilyCell][ordinal]
		if !path.Available() {
			return paths, errors.New("semanticpath: Cell path is unavailable after role issuance")
		}
		resolver.memo[cell] = path
	}
	for family := keyspace.FamilyTypeAlias; family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		if err := resolveFamily(family); err != nil {
			return paths, err
		}
	}
	outcomeCount := sourceView.Identity().FamilyCount(keyspace.FamilyOutcome)
	if outcomeCount > 0 && outcomes.Count() != outcomeCount {
		return paths, errors.New("semanticpath: Outcome denominator disagrees with Source")
	}
	for ordinal := 1; ordinal <= outcomeCount; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(ordinal))
		bodyTerm, outcomeKind, target, ok := outcomes.Get(term)
		if !ok {
			return paths, fmt.Errorf("semanticpath: Outcome %v is unavailable", term)
		}
		bo := keyspace.TermOrdinal(bodyTerm)
		if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || bo == 0 || uint64(bo) > uint64(len(bodyPaths)) {
			return paths, errors.New("semanticpath: Outcome Body is invalid")
		}
		bodyPath := bodyPaths[bo-1]
		if !bodyPath.Available() {
			return paths, errors.New("semanticpath: Outcome Body path is unavailable")
		}
		var targetPath keyspace.ContentID
		if target != 0 {
			tf, to := keyspace.TermFamily(target), keyspace.TermOrdinal(target)
			if tf <= keyspace.FamilyInvalid || tf >= keyspace.FamilyCount || to == 0 || uint64(to) >= uint64(len(paths[tf])) {
				return paths, errors.New("semanticpath: Outcome target is invalid")
			}
			targetPath = paths[tf][to]
			if !targetPath.Available() {
				return paths, errors.New("semanticpath: Outcome target path is unavailable")
			}
		}
		paths[keyspace.FamilyOutcome][ordinal] = digestOutcome(bodyPath, uint32(outcomeKind), targetPath)
	}
	return paths, nil
}

// missingStructuralEdge reports the first unlabelled edge on the exact
// containment ascent. It is diagnostics-only and never manufactures a role.
func (r *structuralResolver) missingStructuralEdge(term, expectedBody keyspace.Term) (keyspace.Term, keyspace.Term) {
	for current := term; current != 0; {
		root, rootOK := r.source.Index().Root(current)
		body, _, _, positioned := r.source.Index().Position(current)
		if !rootOK || !positioned || body != expectedBody || current == root {
			return 0, 0
		}
		parent, parentOK := r.forest.Parent(current)
		family, ordinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
		if !parentOK || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(r.edges[family])) || r.edges[family][ordinal-1].kind == 0 {
			return current, parent
		}
		current = parent
	}
	return 0, 0
}

// deriveCellTermPaths joins the closed Source Cell receipt to Flow's sealed
// Binding roles.  It has no fallback relation: every Cell is claimed once by
// one typed definition role, and local identities fold both lexical Body and
// definition host paths without using a Cell Term or global ordinal.
func deriveCellTermPaths(sourceView source.View, catalog source.CellRoleCatalog, view authored.View, bindings binding.Result, forest *containment.Result, bodyPaths []keyspace.ContentID, paths *[keyspace.FamilyCount][]keyspace.ContentID) error {
	if paths == nil || forest == nil || !catalog.Matches(sourceView) || !binding.Matches(&bindings, sourceView.Identity().ContentID(), view.Cold().ContentID()) {
		return errors.New("semanticpath: Cell role join owners are unavailable")
	}
	cells := view.Storage().Cells()
	if catalog.CellCount() != cells.Count() || bindings.CellCount() != cells.Count() || len(paths[keyspace.FamilyCell]) != cells.Count()+1 {
		return errors.New("semanticpath: Cell role join denominator disagrees")
	}
	claimed := make([]bool, cells.Count()+1)
	claim := func(cell keyspace.Term, want kind.CellRole, host keyspace.Term, bodyTerm keyspace.Term, slot uint32, qualifier uint32) error {
		ordinal := keyspace.TermOrdinal(cell)
		if keyspace.TermFamily(cell) != keyspace.FamilyCell || ordinal == 0 || uint64(ordinal) >= uint64(len(claimed)) || claimed[ordinal] {
			return errors.New("semanticpath: duplicate or invalid Cell role claim")
		}
		role, roleOK := bindings.Role(cell)
		gotHost, hostOK := bindings.Host(cell)
		cellKind, gotBody, key, cellOK := cells.Get(cell)
		if !roleOK || !hostOK || !cellOK || role != want || gotHost != host || cellKind != authored.CellLocal || key != 0 || gotBody != bodyTerm {
			return errors.New("semanticpath: Cell role claim disagrees with Binding or authored row")
		}
		parent, parentOK := forest.Parent(cell)
		if !parentOK || parent != host {
			return errors.New("semanticpath: Cell containment parent disagrees with Binding host")
		}
		bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
		if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || bodyOrdinal == 0 || uint64(bodyOrdinal) > uint64(len(bodyPaths)) || !bodyPaths[bodyOrdinal-1].Available() {
			return errors.New("semanticpath: Cell role Body path is unavailable")
		}
		var hostPath keyspace.ContentID
		if keyspace.TermFamily(host) == keyspace.FamilyBody {
			hostOrdinal := keyspace.TermOrdinal(host)
			if hostOrdinal == 0 || uint64(hostOrdinal) >= uint64(len(paths[keyspace.FamilyBody])) {
				return errors.New("semanticpath: Cell Body host is invalid")
			}
			hostPath = paths[keyspace.FamilyBody][hostOrdinal]
		} else {
			hostFamily, hostOrdinal := keyspace.TermFamily(host), keyspace.TermOrdinal(host)
			if hostFamily <= keyspace.FamilyInvalid || hostFamily >= keyspace.FamilyCount || hostOrdinal == 0 || uint64(hostOrdinal) >= uint64(len(paths[hostFamily])) {
				return errors.New("semanticpath: Cell definition host is invalid")
			}
			hostPath = paths[hostFamily][hostOrdinal]
		}
		if !hostPath.Available() {
			return errors.New("semanticpath: Cell definition host path is unavailable")
		}
		descriptor := digestPath3("semantic-cell-definition-v2", bodyPaths[bodyOrdinal-1], uint32(want), slot, qualifier, source.Span{})
		paths[keyspace.FamilyCell][ordinal] = digestBytes("semantic-cell-occurrence-v2", hostPath, descriptor)
		claimed[ordinal] = true
		return nil
	}
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		role, roleOK := bindings.Role(cell)
		host, hostOK := bindings.Host(cell)
		cellKind, bodyTerm, key, cellOK := cells.Get(cell)
		if !roleOK || !hostOK || !cellOK {
			return errors.New("semanticpath: Cell role row is unavailable")
		}
		switch role {
		case kind.CellGlobal:
			if host != 0 || cellKind != authored.CellGlobal || bodyTerm != 0 || key == 0 {
				return errors.New("semanticpath: global Cell role is invalid")
			}
			if _, hasParent := forest.Parent(cell); hasParent {
				return errors.New("semanticpath: global Cell has a containment parent")
			}
			atomID, atomOK := catalog.ExactIDForKey(key)
			if !atomOK {
				return errors.New("semanticpath: global Cell exact atom is unavailable")
			}
			if claimed[ordinal] {
				return errors.New("semanticpath: duplicate global Cell claim")
			}
			paths[keyspace.FamilyCell][ordinal] = digestPath("semantic-global-cell-v2", atomID, uint32(kind.CellGlobal), 0, source.Span{})
			claimed[ordinal] = true
		case kind.CellLocal:
			sourceRole, sourceOK := catalog.BindRoleForCell(host, cell)
			position, positionOK := sourceRole.Position()
			if !sourceOK || !catalog.Owns(sourceRole) || sourceRole.Kind() != source.CellRoleBind || !sourceRole.MatchesCell(cell) || !positionOK {
				return errors.New("semanticpath: Bind Cell role is invalid")
			}
			if err := claim(cell, kind.CellLocal, host, bodyTerm, uint32(position+1), uint32(source.CellRoleBind)); err != nil {
				return err
			}
		case kind.CellFormal:
			sourceRole, sourceOK := catalog.FormalRoleForCell(host, cell)
			position, positionOK := sourceRole.Position()
			if !sourceOK || !catalog.Owns(sourceRole) || sourceRole.Kind() != source.CellRoleFormal || !sourceRole.MatchesCell(cell) || !positionOK {
				return errors.New("semanticpath: Formal Cell role is invalid")
			}
			if err := claim(cell, kind.CellFormal, host, bodyTerm, uint32(position+1), uint32(source.CellRoleFormal)); err != nil {
				return err
			}
		case kind.CellFunctionVararg:
			functionBody, vararg, ok := functionCellRole(view.Functions(), host)
			if !ok || vararg != cell {
				return errors.New("semanticpath: Function Vararg Cell role is invalid")
			}
			if err := claim(cell, kind.CellFunctionVararg, host, functionBody, 0, 0); err != nil {
				return err
			}
		case kind.CellCapture:
			function, outer, position, inverseOK := bindings.CaptureRoleForCell(cell)
			outerRole, outerOK := bindings.Role(outer)
			if !inverseOK || function != host || outer == cell || !outerOK || outerRole < kind.CellGlobal || outerRole > kind.CellChunkVararg {
				return errors.New("semanticpath: Function Capture inverse is invalid")
			}
			if err := claim(cell, kind.CellCapture, function, bodyTerm, uint32(position+1), 0); err != nil {
				return err
			}
		case kind.CellLoop:
			loop, loopKind, position, inverseOK := bindings.LoopRoleForCell(cell)
			if !inverseOK || loop != host {
				return errors.New("semanticpath: Loop Cell inverse is invalid")
			}
			if err := claim(cell, kind.CellLoop, loop, bodyTerm, uint32(position+1), uint32(loopKind)); err != nil {
				return err
			}
		case kind.CellChunkVararg:
			entry, entryOK := sourceView.Index().Entry()
			chunk, chunkOK := bindings.ChunkVararg()
			if !entryOK || !chunkOK || chunk != cell || host != entry {
				return errors.New("semanticpath: chunk Vararg Cell role is invalid")
			}
			if err := claim(cell, kind.CellChunkVararg, entry, entry, 0, 0); err != nil {
				return err
			}
		default:
			return errors.New("semanticpath: Cell role is invalid")
		}
	}
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		if !claimed[ordinal] || !paths[keyspace.FamilyCell][ordinal].Available() {
			return errors.New("semanticpath: Cell role is uncovered")
		}
	}
	return nil
}

func functionCellRole(functions authored.Functions, function keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	_, bodyTerm, vararg, ok := functions.Get(function)
	return bodyTerm, vararg, ok
}

type structuralResolver struct {
	source      source.View
	forest      *containment.Result
	edges       [keyspace.FamilyCount][]edgeDescriptor
	descriptors [keyspace.FamilyCount][]keyspace.ContentID
	body        []keyspace.ContentID
	memo        map[keyspace.Term]keyspace.ContentID
	visiting    map[keyspace.Term]bool
}

func (r *structuralResolver) resolve(term, expectedBody keyspace.Term) (keyspace.ContentID, bool) {
	if r == nil || term == 0 || keyspace.TermFamily(expectedBody) != keyspace.FamilyBody || keyspace.TermOrdinal(expectedBody) == 0 || uint64(keyspace.TermOrdinal(expectedBody)) > uint64(len(r.body)) {
		return keyspace.ContentID{}, false
	}
	if id, ok := r.memo[term]; ok {
		return id, true
	}
	// One containment parent means an explicit ancestor stack is sufficient:
	// ascend each unresolved edge once, then emit paths in parent-to-child
	// order. No Go call stack or repeated ancestry walk is involved.
	stack := make([]keyspace.Term, 0, 8)
	current := term
	clear := func() {
		for _, node := range stack {
			delete(r.visiting, node)
		}
	}
	for {
		if _, ok := r.memo[current]; ok {
			break
		}
		if r.visiting[current] {
			clear()
			return keyspace.ContentID{}, false
		}
		r.visiting[current] = true
		stack = append(stack, current)
		body, _, _, ok := r.source.Index().Position(current)
		if !ok || body != expectedBody {
			clear()
			return keyspace.ContentID{}, false
		}
		root, ok := r.source.Index().Root(current)
		if !ok || root == 0 {
			clear()
			return keyspace.ContentID{}, false
		}
		if current == root {
			f, o := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
			if f <= keyspace.FamilyInvalid || f >= keyspace.FamilyCount || o == 0 || uint64(o) > uint64(len(r.descriptors[f])) || !r.descriptors[f][o-1].Available() {
				clear()
				return keyspace.ContentID{}, false
			}
			r.memo[current] = digestBytes("semantic-root-occurrence-v2", r.body[keyspace.TermOrdinal(expectedBody)-1], r.descriptors[f][o-1])
			break
		}
		parent, ok := r.forest.Parent(current)
		if !ok || parent == 0 {
			clear()
			return keyspace.ContentID{}, false
		}
		current = parent
	}
	for index := len(stack) - 1; index >= 0; index-- {
		child := stack[index]
		if _, ok := r.memo[child]; ok {
			continue
		}
		parent, ok := r.forest.Parent(child)
		if !ok || parent == 0 {
			clear()
			return keyspace.ContentID{}, false
		}
		parentPath, ok := r.memo[parent]
		f, o := keyspace.TermFamily(child), keyspace.TermOrdinal(child)
		if !ok || f <= keyspace.FamilyInvalid || f >= keyspace.FamilyCount || o == 0 || uint64(o) > uint64(len(r.edges[f])) || r.edges[f][o-1].kind == 0 {
			clear()
			return keyspace.ContentID{}, false
		}
		e := r.edges[f][o-1]
		r.memo[child] = digestPath3("semantic-structural-edge-v2", parentPath, e.kind, e.rank, uint32(f), source.Span{})
	}
	clear()
	id, ok := r.memo[term]
	return id, ok
}

func digestOutcome(bodyPath keyspace.ContentID, outcomeKind uint32, targetPath keyspace.ContentID) keyspace.ContentID {
	var payload [32 + 32 + 4 + 32]byte
	copy(payload[:], []byte("wippy/program/flow/semantic-outcome-path-v2"))
	offset := 32
	copy(payload[offset:], bodyPath[:])
	offset += 32
	binary.BigEndian.PutUint32(payload[offset:], outcomeKind)
	offset += 4
	copy(payload[offset:], targetPath[:])
	return sha256.Sum256(payload[:])
}

func digestPath(label string, parent keyspace.ContentID, role, aux uint32, span source.Span) keyspace.ContentID {
	var payload [64 + 8 + 20]byte
	copy(payload[:], label)
	copy(payload[32:64], parent[:])
	binary.BigEndian.PutUint32(payload[64:68], role)
	binary.BigEndian.PutUint32(payload[68:72], aux)
	binary.BigEndian.PutUint32(payload[72:76], span.StartLine)
	binary.BigEndian.PutUint32(payload[76:80], span.StartCol)
	binary.BigEndian.PutUint32(payload[80:84], span.EndLine)
	binary.BigEndian.PutUint32(payload[84:88], span.EndCol)
	return sha256.Sum256(payload[:])
}

func digestPath3(label string, parent keyspace.ContentID, role, aux, extra uint32, span source.Span) keyspace.ContentID {
	var payload [64 + 12 + 20]byte
	copy(payload[:], label)
	copy(payload[32:64], parent[:])
	binary.BigEndian.PutUint32(payload[64:68], role)
	binary.BigEndian.PutUint32(payload[68:72], aux)
	binary.BigEndian.PutUint32(payload[72:76], extra)
	binary.BigEndian.PutUint32(payload[76:80], span.StartLine)
	binary.BigEndian.PutUint32(payload[80:84], span.StartCol)
	binary.BigEndian.PutUint32(payload[84:88], span.EndLine)
	binary.BigEndian.PutUint32(payload[88:92], span.EndCol)
	return sha256.Sum256(payload[:])
}

func digestBytes(label string, parent, value keyspace.ContentID) keyspace.ContentID {
	var payload [96]byte
	copy(payload[:], label)
	copy(payload[32:64], parent[:])
	copy(payload[64:96], value[:])
	return sha256.Sum256(payload[:])
}

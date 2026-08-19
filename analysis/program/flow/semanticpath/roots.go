package semanticpath

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func deriveRootDescriptors(sourceView source.View, view authored.View, bodies *body.Result) ([keyspace.FamilyCount][]identity.ContentID, error) {
	var descriptors [keyspace.FamilyCount][]identity.ContentID
	sourceIdentity, index := sourceView.Identity(), sourceView.Index()
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if n := sourceIdentity.FamilyCount(family); n > 0 {
			descriptors[family] = make([]identity.ContentID, n)
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
		ranks := make(map[identity.ContentID]uint32)
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

func rootClass(view authored.View, root keyspace.Term) identity.ContentID {
	family := keyspace.TermFamily(root)
	base := digestPath("semantic-root-class-v2", identity.ContentID{}, uint32(family), 0, source.Span{})
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

func deriveRootPaths(sourceView source.View, bodies *body.Result, bodyPaths []identity.ContentID, descriptors [keyspace.FamilyCount][]identity.ContentID) ([keyspace.FamilyCount][]identity.ContentID, error) {
	var paths [keyspace.FamilyCount][]identity.ContentID
	sourceIdentity, index := sourceView.Identity(), sourceView.Index()
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if n := sourceIdentity.FamilyCount(family); n > 0 {
			paths[family] = make([]identity.ContentID, n)
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

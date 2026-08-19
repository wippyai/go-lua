package staticcheck

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// observationPoints is seal-local evidence for static expression roots. A
// root with no Source Position inherits the exact point proved for its
// TypeOf/Annotation scope through its existing containment parent chain.
// The dense planes are discarded with Validate; they are not a second owner
// graph or a retained query result.
type observationPoints struct {
	source     source.View
	flow       authored.View
	forest     *containment.Result
	tree       *contextTree
	descriptor [keyspace.FamilyCount][]observationDescriptor
	state      [keyspace.FamilyCount][]uint8
	resolved   [keyspace.FamilyCount][]int
	path       []keyspace.Term
}

type observationDescriptor struct {
	body        keyspace.Term
	kind        containment.ScopeObservationKind
	observation keyspace.Term
}

func newObservationPoints(sourceView source.View, flowView authored.View, forest *containment.Result, tree *contextTree) *observationPoints {
	evidence := &observationPoints{source: sourceView, flow: flowView, forest: forest, tree: tree}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := sourceView.Identity().FamilyCount(family)
		if count > 0 {
			evidence.descriptor[family] = make([]observationDescriptor, count+1)
			evidence.state[family] = make([]uint8, count+1)
			evidence.resolved[family] = make([]int, count+1)
		}
	}
	return evidence
}

func (e *observationPoints) assignDescriptor(term, body keyspace.Term, kind containment.ScopeObservationKind, observation keyspace.Term) error {
	if e == nil || e.tree == nil || body == 0 || kind == containment.ScopeObservationInvalid || observation == 0 {
		return errors.New("program/flow/staticcheck: static scope descriptor is unavailable")
	}
	family, ordinal, ok := observationTerm(term, e.source)
	if !ok || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 || int(keyspace.TermOrdinal(body)) >= len(e.tree.bodies) {
		return errors.New("program/flow/staticcheck: static scope descriptor endpoint is unavailable")
	}
	descriptor := observationDescriptor{body: body, kind: kind, observation: observation}
	previous := e.descriptor[family][ordinal]
	if previous.kind != containment.ScopeObservationInvalid && previous != descriptor {
		return errors.New("program/flow/staticcheck: static scope descriptors conflict")
	}
	e.descriptor[family][ordinal] = descriptor
	return nil
}

func (e *observationPoints) resolveDescriptors() error {
	if e == nil || e.tree == nil {
		return errors.New("program/flow/staticcheck: static descriptor resolver is unavailable")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := 1; ordinal < len(e.descriptor[family]); ordinal++ {
			if e.descriptor[family][ordinal].kind == containment.ScopeObservationInvalid {
				continue
			}
			term := keyspace.MakeTerm(family, uint32(ordinal))
			if _, err := e.point(term); err != nil {
				return err
			}
		}
	}
	return nil
}

// point resolves Source Position first. Only a term with no genuine Position
// may use a collected scope descriptor or its existing containment Parent
// chain. The descriptor dependency walk is iterative and path-compressed.
func (e *observationPoints) point(term keyspace.Term) (int, error) {
	if e == nil || e.tree == nil {
		return 0, errors.New("program/flow/staticcheck: static observation point tree is unavailable")
	}
	if _, _, _, positioned := e.source.Index().Position(term); positioned {
		point, ok := e.tree.pointForTerm(e.source, term)
		if !ok {
			return 0, errors.New("program/flow/staticcheck: static observation Position is malformed")
		}
		return point, nil
	}
	if _, _, ok := observationTerm(term, e.source); !ok {
		return 0, errors.New("program/flow/staticcheck: static observation term is unavailable")
	}
	e.path = e.path[:0]
	current := term
	resolved := 0
	for current != 0 {
		if _, _, _, positioned := e.source.Index().Position(current); positioned {
			var ok bool
			resolved, ok = e.tree.pointForTerm(e.source, current)
			if !ok {
				return 0, errors.New("program/flow/staticcheck: static parent Position is malformed")
			}
			break
		}
		currentFamily, currentOrdinal, valid := observationTerm(current, e.source)
		if !valid {
			return 0, errors.New("program/flow/staticcheck: static observation parent is invalid")
		}
		switch e.state[currentFamily][currentOrdinal] {
		case 2:
			resolved = e.resolved[currentFamily][currentOrdinal]
			current = 0
			continue
		case 1:
			return 0, errors.New("program/flow/staticcheck: static observation containment cycle")
		}
		e.state[currentFamily][currentOrdinal] = 1
		e.path = append(e.path, current)
		descriptor := e.descriptor[currentFamily][currentOrdinal]
		if descriptor.kind != containment.ScopeObservationInvalid {
			switch descriptor.kind {
			case containment.ScopeObservationCellIntroduction:
				if keyspace.TermFamily(descriptor.observation) != keyspace.FamilyCell || keyspace.TermOrdinal(descriptor.observation) == 0 || int(keyspace.TermOrdinal(descriptor.observation)) >= len(e.tree.cellScope) {
					return 0, errors.New("program/flow/staticcheck: Cell scope descriptor is invalid")
				}
				resolved = e.tree.cellScope[keyspace.TermOrdinal(descriptor.observation)]
				current = 0
			case containment.ScopeObservationSourceOccurrence,
				containment.ScopeObservationFunctionGeneric:
				current = descriptor.observation
			case containment.ScopeObservationFunctionHeader:
				_, functionBody, _, ok := e.flow.Functions().Get(descriptor.observation)
				if !ok {
					return 0, errors.New("program/flow/staticcheck: Function header descriptor is unavailable")
				}
				var pointOK bool
				resolved, pointOK = e.tree.pointAt(functionBody, 0)
				if !pointOK {
					return 0, errors.New("program/flow/staticcheck: Function header descriptor point is unavailable")
				}
				current = 0
			default:
				return 0, errors.New("program/flow/staticcheck: static scope descriptor kind is invalid")
			}
			continue
		}
		parent, hasParent := e.forest.Parent(current)
		if !hasParent {
			return 0, errors.New("program/flow/staticcheck: static observation has no assigned seed")
		}
		current = parent
	}
	if resolved <= 0 || resolved >= len(e.tree.points) {
		return 0, errors.New("program/flow/staticcheck: static observation seed point is invalid")
	}
	for _, pathTerm := range e.path {
		family, ordinal, valid := observationTerm(pathTerm, e.source)
		if !valid {
			return 0, errors.New("program/flow/staticcheck: static observation path is invalid")
		}
		if e.state[family][ordinal] == 2 && e.resolved[family][ordinal] != resolved {
			return 0, errors.New("program/flow/staticcheck: static observation shared seed conflicts")
		}
		e.state[family][ordinal] = 2
		e.resolved[family][ordinal] = resolved
	}
	return resolved, nil
}

func observationTerm(term keyspace.Term, sourceView source.View) (keyspace.Family, int, bool) {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || int(ordinal) > sourceView.Identity().FamilyCount(family) {
		return 0, 0, false
	}
	return family, int(ordinal), true
}

func (e *observationPoints) reanchorFunctions() error {
	if e == nil || e.tree == nil {
		return errors.New("program/flow/staticcheck: Function seed context is unavailable")
	}
	functions := e.flow.Functions()
	for ordinal := 1; ordinal <= functions.Count(); ordinal++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(ordinal))
		if _, _, _, positioned := e.source.Index().Position(function); positioned {
			continue
		}
		point, err := e.point(function)
		if err != nil {
			return err
		}
		owner, functionBody, _, ok := functions.Get(function)
		if !ok {
			return errors.New("program/flow/staticcheck: Function seed owner is unavailable")
		}
		body, bodyOK := e.tree.pointBody(point)
		if !bodyOK || body != owner {
			return errors.New("program/flow/staticcheck: Function seed Body disagrees")
		}
		if err := e.tree.reanchorFunction(functionBody, function, point); err != nil {
			return err
		}
	}
	if err := contextIntervals(e.tree); err != nil {
		return err
	}
	return contextValidateFunctionGenerics(e.flow, e.tree)
}

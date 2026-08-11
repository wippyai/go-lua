package static

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/target"
)

func (a *Authority) sealHotProjections(contract *target.Contract) error {
	if a == nil || a.source == nil || contract == nil {
		return errors.New("static: unavailable hot projection source")
	}
	for index := 0; index < a.source.Static().Namespaces().Count(); index++ {
		namespace, ok := a.source.Static().Namespaces().At(index)
		if !ok {
			return errors.New("static: malformed static namespace family")
		}
		id, ok := a.source.Static().Namespaces().ContentID(namespace)
		if !ok || !id.Available() {
			return errors.New("static: static namespace identity unavailable")
		}
		// Namespace identity commits to the Target, Program, and complete
		// exported static surface. Distinct Link shards may therefore share an
		// identity when their closed resolver-observable context is equal; the
		// opaque shard handles remain distinct while Static safely shares the
		// semantic coordinate for this complete content identity.
		a.namespaceIDs[id] = struct{}{}
	}
	return nil
}

// sealContainedOperands evaluates Static's complete authored input denominator
// once while construction indexes are available. Inputs retain their exact
// TypeOf/Annotation site and Program SourceFrontier: same expression terms at
// distinct sites never collapse.
func (a *Authority) sealContainedOperands() error {
	if a == nil || a.source == nil {
		return errors.New("static: unavailable contained-operand source")
	}
	machine := newEvaluationMachine(a, 0)
	type pendingInput struct {
		key   linkstatic.InputRef
		input containedInput
	}
	pending := make([]pendingInput, 0, a.source.Static().Inputs().Count())
	for index := 0; index < a.source.Static().Inputs().Count(); index++ {
		inputHandle, ok := a.source.Static().Inputs().At(index)
		if !ok {
			return errors.New("static: malformed static input family")
		}
		_, site, expression, term, body, cursor, ok := a.source.Static().Inputs().Source(inputHandle)
		hotReference, referenceOK := a.source.Static().Expressions().Reference(expression)
		resolver, resolverOK := a.source.Static().Expressions().Resolver(expression)
		if !ok || !referenceOK || !resolverOK || term == 0 || body == 0 || cursor < 0 {
			return errors.New("static: malformed static input row")
		}
		shard, shardOK := a.source.Static().Expressions().Shard(expression)
		p, programOK := a.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil || !p.ContentID().Available() {
			return errors.New("static: static input Program unavailable")
		}
		selector, referenceOK := a.types.Find(p.ContentID(), hotReference.Term())
		if !referenceOK {
			return errors.New("static: static input portable reference unavailable")
		}
		reference, referenceOK := a.types.Ref(selector)
		if !referenceOK || reference.Owner() != p.ContentID() || reference.Root() != hotReference.Term() {
			return errors.New("static: static input portable reference unavailable")
		}
		frontierBody, frontierCursor, frontierOK := p.Source().Index().Frontier(site)
		if !frontierOK || frontierBody != body || frontierCursor != cursor {
			return errors.New("static: static input frontier mismatch")
		}
		input, ok := machine.requestContained(p, p.ContentID(), term, site, body, uint32(cursor), resolver, Environment{})
		if !ok {
			if machine.err != nil {
				return machine.err
			}
			return errors.New("static: contained-operand request failed")
		}
		pending = append(pending, pendingInput{key: inputHandle, input: input})
	}
	if err := machine.run(); err != nil {
		return err
	}
	for _, row := range pending {
		value, ok := machine.containedValue(row.input)
		if !ok {
			if machine.err != nil {
				return machine.err
			}
			return errors.New("static: incomplete contained-operand result")
		}
		a.operands[row.key] = value
	}
	return nil
}

// sealTypeOfOutputs freezes the direct hot lookup from Link's existing TypeOf
// StaticInput handles to Static's existing output coordinates. The map is a
// derived index only: neither its key nor its value introduces semantic
// identity, and replay rebuilds it from the same Link/Static authorities.
func (a *Authority) sealTypeOfOutputs() error {
	if a == nil || a.source == nil || a.typeOfOutputs != nil {
		return errors.New("static: unavailable typeof output projection")
	}
	a.typeOfOutputs = make(map[linkstatic.InputRef]Coordinate)
	for index := 0; index < a.source.Static().Inputs().Count(); index++ {
		input, ok := a.source.Static().Inputs().At(index)
		if !ok {
			return errors.New("static: malformed typeof output denominator")
		}
		kind, site, expression, _, _, _, ok := a.source.Static().Inputs().Source(input)
		hotReference, referenceOK := a.source.Static().Expressions().Reference(expression)
		resolver, resolverOK := a.source.Static().Expressions().Resolver(expression)
		if !ok || !referenceOK || !resolverOK {
			return errors.New("static: malformed typeof output source")
		}
		shard, shardOK := a.source.Static().Expressions().Shard(expression)
		p, programOK := a.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil || !p.ContentID().Available() {
			return errors.New("static: typeof output Program unavailable")
		}
		selector, referenceOK := a.types.Find(p.ContentID(), hotReference.Term())
		if !referenceOK {
			return errors.New("static: typeof output portable reference unavailable")
		}
		reference, referenceOK := a.types.Ref(selector)
		if !referenceOK {
			return errors.New("static: typeof output portable reference unavailable")
		}
		typeOf := site
		if kind != linkstatic.InputTypeOf || hotReference.Term() != typeOf {
			continue
		}
		if _, _, typeOfOK := p.Static().Operators().TypeOfs().Get(typeOf); !typeOfOK {
			continue
		}
		namespace, ok := a.source.Static().Namespaces().ResolverContentID(resolver)
		if !ok || !namespace.Available() {
			return errors.New("static: typeof output namespace unavailable")
		}
		output, ok := a.coordinateFor(coordinateKey{
			reference:   reference,
			namespace:   namespace,
			environment: keyspace.ContentID{},
			operation:   0,
		})
		if !ok {
			return errors.New("static: typeof output coordinate unavailable")
		}
		if _, duplicate := a.typeOfOutputs[input]; duplicate {
			return errors.New("static: duplicate typeof output source")
		}
		a.typeOfOutputs[input] = output
	}
	return nil
}

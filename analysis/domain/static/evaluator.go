package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/keyspace"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	"github.com/wippyai/go-lua/program/target"
)

type OperandKind uint8

const (
	// OperandInvalid is the zero disposition so an unavailable lookup can never
	// be mistaken for a legitimate exact static Unknown.
	OperandInvalid OperandKind = iota
	OperandUnknown
	OperandKnown
	OperandRuntimeSubject
)

// ContainedOperand is the context-free typeof boundary. Its four cases are
// intentionally disjoint: only RuntimeSubject may acquire a later State
// dependency; Unknown retains an exact static reason; Invalid retains the
// exact static diagnostic and never falls back to runtime evaluation.
type ContainedOperand struct {
	kind           OperandKind
	known          Value
	subject        RuntimeSubject
	reason         Reason
	fault          Fault
	owner          keyspace.ContentID
	source         keyspace.Term
	namespace      keyspace.ContentID
	environment    keyspace.ContentID
	operation      target.Operation
	law            keyspace.ContentID
	dependency     keyspace.ContentID
	site           keyspace.Term
	frontierBody   keyspace.Term
	frontierCursor uint32
}

func (o ContainedOperand) Kind() OperandKind { return o.kind }
func (o ContainedOperand) Known() (Value, bool) {
	return o.known, o.kind == OperandKnown && o.known.IsClosed()
}
func (o ContainedOperand) RuntimeSubject() (RuntimeSubject, bool) {
	return o.subject, o.kind == OperandRuntimeSubject && o.subject.Valid()
}
func (o ContainedOperand) UnknownReason() (Reason, bool) {
	return o.reason, o.kind == OperandUnknown && o.reason != 0
}
func (o ContainedOperand) Fault() (Fault, bool) {
	return o.fault, o.kind == OperandInvalid && o.fault != 0
}

func (o ContainedOperand) Source() (keyspace.ContentID, keyspace.Term, bool) {
	return o.owner, o.source, o.owner.Available() && o.source != 0
}
func (o ContainedOperand) Namespace() keyspace.ContentID     { return o.namespace }
func (o ContainedOperand) Environment() keyspace.ContentID   { return o.environment }
func (o ContainedOperand) Operation() target.Operation       { return o.operation }
func (o ContainedOperand) Law() keyspace.ContentID           { return o.law }
func (o ContainedOperand) Dependency() keyspace.ContentID    { return o.dependency }
func (o ContainedOperand) StaticSite() (keyspace.Term, bool) { return o.site, o.site != 0 }
func (o ContainedOperand) SourceFrontier() (keyspace.Term, int, bool) {
	return o.frontierBody, int(o.frontierCursor), o.frontierBody != 0
}

// Input returns Static's already-sealed disposition for one exact Link static
// input. It is an O(1) query over the complete TypeOf/Annotation denominator;
// it never re-enters evaluation or walks Program/Link data.
func (a *Authority) Input(input linkstatic.InputRef) (ContainedOperand, bool) {
	if a == nil {
		return ContainedOperand{}, false
	}
	operand, ok := a.operands[input]
	return operand, ok
}

// TypeOf projects one exact Link-owned static operand to the existing Static
// output coordinate and its already-sealed context-free judgment. The Link
// operand is the sole source handle: RuntimeSubject exposes its one existing
// Value through ContainedOperand, while Known, Unknown, and Invalid remain
// pure results with no State dependency. No second typeof identity is minted.
func (a *Authority) TypeOf(source linkstatic.InputRef) (Coordinate, ContainedOperand, bool) {
	if a == nil || a.source == nil {
		return Coordinate{}, ContainedOperand{}, false
	}
	kind, site, expression, operand, _, _, ok := a.source.Static().Inputs().Source(source)
	hotReference, referenceOK := a.source.Static().Expressions().Reference(expression)
	resolver, resolverOK := a.source.Static().Expressions().Resolver(expression)
	shard, shardOK := a.source.Static().Expressions().Shard(expression)
	p, programOK := a.source.Project().Mounts().Program(shard)
	if !ok || !referenceOK || !resolverOK || !shardOK || !programOK || p == nil || operand == 0 {
		return Coordinate{}, ContainedOperand{}, false
	}
	owner, typeOf := p.ContentID(), site
	if kind != linkstatic.InputTypeOf || hotReference.Term() != typeOf {
		return Coordinate{}, ContainedOperand{}, false
	}
	namespace, ok := a.source.Static().Namespaces().ResolverContentID(resolver)
	if !ok || !namespace.Available() {
		return Coordinate{}, ContainedOperand{}, false
	}
	output, ok := a.typeOfOutputs[source]
	if !ok {
		return Coordinate{}, ContainedOperand{}, false
	}
	coordinateIndex, coordinateOK := a.CoordinateIndex(output)
	if !coordinateOK {
		return Coordinate{}, ContainedOperand{}, false
	}
	reference := a.coordinates[coordinateIndex].key.reference
	if !reference.Valid() || reference.Owner() != owner || reference.Root() != typeOf {
		return Coordinate{}, ContainedOperand{}, false
	}
	contained, ok := a.Input(source)
	if !ok {
		return Coordinate{}, ContainedOperand{}, false
	}
	containedOwner, containedSource, exact := contained.Source()
	site, siteOK := contained.StaticSite()
	if !exact || !siteOK || site != typeOf || containedOwner != owner || containedSource != operand || contained.Namespace() != namespace {
		return Coordinate{}, ContainedOperand{}, false
	}
	if result, admitted := a.Result(output); !admitted || !a.Owns(result) {
		return Coordinate{}, ContainedOperand{}, false
	}
	return output, contained, true
}

// sealCoordinates materializes only observable Static cells: one base cell
// for every authored selector. Recursive evaluation is carried by a result,
// not made into an unobservable ref×context coordinate product.
func (a *Authority) sealCoordinates() error {
	if a == nil || a.types == nil || a.source == nil {
		return errors.New("static: unavailable coordinate source")
	}
	for index := 0; index < a.source.Static().Expressions().Count(); index++ {
		expression, ok := a.source.Static().Expressions().At(index)
		if !ok {
			return errors.New("static: malformed Link static expression family")
		}
		hotReference, ok := a.source.Static().Expressions().Reference(expression)
		if !ok {
			return errors.New("static: expression lacks portable reference")
		}
		resolver, ok := a.source.Static().Expressions().Resolver(expression)
		if !ok {
			return errors.New("static: Program lacks resolver namespace")
		}
		shard, shardOK := a.source.Static().Expressions().Shard(expression)
		p, programOK := a.source.Project().Mounts().Program(shard)
		if !shardOK || !programOK || p == nil {
			return errors.New("static: expression lacks Program owner")
		}
		selector, refOK := a.types.Find(p.ContentID(), hotReference.Term())
		if !refOK {
			return errors.New("static: expression lacks portable reference")
		}
		ref, refOK := a.types.Ref(selector)
		if !refOK {
			return errors.New("static: expression lacks portable reference")
		}
		if err := a.addCoordinate(ref, resolver, Environment{}, 0); err != nil {
			return err
		}
	}
	return nil
}

func (a *Authority) addCoordinate(ref typeauthority.StaticTypeRef, resolver linkstatic.Resolver, environment Environment, operation target.Operation) error {
	namespace, ok := a.source.Static().Namespaces().ResolverContentID(resolver)
	if !ok {
		return errors.New("static: invalid resolver")
	}
	key := coordinateKey{reference: ref, namespace: namespace, environment: environment.ContentID(), operation: operation}
	if _, duplicate := a.coordinateIndex[key]; duplicate {
		return nil
	}
	value, err := a.evaluate(ref, resolver, environment, operation)
	if err != nil {
		return err
	}
	index, err := denseOrdinal(len(a.coordinates))
	if err != nil {
		return errors.New("static: coordinate handle is not representable")
	}
	a.coordinates = append(a.coordinates, coordinateRow{key: key, result: value})
	a.coordinateIndex[key] = index
	return nil
}

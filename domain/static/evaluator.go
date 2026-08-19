package static

import (
	"errors"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/domain/type/authority"
)

type OperandKind uint8

const (
	OperandInvalid OperandKind = iota
	OperandUnknown
	OperandKnown
	OperandRuntimeSubject
)

// ContainedOperand is a detached artifact disposition. It carries only
// scalar Program-issued IDs; no Link static handle, authored term, or Program
// pointer survives the seal.
type ContainedOperand struct {
	kind           OperandKind
	known          Value
	subject        RuntimeSubject
	reason         Reason
	fault          Fault
	owner          identity.ContentID
	source         identity.ContentID
	namespace      identity.ContentID
	environment    identity.ContentID
	operation      vocabulary.Operation
	law            identity.ContentID
	dependency     identity.ContentID
	site           identity.ContentID
	frontierBody   identity.ContentID
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
func (o ContainedOperand) Source() (identity.ContentID, identity.ContentID, bool) {
	return o.owner, o.source, o.owner.Available() && o.source.Available()
}
func (o ContainedOperand) Namespace() identity.ContentID          { return o.namespace }
func (o ContainedOperand) Environment() identity.ContentID        { return o.environment }
func (o ContainedOperand) Operation() vocabulary.Operation        { return o.operation }
func (o ContainedOperand) Law() identity.ContentID                { return o.law }
func (o ContainedOperand) Dependency() identity.ContentID         { return o.dependency }
func (o ContainedOperand) StaticSite() (identity.ContentID, bool) { return o.site, o.site.Available() }
func (o ContainedOperand) SourceFrontier() (identity.ContentID, int, bool) {
	return o.frontierBody, int(o.frontierCursor), o.frontierBody.Available()
}

func (a *Authority) Input(input identity.ContentID) (ContainedOperand, bool) {
	if a == nil || !input.Available() {
		return ContainedOperand{}, false
	}
	operand, ok := a.operands[input]
	return operand, ok
}

func (a *Authority) TypeOf(input identity.ContentID) (Coordinate, ContainedOperand, bool) {
	if a == nil || !input.Available() {
		return Coordinate{}, ContainedOperand{}, false
	}
	output, ok := a.typeOfOutputs[input]
	if !ok {
		return Coordinate{}, ContainedOperand{}, false
	}
	operand, ok := a.Input(input)
	if !ok {
		return Coordinate{}, ContainedOperand{}, false
	}
	if result, admitted := a.Result(output); !admitted || !a.Owns(result) {
		return Coordinate{}, ContainedOperand{}, false
	}
	return output, operand, true
}

func (a *Authority) sealCoordinates() error {
	if a == nil || a.types == nil || !a.linkID.Available() {
		return errors.New("static: unavailable coordinate source")
	}
	if len(a.mounts) == 0 {
		return errors.New("static: mounted artifacts required")
	}
	return a.sealMountedCoordinates()
}

func (a *Authority) sealMountedCoordinates() error {
	for _, mount := range a.mounts {
		if !mount.NamespaceID.Available() {
			return errors.New("static: mounted namespace unavailable")
		}
		for index := 0; index < mount.Artifact.StaticExpressionCount(); index++ {
			row, ok := mount.Artifact.StaticExpressionAt(index)
			if !ok || !row.Available() || row.Owner() != mount.ProgramID {
				return errors.New("static: malformed mounted expression row")
			}
			ref, ok := a.types.FindByReferenceID(row.ReferenceID())
			if !ok {
				return errors.New("static: mounted expression reference unavailable")
			}
			if err := a.addCoordinate(ref, mount.NamespaceID, Environment{}, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Authority) addCoordinate(ref typeauthority.StaticTypeRef, namespace identity.ContentID, environment Environment, operation vocabulary.Operation) error {
	if !namespace.Available() {
		return errors.New("static: invalid namespace")
	}
	key := coordinateKey{reference: ref, namespace: namespace, environment: environment.ContentID(), operation: operation}
	if _, duplicate := a.coordinateIndex[key]; duplicate {
		return nil
	}
	value, err := a.evaluate(ref, namespace, environment, operation)
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

// Keep the static operand enum referenced in this file's API documentation
// and guard accidental reintroduction of a second operand vocabulary.
var _ = programstatic.StaticOperandKnown

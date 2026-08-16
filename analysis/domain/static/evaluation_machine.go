package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

// evaluationMachine is intentionally small: ProgramArtifact already owns the
// complete static graph and typeauthority owns its detached typ projection.
// Static only admits that projection and records the exact contextual result.
type evaluationMachine struct {
	authority *Authority
	operation target.Operation
	err       error
}

func newEvaluationMachine(authority *Authority, operation target.Operation) *evaluationMachine {
	return &evaluationMachine{authority: authority, operation: operation}
}

func (a *Authority) evaluate(ref typeauthority.StaticTypeRef, namespace keyspace.ContentID, environment Environment, operation target.Operation) (Value, error) {
	if a == nil || a.types == nil || !ref.Valid() || !namespace.Available() {
		return Value{}, errors.New("static: foreign evaluation coordinate")
	}
	value, ok := a.types.Resolve(ref)
	if !ok || value == nil {
		return Value{}, errors.New("static: artifact reference did not resolve")
	}
	if typ.ContainsTypeParam(value) {
		site := Symbolic{reference: ref, sourceOwner: ref.Owner(), source: ref.NodeID(), namespace: namespace, environment: environment.ContentID(), operation: operation, law: a.lawID, dependency: ref.Owner(), reason: ReasonOpenFormal}
		return a.addSymbolic(site)
	}
	return a.addClosed(value)
}

func (m *evaluationMachine) fail(err error) {
	if m != nil && m.err == nil {
		m.err = err
	}
}

func (m *evaluationMachine) symbolic(site Symbolic) Value {
	if m == nil || m.authority == nil {
		return Value{}
	}
	value, err := m.authority.addSymbolic(site)
	if err != nil {
		m.fail(err)
	}
	return value
}

func (m *evaluationMachine) invalid(site Symbolic, fault Fault) Value {
	if m == nil || m.authority == nil {
		return Value{}
	}
	value, err := m.authority.addInvalid(site, fault)
	if err != nil {
		m.fail(err)
	}
	return value
}

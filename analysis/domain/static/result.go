package static

import (
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	"github.com/wippyai/go-lua/program/target"
)

// Kind is the complete Static result algebra. Bottom and Top exist solely so
// the result can participate in a sparse Factor carrier; evaluator judgments
// are always Closed, Symbolic, or Invalid.
type Kind uint8

const (
	KindBottom Kind = iota
	KindClosed
	KindSymbolic
	KindInvalid
	KindTop
)

type Reason uint8

const (
	ReasonOpenFormal Reason = iota + 1
	ReasonGenerativeRecurrence
	ReasonRuntimeSubject
	ReasonUnresolvedProjection
	ReasonStaticUnknown
)

type Fault uint8

const (
	FaultUnknown Fault = iota + 1
	FaultReference
	FaultArity
	FaultBound
	FaultProjection
	FaultContainment
)

// RuntimeSubject is the exact Boundary-local Cell value occurrence observed at the
// authored Program SourceFrontier. Static never gives a non-executable Read a
// runtime coordinate of its own.
type RuntimeSubject struct {
	linkID keyspace.ContentID
	value  linkboundary.Value
	id     keyspace.ContentID
	body   keyspace.Term
	cursor uint32
}

func (s RuntimeSubject) Valid() bool                       { return s.linkID.Available() && s.id.Available() && s.body != 0 }
func (s RuntimeSubject) LinkID() keyspace.ContentID        { return s.linkID }
func (s RuntimeSubject) Value() (linkboundary.Value, bool) { return s.value, s.Valid() }
func (s RuntimeSubject) SourceFrontier() (keyspace.Term, int, bool) {
	return s.body, int(s.cursor), s.Valid()
}

// Symbolic is a finite residual equation, not an approximation to any/unknown.
// Its identity is the authored static root, immutable resolver namespace, and
// optional exact runtime subject. Environment and operation remain internal
// evaluator provenance and are currently the canonical zero context.
type Symbolic struct {
	reference   typeauthority.StaticTypeRef
	sourceOwner keyspace.ContentID
	source      keyspace.Term
	namespace   keyspace.ContentID
	environment keyspace.ContentID
	operation   target.Operation
	law         keyspace.ContentID
	dependency  keyspace.ContentID
	reason      Reason
	subject     RuntimeSubject
}

func (s Symbolic) Reference() typeauthority.StaticTypeRef { return s.reference }
func (s Symbolic) Namespace() keyspace.ContentID          { return s.namespace }
func (s Symbolic) Environment() keyspace.ContentID        { return s.environment }
func (s Symbolic) Operation() target.Operation            { return s.operation }
func (s Symbolic) Law() keyspace.ContentID                { return s.law }
func (s Symbolic) Dependency() keyspace.ContentID         { return s.dependency }
func (s Symbolic) Reason() Reason                         { return s.reason }
func (s Symbolic) Source() (keyspace.ContentID, keyspace.Term, bool) {
	return s.sourceOwner, s.source, s.sourceOwner.Available() && s.source != 0
}
func (s Symbolic) Subject() (RuntimeSubject, bool) {
	return s.subject, s.reason == ReasonRuntimeSubject && s.subject.Valid()
}

func (s Symbolic) exactSource() bool {
	if s.sourceOwner.Available() != (s.source != 0) {
		return false
	}
	return s.exactOperand()
}

func (s Symbolic) exactOperand() bool {
	programSource := s.sourceOwner.Available() && s.source != 0
	if s.sourceOwner.Available() != (s.source != 0) {
		return false
	}
	return programSource || s.reference.Valid()
}

type resultRow struct {
	kind     Kind
	closed   []byte
	symbolic Symbolic
	fault    Fault
}

// Value is a homogeneous, owner-fenced handle. It contains no typ graph,
// source pointer, key string, or codec representation.
type Value struct {
	owner *Authority
	index uint32
	// class is populated only for an exact normalized union that is not one of
	// the finite Static result rows. It is immutable, owner-fenced by the
	// Authority's ClassSet, and uses the same descriptor algebra as Pack.
	class Class
}

func (v Value) Kind() (Kind, bool) {
	if v.owner == nil {
		return KindBottom, false
	}
	if v.isDerived() {
		return KindClosed, true
	}
	if uint64(v.index) >= uint64(len(v.owner.results)) {
		return KindBottom, false
	}
	return v.owner.results[v.index].kind, true
}

func (v Value) isDerived() bool {
	return v.owner != nil && v.index == ^uint32(0) && v.class.owner != nil && v.class.descriptor != nil && v.class.owner.authority == v.owner
}
func (v Value) IsClosed() bool   { kind, ok := v.Kind(); return ok && kind == KindClosed }
func (v Value) IsSymbolic() bool { kind, ok := v.Kind(); return ok && kind == KindSymbolic }
func (v Value) IsInvalid() bool  { kind, ok := v.Kind(); return ok && kind == KindInvalid }

func (v Value) Symbolic() (Symbolic, bool) {
	if !v.IsSymbolic() {
		return Symbolic{}, false
	}
	return v.owner.results[v.index].symbolic, true
}

func (v Value) Fault() (Fault, bool) {
	if !v.IsInvalid() {
		return 0, false
	}
	return v.owner.results[v.index].fault, true
}

// InvalidSource and the accompanying context accessors expose the exact
// structured site retained for an invalid judgment. They deliberately reuse
// the same authored coordinate vocabulary as Symbolic; Fault remains the
// diagnostic identity.
func (v Value) InvalidSource() (keyspace.ContentID, keyspace.Term, bool) {
	if !v.IsInvalid() {
		return keyspace.ContentID{}, 0, false
	}
	return v.owner.results[v.index].symbolic.Source()
}

func (v Value) InvalidNamespace() (keyspace.ContentID, bool) {
	if !v.IsInvalid() {
		return keyspace.ContentID{}, false
	}
	value := v.owner.results[v.index].symbolic.namespace
	return value, value.Available()
}

func (v Value) InvalidEnvironment() (keyspace.ContentID, bool) {
	if !v.IsInvalid() {
		return keyspace.ContentID{}, false
	}
	return v.owner.results[v.index].symbolic.environment, true
}

func (v Value) InvalidOperation() (target.Operation, bool) {
	if !v.IsInvalid() {
		return 0, false
	}
	return v.owner.results[v.index].symbolic.operation, true
}

func (v Value) InvalidLaw() (keyspace.ContentID, bool) {
	if !v.IsInvalid() {
		return keyspace.ContentID{}, false
	}
	value := v.owner.results[v.index].symbolic.law
	return value, value.Available()
}

func (v Value) InvalidDependency() (keyspace.ContentID, bool) {
	if !v.IsInvalid() {
		return keyspace.ContentID{}, false
	}
	value := v.owner.results[v.index].symbolic.dependency
	return value, value.Available()
}

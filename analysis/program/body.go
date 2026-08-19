package program

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Body is a Program-owned view over one existing Flow Body boundary.
// It retains no body/outcome rows and exposes only existing Causal Sites plus
// the optional Function boundary needed for formal/capture access.
type Body struct {
	program  *Program
	boundary flow.BodyBoundary
	function flow.FunctionBoundary
}

// BodyCount forwards Source's sole canonical Body denominator. It does not
// retain an input-owned index or promote the Function-boundary join to one.
func (input *Program) BodyCount() int {
	if !input.Available() {
		return 0
	}
	return input.Source().Identity().FamilyCount(keyspace.FamilyBody)
}

// BodyAt returns one existing Body view in canonical Body order.
func (input *Program) BodyAt(index int) (Body, bool) {
	if !input.Available() || index < 0 || index >= input.BodyCount() {
		return Body{}, false
	}
	term := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1))
	return input.Body(term)
}

// Body joins an authored Body to its published boundary. Root/non-Function
// Bodies retain an unavailable Function boundary rather than a fabricated one.
func (input *Program) Body(term keyspace.Term) (Body, bool) {
	if !input.Available() {
		return Body{}, false
	}
	boundaries := input.Flow().FunctionBoundaries()
	boundary, ok := boundaries.ForBody(term)
	if !ok {
		return Body{}, false
	}
	function, _ := boundaries.ForFunctionBody(term)
	body := Body{program: input, boundary: boundary, function: function}
	return body, body.Available()
}

// scalarBody returns the existing Body path and Function-boundary context used
// by Program's formal identity queries. It joins existing owner rows; it does
// not derive a second geometry or retain a transport object.
func (program *Program) scalarBody(owner keyspace.Term) (identity.ContentID, identity.ContentID, bool) {
	if !program.Available() || owner == 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	view := program.Flow()
	body, ok := view.FunctionBoundaries().ForBody(owner)
	if !ok || !body.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	path, pathOK := view.BodyPath(owner)
	context := body.ContextID()
	return path, context, pathOK && path.Available() && context.Available()
}

// OwnsBody authenticates a Body issued by this exact Program.
// Equivalent replay Bodies deliberately do not pass: mount-local consumers
// must retain their own issued view rather than substitute one.
func (input *Program) OwnsBody(body Body) bool {
	if !input.Available() || body.program != input || !body.Available() {
		return false
	}
	boundaries := input.Flow().FunctionBoundaries()
	if !boundaries.OwnsBody(body.boundary) {
		return false
	}
	if body.function.Available() {
		return boundaries.OwnsFunction(body.function)
	}
	return true
}

// OwnsSite authenticates an exact hot Causal Site issued by this Program.
// Equivalent replay Sites are intentionally rejected at mount-local joins.
func (input *Program) OwnsSite(site flow.Site) bool {
	if !input.Available() || !site.Available() {
		return false
	}
	term, ok := site.Term()
	if !ok {
		return false
	}
	sites := input.Flow().Causal().Sites()
	issued, ok := sites.ForTerm(term)
	return ok && sites.Owns(site) && issued.Equal(site) && issued.ContextID() == site.ContextID()
}

// ContainingBody resolves Source's existing lexical containment internally
// and returns only the opaque Body proof. It does not expose the containing
// raw Body coordinate for callers to rejoin.
func (input *Program) ContainingBody(term keyspace.Term) (Body, bool) {
	if !input.Available() {
		return Body{}, false
	}
	body, _, _, ok := input.Source().Index().Position(term)
	if !ok {
		return Body{}, false
	}
	return input.Body(body)
}

func (body Body) Available() bool {
	if !body.program.Available() || !body.boundary.Available() {
		return false
	}
	term, ok := body.boundary.Body()
	if !ok {
		return false
	}
	want, wantOK := body.program.Flow().FunctionBoundaries().ForBody(term)
	if !wantOK || !body.boundary.Equal(want) {
		return false
	}
	function, functionOK := body.program.Flow().FunctionBoundaries().ForFunctionBody(term)
	if functionOK {
		return body.function.Available() && body.function.Equal(function)
	}
	return !body.function.Available()
}

// Equal compares the existing exact-quartet Body boundary proof. It never
// compares or exposes an authored Body term.
func (body Body) Equal(other Body) bool {
	return body.Available() && other.Available() && body.boundary.Equal(other.boundary)
}

// ContextID is Flow's stable exact-quartet identity for this Body boundary.
func (body Body) ContextID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	return body.boundary.ContextID()
}

// ProgramID returns the already-published Program owner of this exact Body
// proof. It is a scalar provenance fence for reusable artifact consumers;
// it neither reopens Program state nor exposes the authored Body term.
func (body Body) ProgramID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	return body.program.ContentID()
}

// PathID returns Flow's owner-local lexical Body path. Unlike ContextID it
// carries no quartet identity and is therefore suitable for semantic
// descriptors that must replay across equivalent owner publication.
func (body Body) PathID() identity.ContentID {
	if !body.Available() {
		return identity.ContentID{}
	}
	term, ok := body.boundary.Body()
	if !ok {
		return identity.ContentID{}
	}
	path, ok := body.program.Flow().BodyPath(term)
	if !ok {
		return identity.ContentID{}
	}
	return path
}

// Executable reports the exact sealed Flow executable membership for this
// Body. It is a scalar proof copied for artifact boundary filtering.
func (body Body) Executable() bool {
	if !body.Available() {
		return false
	}
	term, ok := body.boundary.Body()
	return ok && body.program.Flow().Executable().Contains(term)
}

// Function returns the existing sealed Flow boundary for this Body. It is the
// remaining construction seam: Artifact consumes scalar callable identities
// through Program queries and never retains this handle.
func (body Body) Function() (flow.FunctionBoundary, bool) {
	if !body.Available() || !body.function.Available() {
		return flow.FunctionBoundary{}, false
	}
	return body.function, true
}

// EntrySite returns the existing Causal Site at this Body's boundary Entry.
func (body Body) EntrySite() (flow.Site, bool) {
	if !body.Available() {
		return flow.Site{}, false
	}
	entry, ok := body.boundary.Entry()
	if !ok {
		return flow.Site{}, false
	}
	return body.program.Flow().Causal().Sites().ForTerm(entry)
}

// CallBodyTarget is Program's formal, portable identity for one existing
// Body. It is deliberately independent of a mount, Link, or Call selector;
// the exact Body proof issues it, while Link later adds its ModuleKey.
type CallBodyTarget struct{ context identity.ContentID }

// CallTarget returns the formal Call target for this exact Program Body.
func (body Body) CallTarget() (CallBodyTarget, bool) {
	if !body.Available() || !body.ContextID().Available() {
		return CallBodyTarget{}, false
	}
	target := CallBodyTarget{context: body.ContextID()}
	return target, target.Valid()
}

func (target CallBodyTarget) Valid() bool { return target.context.Available() }

func (target CallBodyTarget) ContextID() identity.ContentID {
	if !target.Valid() {
		return identity.ContentID{}
	}
	return target.context
}

const (
	callBodyTargetVersion uint64 = 1
	callBodyTargetTag            = "pcallbod"
)

// ID is the closed role/version identity of this formal. The preimage is
// fixed and contains only the Program Body ContextID; no raw term or ordinal
// participates.
func (target CallBodyTarget) ID() (identity.ContentID, bool) {
	if !target.Valid() {
		return identity.ContentID{}, false
	}
	var payload [8 + 8 + 32]byte
	copy(payload[:8], callBodyTargetTag)
	binary.BigEndian.PutUint64(payload[8:16], callBodyTargetVersion)
	copy(payload[16:], target.context[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return id, id.Available()
}

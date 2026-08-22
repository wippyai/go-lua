package activation

import (
	"encoding/hex"

	"github.com/wippyai/go-lua/analysis/identity"
)

// RefusalReason names the predicate that refused one mounted activation
// admission. It is this package's own vocabulary: the composition carries the
// value erased and never reads it, so no admission verdict outside this
// package spells an activation predicate.
type RefusalReason uint8

const (
	RefusalNone RefusalReason = iota
	// RefusalInput is an absent rule, owner, implementation, or Link context
	// directory: the request never reached an occurrence row.
	RefusalInput
	// RefusalAlgebra is an absent Call algebra, which owns the mounted
	// occurrence rows this admission consumes.
	RefusalAlgebra
	// RefusalOccurrence is a mount and occurrence pair the algebra holds no
	// mounted call key, application key, or semantic application for.
	RefusalOccurrence
	// RefusalRoutes is a sealed route column that does not cover Call's body
	// order.
	RefusalRoutes
	// RefusalCapability is an absent mounted rule-slot capability.
	RefusalCapability
	// RefusalTransport is an unbound mounted candidate issuer.
	RefusalTransport
	// RefusalRead is an occurrence whose owner reference resolves no exact
	// read surface.
	RefusalRead
	// RefusalBodyRow is a body row missing its module key, body path, or
	// sealed route.
	RefusalBodyRow
	// RefusalTriggerNotResident is a trigger module the Link's directory holds
	// no execution Context for: a mount the Link never made.
	RefusalTriggerNotResident
	// RefusalBodyNotResident is a body module the Link's directory holds no
	// execution Context for. A body the directory does hold but connects to
	// this trigger by no activation edge is another actor's copy of a shared
	// library: it is resident, contributes no candidate, and refuses nothing.
	RefusalBodyNotResident
)

func (reason RefusalReason) String() string {
	switch reason {
	case RefusalInput:
		return "input"
	case RefusalAlgebra:
		return "algebra"
	case RefusalOccurrence:
		return "occurrence"
	case RefusalRoutes:
		return "routes"
	case RefusalCapability:
		return "capability"
	case RefusalTransport:
		return "transport"
	case RefusalRead:
		return "read"
	case RefusalBodyRow:
		return "body-row"
	case RefusalTriggerNotResident:
		return "trigger-not-resident"
	case RefusalBodyNotResident:
		return "body-not-resident"
	default:
		return "none"
	}
}

// Refusal is this package's own evidence for a refused mounted activation
// admission: the predicate that refused and the two module identities that
// predicate is about. The composition carries it erased beside the rule that
// produced it, and the analyzer recovers it at this type.
type Refusal struct {
	Reason RefusalReason
	// Trigger is the module of the mount the refused occurrence was placed
	// under. It is absent for a refusal raised before that mount is read.
	Trigger identity.ContentID
	// Body is the module of the candidate body route the refusal is about. It
	// is absent for a refusal no single body row produced.
	Body identity.ContentID
}

func (refusal Refusal) Available() bool { return refusal.Reason != RefusalNone }

// String spells the predicate and the leading bytes of both module
// identities. An operand a refusal never reached spells none, which is itself
// the statement that the predicate is not about that module.
func (refusal Refusal) String() string {
	if !refusal.Available() {
		return "none"
	}
	return refusal.Reason.String() + "@" + refusalModule(refusal.Trigger) + "->" + refusalModule(refusal.Body)
}

func refusalModule(module identity.ContentID) string {
	if !module.Available() {
		return "none"
	}
	return hex.EncodeToString(module[:4])
}

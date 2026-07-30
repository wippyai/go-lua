// Package userlattice defines the declarative SPI for third-party finite
// path-state axes.
package userlattice

import "github.com/wippyai/go-lua/analysis/domain/value/axis"

const extensionKind = "engine.state.userlattice"

// AxisID names one user-defined state axis.
type AxisID string

// ElementID names one element in a finite user lattice.
type ElementID string

// Element is the verified small-integer representation used on hot paths.
type Element uint16

// AxisSlot is a registry-local dense axis index.
type AxisSlot uint16

// OrderPair states Lower <= Upper. The verifier accepts either Hasse edges or
// already-transitive pairs and computes the reflexive-transitive closure.
type OrderPair struct {
	Lower ElementID
	Upper ElementID
}

// ElementMapEntry maps one element to another for a transfer hook. Unlisted
// elements map to themselves.
type ElementMapEntry struct {
	From ElementID
	To   ElementID
}

// AssignMode selects the assignment hook behavior.
type AssignMode uint8

const (
	// AssignDrop clears the target axis element on assignment.
	AssignDrop AssignMode = iota
	// AssignPropagate copies the source axis element through the optional map.
	AssignPropagate
)

// CallBoundaryMode selects whether an axis survives a call boundary.
type CallBoundaryMode uint8

const (
	// CallBoundaryDrop clears facts at a call boundary.
	CallBoundaryDrop CallBoundaryMode = iota
	// CallBoundaryKeep preserves facts through the optional map.
	CallBoundaryKeep
)

// AssignHook describes the closed on-assign trigger.
type AssignHook struct {
	Mode AssignMode
	Map  []ElementMapEntry
}

// CallBoundaryHook describes the closed on-call-boundary trigger.
type CallBoundaryHook struct {
	Mode CallBoundaryMode
	Map  []ElementMapEntry
}

// JoinHook describes the closed on-join trigger. It intentionally has no
// options: registration derives both the least-upper-bound and
// greatest-lower-bound tables from Spec.Order, so every accepted axis is a
// finite lattice usable by exact relational composition.
type JoinHook struct{}

// ClaimHook names an external claim that sets an axis element.
type ClaimHook struct {
	Claim   string
	Element ElementID
}

// Hooks is the complete closed transfer surface for a user lattice. The join
// trigger is derived from the verified partial order and does not accept a
// callback; this preserves the semilattice law verified at registration.
type Hooks struct {
	OnAssign       AssignHook
	OnJoin         JoinHook
	OnCallBoundary CallBoundaryHook
	OnClaim        []ClaimHook
}

// Spec is the declarative input accepted by Register and Verify.
type Spec struct {
	ID       AxisID
	Elements []ElementID
	Order    []OrderPair
	Bottom   ElementID
	Top      ElementID
	Hooks    Hooks
}

// Descriptor is the public handle returned by registration.
type Descriptor struct {
	id AxisID
}

func (d Descriptor) ID() AxisID { return d.id }

// Register verifies spec and attaches it to reg. The registry must not be
// frozen yet; callers freeze the registry after registering value axes and user
// state axes.
func Register(reg *axis.Registry, spec Spec) (Descriptor, error) {
	verified, err := Verify(spec)
	if err != nil {
		return Descriptor{}, err
	}
	if err := reg.RegisterExtension(verified); err != nil {
		return Descriptor{}, err
	}
	return Descriptor{id: verified.id}, nil
}

// MustRegister is the panic-on-error form of Register for package init tables.
func MustRegister(reg *axis.Registry, spec Spec) Descriptor {
	desc, err := Register(reg, spec)
	if err != nil {
		panic(err)
	}
	return desc
}

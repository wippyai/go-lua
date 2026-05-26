// Package ownership is the Ownership/Linearity axis of the reduced-product
// abstract value.
//
// # Axis law
//
// The Ownership/Linearity axis abstracts how a value is owned across a call: the
// Send/Freeze/Store/Borrow discipline that governs whether an update is strong or
// weak and whether cross-actor transfer is legal. Its lattice orders ownership
// states from most-precise (uniquely owned, strong update permitted) to
// least-precise (shared/borrowed, weak update only); Top is fully-shared and
// Bottom is unreachable. Join is the least upper bound of two ownership states.
//
// The axis is independently sound: Join conservatively weakens ownership so that
// no path observes a stronger guarantee than holds on all incoming paths.
//
// # Lattice
//
// The carrier is the three-element chain bottom < unique < shared. Unique admits a
// strong update; shared admits only a weak update. Top is shared (the conservative
// assumption) and Bottom is unreachable. Join takes the higher (less-precise)
// point, so it conservatively weakens toward shared.
//
// The product populates the axis with the safe default (Top) at admission, and its
// cross-axis reducer is the identity: this axis carries the lattice but does not
// yet drive the transfer engine's strong-vs-weak update decision, which stays on
// the engine's SSA-version and alias state.
package ownership

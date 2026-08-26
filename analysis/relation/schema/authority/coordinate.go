package authority

import "github.com/wippyai/go-lua/analysis/schema"

// Coordinate is the closed addressing vocabulary understood by the existing
// relcompile.Registry. It lives here because this owner-local package cannot
// import the compiler package; a later projection maps these declaration
// values to the registry's coordinate vocabulary.
type Coordinate uint8

const (
	CoordinateInvalid Coordinate = iota
	CoordinateAddress
	CoordinateParent
	CoordinateOrdinal
	CoordinateTag
	CoordinateDestination
	CoordinateOccurrence
)

// Available reports whether the coordinate is one of the registry's declared
// relation coordinates.
func (coordinate Coordinate) Available() bool {
	return coordinate >= CoordinateAddress && coordinate <= CoordinateOccurrence
}

// String returns the registry spelling used when a projection emits a
// refusal or builds a relcompile coordinate.
func (coordinate Coordinate) String() string {
	switch coordinate {
	case CoordinateAddress:
		return "address"
	case CoordinateParent:
		return "parent"
	case CoordinateOrdinal:
		return "ordinal"
	case CoordinateTag:
		return "tag"
	case CoordinateDestination:
		return "destination"
	case CoordinateOccurrence:
		return "occurrence"
	default:
		return "invalid"
	}
}

// Address is one owner declaration that a relation publishes a column at a
// named coordinate. Column is a local label, never a physical ordinal.
type Address struct {
	Coordinate Coordinate
	Column     schema.Key
}

// Available reports whether the address names one valid coordinate and a
// local column label.
func (address Address) Available() bool {
	return address.Coordinate.Available() && address.Column.Available()
}

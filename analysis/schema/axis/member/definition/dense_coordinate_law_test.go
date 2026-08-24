package definition

import "testing"

// TestAnAxisDeclaresADenseWidthAndNeverACoordinateType is the no-hand-export
// fence, stated at the declaration.
//
// A Factor's dense coordinate is not an owner's choice of type: it is the
// position a key of that axis occupies in the Factor its owner binds, and the
// generator publishes exactly one such type per axis. An axis that could name
// a coordinate type of its own would be a second authority over that position,
// and a consumer of another axis would have to reach into an owner package -
// or erase the coordinate to a builtin - to name it.
//
// So the declaration admits a WIDTH and refuses a type. A qualified type here
// is a hand-exported coordinate and is refused rather than adopted.
func TestAnAxisDeclaresADenseWidthAndNeverACoordinateType(t *testing.T) {
	for _, test := range []struct {
		name     string
		dense    GoType
		complete bool
	}{
		{name: "narrow-width", dense: GoType{Name: "uint32"}, complete: true},
		{name: "wide-width", dense: GoType{Name: "uint64"}, complete: true},
		{name: "hand-exported-coordinate", dense: GoType{PackagePath: "example/specimen/owner", Name: "Coordinate"}},
		{name: "hand-exported-generated-name", dense: GoType{PackagePath: "example/specimen", Name: "DenseCoordinate"}},
		{name: "not-a-width", dense: GoType{Name: "string"}},
		{name: "absent", dense: GoType{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := specimenBase()
			source.Binding.Key.Dense = test.dense
			if got := source.Complete(); got != test.complete {
				t.Fatalf("Complete() = %t, want %t for dense declaration %+v", got, test.complete, test.dense)
			}
		})
	}
}

// Package static owns Link's one detached namespace identity per concrete
// Project mount. ProgramArtifact owns every Program-internal static fact.
//
// The package is three files at three altitudes, referenced one way only:
// model.go is core, build.go is build, counts.go is read.
package static

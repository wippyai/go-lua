package factor

import (
	"reflect"
	"strings"
	"testing"
)

// TestPostSealEffectCarriersAreDetached guards the Factor's only mounted
// inputs/handles. The opaque MountedCall is Algebra-local; its row stores
// scalar identities only, never Project or Program proof objects.
func TestPostSealEffectCarriersAreDetached(t *testing.T) {
	for _, typeOf := range []reflect.Type{reflect.TypeOf(MountedArtifact{}), reflect.TypeOf(MountedCall{}), reflect.TypeOf(mountedCallRow{})} {
		for index := 0; index < typeOf.NumField(); index++ {
			field := typeOf.Field(index)
			path := field.Type.PkgPath()
			if strings.Contains(path, "/program/link/project") || strings.HasSuffix(path, "/program") {
				t.Fatalf("Effect carrier %s retained legacy proof field %s (%s)", typeOf.Name(), field.Name, path)
			}
		}
	}
}

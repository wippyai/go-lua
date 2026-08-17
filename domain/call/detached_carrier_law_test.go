package call

import (
	"reflect"
	"strings"
	"testing"
)

// TestMountedArtifactIsDataOnly guards the post-seal Call ABI: a mounted
// reusable artifact can carry only its immutable artifact pointer and scalar
// identity, never a Project shard/application authority.
func TestMountedArtifactIsDataOnly(t *testing.T) {
	typeOf := reflect.TypeOf(MountedArtifact{})
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		if strings.Contains(field.Type.PkgPath(), "/program/link/project") {
			t.Fatalf("Call mounted artifact retained Project carrier %s", field.Name)
		}
	}
}

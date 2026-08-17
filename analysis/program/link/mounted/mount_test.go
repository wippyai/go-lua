package mounted

import "testing"

func TestMountAdmissionRequiresArtifactAndUniqueModule(t *testing.T) {
	module := orderLawID("module")
	if (Mount{ModuleKey: module}).Available() {
		t.Fatal("mount without an artifact was available")
	}
	if mountsAvailable([]Mount{{ModuleKey: module}, {ModuleKey: module}}) {
		t.Fatal("duplicate incomplete mounts were admitted")
	}
	if mountsAvailable(nil) {
		t.Fatal("empty mount population was admitted")
	}
}

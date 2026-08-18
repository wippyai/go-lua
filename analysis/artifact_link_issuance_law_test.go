package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
)

// Link construction enumerates the sealed LaneLink table. Bootstrap Spec keys
// stay on their owning registrations.

func TestLinkConstructionUsesSealedLinkKeys(t *testing.T) {
	keys := composite.LinkKeys()
	if len(keys) == 0 {
		t.Fatal("sealed table declares no Link-lane keys")
	}
	for _, key := range keys {
		if !key.Available() || composite.MountedRuleKey(key) {
			t.Fatalf("link key %q is not a sealed LaneLink identity", key)
		}
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("analysis source location")
	}
	root := filepath.Dir(thisFile)
	for _, name := range []string{"artifact_plan.go", "artifact_rule_plan.go"} {
		src, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, literal := range []string{`"value-bootstrap"`, `"heap-bootstrap"`} {
			if strings.Contains(string(src), literal) {
				t.Fatalf("%s restates Link key %s; construction walks composite.LinkKeys", name, literal)
			}
		}
	}
}

package artifact_test

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/program/artifact"
	"github.com/wippyai/go-lua/program/target"
)

// A stored Program is accepted whole or rejected whole: decoder failures never
// expose a partial Program. The payload is produced only through public Lua
// lowering and the public artifact API.
func TestArtifactPublicDecodeRejectsEveryTruncation(t *testing.T) {
	contract := mustProfile(t)
	p := mustLower(t, "artifact-security.lua", `
local function f(x: number): number
  return x + 1
end
return f(0)
`)
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Provenance: "security"})
	if err != nil {
		t.Fatal(err)
	}
	for end := 0; end < len(encoded); end++ {
		artifactMustRejectPublic(t, encoded[:end], contract)
	}
	artifactMustRejectPublic(t, append(append([]byte(nil), encoded...), 0), contract)
	artifactMustRejectPublic(t, flip(encoded), contract)
}

func artifactMustRejectPublic(t *testing.T, data []byte, contract *target.Contract) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("artifact decode panicked: %v", recovered)
		}
	}()
	p, _, err := artifact.Decode(data, contract)
	if !errors.Is(err, artifact.ErrNoncanonical) || p != nil {
		t.Fatalf("artifact decode = %v, %v; want noncanonical rejection without Program", p, err)
	}
}

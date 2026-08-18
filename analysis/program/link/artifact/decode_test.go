package artifact_test

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link/artifact"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestArtifactDecoderRejectsUnavailableProgram(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := testfixture.SealSource(contract, "test.lua", []byte(`return 1`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := artifact.Encode(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := artifact.Decode(data, contract, map[identity.ContentID]*program.Program{}); got != nil || !errors.Is(err, artifact.ErrProgram) {
		t.Fatalf("missing mounted Program = %v/%v, want ErrProgram", got, err)
	}
}

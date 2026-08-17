package link

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestArtifactDecoderRejectsUnavailableProgram(t *testing.T) {
	sealed := contract(t)
	p := source(t, `return 1`)
	linked := linked(t, sealed, linkproject.Module{Name: "main", Program: p})
	data, err := EncodeArtifact(linked)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeArtifact(data, sealed, map[identity.ContentID]*program.Program{}); got != nil || !errors.Is(err, ErrArtifactProgram) {
		t.Fatalf("missing mounted Program = %v/%v, want ErrArtifactProgram", got, err)
	}
}

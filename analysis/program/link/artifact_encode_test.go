package link

import (
	"errors"
	"testing"

	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestArtifactEncoderEnforcesBoundedOutput(t *testing.T) {
	sealed := contract(t)
	p := source(t, `return 1`)
	linked := linked(t, sealed, linkproject.Module{Name: "main", Program: p})
	if _, err := encodeLinkArtifactBounded(linked, 1); !errors.Is(err, ErrArtifactLimit) {
		t.Fatalf("bounded artifact encoding error = %v, want ErrArtifactLimit", err)
	}
	data, err := EncodeArtifact(linked)
	if err != nil || len(data) == 0 {
		t.Fatalf("ordinary artifact encoding = %d/%v", len(data), err)
	}
}

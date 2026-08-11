package directbinding

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbinding "github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func directBindingProof(t *testing.T, preimage source.Preimage, flow authored.View, staticView static.View, entry keyspace.Term) flowbinding.Result {
	t.Helper()
	bodyResult, err := body.Seal(preimage, flow, staticView, entry)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	result, err := flowbinding.Seal(preimage, flow, bodyResult, entry)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	return result
}

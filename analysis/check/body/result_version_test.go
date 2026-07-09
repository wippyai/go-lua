package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestResultVersionCanonicalizesManifestOperationalEffectsAndTypestate(t *testing.T) {
	left := resultVersionForManifest(t, resultVersionManifest(false))
	right := resultVersionForManifest(t, resultVersionManifest(true))
	if left != right {
		t.Fatalf("semantically equivalent manifests produced result versions %d and %d", left, right)
	}
}

func resultVersionForManifest(t *testing.T, m *manifest.Manifest) uint64 {
	t.Helper()
	result, err := CheckChunk(parseChunk(t, "local value = 1"), Config{
		Registry: standard.Registry(),
		Signatures: signaturelookup.Source{
			Manifests: []*manifest.Manifest{m},
		},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	return result.ResultVersion()
}

func resultVersionManifest(reverse bool) *manifest.Manifest {
	effects := []signature.PathPresenceRefinement{
		{Path: pathdom.NewPlaceholder(0).Field("ready"), Presence: presence.Present()},
		{Path: pathdom.NewPlaceholder(0).Field("failed"), Presence: presence.Absent()},
	}
	states := []typestate.State{"active", "finished"}
	transitions := []typestate.TransitionDecl{
		{From: "active", To: "finished"},
		{From: "finished", To: "active"},
	}
	if reverse {
		effects[0], effects[1] = effects[1], effects[0]
		states[0], states[1] = states[1], states[0]
		transitions[0], transitions[1] = transitions[1], transitions[0]
	}
	m := manifest.New("result-version-canonical")
	m.TypestateProtocols["transaction"] = typestate.Definition{
		Protocol:    "transaction",
		States:      states,
		FinalStates: []typestate.State{"finished"},
		Transitions: transitions,
	}
	m.DefineFunctionSignature("f", signature.Function{
		Type: typ.Func().Param("value", typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{
			NormalReturnPresenceRefinements: effects,
		},
	})
	return m
}

package seed

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// The seed law is intentionally about the Call relation, not package wiring:
// every source remains open until an independent domain rule contributes a
// directly known target.
func TestSeedPreservesOpaqueDispatch(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{Name: "dispatch.lua", Text: []byte(`
local function exact() return 1 end
local function dynamic(f) return f() end
exact()
dynamic(exact)
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "dispatch", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := calldomain.New(linked)
	if !ok {
		t.Fatal("Call algebra")
	}
	composition := engine.NewComposition()
	owner, ok := callowner.Declare(composition, seedKey(1), algebra)
	if !ok {
		t.Fatal("Call owner")
	}
	rule, ruleOK := Declare(composition, seedKey(2), seedKey(3), seedKey(4), owner)
	if !ruleOK || rule == nil {
		t.Fatal("Call seed rule")
	}
	var open, applications int
	for index := 0; index < algebra.KeyCount(); index++ {
		key, ok := algebra.KeyAt(index)
		if !ok {
			t.Fatal("Call key")
		}
		if key.IsApplication() {
			applications++
			if _, _, present := result(owner, key); present {
				t.Fatal("application seed admitted a producer")
			}
			if _, present := rule.Instance(key); present {
				t.Fatal("application seed instance remained available")
			}
			continue
		}
		_, value, ok := result(owner, key)
		if !ok || !algebra.Admits(key, value) {
			t.Fatal("seed result")
		}
		if value.IsOpen() {
			open++
			if !value.HasOpaqueAlternative() {
				t.Fatal("open seed lost opaque alternative")
			}
		}
	}
	if open+applications != algebra.KeyCount() || applications == 0 {
		t.Fatalf("open=%d applications=%d keys=%d", open, applications, algebra.KeyCount())
	}
}

func seedKey(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[len(digest)-1] = value
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("seed semantic key")
	}
	return key
}

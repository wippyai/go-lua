package typ

import (
	"context"
	"os"
	"os/exec"
	"runtime/debug"
	"testing"
	"time"
)

func TestIsSoftDeepGraph(t *testing.T) {
	if os.Getenv("GO_LUA_SOFT_STACK_PROBE") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIsSoftDeepGraph$")
		cmd.Env = append(os.Environ(), "GO_LUA_SOFT_STACK_PROBE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("deep graph subprocess: %v\n%s", err, output)
		}
		return
	}

	debug.SetMaxStack(1 << 20)
	for _, leaf := range []Type{Any, String} {
		node := leaf
		for range 20000 {
			node = &Array{Element: node}
		}
		if got, want := IsSoft(node, SoftPlaceholderPolicy), leaf == Any; got != want {
			t.Fatalf("softness = %v, want %v", got, want)
		}
	}
}

func TestIsSoftAnnotatedCycles(t *testing.T) {
	first, second := &Annotated{}, &Annotated{}
	first.Inner, second.Inner = second, first
	if IsSoft(first, SoftPlaceholderPolicy) {
		t.Fatal("an annotation cycle has no placeholder evidence")
	}
}

func TestIsSoftRetainsCompletedResults(t *testing.T) {
	leaf := &countedSoftLeaf{}
	shared := &Array{Element: leaf}
	root := &Union{Members: []Type{shared, String}}
	memo := make(map[Type]bool)
	if isSoft(root, SoftPlaceholderPolicy, memo) {
		t.Fatal("a concrete member must keep the union non-soft")
	}
	if !isSoft(shared, SoftPlaceholderPolicy, memo) {
		t.Fatal("a failed parent must not invalidate a completed soft child")
	}
	if leaf.visits != 1 {
		t.Fatalf("leaf visited %d times, want one", leaf.visits)
	}
}

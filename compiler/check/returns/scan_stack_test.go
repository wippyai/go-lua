package returns

import (
	"context"
	"os"
	"os/exec"
	"runtime/debug"
	"testing"
	"time"

	"github.com/wippyai/go-lua/types/typ"
)

func TestScanTypeDeepGraph(t *testing.T) {
	if os.Getenv("GO_LUA_SCAN_STACK_PROBE") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestScanTypeDeepGraph$")
		cmd.Env = append(os.Environ(), "GO_LUA_SCAN_STACK_PROBE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("deep graph subprocess: %v\n%s", err, output)
		}
		return
	}

	debug.SetMaxStack(1 << 20)
	var node typ.Type = typ.String
	for range 20000 {
		node = &typ.Array{Element: node}
	}
	visits := 0
	if !scanType(node, nil, func(node typ.Type) (bool, bool) {
		visits++
		return node == typ.String, true
	}) {
		t.Fatal("deep scan missed its terminal node")
	}
	if visits != 20001 {
		t.Fatalf("visits = %d, want 20001", visits)
	}
}

func TestScanTypePreservesDepthFirstOrder(t *testing.T) {
	fn := typ.Func().Param("first", typ.String).Param("second", typ.Number).
		Returns(typ.Boolean).Variadic(typ.Integer).Build()
	root := &typ.Record{
		Fields:    []typ.Field{{Name: "fn", Type: fn}},
		Metatable: typ.Nil, MapKey: typ.Self, MapValue: typ.Unknown,
	}
	want := []typ.Type{root, fn, typ.String, typ.Number, typ.Boolean, typ.Integer, typ.Nil, typ.Self, typ.Unknown}
	index := 0
	if scanType(root, nil, func(node typ.Type) (bool, bool) {
		if index >= len(want) || node != want[index] {
			t.Fatalf("unexpected visit %d: %T", index, node)
		}
		index++
		return false, true
	}) {
		t.Fatal("scan reports a match without a matching predicate")
	}
	if index != len(want) {
		t.Fatalf("visits = %d, want %d", index, len(want))
	}
}

func TestScanTypeSkipsMissingMethodTypes(t *testing.T) {
	root := &typ.Interface{Name: "partial", Methods: []typ.Method{{Name: "missing"}}}
	visits := 0
	scanType(root, nil, func(node typ.Type) (bool, bool) {
		if node != root {
			t.Fatalf("unexpected child: %T", node)
		}
		visits++
		return false, true
	})
	if visits != 1 {
		t.Fatalf("visits = %d, want one", visits)
	}
}

package regression

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestFlowFixtures(t *testing.T) {
	cases := []struct {
		name      string
		wantError bool
	}{
		{name: "if-simple"},
		{name: "if-else"},
		{name: "break-in-for"},
		{name: "return-correct-type"},
		{name: "return-multiple-values"},
		{name: "closure-captures-type"},
		{name: "return-wrong-type", wantError: true},
		{name: "do-block-scope", wantError: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			src := readFlowFixtureSource(t, c.name)
			result := testutil.Check(src, testutil.WithStdlib())
			if result.HasError() != c.wantError {
				t.Fatalf("HasError=%v, want %v; errors=%v", result.HasError(), c.wantError, testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

func TestTransferNodeKinds(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "global-print-visible",
			src:  "local x = 1\nprint(x)\n",
		},
		{
			name: "recursive-local-function-return",
			src: "local function fib(n: number): number\n" +
				"    if n < 2 then return n end\n" +
				"    return fib(n - 1) + fib(n - 2)\n" +
				"end\n" +
				"print(fib(10))\n",
		},
		{
			name: "recursive-global-function-return",
			src: "function factorial(n: number): number\n" +
				"    if n <= 1 then return 1 end\n" +
				"    return n * factorial(n - 1)\n" +
				"end\n",
		},
		{
			name: "field-write-read-back",
			src:  "local t = {}\nt.x = 5\nlocal y: number = t.x\n",
		},
		{
			name: "func-def-field-write",
			src: "local M = {}\n" +
				"function M.add(a: number, b: number): number\n" +
				"    return a + b\n" +
				"end\n" +
				"local result: number = M.add(1, 2)\n",
		},
		{
			name: "table-literal-field-call",
			src:  "local t = { id = function(msg) return msg end }\nt.id(\"hi\")\n",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			result := testutil.Check(c.src, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("transfer regression emitted errors: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

func TestNarrowingFixtures(t *testing.T) {
	fixtures := []string{
		"nil-check-optional",
		"nil-check-else",
		"truthiness-narrows",
		"typeof-string",
		"typeof-number",
	}

	for _, name := range fixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			src := readNarrowingFixtureSource(t, name)
			result := testutil.Check(src, testutil.WithStdlib())
			if result.HasError() {
				t.Fatalf("narrowing regression emitted errors: %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}

func readFlowFixtureSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "fixtures", "flow", name, "main.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read flow fixture %s: %v", name, err)
	}
	return string(data)
}

func readNarrowingFixtureSource(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "testdata", "fixtures", "narrowing", name, "main.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read narrowing fixture %s: %v", name, err)
	}
	return string(data)
}

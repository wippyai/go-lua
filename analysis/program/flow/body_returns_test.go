package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestBodyReturnProjectionUsesTheExactBodyBoundary(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "body-return.lua",
		Text: []byte("return 1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := program.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	boundary, ok := program.Flow().FunctionBoundaries().ForBody(entry)
	if !ok || !boundary.Available() {
		t.Fatal("entry Body boundary is unavailable")
	}
	projection, ok := program.Flow().BodyReturns().ForBody(boundary)
	if !ok || !projection.Available() {
		t.Fatal("entry Body Return projection is unavailable")
	}
	if _, ok := projection.Outcome(); !ok || projection.ValuesCount() == 0 {
		t.Fatal("Body Return projection lost its Outcome or Values witness")
	}
}

package project

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
)

// The project boundary is deliberately a value ABI: a sealed binding, one
// mounted runtime, one immutable input Batch, and one sealed target Reader.
// This law prevents a callback stream, mutable builder, or receipt-shaped
// transport from returning to the operator package under a new name.
func TestProjectionConsumesConcreteBatchAndReader(t *testing.T) {
	fn := reflect.TypeOf(Execute)
	if fn.NumIn() != 4 ||
		fn.In(0).Name() != "ProjectBinding" ||
		fn.In(1).Name() != "Mounted" ||
		fn.In(2) != reflect.TypeOf(tuple.Batch{}) ||
		fn.In(3) != reflect.TypeOf(read.Reader{}) ||
		fn.NumOut() != 2 ||
		fn.Out(0) != reflect.TypeOf([]tuple.Batch{}) ||
		fn.Out(1).Kind() != reflect.Bool {
		t.Fatalf("projection ABI = %v", fn)
	}
}

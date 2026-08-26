package merge

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

func TestExecuteIsBatchOnly(t *testing.T) {
	typ := reflect.TypeOf(Execute)
	if typ.NumIn() != 3 || typ.In(0) != reflect.TypeOf(arrangement.MergeBinding{}) || typ.In(1) != reflect.TypeOf(witness.Mounted{}) || typ.In(2) != reflect.TypeOf([]tuple.Batch{}) {
		t.Fatalf("Merge.Execute inputs = %v", typ)
	}
	if typ.NumOut() != 2 || typ.Out(0) != reflect.TypeOf([]tuple.Batch{}) || typ.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Merge.Execute outputs = %v", typ)
	}
}

func TestExecuteRefusesUnavailableAuthorities(t *testing.T) {
	if batches, ok := Execute(arrangement.MergeBinding{}, witness.Mounted{}, []tuple.Batch{{}}); ok || batches != nil {
		t.Fatalf("unavailable Merge authorities accepted: ok=%v batches=%v", ok, batches)
	}
}

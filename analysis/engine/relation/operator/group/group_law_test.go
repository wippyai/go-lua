package group

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestExecuteIsBatchOnly(t *testing.T) {
	typ := reflect.TypeOf(Execute)
	if typ.NumIn() != 3 || typ.In(0) != reflect.TypeOf(arrangement.GroupBinding{}) || typ.In(1) != reflect.TypeOf(witness.Mounted{}) || typ.In(2) != reflect.TypeOf(tuple.Batch{}) {
		t.Fatalf("Group.Execute inputs = %v", typ)
	}
	if typ.NumOut() != 2 || typ.Out(0) != reflect.TypeOf([]tuple.Batch{}) || typ.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Group.Execute outputs = %v", typ)
	}
}

func TestExecuteRefusesUnavailableAuthorities(t *testing.T) {
	if batches, ok := Execute(arrangement.GroupBinding{}, witness.Mounted{}, tuple.Batch{}); ok || batches != nil {
		t.Fatalf("unavailable Group authorities accepted: ok=%v batches=%v", ok, batches)
	}
}

func TestCardinalityBoundHasNoImplicitUnboundedFallback(t *testing.T) {
	if cardinalityBound(model.Cardinality{}) != 0 {
		t.Fatal("unavailable cardinality received a default bound")
	}
}

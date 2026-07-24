package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestCallResultsConsumesExactPublishedApplyArguments(t *testing.T) {
	partition, err := equation.PartitionFromClosures(equation.OutputClosure{Values: callArgumentFacts("op-00000007", [][]byte{
		[]byte("scalar/number/42"),
		[]byte("temp/1"),
	})})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	arguments := map[int][]byte{}
	if err := consumeCallArgumentFacts([]byte("call/op-00000007"), arguments, partition); err != nil {
		t.Fatalf("consume call arguments: %v", err)
	}
	if got := string(arguments[0]); got != "scalar/number/42" {
		t.Fatalf("argument 0 = %q, want exact scalar reference", got)
	}
	if got := string(arguments[1]); got != "temp/1" {
		t.Fatalf("argument 1 = %q, want exact temporary reference", got)
	}
}

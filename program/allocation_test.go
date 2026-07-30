package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
)

func TestNoLoopSealAllocationDoesNotRegress(t *testing.T) {
	b := program.NewBuilder()
	entry := b.Body(program.Span{})
	b.SetEntry(entry)
	value := b.Integer(program.Span{}, entry, 1)
	values := b.Values(program.Span{}, entry, []program.Term{value}, 0)
	b.SetBody(entry, b.Normal(program.Span{}, entry, values))
	allocations := testing.AllocsPerRun(100, func() {
		if _, err := b.Seal(); err != nil {
			t.Fatal(err)
		}
	})
	// Frozen against clean pre-Loop HEAD 0aa847f47. Optional Loop/Break
	// context must not tax Programs that contain neither family.
	const baseline = 20
	if allocations > baseline {
		t.Fatalf("no-loop Seal allocations = %.0f, baseline = %d", allocations, baseline)
	}
}

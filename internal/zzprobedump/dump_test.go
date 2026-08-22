package zzprobedump

import (
	"strings"
	"testing"
	"time"
)

func TestCountersLineIsIndependentOfTiming(t *testing.T) {
	left := Record{
		Fixture: "bench/fibonacci", Class: "ok", Status: "complete", ErrText: "none",
		Signature: "sig", Epochs: 2, Passes: 3, Evaluates: 4, Fails: 1, Folds: 5, Restarts: 6,
		Solve: time.Millisecond, Compile: 2 * time.Millisecond, Seal: 3 * time.Millisecond,
	}
	right := left
	right.Solve = time.Second
	right.Compile = 2 * time.Second
	right.Seal = 3 * time.Second
	counters := CountersLine(left)
	if counters != CountersLine(right) {
		t.Fatal("counters line followed wall-clock durations")
	}
	for _, field := range []string{"solve=", "compile=", "seal="} {
		if strings.Contains(counters, field) {
			t.Fatalf("counters line carries %s", field)
		}
		if !strings.Contains(TimingLine(left), field) {
			t.Fatalf("timing line omits %s", field)
		}
	}
	if got := RecordLines(left); got != counters+"\n"+TimingLine(left) {
		t.Fatalf("dump record is not counters then timing: %q", got)
	}
}

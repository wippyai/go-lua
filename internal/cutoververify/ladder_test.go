package cutoververify

import "testing"

func TestLadderResultByteIdenticalIgnoresTiming(t *testing.T) {
	old := "    zzprobe_solverladder_test.go:717: ZZPROBE fixture bench/fibonacci solve=21.543ms compile=58.107ms epochs=0 folds=0\n" +
		"ZZPROBE M1 refineNodes=26091 refineClasses=22739 ratio=1.1474\n"
	updated := "    zzprobe_solverladder_test.go:717: ZZPROBE fixture bench/fibonacci solve=19.001ms compile=61.220ms epochs=0 folds=0\n" +
		"ZZPROBE M1 refineNodes=26091 refineClasses=22739 ratio=1.1474\n"

	result := LadderResult("bench/fibonacci", old, updated)
	if result.Status != StatusPass {
		t.Fatalf("got status %s, want PASS (only timing differs): %s", result.Status, result.Detail)
	}
	if result.Note != "BYTE-IDENTICAL" {
		t.Fatalf("got note %q, want BYTE-IDENTICAL", result.Note)
	}
}

func TestLadderResultReportsCounterDelta(t *testing.T) {
	old := "ZZPROBE dump counters\tfixture=bench/fibonacci\tepochs=0\tfolds=0\n"
	updated := "ZZPROBE dump counters\tfixture=bench/fibonacci\tepochs=3\tfolds=7\n"

	result := LadderResult("bench/fibonacci", old, updated)
	if result.Status != StatusFail {
		t.Fatalf("got status %s, want FAIL on a counter delta", result.Status)
	}
	if result.Detail == "" {
		t.Fatal("want a non-empty delta detail")
	}
}

func TestNormalizeZZProbeLinesIgnoresNonProbeOutput(t *testing.T) {
	output := "=== RUN   TestZZProbeSolverLadderFixture\n" +
		"    zzprobe_solverladder_test.go:718: ZZPROBE dump counters\tfixture=bench/fibonacci\tepochs=0\n" +
		"        timing\tfixture=bench/fibonacci\tsolve=21.543ms\n" +
		"--- PASS: TestZZProbeSolverLadderFixture (0.58s)\n" +
		"PASS\n"

	lines := normalizeZZProbeLines(output)
	if len(lines) != 1 {
		t.Fatalf("got %d ZZPROBE lines, want 1 (the timing line has no ZZPROBE marker): %v", len(lines), lines)
	}
	if lines[0] != "ZZPROBE dump counters fixture=bench/fibonacci epochs=0" {
		t.Fatalf("got %q", lines[0])
	}
}

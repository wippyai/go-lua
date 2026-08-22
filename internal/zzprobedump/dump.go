package zzprobedump

import (
	"fmt"
	"time"
)

// Record is one fixture's ZZPROBE dump payload. Counters are the A/B strip;
// durations live only on the timing line.
type Record struct {
	Fixture, Class, Status, Signature, ErrText        string
	Err                                               bool
	Epochs, Passes, Evaluates, Fails, Folds, Restarts uint64
	Solve, Compile, Seal                              time.Duration
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// CountersLine is the reproducible strip. It carries no wall-clock durations.
func CountersLine(record Record) string {
	return fmt.Sprintf(
		"counters\tfixture=%s\tclass=%s\tstatus=%s\terr=%t\tepochs=%d\tpasses=%d\tevaluates=%d\tfails=%d\tfolds=%d\trestarts=%d\tsig=%s\terrtext=%s",
		record.Fixture, orDash(record.Class), orDash(record.Status), record.Err,
		record.Epochs, record.Passes, record.Evaluates, record.Fails, record.Folds, record.Restarts,
		record.Signature, orDash(record.ErrText),
	)
}

// TimingLine is the per-fixture wall-clock line.
func TimingLine(record Record) string {
	return fmt.Sprintf(
		"timing\tfixture=%s\tsolve=%s\tcompile=%s\tseal=%s",
		record.Fixture,
		record.Solve.Round(time.Microsecond),
		record.Compile.Round(time.Microsecond),
		record.Seal.Round(time.Microsecond),
	)
}

// RecordLines is counters then timing.
func RecordLines(record Record) string {
	return CountersLine(record) + "\n" + TimingLine(record)
}

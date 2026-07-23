package interproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestProjectedTableSharesExactProjectionAtOneTenAndOneHundred(t *testing.T) {
	for _, fanout := range []int{1, 10, 100} {
		t.Run(fmt.Sprintf("fanout-%d", fanout), func(t *testing.T) {
			table := NewProjectedTable()
			artifact := demandedBodyArtifactFixture(t)
			var executions atomic.Int32
			started := make(chan struct{})
			release := make(chan struct{})
			runner := func(_ context.Context, body DemandedBodyArtifact, entry EntryBinding) (ClosedOutcome, []ReadObservation, error) {
				if executions.Add(1) != 1 {
					return ClosedOutcome{}, nil, errors.New("single-flight executed more than one owner transaction")
				}
				close(started)
				<-release
				projection, err := body.ReadCertificate().Project(entry)
				if err != nil {
					return ClosedOutcome{}, nil, err
				}
				outcome, err := NewClosedOutcome(append([]byte("portable/"), projection.CanonicalBytes()...))
				return outcome, certificateObservations(body.ReadCertificate()), err
			}

			results := make([]ClosedOutcome, fanout)
			errs := make([]error, fanout)
			ready := make(chan struct{})
			var callers sync.WaitGroup
			callers.Add(fanout)
			for index := 0; index < fanout; index++ {
				index := index
				go func() {
					defer callers.Done()
					<-ready
					entry := tableEntry(t, "same", "true", "diagnostic", fmt.Sprintf("unread-%d", index))
					results[index], errs[index] = DirectCall{Table: table, Runner: runner}.Resolve(context.Background(), artifact, entry)
				}()
			}
			close(ready)
			<-started
			close(release)
			callers.Wait()
			for _, err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if executions.Load() != 1 {
				t.Fatalf("executions = %d, want 1", executions.Load())
			}
			for index := 1; index < len(results); index++ {
				if !bytes.Equal(results[0].CanonicalBytes(), results[index].CanonicalBytes()) {
					t.Fatalf("result %d differs for equal projection", index)
				}
			}
			metrics := table.Metrics()
			if metrics.Misses != 1 || metrics.Executions != 1 || metrics.Cells != 1 || metrics.Hits+metrics.Joins != uint64(fanout-1) {
				t.Fatalf("metrics = %+v, want one cell and one execution for %d callers", metrics, fanout)
			}
		})
	}
}

func TestProjectedTableSeparatesDistinctCertifiedReads(t *testing.T) {
	table := NewProjectedTable()
	artifact := demandedBodyArtifactFixture(t)
	var executions atomic.Int32
	runner := countingRunner(&executions)
	first, err := table.Resolve(context.Background(), artifact, tableEntry(t, "first", "true", "diagnostic", "ignored"), runner)
	if err != nil {
		t.Fatal(err)
	}
	second, err := table.Resolve(context.Background(), artifact, tableEntry(t, "second", "true", "diagnostic", "ignored"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first.CanonicalBytes(), second.CanonicalBytes()) || executions.Load() != 2 {
		t.Fatalf("distinct projected entries were merged: executions=%d", executions.Load())
	}
	metrics := table.Metrics()
	if metrics.Misses != 2 || metrics.Cells != 2 {
		t.Fatalf("metrics = %+v, want two exact cells", metrics)
	}
}

func TestProjectedTableConfirmsCanonicalBytesAfterHashCollision(t *testing.T) {
	table := newProjectedTable(func([]byte) ContentID { return ContentID{1} })
	artifact := demandedBodyArtifactFixture(t)
	var executions atomic.Int32
	runner := countingRunner(&executions)
	for _, value := range []string{"left", "right"} {
		if _, err := table.Resolve(context.Background(), artifact, tableEntry(t, value, "true", "diagnostic", "ignored"), runner); err != nil {
			t.Fatal(err)
		}
	}
	if executions.Load() != 2 || table.Metrics().Cells != 2 {
		t.Fatalf("digest bucket collision merged distinct projection bytes: executions=%d metrics=%+v", executions.Load(), table.Metrics())
	}
}

func TestProjectedTableFailsClosedOnUndeclaredReadAndDoesNotPublish(t *testing.T) {
	table := NewProjectedTable()
	artifact := demandedBodyArtifactFixture(t)
	var executions atomic.Int32
	runner := func(_ context.Context, _ DemandedBodyArtifact, _ EntryBinding) (ClosedOutcome, []ReadObservation, error) {
		executions.Add(1)
		outcome, err := NewClosedOutcome([]byte("portable"))
		return outcome, []ReadObservation{{Role: ReadSemantic, Selector: "hidden"}}, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		_, err := table.Resolve(context.Background(), artifact, tableEntry(t, "same", "true", "diagnostic", "ignored"), runner)
		var incomplete *IncompleteReadCertificateError
		if !errors.As(err, &incomplete) {
			t.Fatalf("attempt %d error = %v, want incomplete certificate", attempt, err)
		}
	}
	if executions.Load() != 2 || table.Metrics().Cells != 0 {
		t.Fatalf("failed audit published a cell: executions=%d metrics=%+v", executions.Load(), table.Metrics())
	}
}

func countingRunner(executions *atomic.Int32) DirectCallRunner {
	return func(_ context.Context, artifact DemandedBodyArtifact, entry EntryBinding) (ClosedOutcome, []ReadObservation, error) {
		executions.Add(1)
		projection, err := artifact.ReadCertificate().Project(entry)
		if err != nil {
			return ClosedOutcome{}, nil, err
		}
		outcome, err := NewClosedOutcome(projection.CanonicalBytes())
		return outcome, certificateObservations(artifact.ReadCertificate()), err
	}
}

func tableEntry(t *testing.T, value, guard, diagnostic, unread string) EntryBinding {
	t.Helper()
	entry, err := NewEntryBinding([]EntryValue{
		{Selector: "value", Encoding: []byte(value)},
		{Selector: "guard", Encoding: []byte(guard)},
		{Selector: "diagnostic", Encoding: []byte(diagnostic)},
		{Selector: "unread", Encoding: []byte(unread)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func certificateObservations(certificate ReadProjectionCertificate) []ReadObservation {
	reads := certificate.Reads()
	observed := make([]ReadObservation, len(reads))
	for index, read := range reads {
		observed[index] = ReadObservation{Role: read.Role, Selector: read.Selector}
	}
	return observed
}

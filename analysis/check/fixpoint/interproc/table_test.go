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

func TestProjectedTableInvalidatesExactlyChangedSourceRegistryAndProviderSnapshots(t *testing.T) {
	for _, changedKind := range []string{"source", "registry", "provider"} {
		t.Run(changedKind, func(t *testing.T) {
			snapshots := newDependencySnapshots(map[string]ContentID{
				"source":   testContentID("source-v1"),
				"registry": testContentID("registry-v1"),
				"provider": testContentID("provider-v1"),
				"contract": testContentID("contract-v1"),
			})
			table := NewProjectedTableWithDependencyResolver(snapshots)
			if table == nil {
				t.Fatal("table rejected a valid dependency resolver")
			}
			artifacts := map[string]DemandedBodyArtifact{}
			for _, kind := range []string{"source", "registry", "provider", "contract"} {
				artifacts[kind] = demandedArtifactWithDependency(t, kind, snapshots.get(kind))
			}

			var executions atomic.Int32
			runner := countingRunner(&executions)
			entry := tableEntry(t, "same", "true", "diagnostic", "ignored")
			for _, kind := range []string{"source", "registry", "provider", "contract"} {
				if _, err := table.Resolve(context.Background(), artifacts[kind], entry, runner); err != nil {
					t.Fatalf("initial %s resolution: %v", kind, err)
				}
			}
			if executions.Load() != 4 || table.Metrics().Cells != 4 {
				t.Fatalf("initial cells/executions = %+v/%d, want four", table.Metrics(), executions.Load())
			}

			next := testContentID(changedKind + "-v2")
			snapshots.set(changedKind, next)
			evicted, err := table.InvalidateStale(context.Background())
			if err != nil || evicted != 1 || table.Metrics().Cells != 3 {
				t.Fatalf("stale invalidation = %d, %v; metrics=%+v, want exactly one eviction", evicted, err, table.Metrics())
			}
			for _, kind := range []string{"source", "registry", "provider", "contract"} {
				if kind == changedKind {
					continue
				}
				if _, err := table.Resolve(context.Background(), artifacts[kind], entry, runner); err != nil {
					t.Fatalf("unrelated %s resolution after %s change: %v", kind, changedKind, err)
				}
			}
			if executions.Load() != 4 {
				t.Fatalf("an unrelated %s change reran a dependent-free instance: executions=%d", changedKind, executions.Load())
			}
			if _, err := table.Resolve(context.Background(), artifacts[changedKind], entry, runner); err == nil {
				t.Fatalf("old %s manifest remained reusable after content change", changedKind)
			} else {
				var mismatch *DependencySnapshotMismatchError
				if !errors.As(err, &mismatch) {
					t.Fatalf("old %s error = %v, want content mismatch", changedKind, err)
				}
			}
			updated := demandedArtifactWithDependency(t, changedKind, next)
			if _, err := table.Resolve(context.Background(), updated, entry, runner); err != nil {
				t.Fatalf("updated %s resolution: %v", changedKind, err)
			}
			if executions.Load() != 5 || table.Metrics().Cells != 4 {
				t.Fatalf("updated %s did not create exactly one replacement: executions=%d metrics=%+v", changedKind, executions.Load(), table.Metrics())
			}
		})
	}
}

func TestProjectedTableRejectsInFlightPublicationAfterDependencyChanges(t *testing.T) {
	snapshots := newDependencySnapshots(map[string]ContentID{"source": testContentID("source-v1")})
	table := NewProjectedTableWithDependencyResolver(snapshots)
	artifact := demandedArtifactWithDependency(t, "source", snapshots.get("source"))
	entry := tableEntry(t, "same", "true", "diagnostic", "ignored")
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	runner := func(_ context.Context, body DemandedBodyArtifact, entry EntryBinding) (ClosedOutcome, []ReadObservation, error) {
		executions.Add(1)
		close(started)
		<-release
		projection, err := body.ReadCertificate().Project(entry)
		if err != nil {
			return ClosedOutcome{}, nil, err
		}
		outcome, err := NewClosedOutcome(projection.CanonicalBytes())
		return outcome, certificateObservations(body.ReadCertificate()), err
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := table.Resolve(context.Background(), artifact, entry, runner)
		errCh <- err
	}()
	<-started
	snapshots.set("source", testContentID("source-v2"))
	close(release)
	err := <-errCh
	var mismatch *DependencySnapshotMismatchError
	if !errors.As(err, &mismatch) || table.Metrics().Cells != 0 {
		t.Fatalf("in-flight publication = %v, metrics=%+v; want rejected and unpublished", err, table.Metrics())
	}
	if _, err := table.Resolve(context.Background(), artifact, entry, runner); !errors.As(err, &mismatch) {
		t.Fatalf("stale artifact retry error = %v, want mismatch", err)
	}
	updated := demandedArtifactWithDependency(t, "source", snapshots.get("source"))
	if _, err := table.Resolve(context.Background(), updated, entry, countingRunner(&executions)); err != nil {
		t.Fatalf("updated artifact resolution: %v", err)
	}
	if executions.Load() != 2 {
		t.Fatalf("executions = %d, want rejected old owner plus one updated owner", executions.Load())
	}
}

func TestProjectedTableEvictsTransitiveExactCalleeCallers(t *testing.T) {
	table := NewProjectedTable()
	callee := demandedArtifactWithDependency(t, "source", testContentID("callee-source-v1"))
	middle := demandedArtifactWithDependency(t, "provider", testContentID("middle-provider-v1"))
	caller := demandedArtifactWithDependency(t, "registry", testContentID("caller-registry-v1"))
	entry := tableEntry(t, "same", "true", "diagnostic", "ignored")
	var executions atomic.Int32
	runner := countingRunner(&executions)
	for _, artifact := range []DemandedBodyArtifact{callee, middle, caller} {
		if _, err := table.Resolve(context.Background(), artifact, entry, runner); err != nil {
			t.Fatal(err)
		}
	}
	calleeKey, err := NewInstanceKey(callee, entry)
	if err != nil {
		t.Fatal(err)
	}
	middleKey, err := NewInstanceKey(middle, entry)
	if err != nil {
		t.Fatal(err)
	}
	callerKey, err := NewInstanceKey(caller, entry)
	if err != nil {
		t.Fatal(err)
	}
	if err := table.LinkCallee(middleKey, calleeKey); err != nil {
		t.Fatal(err)
	}
	if err := table.LinkCallee(callerKey, middleKey); err != nil {
		t.Fatal(err)
	}
	if evicted := table.InvalidateDependency(testContentID("callee-source-v1")); evicted != 3 || table.Metrics().Cells != 0 {
		t.Fatalf("callee invalidation evicted %d cells; metrics=%+v, want transitive three-cell eviction", evicted, table.Metrics())
	}
	for _, artifact := range []DemandedBodyArtifact{callee, middle, caller} {
		if _, err := table.Resolve(context.Background(), artifact, entry, runner); err != nil {
			t.Fatal(err)
		}
	}
	if executions.Load() != 6 {
		t.Fatalf("transitive callers survived callee eviction: executions=%d, want 6", executions.Load())
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

func demandedArtifactWithDependency(t *testing.T, kind string, id ContentID) DemandedBodyArtifact {
	t.Helper()
	base := demandedBodyArtifactFixture(t)
	manifest, err := NewDependencyManifest([]Dependency{{Kind: kind, ID: id}})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := NewDemandedBodyArtifact(base.Body(), base.ParameterSchema(), base.DemandKey(), base.ReadCertificate(), base.SolverPolicyID(), manifest, base.DiagnosticReadSets())
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

type dependencySnapshots struct {
	mu  sync.Mutex
	ids map[string]ContentID
}

func newDependencySnapshots(ids map[string]ContentID) *dependencySnapshots {
	copy := make(map[string]ContentID, len(ids))
	for kind, id := range ids {
		copy[kind] = id
	}
	return &dependencySnapshots{ids: copy}
}

func (s *dependencySnapshots) ResolveContentID(_ context.Context, dependency Dependency) (ContentID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, present := s.ids[dependency.Kind]
	if !present {
		return ContentID{}, fmt.Errorf("missing dependency %q", dependency.Kind)
	}
	return id, nil
}

func (s *dependencySnapshots) get(kind string) ContentID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ids[kind]
}

func (s *dependencySnapshots) set(kind string, id ContentID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[kind] = id
}

package interproc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summaryinstance"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// TestIntegrationGateFromScratchEquality proves the harness-only table route
// preserves the result of a representative two-module check. Module B calls
// module A through the demanded-artifact/table/portable-codec path; the
// baseline specializes both bodies directly and has no cache involvement.
func TestIntegrationGateFromScratchEquality(t *testing.T) {
	t.Parallel()
	h := newIntegrationGateHarness(t)
	entry := gateEntry(t, "strict", "number", "caller-a", "site-a")

	cached, err := h.checkThroughSummary(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := h.checkFromScratch(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cached.CanonicalBytes(), fresh.CanonicalBytes()) {
		t.Fatalf("summary-path result differs from fresh specialization\nsummary: %x\nfresh:   %x", cached.CanonicalBytes(), fresh.CanonicalBytes())
	}
	metrics := h.table.Metrics()
	if metrics.Cells != 2 || metrics.Executions != 2 || metrics.Misses != 2 {
		t.Fatalf("two-module summary check did not use exactly its two specialized instances: %+v", metrics)
	}
}

// TestIntegrationGateCallerInvariance1_10_100 proves the read projection is
// the sole specialization dimension: arbitrary caller-only coordinates do
// not create instances, while a certified read creates a distinct exact one.
func TestIntegrationGateCallerInvariance1_10_100(t *testing.T) {
	for _, callers := range []int{1, 10, 100} {
		t.Run(fmt.Sprintf("%d-callers", callers), func(t *testing.T) {
			h := newIntegrationGateHarness(t)
			entries := make([]interproc.EntryBinding, callers)
			for i := range entries {
				entries[i] = gateEntry(t, "strict", "number", fmt.Sprintf("caller-%03d", i), fmt.Sprintf("site-%03d", i))
			}
			outcomes := make([]interproc.ClosedOutcome, callers)
			errs := make([]error, callers)
			start := make(chan struct{})
			var group sync.WaitGroup
			for i := range entries {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					<-start
					outcomes[i], errs[i] = (interproc.DirectCall{Table: h.table, Runner: h.runner()}).Resolve(context.Background(), h.moduleA, entries[i])
				}(i)
			}
			close(start)
			group.Wait()

			fresh, err := h.specializeModuleA(context.Background(), h.moduleA, entries[0])
			if err != nil {
				t.Fatal(err)
			}
			for i := range outcomes {
				if errs[i] != nil {
					t.Fatalf("caller %d: %v", i, errs[i])
				}
				if !bytes.Equal(outcomes[i].CanonicalBytes(), fresh.CanonicalBytes()) {
					t.Fatalf("caller %d received a result different from fresh specialization", i)
				}
			}
			metrics := h.table.Metrics()
			if metrics.Cells != 1 || metrics.Executions != 1 || metrics.Misses != 1 || metrics.Lookups != uint64(callers) {
				t.Fatalf("%d equal-projection callers produced the wrong cache shape: %+v", callers, metrics)
			}
			if h.runs.Load() != 1 {
				t.Fatalf("%d equal-projection callers executed body %d times", callers, h.runs.Load())
			}
		})
	}

	t.Run("distinct-certified-entries", func(t *testing.T) {
		h := newIntegrationGateHarness(t)
		strict := gateEntry(t, "strict", "number", "strict-caller", "strict-site")
		loose := gateEntry(t, "loose", "number", "loose-caller", "loose-site")
		strictOutcome, err := (interproc.DirectCall{Table: h.table, Runner: h.runner()}).Resolve(context.Background(), h.moduleA, strict)
		if err != nil {
			t.Fatal(err)
		}
		looseOutcome, err := (interproc.DirectCall{Table: h.table, Runner: h.runner()}).Resolve(context.Background(), h.moduleA, loose)
		if err != nil {
			t.Fatal(err)
		}
		strictFresh, err := h.specializeModuleA(context.Background(), h.moduleA, strict)
		if err != nil {
			t.Fatal(err)
		}
		looseFresh, err := h.specializeModuleA(context.Background(), h.moduleA, loose)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(strictOutcome.CanonicalBytes(), strictFresh.CanonicalBytes()) ||
			!bytes.Equal(looseOutcome.CanonicalBytes(), looseFresh.CanonicalBytes()) ||
			bytes.Equal(strictOutcome.CanonicalBytes(), looseOutcome.CanonicalBytes()) {
			t.Fatal("distinct read projections cross-contaminated their exact results")
		}
		metrics := h.table.Metrics()
		if metrics.Cells != 2 || metrics.Executions != 2 || metrics.Misses != 2 || h.runs.Load() != 2 {
			t.Fatalf("distinct certified entries did not produce exactly two instances: metrics=%+v runs=%d", metrics, h.runs.Load())
		}
	})
}

// TestIntegrationGateInvalidationRacesSourceChangeMidFlight holds an owner
// after it has installed Solving, changes the source snapshot concurrently,
// and proves the old result is never published.
func TestIntegrationGateInvalidationRacesSourceChangeMidFlight(t *testing.T) {
	snapshots := newGateSnapshots(map[string]interproc.ContentID{
		"registry": gateID("registry-v1"), "source": gateID("module-a-source-v1"), "codec": gateID("codec-v1"),
	})
	h := newIntegrationGateHarness(t)
	h.table = interproc.NewProjectedTableWithDependencyResolver(snapshots)
	entry := gateEntry(t, "strict", "number", "race-caller", "race-site")
	entered := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	var once sync.Once
	blockedRunner := func(ctx context.Context, artifact interproc.DemandedBodyArtifact, bound interproc.EntryBinding) (interproc.ClosedOutcome, []interproc.ReadObservation, error) {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
			return interproc.ClosedOutcome{}, nil, ctx.Err()
		}
		return h.runner()(ctx, artifact, bound)
	}
	go func() {
		_, err := h.table.Resolve(context.Background(), h.moduleA, entry, blockedRunner)
		result <- err
	}()
	<-entered

	snapshots.set("source", gateID("module-a-source-v2"))
	evicted, err := h.table.InvalidateStale(context.Background())
	if err != nil || evicted != 1 {
		t.Fatalf("mid-flight source invalidation = (%d, %v), want (1, nil)", evicted, err)
	}
	if metrics := h.table.Metrics(); metrics.Cells != 0 || metrics.Evictions != 1 {
		t.Fatalf("invalidation exposed a stale or partial cell: %+v", metrics)
	}
	close(release)
	if err := <-result; !errors.Is(err, interproc.ErrInstanceInvalidated) {
		t.Fatalf("old owner error = %v, want ErrInstanceInvalidated", err)
	}
	if metrics := h.table.Metrics(); metrics.Cells != 0 {
		t.Fatalf("stale instance survived owner completion: %+v", metrics)
	}

	v2 := gateArtifact(t, "module-a", gateID("module-a-source-v2"))
	freshRunner := func(ctx context.Context, artifact interproc.DemandedBodyArtifact, bound interproc.EntryBinding) (interproc.ClosedOutcome, []interproc.ReadObservation, error) {
		out, err := h.specializeModuleA(ctx, artifact, bound)
		return out, gateReadAudit(artifact), err
	}
	got, err := h.table.Resolve(context.Background(), v2, entry, freshRunner)
	if err != nil {
		t.Fatal(err)
	}
	want, err := h.specializeModuleA(context.Background(), v2, entry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.CanonicalBytes(), want.CanonicalBytes()) {
		t.Fatal("replacement artifact did not publish its complete v2 result")
	}
	if metrics := h.table.Metrics(); metrics.Cells != 1 || metrics.Executions != 2 || metrics.Evictions != 1 {
		t.Fatalf("post-race cache shape = %+v, want one committed v2 instance only", metrics)
	}
}

// TestIntegrationGateCrossModuleCodecTransport seals module A's cached
// result, treats its bytes as a module boundary, and applies the decoded
// closure in module B. Caller-only entry coordinates are deliberately loud so
// their absence from the sealed artifact is an executable ownership check.
func TestIntegrationGateCrossModuleCodecTransport(t *testing.T) {
	h := newIntegrationGateHarness(t)
	callerNoise := "caller-span:module-b.lua:17"
	site := "caller-anchor:require-A"
	entry := gateEntry(t, "strict", "number", callerNoise, site)
	sealedA, err := (interproc.DirectCall{Table: h.table, Runner: h.runner()}).Resolve(context.Background(), h.moduleA, entry)
	if err != nil {
		t.Fatal(err)
	}
	wire := sealedA.CanonicalBytes()
	if bytes.Contains(wire, []byte(callerNoise)) || bytes.Contains(wire, []byte(site)) {
		t.Fatal("sealed module-A outcome retained caller context")
	}

	inProcess, err := h.applyModuleB(context.Background(), h.moduleB, entry, sealedA)
	if err != nil {
		t.Fatal(err)
	}
	decodedA, err := summaryinstance.Decode(context.Background(), h.schema, summaryinstance.CanonicalArtifact{
		Bytes: wire, Schema: h.schema.ID(), Semantic: interproc.ContentIDFromCanonicalBytes(wire),
	})
	if err != nil {
		t.Fatal(err)
	}
	if decodedA.DemandedArtifactID != h.moduleA.ContentID() || gateFact(decodedA.Values, "module") == nil ||
		bytes.Contains(gateFact(decodedA.Values, "callee-result"), []byte(callerNoise)) {
		t.Fatalf("transported module-A closure is not caller-context-free: %#v", decodedA)
	}
	transported, err := h.sealedOutcome(context.Background(), h.moduleB, entry, []summaryinstance.Fact{
		{Key: "module", Value: []byte("B")},
		{Key: "caller-result", Value: gateFact(decodedA.Values, "callee-result")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transported.CanonicalBytes(), inProcess.CanonicalBytes()) {
		t.Fatalf("cross-module codec application differs from in-process application\ntransport: %x\nin-process: %x", transported.CanonicalBytes(), inProcess.CanonicalBytes())
	}
	if metrics := h.table.Metrics(); metrics.Cells != 1 || metrics.Executions != 1 {
		t.Fatalf("module-A transport check did not use one cached specialized instance: %+v", metrics)
	}
}

type integrationGateHarness struct {
	schema  summaryinstance.FormatSchema
	moduleA interproc.DemandedBodyArtifact
	moduleB interproc.DemandedBodyArtifact
	table   *interproc.ProjectedTable
	runs    atomic.Int32
}

func newIntegrationGateHarness(t *testing.T) *integrationGateHarness {
	t.Helper()
	schema, err := summaryinstance.NewFormatSchema(gateID("registry-v1"), gateID("domain-v1"))
	if err != nil {
		t.Fatal(err)
	}
	return &integrationGateHarness{
		schema:  schema,
		moduleA: gateArtifact(t, "module-a", gateID("module-a-source-v1")),
		moduleB: gateArtifact(t, "module-b", gateID("module-b-source-v1")),
		table:   interproc.NewProjectedTable(),
	}
}

func (h *integrationGateHarness) checkThroughSummary(ctx context.Context, entry interproc.EntryBinding) (interproc.ClosedOutcome, error) {
	return (interproc.DirectCall{Table: h.table, Runner: h.runner()}).Resolve(ctx, h.moduleB, entry)
}

func (h *integrationGateHarness) runner() interproc.DirectCallRunner {
	return func(ctx context.Context, artifact interproc.DemandedBodyArtifact, entry interproc.EntryBinding) (interproc.ClosedOutcome, []interproc.ReadObservation, error) {
		h.runs.Add(1)
		if artifact.ContentID() == h.moduleA.ContentID() {
			out, err := h.specializeModuleA(ctx, artifact, entry)
			return out, gateReadAudit(artifact), err
		}
		if artifact.ContentID() != h.moduleB.ContentID() {
			return interproc.ClosedOutcome{}, nil, fmt.Errorf("unexpected module artifact")
		}
		callee, err := (interproc.DirectCall{Table: h.table, Runner: h.runner()}).Resolve(ctx, h.moduleA, entry)
		if err != nil {
			return interproc.ClosedOutcome{}, nil, err
		}
		out, err := h.applyModuleB(ctx, artifact, entry, callee)
		return out, gateReadAudit(artifact), err
	}
}

func (h *integrationGateHarness) checkFromScratch(ctx context.Context, entry interproc.EntryBinding) (interproc.ClosedOutcome, error) {
	callee, err := h.specializeModuleA(ctx, h.moduleA, entry)
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	return h.applyModuleB(ctx, h.moduleB, entry, callee)
}

func (h *integrationGateHarness) specializeModuleA(ctx context.Context, artifact interproc.DemandedBodyArtifact, entry interproc.EntryBinding) (interproc.ClosedOutcome, error) {
	return h.sealedOutcome(ctx, artifact, entry, []summaryinstance.Fact{
		{Key: "module", Value: []byte("A")},
		{Key: "callee-result", Value: []byte(gateValue(entry, "mode") + ":" + gateValue(entry, "operand"))},
	})
}

func (h *integrationGateHarness) applyModuleB(ctx context.Context, artifact interproc.DemandedBodyArtifact, entry interproc.EntryBinding, callee interproc.ClosedOutcome) (interproc.ClosedOutcome, error) {
	decoded, err := h.unseal(callee)
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	return h.sealedOutcome(ctx, artifact, entry, []summaryinstance.Fact{
		{Key: "module", Value: []byte("B")},
		{Key: "caller-result", Value: gateFact(decoded.Values, "callee-result")},
	})
}

func (h *integrationGateHarness) sealedOutcome(ctx context.Context, artifact interproc.DemandedBodyArtifact, entry interproc.EntryBinding, values []summaryinstance.Fact) (interproc.ClosedOutcome, error) {
	key, err := interproc.NewInstanceKey(artifact, entry)
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	outcome := summaryinstance.PortableClosedOutcome{
		FormatSchemaID:          h.schema.ID(),
		DemandedArtifactID:      artifact.ContentID(),
		InstanceProjectionBytes: key.ProjectionBytes(),
		InstanceProjectionID:    key.ProjectionID(),
		Values:                  values,
		Outcomes:                []summaryinstance.Fact{{Key: "return", Value: []byte("normal")}},
		DependencyIDs:           gateDependencyIDs(artifact),
	}
	outcome.ResultDigest, err = summaryinstance.ResultDigestFor(outcome)
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	sealed, err := summaryinstance.Seal(ctx, h.schema, outcome)
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	return interproc.NewClosedOutcome(sealed.Bytes)
}

func (h *integrationGateHarness) unseal(outcome interproc.ClosedOutcome) (summaryinstance.PortableClosedOutcome, error) {
	bytes := outcome.CanonicalBytes()
	return summaryinstance.Decode(context.Background(), h.schema, summaryinstance.CanonicalArtifact{
		Bytes: bytes, Schema: h.schema.ID(), Semantic: interproc.ContentIDFromCanonicalBytes(bytes),
	})
}

func gateArtifact(t *testing.T, module string, source interproc.ContentID) interproc.DemandedBodyArtifact {
	t.Helper()
	var bodyID equation.BodyID
	copy(bodyID[:], []byte(module))
	entry := equation.EntryParameter{Body: bodyID, Name: "entry"}
	contract := equation.ContentID(gateID(module + "-contract"))
	plain := equation.Artifact{Equations: []equation.Equation{{
		Target: equation.Coordinate{Body: bodyID, Name: "result"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: "entry", ContractID: contract}, KernelID: "gate",
		Operands: []equation.Operand{{Role: equation.MustOperandRole("entry"), Term: equation.EntryTerm(entry)}},
	}}}
	plan, err := solve.FreezeWTOPlan([]equation.CellID{"result"}, []solve.WTOElement[equation.CellID]{{Vertex: "result"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := equation.NewCyclicArtifact(plain, map[equation.Coordinate]equation.CellID{plain.Equations[0].Target: "result"}, plan, nil,
		[]equation.OutputSelector{{ID: "gate-return", Cells: []equation.CellID{"result"}}}, []equation.CellID{"result"})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := interproc.NewParameterSchema("gate-entry", []interproc.EntrySelector{"mode", "operand", "caller-noise", "site"})
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := interproc.NewReadProjectionCertificate("gate-return", interproc.ReadCertificateInputs{Semantic: []interproc.EntrySelector{"mode", "operand"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := interproc.NewDependencyManifest([]interproc.Dependency{{Kind: "registry", ID: gateID("registry-v1")}, {Kind: "source", ID: source}, {Kind: "codec", ID: gateID("codec-v1")}})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := interproc.NewDemandedBodyArtifact(body, schema, "gate-return", certificate, gateID("solver-v1"), manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func gateEntry(t *testing.T, mode, operand, noise, site string) interproc.EntryBinding {
	t.Helper()
	entry, err := interproc.NewEntryBinding([]interproc.EntryValue{
		{Selector: "mode", Encoding: []byte(mode)}, {Selector: "operand", Encoding: []byte(operand)},
		{Selector: "caller-noise", Encoding: []byte(noise)}, {Selector: "site", Encoding: []byte(site)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func gateReadAudit(artifact interproc.DemandedBodyArtifact) []interproc.ReadObservation {
	reads := artifact.ReadCertificate().Reads()
	out := make([]interproc.ReadObservation, len(reads))
	for i, read := range reads {
		out[i] = interproc.ReadObservation{Role: read.Role, Selector: read.Selector}
	}
	return out
}

func gateValue(entry interproc.EntryBinding, selector interproc.EntrySelector) string {
	for _, value := range entry.Values() {
		if value.Selector == selector {
			return string(value.Encoding)
		}
	}
	return ""
}

func gateFact(facts []summaryinstance.Fact, key string) []byte {
	for _, fact := range facts {
		if fact.Key == key {
			return append([]byte(nil), fact.Value...)
		}
	}
	return nil
}

func gateDependencyIDs(artifact interproc.DemandedBodyArtifact) []interproc.ContentID {
	dependencies := artifact.Dependencies().Dependencies()
	ids := make([]interproc.ContentID, len(dependencies))
	for i, dependency := range dependencies {
		ids[i] = dependency.ID
	}
	return ids
}

func gateID(text string) interproc.ContentID {
	return interproc.ContentIDFromCanonicalBytes([]byte(text))
}

type gateSnapshots struct {
	mu  sync.Mutex
	ids map[string]interproc.ContentID
}

func newGateSnapshots(ids map[string]interproc.ContentID) *gateSnapshots {
	copy := make(map[string]interproc.ContentID, len(ids))
	for kind, id := range ids {
		copy[kind] = id
	}
	return &gateSnapshots{ids: copy}
}

func (s *gateSnapshots) ResolveContentID(_ context.Context, dependency interproc.Dependency) (interproc.ContentID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.ids[dependency.Kind]
	if !ok {
		return interproc.ContentID{}, fmt.Errorf("missing dependency %q", dependency.Kind)
	}
	return id, nil
}

func (s *gateSnapshots) set(kind string, id interproc.ContentID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ids[kind] = id
}

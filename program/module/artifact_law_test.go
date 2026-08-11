package module

import (
	"bytes"
	"runtime"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	moduleArtifactTestDomain   = "program/module-test-artifact"
	moduleArtifactTestVersion  = 1
	moduleArtifactTestRoot     = 1
	moduleArtifactTestSentinel = 99
)

func TestArtifactSectionRoundTripRebuildsAuthoredContentID(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	encoded := encodeModuleArtifactTestStream(t, component, false)

	reader := newModuleArtifactTestReader(t, encoded)
	if got, err := reader.Record(); err != nil || got != moduleArtifactTestRoot {
		t.Fatalf("artifact root record = %d/%v, want %d", got, err, moduleArtifactTestRoot)
	}
	input, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatalf("ReadArtifactSection: %v", err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("artifact stream suffix: %v", err)
	}
	if len(input.Imports) != component.View().Count() {
		t.Fatalf("decoded import count = %d, want %d", len(input.Imports), component.View().Count())
	}
	for index, row := range input.Imports {
		want, ok := component.View().ImportAt(index)
		if !ok || row.Term != want.Term || row.Call != want.Call || row.Alias != want.Alias || row.Request != want.Request {
			t.Fatalf("decoded Import[%d] = %#v, want authored %#v", index, row, want)
		}
		if row.Key != 0 {
			t.Fatalf("decoded Import[%d] retained derived Key: %#v", index, row)
		}
	}

	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build(decoded): %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("decoded Finalizer: %v", err)
	}
	rebuilt, err := finalizer.Commit(CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	if err != nil {
		t.Fatalf("decoded Commit: %v", err)
	}
	if got, want := rebuilt.Cold().ContentID(), component.Cold().ContentID(); got != want {
		t.Fatalf("rebuilt ContentID = %x, want %x", got, want)
	}
}

func TestArtifactSectionMatchesCanonicalAuthoredRowsAndExpires(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	view := component.View()

	var artifactBytes, authoredBytes bytes.Buffer
	var artifactWriter, authoredWriter canonical.Writer
	if err := artifactWriter.Reset(&artifactBytes, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&artifactWriter, view); err != nil {
		t.Fatalf("WriteArtifactSection: %v", err)
	}
	if err := artifactWriter.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := authoredWriter.Reset(&authoredBytes, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writeAuthoredRows(&authoredWriter, component.imports); err != nil {
		t.Fatalf("writeAuthoredRows: %v", err)
	}
	if err := authoredWriter.Finish(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(artifactBytes.Bytes(), authoredBytes.Bytes()) {
		t.Fatal("artifact payload diverged from canonical authored-row encoding")
	}

	draft, err := Build(authoredInput())
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	expired := finalizer.View()
	if !finalizer.Abort() {
		t.Fatal("Abort rejected active finalizer")
	}
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(moduleArtifactTestRoot); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, expired); err == nil {
		t.Fatal("WriteArtifactSection accepted an expired View")
	}
	if err := writer.Record(moduleArtifactTestSentinel); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	reader := newModuleArtifactTestReader(t, data.Bytes())
	if got, err := reader.Record(); err != nil || got != moduleArtifactTestRoot {
		t.Fatalf("root record after expired write = %d/%v", got, err)
	}
	if got, err := reader.Record(); err != nil || got != moduleArtifactTestSentinel {
		t.Fatalf("sentinel record after expired write = %d/%v", got, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactSectionHoldsAuthoredRowsAcrossConcurrentTerminalTransition(t *testing.T) {
	for _, test := range []struct {
		name   string
		commit bool
	}{
		{name: "Abort"},
		{name: "Commit", commit: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			draft, err := Build(authoredInput())
			if err != nil {
				t.Fatal(err)
			}
			finalizer, err := draft.Finalizer()
			if err != nil {
				t.Fatal(err)
			}
			view := finalizer.View()

			sink := newBlockingModuleArtifactWriter()
			var writer canonical.Writer
			if err := writer.Reset(sink, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
				t.Fatal(err)
			}
			sink.armed = true
			writeDone := make(chan error, 1)
			go func() {
				err := WriteArtifactSection(&writer, view)
				if err == nil {
					err = writer.Finish()
				}
				writeDone <- err
			}()
			<-sink.entered

			// The first payload write is blocked. The active owner lock must
			// remain held until every authored row has been emitted.
			if finalizer.state.mu.TryLock() {
				finalizer.state.mu.Unlock()
				close(sink.release)
				<-writeDone
				t.Fatal("artifact writer released the owner lock during row emission")
			}

			transitionStarted := make(chan struct{})
			transitionDone := make(chan bool, 1)
			go func() {
				close(transitionStarted)
				if test.commit {
					component, err := finalizer.Commit(CommitInput{
						Resolutions: authoredResolutions(7, 8),
						Entry:       emptyEntry(),
					})
					transitionDone <- err == nil && component != nil
					return
				}
				transitionDone <- finalizer.Abort()
			}()
			<-transitionStarted
			runtime.Gosched()
			select {
			case <-transitionDone:
				close(sink.release)
				<-writeDone
				t.Fatal("terminal transition interleaved with authored row emission")
			default:
			}

			close(sink.release)
			if err := <-writeDone; err != nil {
				t.Fatalf("WriteArtifactSection: %v", err)
			}
			if ok := <-transitionDone; !ok {
				t.Fatal("terminal transition failed after complete artifact emission")
			}

			reader := newModuleArtifactTestReader(t, sink.Bytes())
			input, err := ReadArtifactSection(reader)
			if err != nil {
				t.Fatalf("ReadArtifactSection after concurrent transition: %v", err)
			}
			if len(input.Imports) != 2 {
				t.Fatalf("artifact retained %d imports, want complete denominator 2", len(input.Imports))
			}
			if err := reader.Finish(); err != nil {
				t.Fatalf("artifact suffix after concurrent transition: %v", err)
			}
		})
	}
}

type blockingModuleArtifactWriter struct {
	bytes.Buffer
	armed   bool
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func newBlockingModuleArtifactWriter() *blockingModuleArtifactWriter {
	return &blockingModuleArtifactWriter{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (writer *blockingModuleArtifactWriter) Write(value []byte) (int, error) {
	if writer.armed {
		writer.once.Do(func() {
			close(writer.entered)
			<-writer.release
		})
	}
	return writer.Buffer.Write(value)
}

func TestArtifactSectionExcludesCommittedDerivedState(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	before := encodeModuleArtifactTestStream(t, component, false)

	// These fields are private and immutable by API contract. Mutating only
	// derived state in this package test proves the payload writer does not
	// inspect it; authored Request remains part of the payload.
	component.imports[0].Key = 11
	component.imports[1].Key = 12
	component.entry.returnTerms = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyReturn, 1)}
	component.entry.returnIndex = []uint32{0, 1}
	component.entry.rootRanges = []EntryRange{{}, {Start: 0, End: 1}}

	after := encodeModuleArtifactTestStream(t, component, false)
	if !bytes.Equal(before, after) {
		t.Fatal("derived Key/Entry mutation changed Module artifact payload")
	}
}

func TestArtifactSectionLeavesEnclosingStreamOpen(t *testing.T) {
	component := buildCommitted(t, CommitInput{
		Resolutions: authoredResolutions(7, 8),
		Entry:       emptyEntry(),
	})
	encoded := encodeModuleArtifactTestStream(t, component, true)
	reader := newModuleArtifactTestReader(t, encoded)
	if got, err := reader.Record(); err != nil || got != moduleArtifactTestRoot {
		t.Fatalf("artifact root record = %d/%v, want %d", got, err, moduleArtifactTestRoot)
	}
	if _, err := ReadArtifactSection(reader); err != nil {
		t.Fatalf("ReadArtifactSection: %v", err)
	}
	if got, err := reader.Record(); err != nil || got != moduleArtifactTestSentinel {
		t.Fatalf("sentinel record = %d/%v, want %d", got, err, moduleArtifactTestSentinel)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("artifact stream suffix: %v", err)
	}
}

func TestArtifactSectionRejectsMalformedDenseRows(t *testing.T) {
	validCall := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	validAlias := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	for _, test := range []struct {
		name    string
		term    uint64
		call    uint64
		alias   uint64
		request uint64
	}{
		{name: "import ordinal", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 2)), call: uint64(validCall), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
		{name: "call family", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)), call: uint64(keyspace.MakeTerm(keyspace.FamilyFunction, 1)), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
		{name: "call ordinal", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)), call: uint64(keyspace.FamilyCall), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
		{name: "alias family", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)), call: uint64(validCall), alias: uint64(keyspace.MakeTerm(keyspace.FamilyFunction, 1)), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
		{name: "alias ordinal", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)), call: uint64(validCall), alias: uint64(keyspace.FamilyCell), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
		{name: "request family", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)), call: uint64(validCall), alias: uint64(validAlias), request: uint64(keyspace.MakeTerm(keyspace.FamilyFunction, 1))},
		{name: "request ordinal", term: uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1)), call: uint64(validCall), alias: uint64(validAlias), request: uint64(keyspace.FamilyString)},
		{name: "term family", term: uint64(keyspace.FamilyCount), call: uint64(validCall), alias: uint64(validAlias), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
		{name: "term wider than uint32", term: uint64(^uint32(0)) + 1, call: uint64(validCall), alias: uint64(validAlias), request: uint64(keyspace.MakeTerm(keyspace.FamilyString, 1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := readModuleArtifactPayload(t, func(writer *canonical.Writer) {
				writeModuleArtifactRows(t, writer, 1, test.term, test.call, test.alias, test.request)
			})
			if err == nil {
				t.Fatalf("ReadArtifactSection accepted malformed row: %#v", input)
			}
			if len(input.Imports) != 0 {
				t.Fatalf("malformed decode returned partial imports: %#v", input.Imports)
			}
		})
	}
}

func TestArtifactSectionRejectsHostileCountsAndTruncation(t *testing.T) {
	countTooLarge := uint64(keyspace.MaxTermOrdinal) + 1
	maxInt := uint64(^uint(0) >> 1)
	if maxInt < uint64(keyspace.MaxTermOrdinal) {
		countTooLarge = maxInt + 1
	}
	for _, test := range []struct {
		name  string
		count uint64
		write func(*canonical.Writer)
	}{
		{name: "count above MaxTermOrdinal/int", count: countTooLarge},
		{name: "count above remaining", count: 1},
		{
			name:  "truncated row",
			count: 1,
			write: func(writer *canonical.Writer) {
				if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyImport, 1))); err != nil {
					panic(err)
				}
				if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyCall, 1))); err != nil {
					panic(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := readModuleArtifactPayload(t, func(writer *canonical.Writer) {
				if err := writer.Count(test.count); err != nil {
					t.Fatalf("Count: %v", err)
				}
				if test.write != nil {
					test.write(writer)
				}
			})
			if err == nil {
				t.Fatalf("ReadArtifactSection accepted hostile count/truncation: %#v", input)
			}
			if len(input.Imports) != 0 {
				t.Fatalf("hostile decode returned partial imports: %#v", input.Imports)
			}
		})
	}
}

func TestArtifactSectionHostileCountDoesNotReserveRows(t *testing.T) {
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Count(uint64(keyspace.MaxTermOrdinal) + 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte(nil), data.Bytes()...)
	allocations := testing.AllocsPerRun(100, func() {
		reader, err := canonical.NewReader(payload, len(payload))
		if err != nil {
			panic(err)
		}
		if err := reader.Header(moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
			panic(err)
		}
		input, err := ReadArtifactSection(reader)
		if err == nil || len(input.Imports) != 0 {
			panic("hostile count was accepted or allocated rows")
		}
	})
	if allocations > 32 {
		t.Fatalf("hostile count allocations = %f, want bounded preflight", allocations)
	}
}

func TestArtifactSectionMalformedRowsProbeBeforeAllocation(t *testing.T) {
	const rowCount = 300_000
	for _, test := range []struct {
		name   string
		badRow int
	}{
		{name: "malformed first", badRow: 0},
		{name: "malformed last", badRow: rowCount - 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := hostileModuleArtifactRows(t, rowCount, test.badRow)
			const runs = 2
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			failed := false
			for range runs {
				reader, err := canonical.NewReader(data, len(data))
				if err != nil {
					t.Fatal(err)
				}
				if err := reader.Header(moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
					t.Fatal(err)
				}
				input, err := ReadArtifactSection(reader)
				if err == nil || len(input.Imports) != 0 {
					failed = true
				}
			}
			runtime.ReadMemStats(&after)
			if failed {
				t.Fatal("malformed dense payload was accepted or returned rows")
			}
			allocated := after.TotalAlloc - before.TotalAlloc
			if allocated > uint64(runs)*(1<<20) {
				t.Fatalf("malformed %s payload allocated %d bytes; want allocation-free semantic probe", test.name, allocated)
			}
		})
	}
}

func TestArtifactSectionRejectsNoncanonicalUint(t *testing.T) {
	// Count tag, two-byte overlong payload for the value one. Reader rejects
	// the noncanonical uvarint before the Module decoder can allocate rows.
	data := []byte{4, 2, 0x81, 0x00}
	reader, err := canonical.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	input, err := ReadArtifactSection(reader)
	if err == nil {
		t.Fatalf("ReadArtifactSection accepted overlong Count: %#v", input)
	}
	if len(input.Imports) != 0 {
		t.Fatalf("noncanonical decode returned partial imports: %#v", input.Imports)
	}
}

func encodeModuleArtifactTestStream(t *testing.T, component *Component, sentinel bool) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(moduleArtifactTestRoot); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, component.View()); err != nil {
		t.Fatal(err)
	}
	if sentinel {
		if err := writer.Record(moduleArtifactTestSentinel); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func newModuleArtifactTestReader(t *testing.T, data []byte) *canonical.Reader {
	t.Helper()
	reader, err := canonical.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

func readModuleArtifactPayload(t *testing.T, write func(*canonical.Writer)) (Input, error) {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	write(&writer)
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	reader := newModuleArtifactTestReader(t, data.Bytes())
	return ReadArtifactSection(reader)
}

func writeModuleArtifactRows(t *testing.T, writer *canonical.Writer, count uint64, rows ...uint64) {
	t.Helper()
	if err := writer.Count(count); err != nil {
		t.Fatal(err)
	}
	for _, term := range rows {
		if err := writer.Uint(uint64(term)); err != nil {
			t.Fatal(err)
		}
	}
}

func hostileModuleArtifactRows(t *testing.T, count, badRow int) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, moduleArtifactTestDomain, moduleArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Count(uint64(count)); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < count; index++ {
		term := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
		call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
		alias := keyspace.Term(0)
		if index == badRow {
			if index == 0 {
				term = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
			} else {
				alias = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
			}
		}
		request := keyspace.MakeTerm(keyspace.FamilyString, 1)
		for _, value := range [...]keyspace.Term{term, call, alias, request} {
			if err := writer.Uint(uint64(value)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

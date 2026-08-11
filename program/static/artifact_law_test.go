package static

import (
	"bytes"
	"encoding/binary"
	"errors"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	staticArtifactTestDomain   = "program/static-artifact-test"
	staticArtifactTestVersion  = 1
	staticArtifactTestRoot     = 1
	staticArtifactTestSentinel = 99
)

func TestArtifactSectionRoundTripRebuildsAuthoredContentID(t *testing.T) {
	for _, test := range []struct {
		name  string
		input func(*testing.T) Input
	}{
		{name: "empty", input: func(*testing.T) Input { return Input{} }},
		{name: "all types", input: staticTypeDenominatorInput},
		{name: "declarations", input: declarationFixture},
		{name: "declared types", input: declaredTypeFixture},
		{name: "signatures", input: signatureFixture},
		{name: "contracts", input: contractsFixture},
		{name: "operators", input: func(*testing.T) Input { return operatorFixture() }},
		{name: "operands", input: operandsFixture},
		{name: "publications", input: publicationFixture},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalInput := test.input(t)
			component := staticContentComponent(t, originalInput)
			encoded := encodeStaticArtifactComponent(t, component, true)

			reader := newStaticArtifactReader(t, encoded)
			if got, err := reader.Record(); err != nil || got != staticArtifactTestRoot {
				t.Fatalf("artifact root record = %d/%v, want %d", got, err, staticArtifactTestRoot)
			}
			decoded, err := ReadArtifactSection(reader)
			if err != nil {
				t.Fatalf("ReadArtifactSection: %v", err)
			}
			if decoded.Counts != ([keyspace.FamilyCount]uint32{}) {
				t.Fatalf("decoded Counts = %#v, want zero root-injection input", decoded.Counts)
			}
			if got, err := reader.Record(); err != nil || got != staticArtifactTestSentinel {
				t.Fatalf("artifact suffix = %d/%v, want sentinel %d", got, err, staticArtifactTestSentinel)
			}
			if err := reader.Finish(); err != nil {
				t.Fatalf("artifact Finish: %v", err)
			}

			decoded.Counts = originalInput.Counts
			rebuilt, err := Build(decoded)
			if err != nil {
				t.Fatalf("Build(decoded): %v", err)
			}
			if got, want := rebuilt.state.component.contentID, component.contentID; got != want {
				t.Fatalf("rebuilt ContentID = %x, want %x", got, want)
			}
		})
	}
}

func TestArtifactSectionCanonicalizesSparseClaimOrder(t *testing.T) {
	first := operandsFixture(t)
	claimOne := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	claimTwo := keyspace.MakeTerm(keyspace.FamilyValueClaim, 2)
	primitiveOne := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	primitiveTwo := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)
	first.Operands.Claim = []ClaimTarget{
		{Claim: claimTwo, Target: primitiveTwo},
		{Claim: claimOne, Target: primitiveOne},
	}
	second := first
	second.Operands.Claim = []ClaimTarget{
		{Claim: claimOne, Target: primitiveOne},
		{Claim: claimTwo, Target: primitiveTwo},
	}
	one := encodeStaticArtifactComponent(t, staticContentComponent(t, first), false)
	two := encodeStaticArtifactComponent(t, staticContentComponent(t, second), false)
	if !bytes.Equal(one, two) {
		t.Fatal("permuting sparse Claim input changed canonical artifact payload")
	}

	reader := newStaticArtifactReader(t, one)
	if _, err := reader.Record(); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadArtifactSection(reader)
	if err != nil {
		t.Fatalf("ReadArtifactSection: %v", err)
	}
	if len(decoded.Operands.Claim) != 2 || decoded.Operands.Claim[0].Claim != claimOne || decoded.Operands.Claim[1].Claim != claimTwo {
		t.Fatalf("decoded sparse claims = %#v, want ascending Claim ordinal", decoded.Operands.Claim)
	}
}

func TestArtifactSectionExcludesCommittedDerivedState(t *testing.T) {
	component := staticContentComponent(t, receiptFixture(t))
	before := encodeStaticArtifactComponent(t, component, false)

	component.staticTypes.prefix[1]++
	component.operands.claimTargets[0] = 0
	component.operands.annotationTargets[0] = 0
	component.operands.annotationRanges[0] = poolRange{}
	component.operands.annotationTerms[0] = 0

	after := encodeStaticArtifactComponent(t, component, false)
	if !bytes.Equal(before, after) {
		t.Fatal("derived Static indexes changed artifact payload")
	}
}

func TestArtifactSectionConstructionViewMatchesPublishedViewAndExpires(t *testing.T) {
	draft, err := Build(receiptFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	constructionView := finalizer.View()
	live := encodeStaticArtifactView(t, constructionView, false)
	component, err := finalizer.Commit(validReceiptForFixture())
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	published := encodeStaticArtifactComponent(t, component, false)
	if !bytes.Equal(live, published) {
		t.Fatal("construction View artifact bytes differ from published Component View")
	}

	abortDraft, err := Build(receiptFixture(t))
	if err != nil {
		t.Fatalf("Build(abort) error = %v", err)
	}
	abortFinalizer, err := abortDraft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer(abort) error = %v", err)
	}
	abortedView := abortFinalizer.View()
	if err := abortFinalizer.Abort(); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(staticArtifactTestRoot); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, abortedView); err == nil {
		t.Fatal("expired construction View artifact write succeeded")
	}
}

func TestArtifactSectionLeavesEnclosingStreamOpen(t *testing.T) {
	component := staticContentComponent(t, Input{})
	data := encodeStaticArtifactComponent(t, component, true)
	reader := newStaticArtifactReader(t, data)
	if got, err := reader.Record(); err != nil || got != staticArtifactTestRoot {
		t.Fatalf("artifact root = %d/%v", got, err)
	}
	if _, err := ReadArtifactSection(reader); err != nil {
		t.Fatalf("ReadArtifactSection: %v", err)
	}
	if got, err := reader.Record(); err != nil || got != staticArtifactTestSentinel {
		t.Fatalf("sentinel = %d/%v", got, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestArtifactSectionRejectsEveryTruncationOfNonemptyPayload(t *testing.T) {
	data := encodeStaticArtifactComponent(t, staticContentComponent(t, declarationFixture(t)), false)
	for cut := 0; cut < len(data); cut++ {
		truncated := data[:cut]
		reader, err := canonical.NewReader(truncated, len(truncated))
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.Header(staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
			continue
		}
		if _, err := reader.Record(); err != nil {
			continue
		}
		if input, err := ReadArtifactSection(reader); err == nil {
			t.Fatalf("truncation at byte %d accepted payload: %#v", cut, input)
		}
	}
}

func TestArtifactSectionRejectsNoncanonicalCountFrame(t *testing.T) {
	valid := encodeStaticArtifactComponent(t, staticContentComponent(t, declarationFixture(t)), false)
	mutated, ok := overlongFirstStaticCount(valid)
	if !ok {
		t.Fatal("could not locate first canonical Count frame")
	}
	reader, err := canonical.NewReader(mutated, len(mutated))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Record(); err != nil {
		t.Fatal(err)
	}
	if input, err := ReadArtifactSection(reader); err == nil {
		t.Fatalf("ReadArtifactSection accepted overlong Count frame: %#v", input)
	} else if !errors.Is(err, canonical.ErrMalformed) {
		t.Fatalf("overlong Count frame error = %v, want canonical malformed", err)
	}
}

func TestArtifactSectionRejectsNestedEnumTermAndRangeRows(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "union arity range", data: encodeStaticHostileUnionArity(t)},
		{name: "generic base family", data: encodeStaticHostileGenericBase(t)},
		{name: "record field family", data: encodeStaticHostileRecordField(t)},
		{name: "reference root family", data: encodeStaticHostileReferenceRoot(t)},
		{name: "interface member enum", data: encodeStaticHostileInterfaceMember(t)},
		{name: "signature scope family", data: encodeStaticHostileSignatureScope(t)},
		{name: "sparse claim order", data: encodeStaticHostileClaimOrder(t)},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := newStaticArtifactReader(t, test.data)
			if _, err := reader.Record(); err != nil {
				t.Fatal(err)
			}
			if input, err := ReadArtifactSection(reader); err == nil {
				t.Fatalf("ReadArtifactSection accepted hostile nested row: %#v", input)
			}
		})
	}
}

func TestArtifactSectionRejectsHostileCountsAndMalformedFirstLastWithoutRows(t *testing.T) {
	countTooLarge := uint64(keyspace.MaxTermOrdinal) + 1
	if maxInt := uint64(^uint(0) >> 1); maxInt < countTooLarge {
		countTooLarge = maxInt + 1
	}
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "count above MaxTermOrdinal or int", data: encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
			if err := writer.Count(countTooLarge); err != nil {
				t.Fatal(err)
			}
		})},
		{name: "malformed first row", data: encodeStaticDensePrimitiveSection(t, 100_000, 0)},
		{name: "malformed last row", data: encodeStaticDensePrimitiveSection(t, 100_000, 99_999)},
		{name: "malformed final publication", data: encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
			writeStaticEmptyTypes(t, writer)
			writeStaticEmptySuffixUntil(t, writer, staticArtifactRecordReferences, staticArtifactRecordOperands)
			if err := writer.Record(staticArtifactRecordPublications); err != nil {
				t.Fatal(err)
			}
			if err := writer.Count(1); err != nil {
				t.Fatal(err)
			}
			for _, value := range []uint64{uint64(keyspace.MakeTerm(keyspace.FamilyBody, 1)), 0, uint64(keyspace.MakeTerm(keyspace.FamilyTypeRef, 1))} {
				if err := writer.Uint(value); err != nil {
					t.Fatal(err)
				}
			}
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := newStaticArtifactReader(t, test.data)
			if got, err := reader.Record(); err != nil || got != staticArtifactTestRoot {
				t.Fatalf("root = %d/%v", got, err)
			}
			input, err := ReadArtifactSection(reader)
			if err == nil {
				t.Fatalf("ReadArtifactSection accepted hostile payload: %#v", input)
			}
			if !staticArtifactInputEmpty(input) {
				t.Fatalf("hostile decode returned partial input: %#v", input)
			}
		})
	}
}

func TestArtifactSectionMalformedRowsProbeBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "first", data: encodeStaticDensePrimitiveSection(t, 100_000, 0)},
		{name: "last", data: encodeStaticDensePrimitiveSection(t, 100_000, 99_999)},
		{name: "final publication", data: encodeStaticDensePrimitiveFinalPublicationSection(t, 100_000)},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime.GC()
			const runs = 2
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			for range runs {
				reader, err := canonical.NewReader(test.data, len(test.data))
				if err != nil {
					panic(err)
				}
				if err := reader.Header(staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
					panic(err)
				}
				if _, err := reader.Record(); err != nil {
					panic(err)
				}
				input, err := ReadArtifactSection(reader)
				if err == nil || !staticArtifactInputEmpty(input) {
					panic("malformed payload was accepted or returned partial input")
				}
			}
			runtime.ReadMemStats(&after)
			allocated := after.TotalAlloc - before.TotalAlloc
			if allocated > uint64(runs)*(1<<20) {
				t.Fatalf("malformed %s payload allocated %d bytes; want allocation-free semantic probe", test.name, allocated)
			}
		})
	}
}

func encodeStaticArtifactComponent(t *testing.T, component *Component, sentinel bool) []byte {
	t.Helper()
	return encodeStaticArtifactView(t, component.View(), sentinel)
}

func encodeStaticArtifactView(t *testing.T, view View, sentinel bool) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(staticArtifactTestRoot); err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifactSection(&writer, view); err != nil {
		t.Fatal(err)
	}
	if sentinel {
		if err := writer.Record(staticArtifactTestSentinel); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func encodeStaticMalformedSection(t *testing.T, write func(*canonical.Writer)) []byte {
	t.Helper()
	var data bytes.Buffer
	var writer canonical.Writer
	if err := writer.Reset(&data, staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(staticArtifactTestRoot); err != nil {
		t.Fatal(err)
	}
	write(&writer)
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), data.Bytes()...)
}

func encodeStaticDensePrimitiveSection(t *testing.T, count, badRow int) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(uint64(count)); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < count; index++ {
			kind := uint64(PrimitiveAny)
			if index == badRow {
				kind = 0
			}
			if err := writer.Uint(kind); err != nil {
				t.Fatal(err)
			}
		}
		for index := 0; index < 9; index++ {
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordReferences)
	})
}

func encodeStaticDensePrimitiveFinalPublicationSection(t *testing.T, count int) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(uint64(count)); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < count; index++ {
			if err := writer.Uint(uint64(PrimitiveAny)); err != nil {
				t.Fatal(err)
			}
		}
		for index := 0; index < 9; index++ {
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		writeStaticEmptySuffixUntil(t, writer, staticArtifactRecordReferences, staticArtifactRecordOperands)
		if err := writer.Record(staticArtifactRecordPublications); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		for _, value := range []uint64{uint64(keyspace.MakeTerm(keyspace.FamilyBody, 1)), 0, uint64(keyspace.MakeTerm(keyspace.FamilyTypeRef, 1))} {
			if err := writer.Uint(value); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func writeStaticEmptyPrefix(t *testing.T, writer *canonical.Writer) {
	t.Helper()
	writeStaticEmptyTypes(t, writer)
	writeStaticEmptySuffix(t, writer, staticArtifactRecordReferences)
}

func writeStaticEmptyTypes(t *testing.T, writer *canonical.Writer) {
	t.Helper()
	if err := writer.Record(staticArtifactRecordTypes); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10; index++ {
		if err := writer.Count(0); err != nil {
			t.Fatal(err)
		}
	}
}

func writeStaticEmptySuffix(t *testing.T, writer *canonical.Writer, first uint64) {
	writeStaticEmptySuffixUntil(t, writer, first, staticArtifactRecordPublications)
}

func writeStaticEmptySuffixUntil(t *testing.T, writer *canonical.Writer, first, last uint64) {
	t.Helper()
	for record := first; record <= last; record++ {
		if err := writer.Record(record); err != nil {
			t.Fatal(err)
		}
		counts := 1
		switch record {
		case staticArtifactRecordDeclarations:
			counts = 4
		case staticArtifactRecordSignatures, staticArtifactRecordContracts:
			counts = 2
		case staticArtifactRecordOperators:
			counts = 4
		case staticArtifactRecordOperands:
			counts = 3
		}
		for index := 0; index < counts; index++ {
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func newStaticArtifactReader(t *testing.T, data []byte) *canonical.Reader {
	t.Helper()
	reader, err := canonical.NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header(staticArtifactTestDomain, staticArtifactTestVersion); err != nil {
		t.Fatal(err)
	}
	return reader
}

func overlongFirstStaticCount(data []byte) ([]byte, bool) {
	offset := 0
	for range 4 { // domain, version, enclosing root, and Static Types record
		next, ok := nextStaticEvent(data, offset)
		if !ok {
			return nil, false
		}
		offset = next
	}
	if offset+1 >= len(data) {
		return nil, false
	}
	length, lengthBytes := binary.Uvarint(data[offset+1:])
	if lengthBytes != 1 || length != 1 {
		return nil, false
	}
	payload := offset + 1 + lengthBytes
	if payload >= len(data) {
		return nil, false
	}
	mutated := make([]byte, 0, len(data)+1)
	mutated = append(mutated, data[:offset+1]...)
	mutated = append(mutated, 2, 0x81, 0x00)
	mutated = append(mutated, data[payload+1:]...)
	return mutated, true
}

func nextStaticEvent(data []byte, offset int) (int, bool) {
	if offset < 0 || offset+1 >= len(data) {
		return 0, false
	}
	length, lengthBytes := binary.Uvarint(data[offset+1:])
	if lengthBytes <= 0 {
		return 0, false
	}
	payload := offset + 1 + lengthBytes
	if length > uint64(len(data)-payload) {
		return 0, false
	}
	return payload + int(length), true
}

func encodeStaticHostileUnionArity(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
		for index, count := range []uint64{0, 0, 0, 1, 0, 0, 0, 0, 0, 0} {
			if err := writer.Count(count); err != nil {
				t.Fatal(err)
			}
			if index == 3 {
				if err := writer.Count(1); err != nil {
					t.Fatal(err)
				}
				if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))); err != nil {
					t.Fatal(err)
				}
			}
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordReferences)
	})
}

func encodeStaticHostileGenericBase(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(uint64(PrimitiveAny)); err != nil {
			t.Fatal(err)
		}
		for index, count := range []uint64{0, 0, 0, 0, 1} {
			if err := writer.Count(count); err != nil {
				t.Fatal(err)
			}
			if index == 4 {
				if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))); err != nil {
					t.Fatal(err)
				}
				if err := writer.Count(1); err != nil {
					t.Fatal(err)
				}
				if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))); err != nil {
					t.Fatal(err)
				}
			}
		}
		for range 4 {
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordReferences)
	})
}

func encodeStaticHostileRecordField(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(uint64(PrimitiveAny)); err != nil {
			t.Fatal(err)
		}
		for range 6 { // literal, optional, union, intersection, generic, array
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Count(0); err != nil { // map
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil { // record
			t.Fatal(err)
		}
		if err := writer.Bool(false); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil { // fields
			t.Fatal(err)
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordReferences)
	})
}

func encodeStaticHostileReferenceRoot(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		writeStaticEmptyTypes(t, writer)
		if err := writer.Record(staticArtifactRecordReferences); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		for _, value := range []uint64{
			uint64(TypeRefUnresolved), 0,
			uint64(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)),
			2, 1, 2, 0,
		} {
			if err := writer.Uint(value); err != nil {
				t.Fatal(err)
			}
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordDeclarations)
	})
}

func encodeStaticHostileInterfaceMember(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		writeStaticEmptyTypes(t, writer)
		if err := writer.Record(staticArtifactRecordReferences); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil {
			t.Fatal(err)
		}
		if err := writer.Record(staticArtifactRecordDeclarations); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil { // aliases
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil { // params
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil { // interfaces
			t.Fatal(err)
		}
		for _, value := range []uint64{uint64(keyspace.MakeTerm(keyspace.FamilyBody, 1)), 1, 1, 1, 1, 1, 0, 1} {
			if err := writer.Uint(value); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Uint(3); err != nil { // invalid InterfaceMember enum
			t.Fatal(err)
		}
		for range 7 { // field, name, coordinate, signature
			if err := writer.Uint(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Count(0); err != nil { // declared types
			t.Fatal(err)
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordSignatures)
	})
}

func encodeStaticHostileSignatureScope(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		writeStaticEmptyTypes(t, writer)
		if err := writer.Record(staticArtifactRecordReferences); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil {
			t.Fatal(err)
		}
		if err := writer.Record(staticArtifactRecordDeclarations); err != nil {
			t.Fatal(err)
		}
		for range 4 {
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Record(staticArtifactRecordSignatures); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))); err != nil {
			t.Fatal(err)
		}
		for range 2 { // type params and fixed parameters
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Uint(0); err != nil { // variadic
			t.Fatal(err)
		}
		for range 4 { // absent coordinate
			if err := writer.Uint(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Bool(true); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil { // assertions
			t.Fatal(err)
		}
		writeStaticEmptySuffix(t, writer, staticArtifactRecordContracts)
	})
}

func encodeStaticHostileClaimOrder(t *testing.T) []byte {
	return encodeStaticMalformedSection(t, func(writer *canonical.Writer) {
		if err := writer.Record(staticArtifactRecordTypes); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(1); err != nil {
			t.Fatal(err)
		}
		if err := writer.Uint(uint64(PrimitiveAny)); err != nil {
			t.Fatal(err)
		}
		for range 9 {
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		writeStaticEmptySuffixUntil(t, writer, staticArtifactRecordReferences, staticArtifactRecordOperators)
		if err := writer.Record(staticArtifactRecordOperands); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(2); err != nil {
			t.Fatal(err)
		}
		for _, row := range [][2]keyspace.Term{
			{keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
			{keyspace.MakeTerm(keyspace.FamilyValueClaim, 1), keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
		} {
			if err := writer.Uint(uint64(row[0])); err != nil {
				t.Fatal(err)
			}
			if err := writer.Uint(uint64(row[1])); err != nil {
				t.Fatal(err)
			}
		}
		for range 2 { // TypeValue and Annotation
			if err := writer.Count(0); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Record(staticArtifactRecordPublications); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(0); err != nil {
			t.Fatal(err)
		}
	})
}

func staticArtifactInputEmpty(input Input) bool {
	return input.Counts == ([keyspace.FamilyCount]uint32{}) &&
		len(input.Types.Primitive) == 0 && len(input.Types.Literal) == 0 &&
		len(input.Types.Optional) == 0 && len(input.Types.Union) == 0 &&
		len(input.Types.Intersection) == 0 && len(input.Types.Generic) == 0 &&
		len(input.Types.Array) == 0 && len(input.Types.Map) == 0 &&
		len(input.Types.Record) == 0 && len(input.Types.Field) == 0 &&
		len(input.References.TypeRef) == 0 && len(input.Declarations.Alias) == 0 &&
		len(input.Declarations.TypeParam) == 0 && len(input.Declarations.Interface) == 0 &&
		len(input.Declarations.DeclaredType) == 0 && len(input.Signatures.TypeFunction) == 0 &&
		len(input.Signatures.TypeAsserts) == 0 && len(input.Contracts.Function) == 0 &&
		len(input.Contracts.Call) == 0 && len(input.Operators.TypeOf) == 0 &&
		len(input.Operators.KeyOf) == 0 && len(input.Operators.IndexAccess) == 0 &&
		len(input.Operators.Conditional) == 0 && len(input.Operands.Claim) == 0 &&
		len(input.Operands.TypeValue) == 0 && len(input.Operands.Annotation) == 0 &&
		len(input.Publications.Type) == 0 && len(input.EffectRows.Rows) == 0
}

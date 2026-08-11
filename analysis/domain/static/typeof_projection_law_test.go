package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

const typeOfProjectionSource = `
local subject = 1
type Runtime = typeof(subject)
type Known = typeof("literal")
type Invalid = typeof({})
type Annotated = string @tag(subject, "argument", {})
`

func TestStaticInputDenominatorCoversTypeOfAndAnnotationArguments(t *testing.T) {
	p, source, authority := sealedStatic(t, typeOfProjectionSource)
	staticView := p.Static()
	annotationView := staticView.Operands().Annotations()
	annotation, ok := annotationView.At(0)
	if !ok {
		t.Fatal("AnnotationAt")
	}
	annotationRow, ok := annotationView.Get(annotation)
	if !ok {
		t.Fatal("Annotation")
	}
	annotationCount, ok := p.Flow().Authored().Values().Len(annotationRow.Values)
	if !ok {
		t.Fatal("Annotation Values")
	}
	want := staticView.Operators().TypeOfs().Count() + annotationCount
	if source.Static().Inputs().Count() != want {
		t.Fatalf("StaticInput denominator = %d, want %d", source.Static().Inputs().Count(), want)
	}
	seen := make(map[linkstatic.InputRef]struct{}, want)
	var runtime, known, invalid int
	for index := 0; index < source.Static().Inputs().Count(); index++ {
		input, ok := source.Static().Inputs().At(index)
		if !ok {
			t.Fatalf("StaticInputAt(%d)", index)
		}
		_, site, expression, operand, body, cursor, ok := source.Static().Inputs().Source(input)
		_, referenceOK := source.Static().Expressions().Reference(expression)
		resolver, resolverOK := source.Static().Expressions().Resolver(expression)
		shard, shardOK := source.Static().Expressions().Shard(expression)
		ownerProgram, ownerOK := source.Project().Mounts().Program(shard)
		if !ok || !referenceOK || !resolverOK || !shardOK || !ownerOK || ownerProgram == nil || ownerProgram.ContentID() != p.ContentID() || site == 0 || operand == 0 {
			t.Fatalf("StaticInputSource(%d)", index)
		}
		wantBody, wantCursor, frontierOK := p.Source().Index().Frontier(site)
		if !frontierOK || body != wantBody || cursor != wantCursor {
			t.Fatalf("StaticInput(%d) frontier = %v/%d, want %v/%d", index, body, cursor, wantBody, wantCursor)
		}
		contained, ok := authority.Input(input)
		if !ok {
			t.Fatalf("Static Input(%d) absent", index)
		}
		if gotSite, ok := contained.StaticSite(); !ok || gotSite != site {
			t.Fatalf("contained site %d = %v/%v", index, gotSite, ok)
		}
		if gotBody, gotCursor, ok := contained.SourceFrontier(); !ok || gotBody != body || gotCursor != cursor {
			t.Fatalf("contained frontier %d = %v/%d/%v", index, gotBody, gotCursor, ok)
		}
		if _, duplicate := seen[input]; duplicate {
			t.Fatalf("StaticInput(%d) duplicated", index)
		}
		seen[input] = struct{}{}
		if _, _, typeOf := staticView.Operators().TypeOfs().Get(site); !typeOf {
			if _, _, ok := authority.TypeOf(input); ok {
				t.Fatalf("annotation input %d manufactured TypeOf output", index)
			}
			continue
		}
		output, typed, ok := authority.TypeOf(input)
		if !ok || typed != contained {
			t.Fatalf("TypeOf(%d)", index)
		}
		outputIndex, ok := authority.CoordinateIndex(output)
		if !ok || !referenceOK || authority.coordinates[outputIndex].key.reference.Owner() != p.ContentID() || authority.coordinates[outputIndex].key.reference.Root() != site || authority.coordinates[outputIndex].key.namespace != mustResolverID(t, source, resolver) {
			t.Fatalf("TypeOf output %d lost exact existing coordinate", index)
		}
		switch contained.Kind() {
		case OperandRuntimeSubject:
			runtime++
			subject, _ := contained.RuntimeSubject()
			value, _ := subject.Value()
			_, term, _ := source.Boundary().Values().Origin(value)
			if _, _, _, isCell := p.Flow().Authored().Storage().Cells().Get(term); !isCell {
				t.Fatalf("runtime input %d used non-Cell value %v", index, term)
			}
		case OperandKnown:
			known++
		case OperandInvalid:
			invalid++
		}
	}
	if len(seen) != want || runtime != 1 || known != 1 || invalid != 1 {
		t.Fatalf("input coverage = %d runtime:%d known:%d invalid:%d", len(seen), runtime, known, invalid)
	}
}

func TestStaticInputReplayFenceAndAllocation(t *testing.T) {
	_, leftSource, left := sealedStatic(t, typeOfProjectionSource)
	_, replaySource, replay := sealedStatic(t, typeOfProjectionSource)
	if _, ok := left.Input(linkstatic.InputRef{}); ok {
		t.Fatal("zero StaticInput entered Static")
	}
	for index := 0; index < leftSource.Static().Inputs().Count(); index++ {
		leftInput, _ := leftSource.Static().Inputs().At(index)
		replayInput, _ := replaySource.Static().Inputs().At(index)
		if _, ok := left.Input(replayInput); ok || !leftInputIdentity(t, left, replay, leftInput, replayInput) {
			t.Fatalf("StaticInput replay fence %d", index)
		}
	}
	inputs := make([]linkstatic.InputRef, leftSource.Static().Inputs().Count())
	for index := range inputs {
		inputs[index], _ = leftSource.Static().Inputs().At(index)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		for _, input := range inputs {
			_, _ = left.Input(input)
		}
	}); allocations != 0 {
		t.Fatalf("Static Input allocates %v", allocations)
	}
}

func leftInputIdentity(t testing.TB, left, replay *Authority, leftInput, replayInput linkstatic.InputRef) bool {
	t.Helper()
	leftValue, leftOK := left.Input(leftInput)
	replayValue, replayOK := replay.Input(replayInput)
	return leftOK && replayOK && leftValue.Kind() == replayValue.Kind()
}

func mustResolverID(t testing.TB, source *link.Link, resolver linkstatic.Resolver) keyspace.ContentID {
	t.Helper()
	id, ok := source.Static().Namespaces().ResolverContentID(resolver)
	if !ok {
		t.Fatal("ResolverContentID")
	}
	return id
}

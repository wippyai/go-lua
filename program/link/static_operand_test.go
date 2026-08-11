package link

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	staticpkg "github.com/wippyai/go-lua/program/static"
)

func TestStaticInputProjectionIsCanonicalReplayableAndFrontierBound(t *testing.T) {
	consumer := source(t, `
local value = 1
type First = typeof(value)
type Annotated = string @tag(value, "two") @note(value)
`)
	provider := source(t, `type Third = typeof(true)`)
	contract := contract(t)
	sealed, err := Seal(&Spec{Target: contract, Modules: []linkproject.Module{{Name: "consumer", Program: consumer}, {Name: "provider", Program: provider}}})
	if err != nil {
		t.Fatal(err)
	}
	want := consumer.Static().Operators().TypeOfs().Count() + consumerAnnotationArgumentCount(t, consumer) + provider.Static().Operators().TypeOfs().Count()
	if got := sealed.Static().Inputs().Count(); got != want {
		t.Fatalf("StaticInputCount = %d, want %d", got, want)
	}
	assertStaticInputProjection(t, sealed)
	annotationSites := make(map[keyspace.Term]struct{})
	var annotationTarget staticpkg.StaticTypeRef
	for index := 0; index < sealed.Static().Inputs().Count(); index++ {
		input, _ := sealed.Static().Inputs().At(index)
		kind, site, expression, _, _, _, ok := sealed.Static().Inputs().Source(input)
		if !ok || kind != linkstatic.InputAnnotation {
			continue
		}
		reference, ok := sealed.Static().Expressions().Reference(expression)
		if !ok {
			t.Fatal("annotation input lost typed target expression")
		}
		if annotationTarget.Term() != 0 && annotationTarget != reference {
			continue
		}
		annotationTarget = reference
		annotationSites[site] = struct{}{}
	}
	if len(annotationSites) < 2 {
		t.Fatal("distinct annotation sites sharing one target were conflated")
	}
	if _, ok := sealed.Static().Inputs().At(-1); ok {
		t.Fatal("negative StaticInput index accepted")
	}
	if _, ok := sealed.Static().Inputs().At(sealed.Static().Inputs().Count()); ok {
		t.Fatal("past-end StaticInput index accepted")
	}
	if _, _, _, _, _, _, ok := sealed.Static().Inputs().Source(linkstatic.InputRef{}); ok {
		t.Fatal("zero StaticInput accepted")
	}

	replayed := artifactAssertProjectionRoundTrip(t, sealed, contract, consumer, provider)
	if replayed.ContentID() != sealed.ContentID() {
		t.Fatalf("replayed Link identity = %v, want %v", replayed.ContentID(), sealed.ContentID())
	}
	assertStaticInputProjection(t, replayed)
	for index := 0; index < sealed.Static().Inputs().Count(); index++ {
		left, _ := sealed.Static().Inputs().At(index)
		right, _ := replayed.Static().Inputs().At(index)
		leftID, leftOK := sealed.Static().Inputs().ID(left)
		rightID, rightOK := replayed.Static().Inputs().ID(right)
		if !leftOK || !rightOK || leftID != rightID {
			t.Fatalf("StaticInputID replay %d", index)
		}
	}
	twin, err := Seal(&Spec{Target: contract, Modules: []linkproject.Module{{Name: "consumer", Program: consumer}, {Name: "provider", Program: provider}}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := sealed.Static().Inputs().At(0)
	if _, _, _, _, _, _, ok := twin.Static().Inputs().Source(input); ok {
		t.Fatal("StaticInput crossed Link owner fence")
	}
}

func consumerAnnotationArgumentCount(t testing.TB, p *program.Program) int {
	t.Helper()
	annotations := p.Static().Operands().Annotations()
	count := 0
	for index := 0; index < annotations.Count(); index++ {
		annotation, ok := annotations.At(index)
		if !ok {
			t.Fatal("AnnotationAt")
		}
		row, ok := annotations.Get(annotation)
		if !ok || row.Values == 0 {
			t.Fatal("Annotation")
		}
		width, ok := p.Flow().Authored().Values().Len(row.Values)
		if !ok {
			t.Fatal("Annotation Values")
		}
		count += width
	}
	return count
}

func assertStaticInputProjection(t testing.TB, sealed *Link) {
	t.Helper()
	index := 0
	for shardIndex := 0; shardIndex < sealed.Project().Mounts().Count(); shardIndex++ {
		shard, _ := sealed.Project().Mounts().At(shardIndex)
		p, ok := sealed.Project().Mounts().Program(shard)
		if !ok || p == nil {
			t.Fatal("Program")
		}
		namespace, _ := sealed.Static().Namespaces().At(shardIndex)
		resolver, _ := sealed.Static().Namespaces().Resolver(namespace)
		check := func(source, target, operand keyspace.Term, kind linkstatic.InputKind) {
			input, ok := sealed.Static().Inputs().At(index)
			if !ok {
				t.Fatalf("StaticInputAt(%d) absent", index)
			}
			gotKind, gotSite, expression, gotOperand, body, cursor, ok := sealed.Static().Inputs().Source(input)
			reference, referenceOK := sealed.Static().Expressions().Reference(expression)
			gotResolver, resolverOK := sealed.Static().Expressions().Resolver(expression)
			wantBody, wantCursor, frontierOK := p.Source().Index().Frontier(source)
			if !ok || !referenceOK || !resolverOK || !frontierOK || gotKind != kind || gotSite != source || reference.Term() != target || gotOperand != operand || gotResolver != resolver || body != wantBody || cursor != wantCursor {
				t.Fatalf("StaticInput(%d) does not retain exact source/frontier", index)
			}
			index++
		}
		typeOfs := p.Static().Operators().TypeOfs()
		for at := 0; at < typeOfs.Count(); at++ {
			site, _ := typeOfs.At(at)
			_, operand, _ := typeOfs.Get(site)
			check(site, site, operand, linkstatic.InputTypeOf)
		}
		annotations := p.Static().Operands().Annotations()
		for at := 0; at < annotations.Count(); at++ {
			site, _ := annotations.At(at)
			row, _ := annotations.Get(site)
			width, _ := p.Flow().Authored().Values().Len(row.Values)
			for argument := 0; argument < width; argument++ {
				operand, _ := p.Flow().Authored().Values().Member(row.Values, argument)
				check(site, row.Target, operand, linkstatic.InputAnnotation)
			}
		}
	}
	if index != sealed.Static().Inputs().Count() {
		t.Fatalf("StaticInput rows = %d, want %d", index, sealed.Static().Inputs().Count())
	}
}

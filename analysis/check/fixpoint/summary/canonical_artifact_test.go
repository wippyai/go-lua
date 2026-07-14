package summary

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCanonicalSummaryBytesMatchEqualAcrossAcceptedCorpus(t *testing.T) {
	reg := standard.Registry()
	stringValue := typevalue.String(reg)
	proof := callboundary.BranchProof{
		Kind: pathevidence.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0), Presence: presence.Present(),
	}
	condition := ReturnConditionParamRefinement{
		ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: stringValue,
	}
	aliasSource, _ := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(0))
	pathRefinement := callboundary.PathValueFact{Path: pathdom.NewPlaceholder(0), Value: stringValue}
	corpus := []Summary{
		{},
		{Returns: []product.Value{stringValue}},
		{Returns: []product.Value{stringValue, product.Bottom(reg)}},
		{NormalReturnParams: []product.Value{stringValue}},
		{NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{proof}}},
		{NormalReturnFacts: callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{proof, proof}}},
		{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{pathRefinement}}},
		{ReturnConditionParamRefinements: []ReturnConditionParamRefinement{condition}},
		{ReturnParamPathAliases: []ReturnParamPathAlias{{ReturnIndex: 0, Source: aliasSource}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 0, Kind: ReturnFlowParam, Param: 0}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 1, Kind: ReturnFlowParamMember, Param: 0, Path: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}}}},
		{
			Returns: []product.Value{stringValue}, NormalReturnParams: []product.Value{stringValue},
			NormalReturnFacts:               callboundary.NormalReturnFacts{BranchProofs: []callboundary.BranchProof{proof}},
			ReturnConditionParamRefinements: []ReturnConditionParamRefinement{condition},
		},
	}
	artifacts := make([]CanonicalArtifact, len(corpus))
	for index, item := range corpus {
		var err error
		artifacts[index], err = EncodeCanonical(context.Background(), reg, item)
		if err != nil {
			t.Fatalf("EncodeCanonical(%d): %v", index, err)
		}
		if len(artifacts[index].Bytes) == 0 || artifacts[index].Schema == (CanonicalSchemaIdentity{}) ||
			artifacts[index].Semantic == (CanonicalSemanticIdentity{}) {
			t.Fatalf("EncodeCanonical(%d) returned incomplete authority", index)
		}
	}
	for left := range corpus {
		for right := range corpus {
			equal := Equal(reg, corpus[left], corpus[right])
			sameBytes := bytes.Equal(artifacts[left].Bytes, artifacts[right].Bytes)
			if equal != sameBytes {
				t.Fatalf("corpus[%d]/corpus[%d]: Equal=%t bytes=%t", left, right, equal, sameBytes)
			}
			if sameBytes && artifacts[left].Semantic != artifacts[right].Semantic {
				t.Fatalf("equal bytes produced different semantic identities")
			}
		}
	}
}

func TestCanonicalSummaryRegistrationOrderDoesNotChangeAuthority(t *testing.T) {
	keyA := axis.NewKey[int]("test.summary.canonical.a")
	keyB := axis.NewKey[int]("test.summary.canonical.b")
	specA := canonicalSummaryIntSpec(keyA)
	specB := canonicalSummaryIntSpec(keyB)
	forward, err := product.RegistryWithAxes(specA.Erase(), specB.Erase())
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := product.RegistryWithAxes(specB.Erase(), specA.Erase())
	if err != nil {
		t.Fatal(err)
	}
	leftValue := product.Set(forward, product.Set(forward, product.Top(), keyB, 1), keyA, 2)
	rightValue := product.Set(reverse, product.Set(reverse, product.Top(), keyA, 2), keyB, 1)
	left, err := EncodeCanonical(context.Background(), forward, Summary{Returns: []product.Value{leftValue}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := EncodeCanonical(context.Background(), reverse, Summary{Returns: []product.Value{rightValue}})
	if err != nil {
		t.Fatal(err)
	}
	if left.Schema != right.Schema || left.Semantic != right.Semantic || !bytes.Equal(left.Bytes, right.Bytes) {
		t.Fatal("axis registration order changed canonical summary authority")
	}
}

func TestCanonicalSummaryRejectsEveryUnsupportedTopLevelLane(t *testing.T) {
	reg := standard.Registry()
	for _, descriptor := range summaryFactDescriptors {
		name := string(descriptor.Kind)
		switch name {
		case "Returns", "NormalReturnParams", "NormalReturnFacts", "ReturnConditionParamRefinements", "ReturnParamPathAliases", "ReturnFlows":
			continue
		}
		t.Run(name, func(t *testing.T) {
			fixture := summaryWithPopulatedField(t, name)
			artifact, err := EncodeCanonical(context.Background(), reg, fixture)
			var nonportable *NonportableCanonicalError
			if !errors.As(err, &nonportable) || nonportable.Lane != name || !canonicalArtifactZero(artifact) {
				t.Fatalf("EncodeCanonical = %#v, %v; want typed rejection for %s", artifact, err, name)
			}
		})
	}
}

func TestCanonicalSummaryRejectsEveryUnsupportedNormalReturnFactLane(t *testing.T) {
	reg := standard.Registry()
	for _, lane := range callboundary.NormalReturnFactLanes() {
		if lane.ID() == callboundary.LaneBranchProofs || lane.ID() == callboundary.LanePathRefinements {
			continue
		}
		t.Run(lane.FieldName(), func(t *testing.T) {
			facts := normalReturnFactsWithPopulatedField(t, lane.FieldName())
			artifact, err := EncodeCanonical(context.Background(), reg, Summary{NormalReturnFacts: facts})
			var nonportable *NonportableCanonicalError
			want := "NormalReturnFacts." + lane.FieldName()
			if !errors.As(err, &nonportable) || nonportable.Lane != want || !canonicalArtifactZero(artifact) {
				t.Fatalf("EncodeCanonical = %#v, %v; want typed rejection for %s", artifact, err, want)
			}
		})
	}
}

func TestCanonicalReturnFlowMemberIdentityMatchesSummaryEquality(t *testing.T) {
	reg := standard.Registry()
	clean := Summary{ReturnFlows: []ReturnFlow{{
		ReturnIndex: 0, Kind: ReturnFlowParamMember, Param: 0,
		Path: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}},
	}}}
	phantom := clean.Clone()
	phantom.ReturnFlows[0].Path[0].Index = 99 // ignored by SegmentField syntax identity
	if !Equal(reg, clean, phantom) {
		t.Fatal("canonical-equivalent member paths differ in summary lattice")
	}
	left, err := EncodeCanonical(context.Background(), reg, clean)
	if err != nil {
		t.Fatal(err)
	}
	right, err := EncodeCanonical(context.Background(), reg, phantom)
	if err != nil {
		t.Fatal(err)
	}
	if left.Schema != right.Schema || left.Semantic != right.Semantic || !bytes.Equal(left.Bytes, right.Bytes) {
		t.Fatal("canonical-equivalent member paths produced different authority")
	}

	mutations := []Summary{
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 1, Kind: ReturnFlowParamMember, Param: 0, Path: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 0, Kind: ReturnFlowParamMember, Param: 1, Path: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 0, Kind: ReturnFlowParamMember, Param: 0, Path: []segment.Segment{{Kind: segment.SegmentIndexString, Name: "value"}}}}},
		{ReturnFlows: []ReturnFlow{{ReturnIndex: 0, Kind: ReturnFlowParamMember, Param: 0, Path: []segment.Segment{{Kind: segment.SegmentField, Name: "other"}}}}},
	}
	for index, mutation := range mutations {
		if Equal(reg, clean, mutation) {
			t.Fatalf("mutation %d compares equal", index)
		}
		artifact, err := EncodeCanonical(context.Background(), reg, mutation)
		if err != nil {
			t.Fatalf("mutation %d: %v", index, err)
		}
		if bytes.Equal(left.Bytes, artifact.Bytes) || left.Semantic == artifact.Semantic {
			t.Fatalf("mutation %d retained canonical authority", index)
		}
	}
}

func TestCanonicalPathRefinementIdentityIsExact(t *testing.T) {
	reg := standard.Registry()
	base := Summary{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{{
		Path: pathdom.NewPlaceholder(0), Value: typevalue.String(reg),
	}}}}
	baseArtifact, err := EncodeCanonical(context.Background(), reg, base)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []Summary{
		{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(1), Value: typevalue.String(reg)}}}},
		{NormalReturnFacts: callboundary.NormalReturnFacts{PathRefinements: []callboundary.PathValueFact{{Path: pathdom.NewPlaceholder(0), Value: typevalue.Nil(reg)}}}},
	}
	for index, mutation := range mutations {
		artifact, err := EncodeCanonical(context.Background(), reg, mutation)
		if err != nil {
			t.Fatalf("mutation %d: %v", index, err)
		}
		if Equal(reg, base, mutation) || bytes.Equal(baseArtifact.Bytes, artifact.Bytes) || baseArtifact.Semantic == artifact.Semantic {
			t.Fatalf("path-refinement mutation %d retained semantic authority", index)
		}
	}
}

func TestCanonicalSummaryRejectsKeyspaceProvenanceAndForeignRegistry(t *testing.T) {
	reg := standard.Registry()
	artifact, err := EncodeCanonical(context.Background(), reg, Summary{HeapKeySpace: keyspace.New()})
	var nonportable *NonportableCanonicalError
	if !errors.As(err, &nonportable) || nonportable.Lane != "HeapKeySpace" || !canonicalArtifactZero(artifact) {
		t.Fatalf("keyspace provenance = %#v, %v", artifact, err)
	}

	foreignKey := axis.NewKey[int]("test.summary.foreign")
	foreign, registryErr := product.RegistryWithAxes(canonicalSummaryIntSpec(foreignKey).Erase())
	if registryErr != nil {
		t.Fatal(registryErr)
	}
	value := product.Set(foreign, product.Top(), foreignKey, 1)
	artifact, err = EncodeCanonical(context.Background(), reg, Summary{Returns: []product.Value{value}})
	if !errors.As(err, &nonportable) || nonportable.Lane != "Returns" || !canonicalArtifactZero(artifact) {
		t.Fatalf("foreign product = %#v, %v", artifact, err)
	}
}

func TestCanonicalSummaryRejectsRecursiveWitnessTransactionally(t *testing.T) {
	reg := standard.Registry()
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type { return typ.NewArray(self) })
	value := typevalue.WithWitness(reg, typevalue.String(reg), recursive)
	artifact, err := EncodeCanonical(context.Background(), reg, Summary{Returns: []product.Value{value}})
	var nonportable *NonportableCanonicalError
	if (!errors.Is(err, typewitness.ErrNonportableRecursiveIdentity) && !errors.As(err, &nonportable)) ||
		!canonicalArtifactZero(artifact) {
		t.Fatalf("recursive witness = %#v, %v", artifact, err)
	}
}

func TestCanonicalSummaryCancellationReturnsNoAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	artifact, err := EncodeCanonical(ctx, standard.Registry(), Summary{})
	if !errors.Is(err, context.Canceled) || !canonicalArtifactZero(artifact) {
		t.Fatalf("canceled encoding = %#v, %v", artifact, err)
	}
}

func summaryWithPopulatedField(t testing.TB, fieldName string) Summary {
	t.Helper()
	value := reflect.ValueOf(&Summary{}).Elem()
	field := value.FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() {
		t.Fatalf("summary descriptor field %q is not settable", fieldName)
	}
	switch field.Kind() {
	case reflect.Slice:
		field.Set(reflect.MakeSlice(field.Type(), 1, 1))
	case reflect.Map:
		populated := reflect.MakeMap(field.Type())
		populated.SetMapIndex(reflect.Zero(field.Type().Key()), reflect.Zero(field.Type().Elem()))
		field.Set(populated)
	case reflect.Bool:
		field.SetBool(true)
	case reflect.Struct:
		if fieldName != "ProtectedCallTypestate" {
			t.Fatalf("unsupported struct fixture for %q", fieldName)
		}
		field.FieldByName("HasNormal").SetBool(true)
	default:
		t.Fatalf("unsupported descriptor field kind %s for %q", field.Kind(), fieldName)
	}
	return value.Interface().(Summary)
}

func normalReturnFactsWithPopulatedField(t testing.TB, fieldName string) callboundary.NormalReturnFacts {
	t.Helper()
	value := reflect.ValueOf(&callboundary.NormalReturnFacts{}).Elem()
	field := value.FieldByName(fieldName)
	if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Slice {
		t.Fatalf("normal-return field %q is not a settable slice", fieldName)
	}
	field.Set(reflect.MakeSlice(field.Type(), 1, 1))
	return value.Interface().(callboundary.NormalReturnFacts)
}

func canonicalSummaryIntSpec(key axis.Key[int]) axis.Spec[int] {
	return axis.Spec[int]{
		Key: key, Bottom: func() int { return 0 }, Top: func() int { return 3 },
		Equal: func(a, b int) bool { return a == b }, LessOrEq: func(a, b int) bool { return a <= b },
		Join: func(a, b int) int { return max(a, b) }, Meet: func(a, b int) int { return min(a, b) },
		Widen: func(a, b int) int { return max(a, b) }, Hash: func(value int) uint64 { return uint64(value) },
		Retention: axis.ImmutableRetention[int](), Boundary: axis.PortableIdentity,
		Canonical: axis.ReadyCanonical("test.summary.int", 1, func(writer *canonical.Writer, value int) error {
			return writer.Int(int64(value))
		}),
	}
}

func canonicalArtifactZero(artifact CanonicalArtifact) bool {
	return artifact.Bytes == nil && artifact.Schema == (CanonicalSchemaIdentity{}) &&
		artifact.Semantic == (CanonicalSemanticIdentity{})
}

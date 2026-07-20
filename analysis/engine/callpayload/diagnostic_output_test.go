package callpayload

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDiagnosticOutputJoinSemilatticeLaws(t *testing.T) {
	reg := standard.Registry()
	stringValue := typevalue.String(reg)
	nilValue := typevalue.Nil(reg)
	origin := CallParamObligationOrigin{
		HasOrigin: true, ReceiverParam: 0, Member: segment.Segment{Kind: segment.SegmentField, Name: "send"},
		ArgParam: 1, MemberParamIndex: 0, SubjectLabel: "payload", ProviderLabel: "channel.send",
	}
	sample := []DiagnosticOutput{
		{},
		{MaySuspend: true},
		{SuspensionKnown: true},
		{SuspensionKnown: true, MaySuspend: true},
		{ParamObligations: []CallParamObligation{{ParamIndex: 0, Value: stringValue, SignatureSurface: true}}},
		{ParamObligations: []CallParamObligation{{ParamIndex: 0, Value: nilValue, Origin: origin}}},
		{PathObligations: []CallPathObligation{{Path: pathdom.NewPlaceholder(1).Field("name"), Value: stringValue}}},
		{ParamExposures: []CallParamExposure{{Source: pathdom.NewPlaceholder(0), Contract: stringValue, Kind: factflow.CovariantExposureRecord}}},
		{
			SuspensionKnown:  true,
			ParamObligations: []CallParamObligation{{ParamIndex: 1, Value: stringValue}},
			PathObligations:  []CallPathObligation{{Path: pathdom.NewPlaceholder(0), Value: nilValue}},
			ParamExposures:   []CallParamExposure{{Source: pathdom.NewPlaceholder(1), Contract: nilValue, Kind: factflow.CovariantExposureArray}},
		},
	}
	for i := range sample {
		sample[i] = sample[i].Normalize(reg)
		if !sample[i].Valid(reg) {
			t.Fatalf("sample[%d] is invalid: %#v", i, sample[i])
		}
	}

	for ai, a := range sample {
		if !a.Equal(reg, a) || !a.LessOrEq(reg, a) {
			t.Fatalf("sample[%d] violates reflexivity", ai)
		}
		if got := a.Join(reg, a); !got.Equal(reg, a) {
			t.Fatalf("sample[%d] violates join idempotence: %#v", ai, got)
		}
		for bi, b := range sample {
			ab, ba := a.Join(reg, b), b.Join(reg, a)
			if !ab.Equal(reg, ba) {
				t.Fatalf("join commutativity failed for %d,%d: %#v != %#v", ai, bi, ab, ba)
			}
			if !a.LessOrEq(reg, ab) || !b.LessOrEq(reg, ab) {
				t.Fatalf("join upper bound failed for %d,%d", ai, bi)
			}
			if got, want := a.LessOrEq(reg, b), ab.Equal(reg, b); got != want {
				t.Fatalf("order/join consistency for %d,%d = %v, want %v", ai, bi, got, want)
			}
			if a.LessOrEq(reg, b) && b.LessOrEq(reg, a) && !a.Equal(reg, b) {
				t.Fatalf("antisymmetry failed for %d,%d", ai, bi)
			}
			for ci, c := range sample {
				left := ab.Join(reg, c)
				right := a.Join(reg, b.Join(reg, c))
				if !left.Equal(reg, right) {
					t.Fatalf("join associativity failed for %d,%d,%d", ai, bi, ci)
				}
				if a.LessOrEq(reg, c) && b.LessOrEq(reg, c) && !ab.LessOrEq(reg, c) {
					t.Fatalf("least upper bound failed for %d,%d below %d", ai, bi, ci)
				}
			}
		}
	}
}

func TestDiagnosticOutputObligationLawsAndProvenance(t *testing.T) {
	reg := standard.Registry()
	stringValue := typevalue.String(reg)
	nilValue := typevalue.Nil(reg)
	path := pathdom.NewPlaceholder(0).Field("value")
	origin := CallParamObligationOrigin{
		HasOrigin: true, ReceiverParam: 0, Member: segment.Segment{Kind: segment.SegmentField, Name: "accept"},
		ArgParam: 1, MemberParamIndex: 0, SubjectLabel: "value", ProviderLabel: "provider",
	}
	left := DiagnosticOutput{
		ParamObligations: []CallParamObligation{{ParamIndex: 0, Value: stringValue, SignatureSurface: true}},
		PathObligations:  []CallPathObligation{{Path: path, Value: stringValue}},
	}
	right := DiagnosticOutput{
		ParamObligations: []CallParamObligation{
			{ParamIndex: 0, Value: nilValue, SignatureSurface: true},
			{ParamIndex: 0, Value: nilValue, Origin: origin},
		},
		PathObligations: []CallPathObligation{
			{Path: path, Value: nilValue},
			{Path: pathdom.NewPlaceholder(1), Value: nilValue},
		},
	}
	got := left.Join(reg, right)
	if len(got.ParamObligations) != 2 {
		t.Fatalf("param obligations = %#v, want signature meet plus distinct origin", got.ParamObligations)
	}
	var signature, attributed *CallParamObligation
	for i := range got.ParamObligations {
		if got.ParamObligations[i].SignatureSurface {
			signature = &got.ParamObligations[i]
		}
		if got.ParamObligations[i].Origin == origin {
			attributed = &got.ParamObligations[i]
		}
	}
	if signature == nil || !product.Equal(reg, signature.Value, product.Meet(reg, stringValue, nilValue)) {
		t.Fatalf("signature obligation did not use product.Meet: %#v", got.ParamObligations)
	}
	if attributed == nil {
		t.Fatalf("origin %#v was not preserved in %#v", origin, got.ParamObligations)
	}
	if len(got.PathObligations) != 2 {
		t.Fatalf("path obligations = %#v, want keyed union", got.PathObligations)
	}
	for _, obligation := range got.PathObligations {
		if obligation.Path.Equal(path) && !product.Equal(reg, obligation.Value, product.Meet(reg, stringValue, nilValue)) {
			t.Fatalf("colliding path value = %#v, want meet", obligation.Value)
		}
	}
	if !product.Equal(reg, product.Meet(reg, stringValue, nilValue), product.Bottom(reg)) {
		t.Fatal("test values no longer form a contradictory obligation meet")
	}
	var published CallOutcome
	got.ApplyTo(reg, &published)
	if len(published.ParamObligations) != 1 || published.ParamObligations[0].Origin != origin {
		t.Fatalf("published ParamObligations = %#v, want only useful attributed obligation", published.ParamObligations)
	}
	if len(published.PathObligations) != 1 || !published.PathObligations[0].Path.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("published PathObligations = %#v, want only useful non-contradictory path", published.PathObligations)
	}
}

func TestDiagnosticOutputCanonicalSpellingFingerprintAndClone(t *testing.T) {
	reg := standard.Registry()
	stringValue := typevalue.String(reg)
	nilValue := typevalue.Nil(reg)
	first := DiagnosticOutput{
		SuspensionKnown: true,
		ParamObligations: []CallParamObligation{
			{ParamIndex: 2, Value: nilValue},
			{ParamIndex: 0, Value: stringValue},
		},
		PathObligations: []CallPathObligation{
			{Path: pathdom.NewPlaceholder(1), Value: nilValue},
			{Path: pathdom.NewPlaceholder(0).Field("x"), Value: stringValue},
		},
		ParamExposures: []CallParamExposure{
			{Source: pathdom.NewPlaceholder(1), Contract: nilValue, Kind: factflow.CovariantExposureArray},
			{Source: pathdom.NewPlaceholder(0), Contract: stringValue, Kind: factflow.CovariantExposureRecord},
		},
	}
	second := DiagnosticOutput{
		SuspensionKnown:  true,
		ParamObligations: reverseCopy(first.ParamObligations),
		PathObligations:  reverseCopy(first.PathObligations),
		ParamExposures:   reverseCopy(first.ParamExposures),
	}
	a, b := first.Normalize(reg), second.Normalize(reg)
	if !reflect.DeepEqual(a, b) || !a.Equal(reg, b) {
		t.Fatalf("canonical spelling differs:\n%#v\n%#v", a, b)
	}
	if a.Fingerprint(reg) != b.Fingerprint(reg) {
		t.Fatalf("fingerprints differ: %x != %x", a.Fingerprint(reg), b.Fingerprint(reg))
	}

	clone := a.Clone()
	clone.PathObligations[0].Path.Segments[0].Name = "mutated"
	clone.ParamExposures[0].Source.Root = "$99"
	if a.PathObligations[0].Path.Segments[0].Name != "x" || a.ParamExposures[0].Source.Root == "$99" {
		t.Fatal("Clone retained mutable path storage")
	}
}

func TestDiagnosticOutputDescriptorOwnership(t *testing.T) {
	want := []string{"SuspensionKnown", "MaySuspend", "ParamObligations", "PathObligations", "ParamExposures"}
	roles := DiagnosticOutputFieldRoles()
	if len(roles) != len(want) {
		t.Fatalf("roles = %#v, want %v", roles, want)
	}
	all := make(map[string]int)
	for _, role := range CallOutcomeFieldRoles() {
		all[role.FieldName]++
	}
	for i, role := range roles {
		if role.FieldName != want[i] {
			t.Fatalf("role[%d] = %q, want %q", i, role.FieldName, want[i])
		}
		if all[role.FieldName] != 1 {
			t.Fatalf("diagnostic role %q has %d CallOutcome descriptor owners", role.FieldName, all[role.FieldName])
		}
	}
}

func TestDiagnosticOutputCallOutcomeRoundTrip(t *testing.T) {
	reg := standard.Registry()
	source := CallOutcome{
		SuspensionKnown: true, MaySuspend: true,
		ParamObligations: []CallParamObligation{{ParamIndex: 0, Value: typevalue.String(reg)}},
		PathObligations:  []CallPathObligation{{Path: pathdom.NewPlaceholder(0), Value: typevalue.Nil(reg)}},
		ParamExposures:   []CallParamExposure{{Source: pathdom.NewPlaceholder(0), Contract: typevalue.String(reg), Kind: factflow.CovariantExposureRecord}},
	}
	diagnostic := DiagnosticOutputFromCallOutcome(reg, source)
	var target CallOutcome
	diagnostic.ApplyTo(reg, &target)
	if !diagnostic.Equal(reg, DiagnosticOutputFromCallOutcome(reg, target)) {
		t.Fatalf("round trip = %#v, want %#v", target, source)
	}
	if source.ParamObligations[0].ParamIndex = 8; target.ParamObligations[0].ParamIndex == 8 {
		t.Fatal("ApplyTo retained source slice storage")
	}
}

func BenchmarkDiagnosticOutputCertifiedIdentity(b *testing.B) {
	reg := standard.Registry()
	identity := DiagnosticOutput{SuspensionKnown: true}
	b.Run("Fingerprint", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = identity.Fingerprint(reg)
		}
	})
	b.Run("Join", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = identity.Join(reg, identity)
		}
	})
	b.Run("Equal", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = identity.Equal(reg, identity)
		}
	})
	b.Run("LessOrEq", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = identity.LessOrEq(reg, identity)
		}
	})
}

func reverseCopy[T any](in []T) []T {
	out := append([]T(nil), in...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

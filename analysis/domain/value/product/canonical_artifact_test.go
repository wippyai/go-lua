package product_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCanonicalArtifactMaterializesLiteralAndCompleteStandardInventory(t *testing.T) {
	reg := value.Registry()
	literal := typevalue.LiteralBool(reg, true)
	if product.RetentionSafe(reg, literal) {
		t.Fatal("pointer-backed literal unexpectedly crossed the ordinary retention boundary")
	}
	assertCanonicalArtifactRoundTrip(t, reg, literal)

	compositeType := typ.NewArray(typ.NewMap(typ.String, typ.Number))
	ed := product.Edit(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, compositeType), compositeType))
	ed.SetPresence(presence.Present())
	product.EditSet(&ed, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	product.EditSet(&ed, escape.Key, escape.Fresh())
	product.EditSet(&ed, evidence.Key, evidence.GradualTop().
		WithOrigin(evidence.Origin{Kind: evidence.OriginSource, ID: 11}).
		WithOrigin(evidence.Origin{Kind: evidence.OriginCall, ID: 29}))
	product.EditSet(&ed, identity.Key, identity.Singleton(identity.ID{Kind: "test.object", Site: "artifact", Index: 7}))
	product.EditSet(&ed, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	product.EditSet(&ed, variantorigin.Key, variantorigin.Of(91, []int{-3, 0, 8}))
	complete := ed.Done()
	if product.RetentionSafe(reg, complete) {
		t.Fatal("composite witness unexpectedly crossed the ordinary retention boundary")
	}
	assertCanonicalArtifactRoundTrip(t, reg, complete)
}

func TestCanonicalArtifactOwnsBytesAndReconstructedTypeGraph(t *testing.T) {
	reg := value.Registry()
	callerType := typ.NewArray(typ.String)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, callerType), callerType)
	artifact, err := product.SealCanonical(context.Background(), reg, value)
	if err != nil {
		t.Fatal(err)
	}

	bytesCopy := artifact.Bytes()
	for i := range bytesCopy {
		bytesCopy[i] ^= 0xff
	}
	callerType.Element = typ.Number

	materialized, err := artifact.Materialize(context.Background(), reg)
	if err != nil {
		t.Fatal(err)
	}
	witnessType, ok := product.Get(reg, materialized, typewitness.Key).Type()
	witnessArray, ok := witnessType.(*typ.Array)
	if !ok || !typ.TypeEquals(witnessArray.Element, typ.String) {
		t.Fatalf("materialized witness = %T %v, want independently owned string[]", witnessType, witnessType)
	}
	if witnessArray == callerType {
		t.Fatal("materialization retained the caller-owned type node")
	}
}

func TestCanonicalArtifactRejectsMalformedSchemaAndCancellationWithoutPublication(t *testing.T) {
	reg := value.Registry()
	artifact, err := product.SealCanonical(context.Background(), reg, typevalue.LiteralBool(reg, false))
	if err != nil {
		t.Fatal(err)
	}

	badSchema := artifact.SchemaIdentity()
	badSchema[0] ^= 0xff
	assertCanonicalDecodeFailure(t, reg, artifact.Bytes(), badSchema, product.ErrCanonicalMaterializationUnavailable)

	trailing := append(artifact.Bytes(), 0)
	assertCanonicalDecodeFailure(t, reg, trailing, artifact.SchemaIdentity(), canonical.ErrTrailing)

	truncated := artifact.Bytes()
	truncated = truncated[:len(truncated)-1]
	assertCanonicalDecodeFailure(t, reg, truncated, artifact.SchemaIdentity(), canonical.ErrMalformed)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	zeroArtifact, err := product.SealCanonical(ctx, reg, typevalue.LiteralBool(reg, true))
	if !errors.Is(err, context.Canceled) || zeroArtifact.Valid() || len(zeroArtifact.Bytes()) != 0 ||
		zeroArtifact.SchemaIdentity() != (axis.SchemaIdentity{}) {
		t.Fatalf("canceled seal = valid %t, bytes %x, schema %x, error %v", zeroArtifact.Valid(), zeroArtifact.Bytes(), zeroArtifact.SchemaIdentity(), err)
	}
	materialized, err := artifact.Materialize(ctx, reg)
	if !errors.Is(err, context.Canceled) || materialized != (product.Value{}) {
		t.Fatalf("canceled materialization = %#v, %v; want zero publication", materialized, err)
	}
}

func TestDecodeCanonicalRejectsConstructorUnreachableAxisPayloads(t *testing.T) {
	reg := value.Registry()
	cases := []struct {
		name   string
		axisID string
		write  func(testing.TB, *canonical.Writer)
	}{
		{name: "presence unknown state", axisID: presence.Key.ID(), write: canonicalRawUintAxis(1, 4)},
		{name: "assertion unknown state", axisID: assertion.Key.ID(), write: canonicalRawUintAxis(1, 3)},
		{name: "assertion empty concrete", axisID: assertion.Key.ID(), write: func(t testing.TB, writer *canonical.Writer) {
			canonicalWriteRecordUint(t, writer, 1, 1)
			if err := writer.Uint(0); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "escape unknown state", axisID: escape.Key.ID(), write: canonicalRawUintAxis(1, 3)},
		{name: "evidence count five", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(1, 5, false, nil)},
		{name: "evidence count 255", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(1, 255, false, nil)},
		{name: "evidence unknown kind", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(4, 0, false, nil)},
		{name: "evidence unknown active origin", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(1, 1, false, [][2]uint64{{0, 1}})},
		{name: "evidence duplicate active origins", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(1, 2, false, [][2]uint64{{1, 7}, {1, 7}})},
		{name: "evidence nonzero inactive origin", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(1, 0, false, [][2]uint64{{1, 7}})},
		{name: "evidence premature truncation", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(1, 1, true, [][2]uint64{{1, 7}})},
		{name: "evidence terminal payload", axisID: evidence.Key.ID(), write: canonicalMalformedEvidence(0, 1, false, [][2]uint64{{1, 7}})},
		{name: "identity unknown state", axisID: identity.Key.ID(), write: canonicalRawUintAxis(1, 3)},
		{name: "identity zero singleton", axisID: identity.Key.ID(), write: func(t testing.TB, writer *canonical.Writer) {
			canonicalWriteRecordUint(t, writer, 1, 1)
			if err := writer.String(""); err != nil {
				t.Fatal(err)
			}
			if err := writer.String(""); err != nil {
				t.Fatal(err)
			}
			if err := writer.Uint(0); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "runtimekind unknown bit", axisID: runtimekind.Key.ID(), write: canonicalRawUintAxis(1, 1<<8)},
		{name: "typewitness unknown state", axisID: typewitness.Key.ID(), write: canonicalRawUintAxis(1, 3)},
		{name: "variantorigin unknown state", axisID: variantorigin.Key.ID(), write: canonicalMalformedVariant(3, 0, nil)},
		{name: "variantorigin terminal payload", axisID: variantorigin.Key.ID(), write: canonicalMalformedVariant(0, 9, []int64{1})},
		{name: "variantorigin empty concrete", axisID: variantorigin.Key.ID(), write: canonicalMalformedVariant(1, 9, nil)},
		{name: "variantorigin zero-family concrete", axisID: variantorigin.Key.ID(), write: canonicalMalformedVariant(1, 0, []int64{1})},
		{name: "variantorigin duplicate cases", axisID: variantorigin.Key.ID(), write: canonicalMalformedVariant(1, 9, []int64{1, 1})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := canonicalMalformedProduct(t, reg, tc.axisID, tc.write)
			materialized, err := product.DecodeCanonical(context.Background(), reg, encoded, canonicalSchema(t, reg))
			if err == nil || materialized != (product.Value{}) {
				t.Fatalf("DecodeCanonical = %#v, %v; want zero publication and an error", materialized, err)
			}
		})
	}
}

func TestCanonicalArtifactRejectsRecursiveWitnessAndEncodeOnlyAxis(t *testing.T) {
	reg := value.Registry()
	recursive := typ.NewRecursive("Node", func(self typ.Type) typ.Type { return typ.NewArray(self) })
	value := typevalue.WithWitness(reg, product.Top(), recursive)
	artifact, err := product.SealCanonical(context.Background(), reg, value)
	if !errors.Is(err, typewitness.ErrNonportableRecursiveIdentity) || artifact.Valid() || len(artifact.Bytes()) != 0 {
		t.Fatalf("recursive seal = valid %t, bytes %x, error %v", artifact.Valid(), artifact.Bytes(), err)
	}

	key := axis.NewKey[int]("test.product.encode-only")
	encodeOnly := axis.Spec[int]{
		Key: key, Bottom: func() int { return 0 }, Top: func() int { return 2 },
		Equal: func(a, b int) bool { return a == b }, LessOrEq: func(a, b int) bool { return a <= b },
		Join: func(a, b int) int { return max(a, b) }, Meet: func(a, b int) int { return min(a, b) },
		Widen: func(a, b int) int { return max(a, b) }, Hash: func(value int) uint64 { return uint64(value) },
		Retention: axis.ImmutableRetention[int](), Boundary: axis.PortableIdentity,
		Canonical: axis.ReadyCanonical("test.product.encode-only", 1, func(writer *canonical.Writer, value int) error {
			return writer.Int(int64(value))
		}),
	}
	encodeOnlyRegistry, err := product.RegistryWithAxes(encodeOnly.Erase())
	if err != nil {
		t.Fatal(err)
	}
	encodeOnlyValue := product.Set(encodeOnlyRegistry, product.Top(), key, 1)
	if encoded, schema, encodeErr := product.EncodeCanonical(context.Background(), encodeOnlyRegistry, encodeOnlyValue); encodeErr != nil || len(encoded) == 0 || schema == (axis.SchemaIdentity{}) {
		t.Fatalf("existing encode-only descriptor stopped encoding = %x, %x, %v", encoded, schema, encodeErr)
	}
	artifact, err = product.SealCanonical(context.Background(), encodeOnlyRegistry, encodeOnlyValue)
	if !errors.Is(err, product.ErrCanonicalMaterializationUnavailable) || artifact.Valid() || len(artifact.Bytes()) != 0 {
		t.Fatalf("encode-only seal = valid %t, bytes %x, error %v", artifact.Valid(), artifact.Bytes(), err)
	}
}

func assertCanonicalArtifactRoundTrip(t testing.TB, reg *axis.Registry, value product.Value) {
	t.Helper()
	artifact, err := product.SealCanonical(context.Background(), reg, value)
	if err != nil {
		t.Fatal(err)
	}
	if !artifact.Valid() || len(artifact.Bytes()) == 0 || artifact.SchemaIdentity() == (axis.SchemaIdentity{}) {
		t.Fatal("seal returned incomplete authority")
	}
	materialized, err := artifact.Materialize(context.Background(), reg)
	if err != nil {
		t.Fatal(err)
	}
	if !product.Equal(reg, value, materialized) {
		t.Fatal("canonical artifact changed product semantics")
	}
	want, wantSchema, err := product.EncodeCanonical(context.Background(), reg, value)
	if err != nil {
		t.Fatal(err)
	}
	got, gotSchema, err := product.EncodeCanonical(context.Background(), reg, materialized)
	if err != nil {
		t.Fatal(err)
	}
	if wantSchema != gotSchema || string(want) != string(got) {
		t.Fatal("canonical artifact changed canonical authority")
	}
}

func assertCanonicalDecodeFailure(t testing.TB, reg *axis.Registry, encoded []byte, schema axis.SchemaIdentity, want error) {
	t.Helper()
	materialized, err := product.DecodeCanonical(context.Background(), reg, encoded, schema)
	if !errors.Is(err, want) || materialized != (product.Value{}) {
		t.Fatalf("DecodeCanonical = %#v, %v; want zero publication and %v", materialized, err, want)
	}
}

func canonicalSchema(t testing.TB, reg *axis.Registry) axis.SchemaIdentity {
	t.Helper()
	plan, err := reg.CanonicalPlan()
	if err != nil {
		t.Fatal(err)
	}
	schema, ok := plan.AuthorityIdentity()
	if !ok {
		t.Fatal("standard registry has no canonical authority")
	}
	return schema
}

func canonicalMalformedProduct(t testing.TB, reg *axis.Registry, targetAxis string, writePayload func(testing.TB, *canonical.Writer)) []byte {
	t.Helper()
	plan, err := reg.CanonicalPlan()
	if err != nil {
		t.Fatal(err)
	}
	entries := plan.Entries()
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), "analysis.value-product", 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(1); err != nil {
		t.Fatal(err)
	}
	schema := canonicalSchema(t, reg)
	if err := writer.Bytes(schema[:]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(uint64(product.ShapeTop)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(2); err != nil {
		t.Fatal(err)
	}
	if targetAxis == presence.Key.ID() {
		writePayload(t, &writer)
	} else {
		canonicalWriteRecordUint(t, &writer, 1, uint64(presence.Present()))
	}
	if err := writer.Count(uint64(len(entries) - 1)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.AxisID == presence.Key.ID() {
			continue
		}
		if err := writer.Record(3); err != nil {
			t.Fatal(err)
		}
		if err := writer.String(entry.AxisID); err != nil {
			t.Fatal(err)
		}
		present := entry.AxisID == targetAxis
		if err := writer.Bool(present); err != nil {
			t.Fatal(err)
		}
		if present {
			writePayload(t, &writer)
		}
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func canonicalRawUintAxis(record, value uint64) func(testing.TB, *canonical.Writer) {
	return func(t testing.TB, writer *canonical.Writer) {
		canonicalWriteRecordUint(t, writer, record, value)
	}
}

func canonicalWriteRecordUint(t testing.TB, writer *canonical.Writer, record, value uint64) {
	t.Helper()
	if err := writer.Record(record); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(value); err != nil {
		t.Fatal(err)
	}
}

func canonicalMalformedEvidence(kind, count uint64, truncated bool, origins [][2]uint64) func(testing.TB, *canonical.Writer) {
	return func(t testing.TB, writer *canonical.Writer) {
		canonicalWriteRecordUint(t, writer, 1, kind)
		if err := writer.Uint(count); err != nil {
			t.Fatal(err)
		}
		if err := writer.Bool(truncated); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 4; index++ {
			var origin [2]uint64
			if index < len(origins) {
				origin = origins[index]
			}
			if err := writer.Uint(origin[0]); err != nil {
				t.Fatal(err)
			}
			if err := writer.Uint(origin[1]); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func canonicalMalformedVariant(state, family uint64, cases []int64) func(testing.TB, *canonical.Writer) {
	return func(t testing.TB, writer *canonical.Writer) {
		canonicalWriteRecordUint(t, writer, 1, state)
		if err := writer.Uint(family); err != nil {
			t.Fatal(err)
		}
		if err := writer.Count(uint64(len(cases))); err != nil {
			t.Fatal(err)
		}
		for _, value := range cases {
			if err := writer.Int(value); err != nil {
				t.Fatal(err)
			}
		}
	}
}

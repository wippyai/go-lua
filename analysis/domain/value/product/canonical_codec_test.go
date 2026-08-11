package product

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

var (
	canonicalTestA = axis.NewKey[int]("test.canonical.product.a")
	canonicalTestB = axis.NewKey[int]("test.canonical.product.b")
)

func TestCanonicalProductBytesMatchEqualPartition(t *testing.T) {
	reg := canonicalTestRegistry(t,
		canonicalIntSpec(canonicalTestB, "test.product.int", 1, encodeCanonicalTestInt),
		canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt),
	)
	values := canonicalProductCorpus(reg)
	encoded := make([][]byte, len(values))
	for i, value := range values {
		var schema axis.SchemaIdentity
		var err error
		encoded[i], schema, err = EncodeCanonical(context.Background(), reg, value)
		if err != nil {
			t.Fatalf("EncodeCanonical(%d) error = %v", i, err)
		}
		if schema == (axis.SchemaIdentity{}) {
			t.Fatalf("EncodeCanonical(%d) returned zero schema authority", i)
		}
	}
	for i := range values {
		for j := range values {
			if got, want := bytes.Equal(encoded[i], encoded[j]), Equal(reg, values[i], values[j]); got != want {
				t.Fatalf("byte equality for corpus[%d]/corpus[%d] = %t, product.Equal = %t", i, j, got, want)
			}
		}
	}
}

func TestCanonicalProductRegistrationOrderDoesNotChangeBytes(t *testing.T) {
	forward := canonicalTestRegistry(t,
		canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt),
		canonicalIntSpec(canonicalTestB, "test.product.int", 1, encodeCanonicalTestInt),
	)
	reverse := canonicalTestRegistry(t,
		canonicalIntSpec(canonicalTestB, "test.product.int", 1, encodeCanonicalTestInt),
		canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt),
	)
	left := WithPresence(forward, Set(forward, Set(forward, Top(), canonicalTestB, 1), canonicalTestA, 2), presence.Absent())
	right := WithPresence(reverse, Set(reverse, Set(reverse, Top(), canonicalTestA, 2), canonicalTestB, 1), presence.Absent())
	leftBytes, leftSchema, err := EncodeCanonical(context.Background(), forward, left)
	if err != nil {
		t.Fatal(err)
	}
	rightBytes, rightSchema, err := EncodeCanonical(context.Background(), reverse, right)
	if err != nil {
		t.Fatal(err)
	}
	if leftSchema != rightSchema || !bytes.Equal(leftBytes, rightBytes) {
		t.Fatalf("registration order changed canonical authority or bytes: %x/%x, equal=%t", leftSchema, rightSchema, bytes.Equal(leftBytes, rightBytes))
	}
}

func TestCanonicalProductCodecMetadataIsSafeForConcurrentReuse(t *testing.T) {
	reg := canonicalTestRegistry(t,
		canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt),
		canonicalIntSpec(canonicalTestB, "test.product.int", 1, encodeCanonicalTestInt),
	)
	value := Set(reg, Set(reg, Top(), canonicalTestA, 1), canonicalTestB, 2)
	want, wantSchema, err := EncodeCanonical(context.Background(), reg, value)
	if err != nil {
		t.Fatal(err)
	}
	errorsOut := make(chan error, 32)
	for range 32 {
		go func() {
			got, schema, err := EncodeCanonical(context.Background(), reg, value)
			if err == nil && (schema != wantSchema || !bytes.Equal(got, want)) {
				err = fmt.Errorf("concurrent encoding changed authority or bytes")
			}
			errorsOut <- err
		}()
	}
	for range 32 {
		if err := <-errorsOut; err != nil {
			t.Fatal(err)
		}
	}
}

func TestCanonicalProductFramesPresenceAndEverySparseAbsence(t *testing.T) {
	encodeCalls := 0
	counting := func(writer *canonical.Writer, value int) error {
		encodeCalls++
		return encodeCanonicalTestInt(writer, value)
	}
	reg := canonicalTestRegistry(t,
		canonicalIntSpec(canonicalTestB, "test.product.int", 1, counting),
		canonicalIntSpec(canonicalTestA, "test.product.int", 1, counting),
	)

	omitted, _, err := EncodeCanonical(context.Background(), reg, WithPresence(reg, Top(), presence.Absent()))
	if err != nil {
		t.Fatal(err)
	}
	if encodeCalls != 0 {
		t.Fatalf("omitted sparse axes invoked %d payload encoders, want 0", encodeCalls)
	}
	records, bools := canonicalProductStructuralEvents(t, omitted)
	if got := countUint(records, canonicalPresenceRecord); got != 1 {
		t.Fatalf("presence record count = %d, want exactly 1", got)
	}
	if got := countUint(records, canonicalSparseRecord); got != 2 {
		t.Fatalf("sparse record count = %d, want 2", got)
	}
	if !slices.Equal(bools, []bool{false, false}) {
		t.Fatalf("omitted sparse presence framing = %v, want [false false]", bools)
	}

	present, _, err := EncodeCanonical(context.Background(), reg, Set(reg, Top(), canonicalTestA, 1))
	if err != nil {
		t.Fatal(err)
	}
	if encodeCalls != 1 {
		t.Fatalf("one present sparse axis invoked %d total payload encoders, want 1", encodeCalls)
	}
	_, bools = canonicalProductStructuralEvents(t, present)
	if !slices.Equal(bools, []bool{true, false}) {
		t.Fatalf("sparse presence framing = %v, want [true false] in AxisID order", bools)
	}
	if bytes.Equal(omitted, present) {
		t.Fatal("omitted and present sparse slots encoded identically")
	}

	coreAbsent, _, err := EncodeCanonical(context.Background(), reg, WithPresence(reg, Top(), presence.Absent()))
	if err != nil {
		t.Fatal(err)
	}
	sparseScalar, _, err := EncodeCanonical(context.Background(), reg, Set(reg, Top(), canonicalTestA, int(presence.Absent())))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(coreAbsent, sparseScalar) {
		t.Fatal("core presence and a same-valued sparse payload collided")
	}
}

func TestCanonicalProductSchemaAndCodecDriftChangeAuthorityAndBytes(t *testing.T) {
	base := canonicalTestRegistry(t, canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt))
	versioned := canonicalTestRegistry(t, canonicalIntSpec(canonicalTestA, "test.product.int", 2, encodeCanonicalTestInt))
	renamed := canonicalTestRegistry(t, canonicalIntSpec(canonicalTestA, "test.product.int.v2", 1, encodeCanonicalTestInt))
	added := canonicalTestRegistry(t,
		canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt),
		canonicalIntSpec(canonicalTestB, "test.product.int", 1, encodeCanonicalTestInt),
	)
	registries := []*axis.Registry{base, versioned, renamed, added}
	seenSchema := make(map[axis.SchemaIdentity]struct{})
	seenBytes := make(map[string]struct{})
	for _, reg := range registries {
		encoded, schema, err := EncodeCanonical(context.Background(), reg, Set(reg, Top(), canonicalTestA, 1))
		if err != nil {
			t.Fatal(err)
		}
		seenSchema[schema] = struct{}{}
		seenBytes[string(encoded)] = struct{}{}
	}
	if len(seenSchema) != len(registries) || len(seenBytes) != len(registries) {
		t.Fatalf("schema/codec drift collapsed authority or bytes: schemas=%d bytes=%d want=%d", len(seenSchema), len(seenBytes), len(registries))
	}
}

func TestCanonicalProductBytesDoNotDependOnLatticeHash(t *testing.T) {
	leftSpec := canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt)
	rightSpec := canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt)
	leftSpec.Hash = func(int) uint64 { return 1 }
	rightSpec.Hash = func(value int) uint64 { return ^uint64(value) }
	leftReg := canonicalTestRegistry(t, leftSpec)
	rightReg := canonicalTestRegistry(t, rightSpec)
	left, leftSchema, err := EncodeCanonical(context.Background(), leftReg, Set(leftReg, Top(), canonicalTestA, 1))
	if err != nil {
		t.Fatal(err)
	}
	right, rightSchema, err := EncodeCanonical(context.Background(), rightReg, Set(rightReg, Top(), canonicalTestA, 1))
	if err != nil {
		t.Fatal(err)
	}
	if leftSchema != rightSchema || !bytes.Equal(left, right) {
		t.Fatal("lattice Hash implementation changed canonical authority or bytes")
	}
}

func TestCanonicalProductFailsClosedWithoutAuthority(t *testing.T) {
	ready := canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt)
	pending := ready
	pending.Canonical = axis.PendingCanonical[int]("portable codec intentionally unavailable")

	unsealed := axis.NewRegistry()
	axis.RegisterCanonicalCore(unsealed, presence.Spec())
	axis.Register(unsealed, ready)
	unsealed.Freeze()

	mutable := axis.NewRegistry()
	axis.RegisterCanonicalCore(mutable, presence.Spec())
	axis.Register(mutable, ready)
	if err := mutable.SealCanonicalInventory(); err != nil {
		t.Fatal(err)
	}

	for name, fixture := range map[string]struct {
		reg   *axis.Registry
		value Value
	}{
		"nil":      {nil, Top()},
		"unsealed": {unsealed, Top()},
		"mutable":  {mutable, Top()},
		"pending":  {canonicalTestRegistry(t, pending), Top()},
	} {
		t.Run(name, func(t *testing.T) {
			encoded, schema, err := EncodeCanonical(context.Background(), fixture.reg, fixture.value)
			if err == nil || encoded != nil || schema != (axis.SchemaIdentity{}) {
				t.Fatalf("EncodeCanonical = %x, %x, %v; want transactional authority rejection", encoded, schema, err)
			}
		})
	}
}

func TestCanonicalProductRejectsMismatchedAndForgedRegistry(t *testing.T) {
	regA := canonicalTestRegistry(t, canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt))
	regB := canonicalTestRegistry(t, canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt))
	foreign := Set(regA, Top(), canonicalTestA, 1)
	encoded, schema, err := EncodeCanonical(context.Background(), regB, foreign)
	if err == nil || encoded != nil || schema != (axis.SchemaIdentity{}) {
		t.Fatalf("mismatched registry = %x, %x, %v", encoded, schema, err)
	}

	forgedPresence := presence.Spec()
	forgedPresence.Canonical = axis.ReadyCanonical("test.forged.presence", 1, func(writer *canonical.Writer, value presence.Value) error {
		return writer.Uint(uint64(value))
	})
	forged := axis.NewRegistry()
	axis.RegisterCanonicalCore(forged, forgedPresence)
	axis.Register(forged, canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt))
	if err := forged.SealCanonicalInventory(); err != nil {
		t.Fatal(err)
	}
	runtime := buildRegistryRuntime(forged)
	if runtime.err != nil {
		t.Fatal(runtime.err)
	}
	if err := forged.FreezeWithCompiledProduct(runtime); err != nil {
		t.Fatal(err)
	}
	encoded, schema, err = EncodeCanonical(context.Background(), forged, Top())
	if err == nil || encoded != nil || schema != (axis.SchemaIdentity{}) || !strings.Contains(err.Error(), "presence inventory") {
		t.Fatalf("forged core registry = %x, %x, %v", encoded, schema, err)
	}
}

func TestCanonicalProductRejectsMalformedNoncanonicalValues(t *testing.T) {
	reg := canonicalTestRegistry(t, canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt))
	rt := mustRuntime(reg)
	fixtures := map[string]Value{
		"shape":             {n: &node{reg: reg, shape: Shape(9), presence: presence.Top()}},
		"presence":          {n: &node{reg: reg, shape: ShapeTop, presence: presence.Value(9)}},
		"explicit-top":      {n: &node{reg: reg, shape: ShapeTop, presence: presence.Top(), hash: topHash()}},
		"unreduced":         {n: &node{reg: reg, shape: ShapeTop, presence: presence.Absent()}},
		"outside":           {n: &node{reg: reg, shape: ShapeTop, presence: presence.Top(), slots: []slot{{ordinal: 99, value: 1}}}},
		"wrong-type":        {n: &node{reg: reg, shape: ShapeTop, presence: presence.Top(), slots: []slot{{ordinal: 0, value: "1"}}}},
		"explicit-axis-top": {n: &node{reg: reg, shape: ShapeTop, presence: presence.Top(), slots: []slot{{ordinal: 0, value: 3}}}},
		"duplicate-slot":    {n: &node{reg: reg, shape: ShapeTop, presence: presence.Top(), slots: []slot{{ordinal: 0, value: 1}, {ordinal: 0, value: 2}}}},
	}
	if len(rt.canonicalAxes) != 1 {
		t.Fatalf("test registry axes = %d, want 1", len(rt.canonicalAxes))
	}
	for name, value := range fixtures {
		t.Run(name, func(t *testing.T) {
			encoded, schema, err := EncodeCanonical(context.Background(), reg, value)
			if err == nil || encoded != nil || schema != (axis.SchemaIdentity{}) {
				t.Fatalf("malformed value encoded: %x, %x, %v", encoded, schema, err)
			}
		})
	}
}

func TestCanonicalProductEncoderErrorAndCancellationAreTransactional(t *testing.T) {
	sentinel := errors.New("nonportable test value")
	failing := canonicalIntSpec(canonicalTestA, "test.product.fail", 1, func(writer *canonical.Writer, value int) error {
		if err := writer.Int(int64(value)); err != nil {
			return err
		}
		return sentinel
	})
	reg := canonicalTestRegistry(t, failing)
	encoded, schema, err := EncodeCanonical(context.Background(), reg, Set(reg, Top(), canonicalTestA, 1))
	if !errors.Is(err, sentinel) || encoded != nil || schema != (axis.SchemaIdentity{}) {
		t.Fatalf("encoder failure = %x, %x, %v", encoded, schema, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	canceling := canonicalIntSpec(canonicalTestA, "test.product.cancel", 1, func(writer *canonical.Writer, value int) error {
		cancel()
		return writer.Int(int64(value))
	})
	reg = canonicalTestRegistry(t, canceling)
	encoded, schema, err = EncodeCanonical(ctx, reg, Set(reg, Top(), canonicalTestA, 1))
	if !errors.Is(err, context.Canceled) || encoded != nil || schema != (axis.SchemaIdentity{}) {
		t.Fatalf("canceled encoding = %x, %x, %v", encoded, schema, err)
	}
}

func canonicalTestRegistry(t testing.TB, specs ...axis.Spec[int]) *axis.Registry {
	t.Helper()
	erased := make([]axis.ErasedSpec, len(specs))
	for i := range specs {
		erased[i] = specs[i].Erase()
	}
	reg, err := RegistryWithAxes(erased...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func canonicalIntSpec(key axis.Key[int], codecID string, version uint64, encode func(*canonical.Writer, int) error) axis.Spec[int] {
	return axis.Spec[int]{
		Key:       key,
		Bottom:    func() int { return 0 },
		Top:       func() int { return 3 },
		Equal:     func(a, b int) bool { return a == b },
		LessOrEq:  func(a, b int) bool { return a <= b },
		Join:      func(a, b int) int { return max(a, b) },
		Meet:      func(a, b int) int { return min(a, b) },
		Widen:     func(a, b int) int { return max(a, b) },
		Hash:      func(value int) uint64 { return uint64(value) },
		Retention: axis.ImmutableRetention[int](),
		Boundary:  axis.PortableIdentity,
		Canonical: axis.ReadyCanonical(codecID, version, encode),
	}
}

func encodeCanonicalTestInt(writer *canonical.Writer, value int) error {
	return writer.Int(int64(value))
}

func canonicalProductCorpus(reg *axis.Registry) []Value {
	values := []Value{Top(), Bottom(reg)}
	for _, shape := range []Shape{ShapeBottom, ShapeTop} {
		for _, p := range []presence.Value{presence.Bottom(), presence.Present(), presence.Absent(), presence.Maybe()} {
			base := NewWithPresence(reg, shape, p)
			values = append(values, base)
			for _, a := range []int{0, 1, 2, 3} {
				values = append(values, Set(reg, base, canonicalTestA, a))
				for _, b := range []int{0, 1, 2, 3} {
					values = append(values, Set(reg, Set(reg, base, canonicalTestA, a), canonicalTestB, b))
				}
			}
		}
	}
	return values
}

func canonicalProductStructuralEvents(t testing.TB, encoded []byte) (records []uint64, bools []bool) {
	t.Helper()
	for len(encoded) > 0 {
		tag := encoded[0]
		encoded = encoded[1:]
		length, n := binary.Uvarint(encoded)
		if n <= 0 || length > uint64(len(encoded)-n) {
			t.Fatalf("malformed canonical event framing")
		}
		payload := encoded[n : n+int(length)]
		encoded = encoded[n+int(length):]
		switch tag {
		case 0x03: // canonical.Writer Record wire tag.
			value, used := binary.Uvarint(payload)
			if used <= 0 || used != len(payload) {
				t.Fatalf("malformed Record payload %x", payload)
			}
			records = append(records, value)
		case 0x05: // canonical.Writer Bool wire tag.
			if len(payload) != 1 || payload[0] > 1 {
				t.Fatalf("malformed Bool payload %x", payload)
			}
			bools = append(bools, payload[0] == 1)
		}
	}
	return records, bools
}

func countUint(values []uint64, want uint64) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}

func ExampleEncodeCanonical() {
	reg, _ := RegistryWithAxes(canonicalIntSpec(canonicalTestA, "test.product.int", 1, encodeCanonicalTestInt).Erase())
	encoded, schema, _ := EncodeCanonical(context.Background(), reg, Set(reg, Top(), canonicalTestA, 1))
	fmt.Println(len(encoded) > 0, schema != (axis.SchemaIdentity{}))
	// Output: true true
}

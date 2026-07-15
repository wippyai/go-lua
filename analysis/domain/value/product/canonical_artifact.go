package product

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

var ErrCanonicalMaterializationUnavailable = errors.New("product: canonical materialization unavailable")

// CanonicalArtifact is an ownership-isolated, schema-fenced product value. It
// retains canonical bytes only: no product node, axis payload, registry, or
// caller-owned type graph crosses this boundary.
type CanonicalArtifact struct {
	encoded []byte
	schema  axis.SchemaIdentity
}

func (a CanonicalArtifact) Valid() bool {
	return len(a.encoded) != 0 && a.schema != (axis.SchemaIdentity{})
}

func (a CanonicalArtifact) Bytes() []byte { return append([]byte(nil), a.encoded...) }

func (a CanonicalArtifact) SchemaIdentity() axis.SchemaIdentity { return a.schema }

// SealCanonical transactionally proves that value both encodes and
// materializes exactly under reg. The artifact remains zero on every failure.
func SealCanonical(ctx context.Context, reg *axis.Registry, value Value) (CanonicalArtifact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	encoded, schema, err := EncodeCanonical(ctx, reg, value)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	materialized, err := DecodeCanonical(ctx, reg, encoded, schema)
	if err != nil {
		return CanonicalArtifact{}, err
	}
	if !Equal(reg, value, materialized) {
		return CanonicalArtifact{}, fmt.Errorf("%w: roundtrip changed product equality", ErrCanonicalMaterializationUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return CanonicalArtifact{}, err
	}
	return CanonicalArtifact{encoded: encoded, schema: schema}, nil
}

// Materialize returns a fresh product reconstruction under the exact registry
// schema named by the artifact.
func (a CanonicalArtifact) Materialize(ctx context.Context, reg *axis.Registry) (Value, error) {
	if !a.Valid() {
		return Value{}, fmt.Errorf("%w: zero artifact", ErrCanonicalMaterializationUnavailable)
	}
	return DecodeCanonical(ctx, reg, a.encoded, a.schema)
}

// DecodeCanonical reconstructs one product from its exact canonical encoding.
// It accepts no schema substitution and requires encode/decode/encode byte
// identity before returning the value.
func DecodeCanonical(ctx context.Context, reg *axis.Registry, encoded []byte, schema axis.SchemaIdentity) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Value{}, err
	}
	rt := runtimeFor(reg)
	if rt.err != nil {
		return Value{}, rt.err
	}
	codec := rt.canonicalValueCodec()
	if codec.err != nil {
		return Value{}, codec.err
	}
	if schema == (axis.SchemaIdentity{}) || schema != codec.authority {
		return Value{}, fmt.Errorf("%w: registry schema mismatch", ErrCanonicalMaterializationUnavailable)
	}
	if !codec.presence.CanonicalDecodeReady() {
		return Value{}, fmt.Errorf("%w: axis %q has no decoder", ErrCanonicalMaterializationUnavailable, codec.presence.ID())
	}
	for _, info := range codec.sparse {
		if !info.spec.CanonicalDecodeReady() {
			return Value{}, fmt.Errorf("%w: axis %q has no decoder", ErrCanonicalMaterializationUnavailable, info.id)
		}
	}

	var reader canonical.Reader
	if err := reader.Reset(ctx, encoded, canonicalProductDomain, canonicalProductVersion); err != nil {
		return Value{}, err
	}
	record, err := reader.Record()
	if err != nil || record != canonicalProductRecord {
		return Value{}, canonicalProductDecodeError("product record", err)
	}
	authority, err := reader.Bytes()
	if err != nil || !bytes.Equal(authority, codec.authority[:]) {
		return Value{}, canonicalProductDecodeError("schema authority", err)
	}
	rawShape, err := reader.Uint()
	if err != nil || rawShape > uint64(ShapeTop) {
		return Value{}, canonicalProductDecodeError("shape", err)
	}
	record, err = reader.Record()
	if err != nil || record != canonicalPresenceRecord {
		return Value{}, canonicalProductDecodeError("presence record", err)
	}
	presenceAny, err := codec.presence.DecodeCanonicalAny(ctx, &reader)
	if err != nil {
		return Value{}, err
	}
	p, ok := presenceAny.(presence.Value)
	if !ok {
		return Value{}, fmt.Errorf("%w: presence decoder returned %T", ErrCanonicalMaterializationUnavailable, presenceAny)
	}
	if !validPresence(p) {
		return Value{}, fmt.Errorf("%w: decoded invalid product presence", ErrCanonicalMaterializationUnavailable)
	}
	count, err := reader.Count()
	if err != nil || count != uint64(len(codec.sparse)) {
		return Value{}, canonicalProductDecodeError("sparse axis count", err)
	}
	slots := make([]slot, 0, min(len(codec.sparse), reader.RemainingBytes()/4))
	for _, info := range codec.sparse {
		if err := ctx.Err(); err != nil {
			return Value{}, err
		}
		record, err := reader.Record()
		if err != nil || record != canonicalSparseRecord {
			return Value{}, canonicalProductDecodeError("sparse axis record", err)
		}
		id, err := reader.String()
		if err != nil || id != info.id {
			return Value{}, canonicalProductDecodeError("sparse axis identity", err)
		}
		present, err := reader.Bool()
		if err != nil {
			return Value{}, err
		}
		if !present {
			continue
		}
		payload, err := info.spec.DecodeCanonicalAny(ctx, &reader)
		if err != nil {
			return Value{}, fmt.Errorf("product: decode canonical axis %q: %w", info.id, err)
		}
		if reflect.TypeOf(payload) != info.topType || info.spec.IsTopAny(payload) {
			return Value{}, fmt.Errorf("%w: axis %q decoded an invalid or explicit Top payload", ErrCanonicalMaterializationUnavailable, info.id)
		}
		slots = append(slots, slot{ordinal: info.ordinal, value: payload})
	}
	if err := reader.Finish(); err != nil {
		return Value{}, err
	}
	value := internRuntime(rt, Shape(rawShape), p, slots)
	roundTrip, roundTripSchema, err := EncodeCanonical(ctx, reg, value)
	if err != nil {
		return Value{}, err
	}
	if roundTripSchema != schema || !bytes.Equal(roundTrip, encoded) {
		return Value{}, fmt.Errorf("%w: reconstructed product changed canonical bytes", ErrCanonicalMaterializationUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return Value{}, err
	}
	return value, nil
}

func canonicalProductDecodeError(field string, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrCanonicalMaterializationUnavailable, field, err)
	}
	return fmt.Errorf("%w: %s", ErrCanonicalMaterializationUnavailable, field)
}

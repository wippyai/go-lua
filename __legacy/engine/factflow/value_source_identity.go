package factflow

import (
	"context"
	"crypto/sha256"
	"errors"
	"reflect"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

const (
	canonicalValueSourceDomain  = "analysis.factflow.value-source"
	canonicalValueSourceVersion = 1
	canonicalValueSourceRecord  = 1
	canonicalValueSourceFields  = 20
)

var errCanonicalValueSourceSchemaStale = errors.New("factflow: canonical ValueSource schema is stale")

// ValueSourceContentID is the canonical full-width identity of one exact
// source descriptor. The codec lives beside ValueSource so additions to that
// DTO have one identity authority rather than producer-specific field mirrors.
type ValueSourceContentID [sha256.Size]byte

func (id ValueSourceContentID) Available() bool { return id != ValueSourceContentID{} }

// CanonicalValueSourceContentID derives the exact source identity used by
// immutable operation artifacts.
func CanonicalValueSourceContentID(ctx context.Context, source ValueSource) (ValueSourceContentID, error) {
	// Fail closed if ValueSource grows before this owner updates and versions its
	// codec. This prevents a newly added semantic field from being silently
	// absent from artifact identity.
	if reflect.TypeOf(source).NumField() != canonicalValueSourceFields {
		return ValueSourceContentID{}, errCanonicalValueSourceSchemaStale
	}
	hash := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(ctx, hash, canonicalValueSourceDomain, canonicalValueSourceVersion); err != nil {
		return ValueSourceContentID{}, err
	}
	if err := writer.Record(canonicalValueSourceRecord); err != nil {
		return ValueSourceContentID{}, err
	}
	events := []func() error{
		func() error { return writer.Uint(uint64(source.Kind)) },
		func() error { return writer.Uint(uint64(source.ExprRef)) },
		func() error { return writer.Bool(source.HasExpr) },
		func() error { return writer.Uint(uint64(source.SourcePoint)) },
		func() error { return writer.Bool(source.HasSourcePoint) },
		func() error { return writer.Int(int64(source.ExprIndex)) },
		func() error { return writer.Int(int64(source.TargetIndex)) },
		func() error { return writer.Int(int64(source.ResultIndex)) },
		func() error { return writer.Uint(uint64(source.CallPoint)) },
		func() error { return writer.Bool(source.HasCallPoint) },
		func() error { return writer.String(string(source.PathKey)) },
		func() error { return writer.Uint(uint64(source.LiteralKind)) },
		func() error { return writer.Bool(source.Bool) },
		func() error { return writer.Int(source.Int) },
		func() error { return writer.Float64(source.Float) },
		func() error { return writer.String(source.String) },
		func() error { return writer.Bool(source.Final) },
		func() error { return writer.Bool(source.Expanded) },
		func() error { return writer.Bool(source.Adjusted) },
		func() error { return writer.Bool(source.OpenTail) },
	}
	for _, event := range events {
		if err := event(); err != nil {
			return ValueSourceContentID{}, err
		}
	}
	if err := writer.Finish(); err != nil {
		return ValueSourceContentID{}, err
	}
	var out ValueSourceContentID
	copy(out[:], hash.Sum(nil))
	return out, nil
}

// ValueSourceEqual is the exact equality authority paired with the canonical
// identity. If ValueSource ever becomes non-comparable this fails at its owner,
// forcing an explicit equality update.
func ValueSourceEqual(left, right ValueSource) bool { return left == right }

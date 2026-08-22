package subtype

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// channelDeclaration is a generic whose body never mentions its parameter.
// Expanding such a declaration erases the argument entirely, so expansion can
// never stand in for the invariant application rule.
func channelDeclaration() *typ.Generic {
	return typ.NewGeneric("Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("Channel", nil))
}

func decodedChannel(t *testing.T, padding int) *typ.Generic {
	t.Helper()
	var payload typ.Type = channelDeclaration()
	for i := 0; i < padding; i++ {
		payload = typ.NewTuple(typ.String, payload)
	}
	receipt, err := typ.EncodeCanonicalFormals(context.Background(), payload, nil)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals: %v", err)
	}
	decoded, err := typ.DecodeCanonicalFormals(context.Background(), receipt, nil)
	if err != nil {
		t.Fatalf("DecodeCanonicalFormals: %v", err)
	}
	for i := 0; i < padding; i++ {
		tuple, ok := decoded.(*typ.Tuple)
		if !ok || len(tuple.Elements) != 2 {
			t.Fatalf("padding %d unwrapped to %T", i, decoded)
		}
		decoded = tuple.Elements[1]
	}
	generic, ok := decoded.(*typ.Generic)
	if !ok {
		t.Fatalf("decoded %T, want *typ.Generic", decoded)
	}
	return generic
}

// TestGenericApplicationIsInvariantInItsArgument states container invariance
// for a declaration whose body does not mention its parameter. Identity of the
// application is the pair (declaration, arguments); the argument is neither
// covariant nor erasable.
func TestGenericApplicationIsInvariantInItsArgument(t *testing.T) {
	message := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "payload", Type: typ.String}}})
	instant := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "unix", Type: typ.Integer}}})

	cases := []struct {
		name        string
		sub, super  typ.Type
		wantSubtype bool
	}{
		{"instant-into-any", typ.Instantiate(channelDeclaration(), instant), typ.Instantiate(channelDeclaration(), typ.Any), false},
		{"instant-into-message", typ.Instantiate(channelDeclaration(), instant), typ.Instantiate(channelDeclaration(), message), false},
		{"any-into-instant", typ.Instantiate(channelDeclaration(), typ.Any), typ.Instantiate(channelDeclaration(), instant), false},
		{"instant-into-instant", typ.Instantiate(channelDeclaration(), instant), typ.Instantiate(channelDeclaration(), instant), true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := IsSubtype(test.sub, test.super); got != test.wantSubtype {
				t.Fatalf("IsSubtype = %v, want %v", got, test.wantSubtype)
			}
		})
	}
}

// TestGenericApplicationInvarianceSurvivesDecodePosition states the same law
// across the artifact boundary: the two applications name one declaration
// decoded at two graph positions, so their arguments still have to match.
func TestGenericApplicationInvarianceSurvivesDecodePosition(t *testing.T) {
	instant := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "unix", Type: typ.Integer}}})
	flat := decodedChannel(t, 0)
	shifted := decodedChannel(t, 3)

	if IsSubtype(typ.Instantiate(flat, instant), typ.Instantiate(shifted, typ.Any)) {
		t.Fatalf("Channel<instant> admitted as Channel<any> across two decodes of one declaration")
	}
	if !IsSubtype(typ.Instantiate(flat, instant), typ.Instantiate(shifted, instant)) {
		t.Fatalf("Channel<instant> refused against itself across two decodes of one declaration")
	}
}

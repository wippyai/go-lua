package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestClosedDiscriminantDomain_StringTags(t *testing.T) {
	event := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.LiteralString("message")).Field("text", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("tool")).Field("name", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("timeout")).Field("at", typ.Number).Build(),
	)

	domain, ok := ClosedDiscriminantDomain(event, "kind")
	if !ok {
		t.Fatal("expected closed discriminant domain")
	}
	missing := domain.Missing([]*typ.Literal{typ.LiteralString("message"), typ.LiteralString("tool")})
	if len(missing) != 1 || !typ.LiteralEquals(missing[0], typ.LiteralString("timeout")) {
		t.Fatalf("missing=%v, want timeout", missing)
	}
}

func TestClosedDiscriminantDomain_RejectsBroadTag(t *testing.T) {
	event := typ.NewUnion(
		typ.NewRecord().Field("kind", typ.String).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("tool")).Build(),
	)

	if _, ok := ClosedDiscriminantDomain(event, "kind"); ok {
		t.Fatal("broad string tag must keep the domain open")
	}
}

func TestClosedDiscriminantDomain_RejectsOptionalTag(t *testing.T) {
	event := typ.NewUnion(
		typ.NewRecord().OptField("kind", typ.LiteralString("message")).Build(),
		typ.NewRecord().Field("kind", typ.LiteralString("tool")).Build(),
	)

	if _, ok := ClosedDiscriminantDomain(event, "kind"); ok {
		t.Fatal("optional discriminant must keep the domain open")
	}
}

func TestClosedDiscriminantDomain_NumberTags(t *testing.T) {
	event := typ.NewUnion(
		typ.NewRecord().Field("case", typ.LiteralInt(1)).Build(),
		typ.NewRecord().Field("case", typ.LiteralInt(2)).Build(),
		typ.NewRecord().Field("case", typ.LiteralInt(3)).Build(),
	)

	domain, ok := ClosedDiscriminantDomain(event, "case")
	if !ok {
		t.Fatal("expected closed numeric discriminant domain")
	}
	if !domain.Contains(typ.LiteralInt(2)) {
		t.Fatal("expected domain to contain numeric tag 2")
	}
}

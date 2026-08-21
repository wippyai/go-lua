package digest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

func TestDigestUsesFramedFieldKinds(t *testing.T) {
	zero := identity.ContentID{}
	left := Digest("artifact/test/digest", 1, ContentID(zero), Uint(1))
	right := Digest("artifact/test/digest", 1, ContentID(zero), Uint(2))
	if !left.Available() || !right.Available() || left == right {
		t.Fatal("digest did not commit the scalar field value")
	}
	if got := Digest("artifact/test/digest", 1, Field{}); got.Available() {
		t.Fatal("digest admitted an unknown field kind")
	}
}

func TestSequentialDigestsMatchIndependentWriters(t *testing.T) {
	zero := identity.ContentID{}
	wantLeft := Digest("artifact/test/digest", 1, ContentID(zero), Uint(1))
	wantRight := Digest("artifact/test/digest", 1, ContentID(zero), Uint(2))
	if !wantLeft.Available() || !wantRight.Available() || wantLeft == wantRight {
		t.Fatal("independent digests were not distinct")
	}
	for index := 0; index < 8; index++ {
		gotLeft := Digest("artifact/test/digest", 1, ContentID(zero), Uint(1))
		gotRight := Digest("artifact/test/digest", 1, ContentID(zero), Uint(2))
		if gotLeft != wantLeft {
			t.Fatalf("left digest %d diverged from the independent writer", index)
		}
		if gotRight != wantRight {
			t.Fatalf("right digest %d diverged from the independent writer", index)
		}
	}
}

func TestStreamingSinkMatchesLegacyTaggedPreimage(t *testing.T) {
	zero := identity.ContentID{}
	sink := NewSink("artifact/test/legacy", 7)
	sink.Add(ContentID(zero), String("payload"), Uint(11), Bool(true), Bool(false))
	got := sink.Sum()
	const legacyIdentity = "97b28bd874e10187334527d13db16c283b3cce30d01d3185dea1046d45ab9390"
	if got.String() != legacyIdentity {
		t.Fatalf("streaming digest changed legacy identity: got %s want %s", got, legacyIdentity)
	}

	var writer canonical.DigestWriter
	if err := writer.Reset("artifact/test/legacy", 7); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes(zero[:]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(11); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(0); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	want := identity.ContentID(writer.Sum())
	if got != want {
		t.Fatalf("streaming digest changed tagged preimage: got %v want %v", got, want)
	}
}

func TestSinkFailsClosedAfterInvalidField(t *testing.T) {
	sink := NewSink("artifact/test/fail-closed", 1)
	sink.Add(Field{})
	if sink.Available() || sink.Sum().Available() {
		t.Fatal("invalid field reopened or admitted a failed sink")
	}
	sink.Add(Uint(1))
	if sink.Sum().Available() {
		t.Fatal("failed sink accepted a later field")
	}
}

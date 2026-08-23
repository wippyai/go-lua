package testfixture

import "testing"

func TestStandardLibraryTargetPublishesQualifiedTypeIndex(t *testing.T) {
	target, err := StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal StandardLibraryTarget: %v", err)
	}
	stream, ok := target.Types().Lookup("stream.Stream")
	if !ok || stream == 0 {
		t.Fatal("stream.Stream was not published by the canonical Target")
	}
	for _, missing := range []string{"stream.Streamer", "other.Stream"} {
		if _, ok := target.Types().Lookup(missing); ok {
			t.Fatalf("missing qualified type %q unexpectedly resolved", missing)
		}
	}
	name, enumerated, ok := target.Types().At(0)
	if !ok || name == "" || enumerated == 0 {
		t.Fatalf("qualified type enumeration returned %q/%d/%v", name, enumerated, ok)
	}
}

package testfixture

import "testing"

// The public fixture deliberately seeds its input relations through the same
// Apply -> Publish door used by production. Its zero-input seed signatures
// therefore retain direct Publish specimens in the sealed schema: those
// specimens, rather than an Ascending capability advertisement or a terminal
// signature alone, are the authority that admits the value algebra below.
func TestDirectFixtureSeedsHavePublishedAscentAuthority(t *testing.T) {
	fixture := New(t, 0x6d)
	if len(fixture.seedAscent) != 1 || fixture.seedAscent[0] != fixture.seedType {
		t.Fatalf("fixture seed algebra requirements = %+v, want its one published seed TypeID", fixture.seedAscent)
	}
	if algebra, ok := fixture.Mounted().Algebra(fixture.seedType); !ok || algebra == nil || algebra.Type() != fixture.seedType {
		t.Fatal("fixture direct seeds did not mount their schema-certified ascent algebra")
	}
}

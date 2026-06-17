package hash

import (
	"hash/fnv"
	"testing"
)

func TestFnvString(t *testing.T) {
	t.Parallel()

	if got := FnvString(""); got != 14695981039346656037 {
		t.Fatalf("empty string hash = %d, want FNV offset", got)
	}

	hash1 := FnvString("hello")
	hash2 := FnvString("hello")

	if hash1 != hash2 {
		t.Error("same string should produce same hash")
	}

	hash3 := FnvString("world")
	if hash1 == hash3 {
		t.Error("different strings should produce different hashes")
	}
}

func TestFnvStringMatchesStandardLibraryFNV1a64(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"",
		"a",
		"hello",
		"number:string:boolean",
		"nul\x00byte",
		"unicode:\xe2\x98\x83",
	} {
		hasher := fnv.New64a()
		if _, err := hasher.Write([]byte(input)); err != nil {
			t.Fatalf("fnv.Write(%q) failed: %v", input, err)
		}
		if got, want := FnvString(input), hasher.Sum64(); got != want {
			t.Fatalf("FnvString(%q) = %d, want standard FNV-1a %d", input, got, want)
		}
	}
}

func TestFnvStringUniqueness(t *testing.T) {
	t.Parallel()

	strings := []string{
		"", "a", "b", "ab", "ba",
		"hello", "world", "foo", "bar",
		"number", "string", "boolean",
	}

	hashes := make(map[uint64]string)

	for _, str := range strings {
		hashValue := FnvString(str)
		if existing, ok := hashes[hashValue]; ok {
			t.Errorf("hash collision: %q and %q", existing, str)
		}

		hashes[hashValue] = str
	}
}

func TestMixHash(t *testing.T) {
	t.Parallel()

	hashResult := MixHash(1, 2)
	if hashResult == 1 || hashResult == 2 {
		t.Error("MixHash should transform input")
	}

	hash1 := MixHash(100, 200)
	hash2 := MixHash(100, 200)

	if hash1 != hash2 {
		t.Error("MixHash should be deterministic")
	}

	if MixHash(1, 2) == MixHash(1, 3) {
		t.Error("different inputs should produce different outputs")
	}
}

func TestMixHashMatchesOneFNV1aUint64Step(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		seed  uint64
		value uint64
	}{
		{name: "zero", seed: 0, value: 0},
		{name: "offset", seed: fnvOffset64, value: 1},
		{name: "max", seed: ^uint64(0), value: ^uint64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := (tc.seed ^ tc.value) * fnvPrime64
			if got := MixHash(tc.seed, tc.value); got != want {
				t.Fatalf("MixHash(%d, %d) = %d, want %d", tc.seed, tc.value, got, want)
			}
		})
	}
}

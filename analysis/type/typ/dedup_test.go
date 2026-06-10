package typ

import "testing"

func TestDeduplicateTypesWithHashes_Empty(t *testing.T) {
	result, hashes := deduplicateTypesWithHashes(nil)
	if result != nil || hashes != nil {
		t.Error("deduplicateTypesWithHashes(nil) should return nil slices")
	}

	result, hashes = deduplicateTypesWithHashes([]Type{})
	if result != nil || hashes != nil {
		t.Error("deduplicateTypesWithHashes([]) should return nil slices")
	}
}

func TestDeduplicateTypesWithHashes_NoDuplicates(t *testing.T) {
	types := []Type{String, Number, Boolean}
	result, hashes := deduplicateTypesWithHashes(types)
	if len(result) != 3 || len(hashes) != 3 {
		t.Errorf("len = %d/%d, want 3/3", len(result), len(hashes))
	}
}

func TestDeduplicateTypesWithHashes_WithDuplicates(t *testing.T) {
	types := []Type{String, Number, String, Number, Boolean}
	result, hashes := deduplicateTypesWithHashes(types)
	if len(result) != 3 || len(hashes) != 3 {
		t.Errorf("len = %d/%d, want 3/3", len(result), len(hashes))
	}
}

func TestDeduplicateTypesWithHashes_AllSame(t *testing.T) {
	types := []Type{String, String, String}
	result, hashes := deduplicateTypesWithHashes(types)
	if len(result) != 1 || len(hashes) != 1 {
		t.Errorf("len = %d/%d, want 1/1", len(result), len(hashes))
	}
}

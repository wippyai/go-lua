package engine_test

import "testing"

// TestStaticIndexStoreRevokesTheLengthFloor pins the sequence effect of a store
// written with a literal key. The guard establishes a floor over the container,
// the store clears the slot the floor was measured over, and the read at the
// captured length is therefore no longer proven occupied. Without the
// revocation the floor outlives the store and the read is accepted while it is
// nil at run time.
func TestStaticIndexStoreRevokesTheLengthFloor(t *testing.T) {
	diagnostics := checkSource(t, `local function f(xs: {string}): string
    if #xs >= 1 then
        local n = #xs
        xs[1] = nil
        local v: string = xs[n]
        return v
    end
    return ""
end
return f
`)
	if len(diagnostics) == 0 {
		t.Fatal("the length floor survived a store at a literal index")
	}
}

// TestLiteralAndVariableIndexStoresAgree pins that the key's spelling decides
// nothing: the same store written through a variable already revoked, and the
// literal one states the same effect on the same sequence.
func TestLiteralAndVariableIndexStoresAgree(t *testing.T) {
	diagnostics := checkSource(t, `local function f(xs: {string}): string
    if #xs >= 1 then
        local n = #xs
        local k = 1
        xs[k] = nil
        local v: string = xs[n]
        return v
    end
    return ""
end
return f
`)
	if len(diagnostics) == 0 {
		t.Fatal("the length floor survived a store at a variable index")
	}
}

// TestStaticIndexStoreLeavesOtherContainersProven pins the revocation's scope.
// It names the container the index addresses, so a store into a different one
// leaves this container's floor exactly where it was.
func TestStaticIndexStoreLeavesOtherContainersProven(t *testing.T) {
	diagnostics := checkSource(t, `local function f(xs: {string}, ys: {string}): string
    if #xs >= 1 then
        local n = #xs
        ys[1] = nil
        local v: string = xs[n]
        return v
    end
    return ""
end
return f
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a store into another container revoked this one's floor:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestNamedSlotStoreMovesNoBorder pins the other side of the structural test: a
// store at a named slot occupies no sequence position, so it moves no border
// and revokes nothing.
func TestNamedSlotStoreMovesNoBorder(t *testing.T) {
	diagnostics := checkSource(t, `local function f(xs: {string}, box: {tag: string}): string
    if #xs >= 1 then
        local n = #xs
        box.tag = "x"
        local v: string = xs[n]
        return v
    end
    return ""
end
return f
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a store at a named slot revoked a sequence proof:\n%s", diagnosticSummaries(diagnostics))
	}
}

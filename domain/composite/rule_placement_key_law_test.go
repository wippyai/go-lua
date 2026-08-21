package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
)

// A compiled artifact carries no rule catalog of its own. Every placement it
// issues names the rule that issued it by the key that rule is declared under,
// so the carried key is the sole authority a consumer resolves through, and the
// sealed declaration table is the sole place it resolves.
//
// This law states that the two meet: for every placement of every mounted
// artifact, the carried key names a declaration the table holds, and that
// declaration is on the mounted lane. A placement whose key the table does not
// declare, or which reaches a Link-owned declaration, is rejected here and named
// by its issuance position, so a key the artifact invents cannot reach a
// projection through the resolution the composition performs.
//
// The companion adoption-position law pins the table's own declaration order.
// Together they close the circle: that law fixes which key sits at which
// position, this one fixes that every artifact-issued key is one of them.

// placementKeyAgreement states one artifact's agreement with a rule inventory
// and names the first placement that disagrees. The inventory is given as the
// declared keys in table order, so the law reads the same over the production
// table and over a copy one of whose members has been displaced.
func placementKeyAgreement(compilation Compilation, artifact *ingress.Snapshot, keys []schema.Key) (int, schema.Key, bool) {
	program := artifact.Program()
	count, published := program.RuleOccurrenceCount()
	if !published {
		return 0, "", false
	}
	for index := 0; index < count; index++ {
		row, ok := program.RuleOccurrenceAt(index)
		if !ok {
			return index, "", false
		}
		if !inventoryDeclares(keys, row.Key()) || !MountedRuleKey(compilation, row.Key()) {
			return index, row.Key(), false
		}
	}
	return 0, "", true
}

// inventoryDeclares reports whether one declared-key inventory holds key.
func inventoryDeclares(keys []schema.Key, key schema.Key) bool {
	for _, declared := range keys {
		if declared == key {
			return true
		}
	}
	return false
}

// placementKeySources are the fixtures the law is stated over. Each one is
// compiled through the real seal chain, and between them they issue the value,
// pack, heap, call, and effect lanes, so the inventory the law walks is the
// whole mounted catalog rather than one lane of it.
func placementKeySources() []struct{ name, source string } {
	return []struct{ name, source string }{
		{"records", `
local child = { value = 1 }
local record = { child = child, name = "leaf" }
record.child.value = record.child.value + 1
local read = record.child
return read, record.name
`},
		{"calls", `
local function pair(left, right) return left, right end
local function apply(fn, value) return fn(value, value) end
local first, second = apply(pair, 7)
local held = { first = first, second = second }
return held
`},
		{"scalars", `
local function classify(value)
  if value == nil then return "absent" end
  if value < 10 then return "small" end
  return "large"
end
local total = 3 * 4 - 1
return classify(total), classify(nil), total
`},
	}
}

// placementKeyArtifacts compiles every fixture through the mount phase and
// returns each mounted artifact. The artifacts are the phase's own output, so
// the placements the law reads are the ones a consumer of the analyzer receives.
func placementKeyArtifacts(t *testing.T) []*ingress.Snapshot {
	t.Helper()
	var artifacts []*ingress.Snapshot
	for _, fixture := range placementKeySources() {
		record := mountedRecord(t, fixture.name, fixture.source)
		if len(record.Artifacts) == 0 {
			t.Fatalf("fixture %q mounted no artifact", fixture.name)
		}
		for index, row := range record.Artifacts {
			if row.Snapshot == nil || !row.Snapshot.Available() {
				t.Fatalf("fixture %q mount %d carries no sealed snapshot", fixture.name, index)
			}
			artifacts = append(artifacts, row.Snapshot)
		}
	}
	return artifacts
}

// TestEveryRulePlacementCarriesItsSealedTableKey states the law over the real
// path, and states it as a law rather than as an observation: an inventory one
// of whose members has been displaced is rejected, naming the first placement
// that disagrees.
func TestEveryRulePlacementCarriesItsSealedTableKey(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	keys := sealedRuleKeys(t, compilation)
	artifacts := placementKeyArtifacts(t)
	placements := 0
	for _, artifact := range artifacts {
		program := artifact.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("cold rule-occurrence family")
		}
		placements += count
		if index, blamed, agreed := placementKeyAgreement(compilation, artifact, keys); !agreed {
			t.Fatalf("placement %d carries %q, which the sealed table does not declare on the mounted lane", index, blamed)
		}
	}
	if placements == 0 {
		t.Fatal("the fixtures issued no placement, so the law states nothing")
	}
	// The inventory is what the law resolves through, so displacing one of its
	// members must reject every placement the displaced rule issued, and must
	// blame the displaced member rather than a later one. A displacement no
	// fixture issues is silent by construction, so the detection is only proven
	// once a displacement has actually been caught.
	detected := 0
	for position := range keys {
		displaced := append([]schema.Key(nil), keys...)
		displaced[position] = displaced[position] + "/displaced"
		for _, artifact := range artifacts {
			index, blamed, agreed := placementKeyAgreement(compilation, artifact, displaced)
			if agreed {
				continue
			}
			if blamed != keys[position] {
				t.Fatalf("displacing %q was blamed on %q at placement %d, not on the first placement that disagrees", keys[position], blamed, index)
			}
			detected++
			break
		}
	}
	if detected == 0 {
		t.Fatal("no displaced inventory member was caught, so the law detects nothing")
	}
}

// TestRulePlacementAtWalksDeclarationKeyOrder states that issuance order is
// the sealed table's mounted-key order: first-seen placement keys are a
// subsequence of the mounted declarations, and a later key never precedes an
// earlier table member.
func TestRulePlacementAtWalksDeclarationKeyOrder(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	keys := sealedRuleKeys(t, compilation)
	var mounted []schema.Key
	for _, key := range keys {
		if MountedRuleKey(compilation, key) {
			mounted = append(mounted, key)
		}
	}
	if len(mounted) == 0 {
		t.Fatal("the table declares no mounted key")
	}
	position := make(map[schema.Key]int, len(mounted))
	for index, key := range mounted {
		position[key] = index
	}
	seen := 0
	for _, artifact := range placementKeyArtifacts(t) {
		last := -1
		program := artifact.Program()
		count, published := program.RuleOccurrenceCount()
		if !published {
			t.Fatal("cold rule-occurrence family")
		}
		for index := 0; index < count; index++ {
			row, ok := program.RuleOccurrenceAt(index)
			if !ok {
				t.Fatalf("placement %d unavailable", index)
			}
			at, known := position[row.Key()]
			if !known {
				t.Fatalf("placement %d carries %q, outside the mounted table order", index, row.Key())
			}
			if at < last {
				t.Fatalf("placement %d carries %q after a later table key", index, row.Key())
			}
			last = at
			seen++
		}
	}
	if seen == 0 {
		t.Fatal("the fixtures issued no placement")
	}
}

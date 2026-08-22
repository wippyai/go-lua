package oracle

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// TestZZCountDump prints the live corpus census so the pinned denominators can
// be set to the tree's actual values. Scratch instrumentation.
func TestZZCountDump(t *testing.T) {
	catalog, err := frozenCorpusDiagnosticExpectationCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	inv := catalog.inventory
	t.Logf("projects=%d luaFiles=%d manifests=%d annotatedFiles=%d", inv.projects, inv.luaFiles, inv.manifests, inv.annotatedFiles)
	t.Logf("inlineErrors=%d inlineWarnings=%d", inv.inlineErrors, inv.inlineWarnings)
	t.Logf("structuredManifests=%d structuredFindings=%d structuredErrors=%d structuredWarnings=%d structuredHints=%d",
		inv.structuredManifests, inv.structuredFindings, inv.structuredErrors, inv.structuredWarnings, inv.structuredHints)
	t.Logf("errorCountManifests=%d errorCounts=%v", inv.errorCountManifests, inv.errorCounts)
	t.Logf("ruleCount=%d enabled=%d disabled=%d", inv.ruleCount, inv.enabledRuleCount, inv.disabledRuleCount)
	t.Logf("manifestContract={%d, %d, %d, %d, %d, %d, %d, %d, %d, %d}",
		inv.declaredFileManifests, inv.declaredFiles, inv.packageManifests, inv.packageDeclarations,
		inv.stdlibManifests, inv.renderOptionManifests, inv.nativeManifests, inv.nativeFacts,
		inv.nativeInvalidations, inv.placementManifests)
	indexedInline := 0
	for _, rows := range catalog.inlineByLocation {
		indexedInline += len(rows)
	}
	indexedStructured := 0
	for _, refs := range catalog.structuredByCode {
		indexedStructured += len(refs)
	}
	indexedLocations := 0
	for _, refs := range catalog.structuredByLocation {
		indexedLocations += len(refs)
	}
	t.Logf("catalogProjects=%d indexedInline=%d codes=%d indexedStructured=%d byLocation=%d indexedLocations=%d",
		len(catalog.projects), indexedInline, len(catalog.structuredByCode), indexedStructured, len(catalog.structuredByLocation), indexedLocations)
	t.Logf("structuredCodes:\n%s", dumpZZCounts(inv.structuredCodes))
	t.Logf("ruleCodes:\n%s", dumpZZCounts(inv.ruleCodes))
	counts, err := corpusDiagnosticSupportCensus(catalog)
	if err != nil {
		t.Logf("supportCensus error: %v", err)
	} else {
		t.Logf("supportCensus=%+v", counts)
	}
}

func dumpZZCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, fmt.Sprintf("\t\t%q: %d,", key, counts[key]))
	}
	return strings.Join(rows, "\n")
}

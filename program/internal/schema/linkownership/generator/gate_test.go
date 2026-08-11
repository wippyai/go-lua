package generator

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func validManifestFiles() ManifestFiles {
	splitPlan := splitPlanID([]string{"owner-a", "owner-b"})
	return ManifestFiles{
		Catalog: []byte("catalog.schema\tv2\n" +
			"decl\tdecl-a\texample.test/program/link\tfunc\towner-a\tsurface-a\tChoose\tfunc(value int) string\tfunc(value int) string\n" +
			"owner\towner-a\texample.test/program/link\tsurface-a\tcomponent\n" +
			"owner\towner-b\texample.test/caller\tpackage:example.test/caller\tcomponent\n" +
			"ownership-import-edge\towner-a\towner-b\tprogram/link/link.go\t1\t1\n" +
			"use\tuse-a\texample.test/caller\tcaller/caller.go\t2\t1\tChoose\tselector\tcall-callee\tdecl-a\tfunc\n"),
		Indexes: []byte("indexes.schema\tv2\n" +
			"cold-ref\tref-c\towner-a\towner-b\tdecl-a\tfact-a\tpattern:cold\tuse-a\t\n" +
			"contextual-ref\tref-x\towner-a\towner-b\tdecl-a\tfact-a\tpattern:context\tuse-a\t\n" +
			"hot-ref\tref-h\towner-a\towner-b\tdecl-a\tfact-a\tpattern:hot\tuse-a\t\n" +
			"identity\tidentity-v2-test\towner-a\tdecl-a\tv1:direct\tfact-a\t\tpattern:identity\n" +
			"identity-plan\tidentity-plan-test\towner-a\tdecl-a\tv1:direct\tfact-a\t\n" +
			"index\tindex-v2-test\towner-a\tdecl-a\tuse-a\tfact-a\tpattern:index\t\n" +
			"index-plan\tindex-plan-test\towner-a\tdecl-a\tfact-a\tuse-a\n" +
			"reference-plan\treference-plan-test\towner-a\towner-b\tdecl-a\tfact-a\tuse-a\n"),
		Surfaces: []byte("surfaces.schema\tv2\n" +
			"effective-method\tfact-method\tsurface-a\tDo\n" +
			"field\tfact-field\tsurface-a\tValue\n" +
			"storage\tfact-field\tsurface-a\tv1:public-surface\n"),
		Residue: []byte("residue.schema\tv2\n" +
			"delete\tfact-delete\tv1:private-representation\n" +
			"move\tfact-move\towner-a\n" +
			"split\tfact-split\t" + splitPlan + "\n" +
			"split-plan\t" + splitPlan + "\towner-a,owner-b\n"),
	}
}

func TestParseResidueDestinationsAreClosedAndOwnerJoined(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		{name: "junk move destination", from: "move\tfact-move\towner-a", to: "move\tfact-move\tjunk"},
		{name: "wrong delete destination", from: "delete\tfact-delete\tv1:private-representation", to: "delete\tfact-delete\tv1:other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := validManifestFiles()
			files.Residue = bytes.Replace(files.Residue, []byte(test.from), []byte(test.to), 1)
			if _, err := ParseManifestFiles(files); !errors.Is(err, ErrManifestParse) {
				t.Fatalf("error=%v, want ErrManifestParse", err)
			}
		})
	}
}

func TestParseSplitPlansAreCanonicalAndUsed(t *testing.T) {
	plan := splitPlanID([]string{"owner-a", "owner-b"})
	for _, test := range []struct {
		name string
		edit func([]byte) []byte
	}{
		{name: "alias for recipient set", edit: func(raw []byte) []byte {
			return bytes.ReplaceAll(raw, []byte(plan), []byte("split-plan-alias"))
		}},
		{name: "unused plan", edit: func(raw []byte) []byte {
			return bytes.Replace(raw, []byte("split\tfact-split\t"+plan+"\n"), nil, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := validManifestFiles()
			files.Residue = test.edit(files.Residue)
			if _, err := ParseManifestFiles(files); !errors.Is(err, ErrManifestParse) {
				t.Fatalf("error=%v, want ErrManifestParse", err)
			}
		})
	}
}

func TestParseManifestFilesCanonicalRoundTrip(t *testing.T) {
	want := validManifestFiles()
	parsed, err := ParseManifestFiles(want)
	if err != nil {
		t.Fatal(err)
	}
	got := parsed.CanonicalFiles()
	if !bytes.Equal(got.Catalog, want.Catalog) || !bytes.Equal(got.Indexes, want.Indexes) ||
		!bytes.Equal(got.Surfaces, want.Surfaces) || !bytes.Equal(got.Residue, want.Residue) {
		t.Fatalf("canonical bytes did not round-trip: got=%+v want=%+v", got, want)
	}
	if len(parsed.Catalog.Owners) != 2 || len(parsed.Catalog.Declarations) != 1 || len(parsed.Catalog.Uses) != 1 || len(parsed.Catalog.ImportEdges) != 1 {
		t.Fatalf("catalog typed rows incomplete: %+v", parsed.Catalog)
	}
	if parsed.Catalog.Uses[0].Role != CallCallee {
		t.Fatalf("catalog role was not parsed as the sole closed role enum: %+v", parsed.Catalog.Uses[0])
	}
	if len(parsed.Indexes.Indexes) != 1 || len(parsed.Indexes.HotReferences) != 1 || len(parsed.Indexes.ColdReferences) != 1 || len(parsed.Indexes.ContextualReferences) != 1 || len(parsed.Indexes.Identities) != 1 || len(parsed.Indexes.IndexPlans) != 1 || len(parsed.Indexes.ReferencePlans) != 1 || len(parsed.Indexes.IdentityPlans) != 1 {
		t.Fatalf("typed index families incomplete: %+v", parsed.Indexes)
	}
	index := parsed.Indexes.Indexes[0]
	if index.QueryFactID != "decl-a" || index.CallerUseFactIDs[0] != "use-a" || index.SourceFactIDs[0] != "fact-a" || index.PatternID != "pattern:index" || index.BenchmarkReceiptDigest != "" {
		t.Fatalf("typed index evidence was not retained: %+v", index)
	}
	identity := parsed.Indexes.Identities[0]
	if identity.RelationKind != IdentityRelationDirect || identity.DirectFactIDs[0] != "fact-a" {
		t.Fatalf("typed identity relation was not retained: %+v", identity)
	}
	if len(parsed.Surfaces.Assignments) != 2 || len(parsed.Surfaces.Storage) != 1 || len(parsed.Residue.Rows) != 3 || len(parsed.Residue.SplitPlans) != 1 {
		t.Fatalf("surface/residue typed rows incomplete: surfaces=%+v residue=%+v", parsed.Surfaces, parsed.Residue)
	}
	if parsed.Residue.Rows[2].Destination != splitPlanID([]string{"owner-a", "owner-b"}) || parsed.Residue.SplitPlans[0].OwnerIDs[1] != "owner-b" {
		t.Fatalf("split residue plan was not parsed as typed recipients: rows=%+v plans=%+v", parsed.Residue.Rows, parsed.Residue.SplitPlans)
	}
	got.Catalog[0] ^= 1
	if bytes.Equal(got.Catalog, want.Catalog) {
		t.Fatal("CanonicalFiles exposed mutable parser storage")
	}
}

func TestIdentityDigestCommitsTypedRelationAndParents(t *testing.T) {
	parent := IdentityRow{ID: "parent", Owner: "owner-a", DeclarationFactID: "decl-a", PatternID: "pattern:parent", RelationKind: IdentityRelationDirect, DirectFactIDs: []string{"fact-a"}}
	child := IdentityRow{ID: "child", Owner: "owner-a", DeclarationFactID: "decl-a", PatternID: "pattern:child", RelationKind: IdentityRelationComposite, DirectFactIDs: []string{"fact-b"}, ParentIdentityIDs: []string{"parent"}}
	first, err := identityDigest(child, []IdentityRow{parent, child})
	if err != nil {
		t.Fatal(err)
	}
	alt := child
	alt.ID = "different-authored-id"
	got, err := identityDigest(alt, []IdentityRow{parent, alt})
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Fatalf("identity digest accepted authored ID as an input: got=%q want=%q", got, first)
	}
	parent.PatternID = "pattern:parent-alt"
	second, err := identityDigest(child, []IdentityRow{parent, child})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("parent relation kind did not affect computed identity digest")
	}
}

func TestIdentityDigestAllowsLexicallyLaterParent(t *testing.T) {
	var parent, child IdentityRow
	found := false
	for index := 0; index < 4096 && !found; index++ {
		parent = IdentityRow{
			ID: "parent-placeholder", Owner: "owner-a", DeclarationFactID: "decl-a",
			PatternID: fmt.Sprintf("pattern:parent-%04d", index), RelationKind: IdentityRelationDirect,
			DirectFactIDs: []string{"fact-parent"},
		}
		parentID, err := identityDigest(parent, []IdentityRow{parent})
		if err != nil {
			t.Fatal(err)
		}
		parent.ID = parentID
		child = IdentityRow{
			ID: "child-placeholder", Owner: "owner-a", DeclarationFactID: "decl-a",
			PatternID: fmt.Sprintf("pattern:child-%04d", index), RelationKind: IdentityRelationComposite,
			DirectFactIDs: []string{"fact-child"}, ParentIdentityIDs: []string{parent.ID},
		}
		childID, err := identityDigest(child, []IdentityRow{parent, child})
		if err != nil {
			t.Fatal(err)
		}
		child.ID = childID
		found = parent.ID > child.ID
	}
	if !found {
		t.Fatal("could not construct a parent digest that sorts after its child")
	}
	got, err := identityDigest(child, []IdentityRow{child, parent})
	if err != nil {
		t.Fatal(err)
	}
	if got != child.ID {
		t.Fatalf("child digest changed with lexical parent order: got=%q want=%q", got, child.ID)
	}
}

func TestIdentityDigestRejectsMalformedParentGraph(t *testing.T) {
	cycleA := IdentityRow{ID: "identity-a", Owner: "owner-a", DeclarationFactID: "decl-a", PatternID: "pattern:a", RelationKind: IdentityRelationComposite, DirectFactIDs: []string{"fact-a"}, ParentIdentityIDs: []string{"identity-b"}}
	cycleB := IdentityRow{ID: "identity-b", Owner: "owner-a", DeclarationFactID: "decl-a", PatternID: "pattern:b", RelationKind: IdentityRelationComposite, DirectFactIDs: []string{"fact-b"}, ParentIdentityIDs: []string{"identity-a"}}
	if _, err := identityDigest(cycleA, []IdentityRow{cycleA, cycleB}); err == nil {
		t.Fatal("identity digest accepted a parent cycle")
	}
	unknown := cycleA
	unknown.ID = "identity-unknown"
	unknown.ParentIdentityIDs = []string{"identity-missing"}
	if _, err := identityDigest(unknown, []IdentityRow{unknown}); err == nil {
		t.Fatal("identity digest accepted an unknown parent")
	}
}

func TestRunWithScanCannotBypassPopulationBeforeWrite(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, schemaDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{catalogFile, indexesFile, surfacesFile, residueFile} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\tv2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scan, _ := populationFixture(1, 1)
	report, err := runWithScan(root, ModeInventory, true, scan)
	if !errors.Is(err, ErrManifestPopulation) {
		t.Fatalf("error=%v, want population failure", err)
	}
	if !report.ManifestPresent || report.GeneratedFresh {
		t.Fatalf("report accepted an unproved manifest: %+v", report)
	}
	if _, statErr := os.Stat(filepath.Join(dir, generatedFile)); !os.IsNotExist(statErr) {
		t.Fatalf("invalid population wrote generated output: %v", statErr)
	}
}

func TestRenderGeneratedIsDeterministicTypedAndFixed(t *testing.T) {
	_, manifests := populationFixture(1, 1)
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	first, err := renderGenerated(manifests, digest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderGenerated(manifests, digest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("render is nondeterministic")
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", first, parser.AllErrors); err != nil {
		t.Fatalf("rendered output is not Go: %v\n%s", err, first)
	}
	for _, required := range []string{"generatedIndexes", "generatedHotReferences", "generatedColdReferences", "generatedContextualReferences", "generatedIdentities", "generatedStorages", "[...]generated"} {
		if !bytes.Contains(first, []byte(required)) {
			t.Fatalf("render omitted typed ledger plane %q", required)
		}
	}
	for _, forbidden := range []string{"map["} {
		if bytes.Contains(first, []byte(forbidden)) {
			t.Fatalf("render contains forbidden schema/generic token %q:\n%s", forbidden, first)
		}
	}
	if bytes.Contains(first, []byte("split-plan-v1-")) || bytes.Contains(first, []byte("\"split\"")) {
		t.Fatalf("render emitted migration-only split residue authority:\n%s", first)
	}
}

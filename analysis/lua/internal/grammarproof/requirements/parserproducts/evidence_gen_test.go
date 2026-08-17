package parserproducts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
)

func TestCanonicalOwnsEveryTypedCoordinate(t *testing.T) {
	baseline := clone(Generated)
	helperIndex, returnIndex := firstHelperReturn(t, baseline)
	sequenceIndex, segmentIndex := firstSequenceSegment(t, baseline)
	first, second := baseline.Canonical(), baseline.Canonical()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("equivalent parser-products evidence has nondeterministic canonical bytes")
	}
	sum := sha256.Sum256(first)
	if got := hex.EncodeToString(sum[:]); got != baseline.Digest {
		t.Fatalf("digest = %s, want SHA-256(canonical) %s", baseline.Digest, got)
	}
	withoutDigest := baseline
	withoutDigest.Digest = "untrusted-self-report"
	if !bytes.Equal(first, withoutDigest.Canonical()) {
		t.Fatal("self-reported digest changed canonical authority")
	}

	cases := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{"field", func(e *Evidence) { e.Fields[0].Source = "other" }},
		{"product", func(e *Evidence) { e.Products[0].States[0]++ }},
		{"product-action", func(e *Evidence) { e.ProductLaws[0].ActionDigest = "other" }},
		{"helper-return", func(e *Evidence) { e.HelperLaws[helperIndex].Returns[returnIndex].Values[0]++ }},
		{"sequence", func(e *Evidence) { e.Sequences[sequenceIndex].Segments[segmentIndex].Term++ }},
		{"mutation", func(e *Evidence) { e.Mutations[0].Edit.Value++ }},
		{"term", func(e *Evidence) { e.ActionTerms.Terms[0].Slot++ }},
		{"guard-symbol", func(e *Evidence) { e.ActionTerms.GuardSymbols[0]++ }},
		{"carrier", func(e *Evidence) { e.Carriers[0].ChildType = "other" }},
		{"recursion", func(e *Evidence) { e.Recursion[0].Nonterminal = "other" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := clone(baseline)
			test.mutate(&changed)
			if bytes.Equal(first, changed.Canonical()) {
				t.Fatal("semantic evidence mutation preserved canonical bytes")
			}
		})
	}
}

func firstHelperReturn(t *testing.T, evidence Evidence) (int, int) {
	t.Helper()
	for helperIndex, law := range evidence.HelperLaws {
		for returnIndex, row := range law.Returns {
			if len(row.Values) != 0 {
				return helperIndex, returnIndex
			}
		}
	}
	t.Fatal("missing helper return value")
	return 0, 0
}

func firstSequenceSegment(t *testing.T, evidence Evidence) (int, int) {
	t.Helper()
	for sequenceIndex, law := range evidence.Sequences {
		if len(law.Segments) != 0 {
			return sequenceIndex, 0
		}
	}
	t.Fatal("missing sequence segment")
	return 0, 0
}

func TestBuildRederivesAndRejectsReSignedChanges(t *testing.T) {
	root := repositoryRoot(t)
	snapshot, err := grammarproof.Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(root, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(root, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Canonical(), second.Canonical()) || first.Digest != second.Digest {
		t.Fatal("independent parser-products builds differ")
	}
	if err := first.Validate(root, snapshot); err != nil {
		t.Fatal(err)
	}
	if len(first.Fields) == 0 || len(first.ProductLaws) == 0 || len(first.HelperLaws) != 19 || len(first.ActionTerms.Terms) == 0 {
		t.Fatalf("incomplete parser-products denominator: %#v", first)
	}

	changed := clone(first)
	changed.ProductLaws[0], changed.ProductLaws[1] = changed.ProductLaws[1], changed.ProductLaws[0]
	changed.Digest = digest(changed)
	if err := changed.Validate(root, snapshot); err == nil {
		t.Fatal("re-signed reordered product laws were accepted")
	}
}

func TestGeneratedEvidenceIsCurrentAndDetached(t *testing.T) {
	root := repositoryRoot(t)
	first, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	second := clone(first)
	first.ActionTerms.Symbols[0].Text = "caller mutation"
	first.ProductLaws[0].RHS[0] = "caller mutation"
	if second.ActionTerms.Symbols[0].Text == "caller mutation" || Generated.ActionTerms.Symbols[0].Text == "caller mutation" {
		t.Fatal("Current exposed generated action-term backing storage")
	}
	if second.ProductLaws[0].RHS[0] == "caller mutation" || Generated.ProductLaws[0].RHS[0] == "caller mutation" {
		t.Fatal("Current exposed generated product-law backing storage")
	}
}

func TestGeneratedFilesAreClosedAndVertical(t *testing.T) {
	rendered, err := renderFiles(Generated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expectedGeneratedFiles(), sortedGeneratedNames(rendered)) {
		t.Fatalf("generated file set = %v", sortedGeneratedNames(rendered))
	}
	directory := t.TempDir()
	for name, source := range rendered {
		if err := os.WriteFile(filepath.Join(directory, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkRendered(directory, rendered); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "evidence_gen_orphan.go"), []byte("package parserproducts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkRendered(directory, rendered); err == nil {
		t.Fatal("stale generated semantic family was accepted")
	}
}

func TestActionTermsFailClosedForRolesAndStructure(t *testing.T) {
	table := clone(Generated).ActionTerms
	table.Terms[0].Scope = 0
	if err := table.Validate(); err == nil {
		t.Fatal("invalid action scope accepted")
	}

	table = clone(Generated).ActionTerms
	formal := findTerm(t, table, ActionTermFormal)
	table.Terms[formal-1].Slot = ^uint16(0)
	if err := table.Validate(); err == nil {
		t.Fatal("out-of-range formal slot accepted")
	}

	table = clone(Generated).ActionTerms
	record := findTerm(t, table, ActionTermRecord)
	if table.Terms[record-1].EdgeCount == 0 {
		t.Fatal("expected a nonempty typed record")
	}
	table.Edges[table.Terms[record-1].EdgeStart].Label = 0
	if err := table.Validate(); err == nil {
		t.Fatal("unlabelled record edge accepted")
	}

	table = clone(Generated).ActionTerms
	// Index-place validation is a vocabulary law, not a requirement that the
	// current parser implementation happen to contain an indexed Go assignment.
	// Inject the malformed form directly so declaration layout cannot make this
	// negative law pass or fail.
	table.PlaceSteps = append(table.PlaceSteps, PlaceStep{Kind: PlaceStepIndex})
	if err := table.Validate(); err == nil {
		t.Fatal("invalid index place step accepted")
	}
}

func TestTermInstancesAreScopeBound(t *testing.T) {
	table := Generated.ActionTerms
	formal := findTerm(t, table, ActionTermFormal)
	helperScope := table.Terms[formal-1].Scope
	callerInput := ActionTermID(0)
	callerScope := ActionScopeID(0)
	for index, term := range table.Terms {
		if term.Kind != ActionTermInput {
			continue
		}
		scope, _ := table.Scope(term.Scope)
		if scope.Kind == ActionScopeProduction {
			callerInput, callerScope = ActionTermID(index+1), term.Scope
			break
		}
	}
	if callerInput == 0 {
		t.Fatal("missing production input")
	}
	helper, _ := table.Scope(helperScope)
	actuals := make([]ActionTermID, helper.Formals)
	for index := range actuals {
		actuals[index] = callerInput
	}
	instance := TermInstance{CallerScope: callerScope, HelperScope: helperScope, Root: formal, Actuals: actuals}
	if err := table.ValidateInstance(instance); err != nil {
		t.Fatalf("valid formal-only instance: %v", err)
	}
	instance.Actuals[0] = formal
	if err := table.ValidateInstance(instance); err == nil {
		t.Fatal("cross-scope actual accepted")
	}
}

func TestActionControlsAreGuardedAndComplete(t *testing.T) {
	statCall := productLaw(t, "stat#2")
	if len(statCall.Products) != 1 || len(statCall.Rejects) != 1 || statCall.Rejects[0].Condition != RejectWhenAll {
		t.Fatalf("stat#2 = %#v", statCall)
	}
	requireGuardKind(t, statCall.Products[0].Guard, GuardAtomTypeIn)
	requireGuardKind(t, statCall.Rejects[0].Guard, GuardAtomTypeIn)

	for _, production := range []string{"stat#6", "stat#7"} {
		law := productLaw(t, production)
		if len(law.Chains) != 1 {
			t.Fatalf("%s chain count = %d", production, len(law.Chains))
		}
		chain := law.Chains[0]
		field, _ := Generated.ActionTerms.Symbol(chain.LinkField)
		if field.Kind != ActionSymbolField || field.Text != "Else" {
			t.Fatalf("%s link field = %#v", production, field)
		}
		if production == "stat#6" && chain.TailCount != 0 {
			t.Fatalf("stat#6 tail count = %d", chain.TailCount)
		}
		if production == "stat#7" && chain.TailCount != 2 {
			t.Fatalf("stat#7 tail count = %d", chain.TailCount)
		}
	}

	number := productLaw(t, "primarytypeexpr#5")
	seenClass := map[NumberParseClass]bool{}
	for _, row := range number.Products {
		for _, atom := range row.Guard.Atoms {
			if atom.Kind == GuardAtomNumberParseClass {
				seenClass[atom.ParseClass] = true
			}
		}
	}
	for _, class := range []NumberParseClass{NumberParseClassInteger, NumberParseClassFloat, NumberParseClassInvalid} {
		if !seenClass[class] {
			t.Fatalf("primarytypeexpr#5 lacks number class %d", class)
		}
	}

	for _, production := range []string{"stat#17", "stat#18", "args#1", "args#2"} {
		law := productLaw(t, production)
		if len(law.Rejects) != 1 || law.Rejects[0].Condition != RejectWhenAll {
			t.Fatalf("%s rejects = %#v", production, law.Rejects)
		}
		diagnostic, _ := Generated.ActionTerms.Symbol(law.Rejects[0].Diagnostic)
		if diagnostic.Kind != ActionSymbolDiagnostic {
			t.Fatalf("%s diagnostic = %#v", production, diagnostic)
		}
	}

	for _, production := range []string{"interfacemethod#1", "interfacemethod#2"} {
		law := productLaw(t, production)
		if len(law.Edits) != 1 {
			t.Fatalf("%s edits = %#v", production, law.Edits)
		}
		requireGuardKind(t, law.Edits[0].Guard, GuardAtomNil)
		requireGuardKind(t, law.Edits[0].Guard, GuardAtomEqConst)
	}
}

func TestHelperLedgerAndMapSummariesAreExact(t *testing.T) {
	semantic, metadata, diagnostic, returns, rejects := 0, 0, 0, 0, 0
	for _, law := range Generated.HelperLaws {
		switch law.Disposition {
		case HelperDispositionSemantic:
			semantic++
		case HelperDispositionMetadata:
			metadata++
		case HelperDispositionDiagnostic:
			diagnostic++
		}
		returns += len(law.Returns)
		rejects += len(law.Rejects)
	}
	if semantic != 15 || metadata != 3 || diagnostic != 1 || returns != 18 || rejects != 5 {
		t.Fatalf("helper ledger semantic=%d metadata=%d diagnostic=%d returns=%d rejects=%d", semantic, metadata, diagnostic, returns, rejects)
	}

	nameList := helperLaw(t, "splitNameList")
	typedNames := helperLaw(t, "splitTypedNames")
	params := helperLaw(t, "toFuncParams")
	if len(nameList.Summary.Maps) != 2 || len(nameList.Summary.Presence) != 0 {
		t.Fatalf("splitNameList summary = %#v", nameList.Summary)
	}
	if len(typedNames.Summary.Maps) != 3 || len(typedNames.Summary.Presence) != 1 || typedNames.Summary.Presence[0].Output != 2 {
		t.Fatalf("splitTypedNames summary = %#v", typedNames.Summary)
	}
	if len(params.Summary.Maps) != 1 || len(params.Summary.Presence) != 0 {
		t.Fatalf("toFuncParams summary = %#v", params.Summary)
	}
	for _, law := range []HelperLaw{nameList, typedNames, params} {
		for _, row := range law.Summary.Maps {
			item, ok := Generated.ActionTerms.Scope(row.ItemScope)
			if !ok || item.Kind != ActionScopeMapItem || item.Inputs != 1 || item.Formals != 0 || item.Locals != 0 || item.Results != 0 {
				t.Fatalf("map scope = %#v", item)
			}
			term, _ := Generated.ActionTerms.Term(row.Element)
			if term.Scope != row.ItemScope {
				t.Fatalf("map element crosses item scope: %#v", row)
			}
		}
	}
	mapTerm, _ := Generated.ActionTerms.Term(params.Summary.Maps[0].Element)
	if mapTerm.Kind != ActionTermRecord {
		t.Fatalf("toFuncParams map element = %#v", mapTerm)
	}
}

func TestSemanticHelperPartitionFailsClosed(t *testing.T) {
	evidence := clone(Generated)
	for index := range evidence.HelperLaws {
		law := &evidence.HelperLaws[index]
		if law.Disposition != HelperDispositionSemantic || len(law.Returns) < 2 {
			continue
		}
		law.Returns[1].Guard = law.Returns[0].Guard
		if err := validateEvidenceRows(evidence); err == nil {
			t.Fatal("overlapping helper branches were accepted")
		}
		return
	}
	t.Fatal("missing multi-branch semantic helper")
}

func TestProductLawOrderCheckNeverRepairs(t *testing.T) {
	rows := append([]ProductLaw(nil), Generated.ProductLaws...)
	if err := validateProductLawOrder(rows); err != nil {
		t.Fatal(err)
	}
	rows[0], rows[1] = rows[1], rows[0]
	before := append([]ProductLaw(nil), rows...)
	if err := validateProductLawOrder(rows); err == nil {
		t.Fatal("reordered product laws accepted")
	}
	if !reflect.DeepEqual(rows, before) {
		t.Fatal("product-law validation repaired caller data")
	}
}

func findTerm(t *testing.T, table ActionTerms, kind ActionTermKind) ActionTermID {
	t.Helper()
	for index, term := range table.Terms {
		if term.Kind == kind {
			return ActionTermID(index + 1)
		}
	}
	t.Fatalf("missing action term kind %d", kind)
	return 0
}

func productLaw(t *testing.T, production string) ProductLaw {
	t.Helper()
	for _, law := range Generated.ProductLaws {
		if law.Production == production {
			return law
		}
	}
	t.Fatalf("missing product law %s", production)
	return ProductLaw{}
}

func helperLaw(t *testing.T, owner string) HelperLaw {
	t.Helper()
	for _, law := range Generated.HelperLaws {
		scope, ok := Generated.ActionTerms.Scope(law.Scope)
		if !ok {
			continue
		}
		symbol, ok := Generated.ActionTerms.Symbol(scope.Owner)
		if ok && symbol.Kind == ActionSymbolCallable && symbol.Text == owner {
			return law
		}
	}
	t.Fatalf("missing helper law %s", owner)
	return HelperLaw{}
}

func requireGuardKind(t *testing.T, guard Guard, kind GuardAtomKind) {
	t.Helper()
	for _, atom := range guard.Atoms {
		if atom.Kind == kind {
			return
		}
	}
	t.Fatalf("guard %#v lacks kind %d", guard, kind)
}

func sortedGeneratedNames(rendered map[string][]byte) []string {
	result := make([]string, 0, len(rendered))
	for name := range rendered {
		result = append(result, name)
	}
	for left := 0; left < len(result); left++ {
		for right := left + 1; right < len(result); right++ {
			if result[right] < result[left] {
				result[left], result[right] = result[right], result[left]
			}
		}
	}
	return result
}

// repositoryRoot walks up from this test source until it finds the directory
// that owns go.mod. Anchoring on the module marker keeps the proof independent
// of where the grammarproof tree sits inside the module.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("discover parser-products test path")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("module root: no go.mod above test file")
		}
		root = parent
	}
}

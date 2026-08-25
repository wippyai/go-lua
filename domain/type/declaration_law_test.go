package typedomain

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/domain/type/channelselect"
)

// surfacePackages is the closed set of declaration-surface packages. A row on
// any surface of the analyzer declaration table is reached by importing the
// surface's package, so this list is what the domain's declaration statement
// is checked against.
//
// The schema's type-contract package is deliberately absent: it is the neutral
// portable envelope one authored type declaration travels in, not a surface, and
// this domain's adapter is its implementation rather than a declarer of rows.
var surfacePackages = []string{
	"github.com/wippyai/go-lua/analysis/schema/structure",
	"github.com/wippyai/go-lua/analysis/schema/axis",
	"github.com/wippyai/go-lua/analysis/schema/rule",
	"github.com/wippyai/go-lua/analysis/schema/diagnostic",
	"github.com/wippyai/go-lua/analysis/schema/composite",
	"github.com/wippyai/go-lua/analysis/schema/denominator",
	"github.com/wippyai/go-lua/analysis/schema/query",
}

// declarationSource is the one file of this domain that names a declaration
// surface. The domain's statement is a single statement, so it is made from a
// single place and every other source beneath this directory is held to the
// zero-row form.
const declarationSource = "declaration.go"

// TestTypeDomainDeclaresItsRowFromOneSource is the executable form of this
// domain's declaration statement. The domain declares its diagnostic rows and
// the channel-select case fact role here: a row reached from anywhere else
// beneath this directory would be a second declaration for a domain that has
// one, and a row on any other surface would be a claim the domain's position
// does not support.
func TestTypeDomainDeclaresItsRowFromOneSource(t *testing.T) {
	for _, path := range domainSources(t) {
		declares := relative(t, path) == declarationSource
		for _, imported := range sourceImports(t, path) {
			for _, surface := range surfacePackages {
				if imported != surface && !strings.HasPrefix(imported, surface+"/") {
					continue
				}
				if declares && (surface == "github.com/wippyai/go-lua/analysis/schema/diagnostic" ||
					surface == "github.com/wippyai/go-lua/analysis/schema/structure") {
					continue
				}
				t.Errorf("type domain source %s imports declaration surface %s outside the domain's one declaration", relative(t, path), imported)
			}
		}
	}
}

// TestTypeDomainDiagnosticRowIsAdmissible states that the declared row is a row:
// the diagnostic surface admits it, and the identity it publishes under is the
// domain's own published code.
func TestTypeDomainChannelSelectStructureRowMatchesOwnerSpelling(t *testing.T) {
	specs := ChannelSelectStructureSpecs()
	if len(specs) != 1 || specs[0].Key != channelselect.FamilyKey || specs[0].Spelling != channelselect.Role {
		t.Fatalf("ChannelSelectStructureSpecs = %+v, want family %q", specs, channelselect.FamilyKey)
	}
}

func TestTypeDomainDiagnosticRowIsAdmissible(t *testing.T) {
	entry, ok := diagnostic.New(DiagnosticSpec())
	if !ok {
		t.Fatal("the domain's declared diagnostic row was refused by the surface")
	}
	if entry.Code() != Code {
		t.Fatalf("declared row publishes %q, not %q", entry.Code(), Code)
	}
	if entry.ID() != schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(Code)) {
		t.Fatal("declared row derives a foreign entry identity")
	}
}

func TestTypeDomainChannelSelectExhaustivenessRowIsAdmissible(t *testing.T) {
	entry, ok := diagnostic.New(DiagnosticChannelSelectExhaustivenessSpec())
	if !ok {
		t.Fatal("the domain's exhaustiveness diagnostic row was refused by the surface")
	}
	if entry.Code() != ChannelSelectExhaustivenessCode {
		t.Fatalf("declared row publishes %q, not %q", entry.Code(), ChannelSelectExhaustivenessCode)
	}
	if entry.ID() != schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(ChannelSelectExhaustivenessCode)) {
		t.Fatal("declared row derives a foreign entry identity")
	}
	family, familyOK := entry.Code().Family()
	if !familyOK || schema.Key("family/"+family) != ChannelSelectFamilyKey {
		t.Fatalf("published code %q reads as a family other than %q", entry.Code(), ChannelSelectFamilyKey)
	}
	if entry.Lane() != diagnostic.LaneBranch || !entry.Collectable() {
		t.Fatalf("exhaustiveness row is installed on lane %d", entry.Lane())
	}
	if entry.DefaultSeverity() != diagnostic.SeverityWarning {
		t.Fatalf("exhaustiveness row defaults to severity %d", entry.DefaultSeverity())
	}
	if entry.Fact().Key != ChannelSelectFactKey || entry.Observation().Key != ChannelSelectObservationKey {
		t.Fatalf("exhaustiveness row names fact %q observation %q", entry.Fact().Key, entry.Observation().Key)
	}
}

func TestTypeDomainCallArgumentRowIsAdmissible(t *testing.T) {
	entry, ok := diagnostic.New(DiagnosticCallArgumentSpec())
	if !ok {
		t.Fatal("the domain's call-argument diagnostic row was refused by the surface")
	}
	if entry.Code() != CallArgumentCode {
		t.Fatalf("declared row publishes %q, not %q", entry.Code(), CallArgumentCode)
	}
	if entry.ID() != schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(CallArgumentCode)) {
		t.Fatal("declared row derives a foreign entry identity")
	}
}

// TestTypeDomainDiagnosticRowNamesForeignDeclarations states the half of the
// row this domain cannot own: the family it publishes under, the population it
// is measured over, and the declaration whose facts decide it are all declared
// elsewhere, and the row names each by reference on the surface that declares
// it. The references resolve when the row is sealed beside them; what is stated
// here is that the row names them at all, and names them on the right surface.
func TestTypeDomainDiagnosticRowNamesForeignDeclarations(t *testing.T) {
	spec := DiagnosticSpec()
	for _, named := range []struct {
		what      string
		reference diagnostic.Reference
		surface   schema.SurfaceKind
		key       schema.Key
	}{
		{"family", spec.Family, schema.SurfaceKindStructure, FamilyKey},
		{"observation population", spec.Observation, schema.SurfaceKindStructure, ObservationKey},
		{"fact declaration", spec.Fact, schema.SurfaceKindAxis, FactKey},
	} {
		if !named.reference.Declared() || !named.reference.Available() {
			t.Fatalf("declared row names no %s", named.what)
		}
		if named.reference.Surface != named.surface || named.reference.Key != named.key {
			t.Fatalf("declared row names its %s as %q on surface %d", named.what, named.reference.Key, named.reference.Surface)
		}
	}
	family, familyOK := spec.Code.Family()
	if !familyOK || schema.Key("family/"+family) != FamilyKey {
		t.Fatalf("published code %q reads as a family other than %q", spec.Code, FamilyKey)
	}
}

// TestTypeDomainDiagnosticRowRequiresExactlyWhatItReads states the row's own
// payload law from this side of it: the typed payload a producer is obliged to
// supply is exactly the payload the row's message, help, evidence, and labels
// read. A field nothing reads is dead weight on the publication half; a read
// nothing requires is a hole that would be found at render time.
func TestTypeDomainDiagnosticRowRequiresExactlyWhatItReads(t *testing.T) {
	entry, ok := diagnostic.New(DiagnosticSpec())
	if !ok {
		t.Fatal("the domain's declared diagnostic row was refused by the surface")
	}
	reads := entry.Message().Requires() | entry.Help().Requires()
	for index := 0; index < entry.EvidenceCount(); index++ {
		evidence, evidenceOK := entry.EvidenceAt(index)
		if !evidenceOK {
			t.Fatalf("evidence line %d is unavailable", index)
		}
		reads |= evidence.Detail.Requires() | evidence.Anchor.Requires()
	}
	for index := 0; index < entry.LabelCount(); index++ {
		label, labelOK := entry.LabelAt(index)
		if !labelOK {
			t.Fatalf("label %d is unavailable", index)
		}
		reads |= label.Text.Requires() | label.Anchor.Requires()
	}
	if entry.Requirements() != reads {
		t.Fatalf("declared row requires %d and reads %d", entry.Requirements(), reads)
	}
}

// TestTypeDomainDiagnosticRowPublishesEverySectionItDeclares states the render
// half: the row renders its summary, and it renders the sections whose content
// it declares. A declared help line or evidence line the row never renders
// would publish nothing.
func TestTypeDomainDiagnosticRowPublishesEverySectionItDeclares(t *testing.T) {
	entry, ok := diagnostic.New(DiagnosticSpec())
	if !ok {
		t.Fatal("the domain's declared diagnostic row was refused by the surface")
	}
	if !entry.Renders(diagnostic.SectionSummary) {
		t.Fatal("declared row renders no summary")
	}
	if entry.Help().Available() && !entry.Renders(diagnostic.SectionHelp) {
		t.Fatal("declared row declares help it never renders")
	}
	if entry.EvidenceCount() > 0 && !entry.Renders(diagnostic.SectionEvidence) {
		t.Fatal("declared row declares evidence it never renders")
	}
}

// TestTypeDomainDiagnosticRowIsCollectedOnTheBranchLane states the lane the row
// is installed on, and the two obligations that lane carries: a producing lane
// is measured over a declared population, and a solver-observed lane is decided
// by a declaration it names. The publication half is written against this lane,
// so the lane is declared data rather than a property of the collector.
func TestTypeDomainDiagnosticRowIsCollectedOnTheBranchLane(t *testing.T) {
	entry, ok := diagnostic.New(DiagnosticSpec())
	if !ok {
		t.Fatal("the domain's declared diagnostic row was refused by the surface")
	}
	if entry.Lane() != diagnostic.LaneBranch || !entry.Collectable() {
		t.Fatalf("declared row is installed on lane %d", entry.Lane())
	}
	if entry.DefaultSeverity() != diagnostic.SeverityError || entry.Tier() != diagnostic.TierError {
		t.Fatalf("declared row defaults to severity %d in tier %d", entry.DefaultSeverity(), entry.Tier())
	}
	if !entry.Observation().Declared() || !entry.Fact().Declared() {
		t.Fatal("a solver-observed row must name both the population it measures and the declaration that decides it")
	}
}

// TestTypeDomainImportsNoPeerDomain states the position the declaration rests
// on: this domain is the base of the domain layer, so it reads no peer domain
// and every peer that reasons about types reads it. An edge in the other
// direction would make the declaration a statement about a cycle rather than
// about a domain, and it would put the domain's declaration above a domain that
// already declares rows of its own.
//
// The closed runtime family vocabulary is below this domain rather than beside
// it: it is scalar vocabulary and set algebra with no dependency on any domain
// at all, which the law beside this one states rather than assumes. Reading it
// is reading the analyzer's one spelling of the families type() distinguishes,
// so the alternative is a second spelling of them in this domain.
func TestTypeDomainImportsNoPeerDomain(t *testing.T) {
	const domainRoot = "github.com/wippyai/go-lua/domain/"
	const self = domainRoot + "type"
	for _, path := range domainSources(t) {
		for _, imported := range sourceImports(t, path) {
			if !strings.HasPrefix(imported, domainRoot) {
				continue
			}
			if imported == self || strings.HasPrefix(imported, self+"/") || imported == sharedVocabulary {
				continue
			}
			t.Errorf("type domain source %s imports peer domain %s", relative(t, path), imported)
		}
	}
}

// sharedVocabulary is the one domain package this domain reads: the closed Lua
// runtime family vocabulary the analyzer's domains share.
const sharedVocabulary = "github.com/wippyai/go-lua/domain/runtimekind"

// TestSharedVocabularyIsBelowTheDomainLayer is what makes the exemption in the
// law above a statement rather than a hole: the vocabulary this domain reads
// reads no domain itself, so the edge to it cannot be part of a cycle and
// cannot carry another domain's judgment in behind it.
func TestSharedVocabularyIsBelowTheDomainLayer(t *testing.T) {
	const domainRoot = "github.com/wippyai/go-lua/domain/"
	for _, path := range packageSources(t, filepath.Join(filepath.Dir(domainRootDir(t)), "runtimekind")) {
		for _, imported := range sourceImports(t, path) {
			if strings.HasPrefix(imported, domainRoot) {
				t.Errorf("shared vocabulary source %s imports domain package %s", filepath.Base(path), imported)
			}
		}
	}
}

// domainSources returns every non-test Go source file of this domain, this
// directory and every package beneath it.
func domainSources(t *testing.T) []string {
	t.Helper()
	return packageSources(t, domainRootDir(t))
}

// packageSources returns every non-test Go source file at or beneath root.
func packageSources(t *testing.T, root string) []string {
	t.Helper()
	var sources []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		sources = append(sources, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(sources) == 0 {
		t.Fatalf("no production sources found under %s", root)
	}
	return sources
}

func sourceImports(t *testing.T, path string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		value, unquoteErr := strconv.Unquote(imported.Path.Value)
		if unquoteErr != nil {
			t.Fatalf("unquote import in %s: %v", path, unquoteErr)
		}
		imports = append(imports, value)
	}
	return imports
}

func domainRootDir(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("type domain source location unavailable")
	}
	return filepath.Dir(current)
}

func relative(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel(domainRootDir(t), path)
	if err != nil {
		return path
	}
	return rel
}

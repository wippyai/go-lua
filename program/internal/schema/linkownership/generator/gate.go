package generator

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	schemaDir       = "program/internal/schema/linkownership"
	catalogFile     = "catalog.schema"
	indexesFile     = "indexes.schema"
	surfacesFile    = "surfaces.schema"
	residueFile     = "residue.schema"
	generatedFile   = "generated.go"
	manifestVersion = "v2"
)

var (
	ErrManifestParse          = errors.New("link ownership: manifest parse failed")
	ErrManifestClassification = errors.New("link ownership: manifest fact is multiply classified")
)

// ManifestFiles is the complete authored input to the cold ownership gate.
// The four members are intentionally separate: no combined stream or
// fallback/default manifest is accepted.
type ManifestFiles struct {
	Catalog  []byte
	Indexes  []byte
	Surfaces []byte
	Residue  []byte
}

// ManifestSet is the typed, map-free result of parsing all four authored
// manifests. The private canonical members are immutable snapshots of the
// accepted bytes; public callers receive copies through CanonicalFiles.
type ManifestSet struct {
	Catalog  CatalogManifest
	Indexes  IndexManifest
	Surfaces SurfaceManifest
	Residue  ResidueManifest

	canonical ManifestFiles
}

// CatalogManifest contains the four catalog row families. Rows are retained
// in their authored canonical order; no owner decision is inferred here. The
// exact catalog.schema columns are:
// owner: kind,id,packagePath,surface,ownerKind
// decl: kind,factID,packagePath,declarationKind,owner,surface,name,type,signature
// use: kind,factID,packagePath,sourceFile,line,column,symbol,evidence,role,
//
//	targetDeclID,type
//
// ownership-import-edge: kind,fromOwner,toOwner,sourceFile,line,column
type CatalogManifest struct {
	Owners       []OwnerRow
	Declarations []DeclarationRow
	Uses         []UseRow
	ImportEdges  []OwnershipImportEdgeRow
}

type OwnerRow struct {
	ID, PackagePath, Surface, Kind string
}

type DeclarationRow struct {
	FactID, PackagePath, Kind, Owner, Surface, Name, Type, Signature string
}

type UseRow struct {
	FactID, PackagePath, SourceFile, Symbol, Evidence, TargetDeclID string
	Type                                                            string
	Role                                                            UseRole
	Line, Column                                                    int
}

type OwnershipImportEdgeRow struct {
	FromOwner, ToOwner, SourceFile string
	Line, Column                   int
}

// IndexManifest keeps each v2 row family in a distinct typed slice. Final
// rows refer only to scanner FactIDs and one atomic pattern ID; plan rows are
// inventory-only audit records and are never rendered.
// The exact indexes.schema columns are:
//
//	index: kind,id,owner,queryFactID,callerUseFactIDs,sourceFactIDs,
//	  patternID,benchmarkReceiptDigest
//	hot-ref|cold-ref|contextual-ref: kind,id,issuer,consumer,queryFactID,
//	  sourceFactIDs,patternID,callerUseFactIDs,benchmarkReceiptDigest
//	identity: kind,id,owner,declarationFactID,relationKind,directFactIDs,
//	  parentIdentityIDs,patternID
//	index-plan: kind,id,owner,declarationFactID,sourceFactIDs,
//	  callerUseFactIDs
//	reference-plan: kind,id,issuer,consumer,declarationFactID,sourceFactIDs,
//	  callerUseFactIDs
//	identity-plan: kind,id,owner,declarationFactID,relationKind,directFactIDs,
//	  parentIdentityIDs
type IndexManifest struct {
	Indexes              []IndexRow
	HotReferences        []HotReferenceRow
	ColdReferences       []ColdReferenceRow
	ContextualReferences []ContextualReferenceRow
	Identities           []IdentityRow
	IndexPlans           []IndexPlanRow
	ReferencePlans       []ReferencePlanRow
	IdentityPlans        []IdentityPlanRow
}

type IndexRow struct {
	ID, Owner, QueryFactID, PatternID, BenchmarkReceiptDigest string
	CallerUseFactIDs, SourceFactIDs                           []string
}

type HotReferenceRow struct {
	ID, Issuer, Consumer, QueryFactID, PatternID, BenchmarkReceiptDigest string
	CallerUseFactIDs, SourceFactIDs                                      []string
}

type ColdReferenceRow struct {
	ID, Issuer, Consumer, QueryFactID, PatternID, BenchmarkReceiptDigest string
	CallerUseFactIDs, SourceFactIDs                                      []string
}

type ContextualReferenceRow struct {
	ID, Issuer, Consumer, QueryFactID, PatternID, BenchmarkReceiptDigest string
	CallerUseFactIDs, SourceFactIDs                                      []string
}

type IdentityRow struct {
	ID, Owner, DeclarationFactID, PatternID string
	RelationKind                            IdentityRelationKind
	DirectFactIDs, ParentIdentityIDs        []string
}

type SurfaceManifest struct {
	Assignments []SurfaceAssignmentRow
	Storage     []StorageRow
}

// surfaces.schema has one assignment row (kind,factID,ownerSurface,name) for
// each published field/method/package function and one storage row
// (kind,factID,ownerSurface,disposition) for each structural representation.

type SurfaceAssignmentRow struct {
	Kind, FactID, OwnerSurface, Name string
}

type ResidueManifest struct {
	Rows       []ResidueRow
	SplitPlans []SplitPlanRow
}

// residue.schema fact rows are exactly (kind,currentFact,destination), where
// move names an exact Catalog OwnerRow ID, split names a typed split plan ID,
// and delete names the sole closed private-representation destination.

type ResidueRow struct {
	Kind, CurrentFact, Destination string
}

// SplitPlanRow is an inventory-only residue plan. It names the exact sorted
// recipient OwnerRow IDs for one or more split facts; it is never rendered as
// live generated state.
type SplitPlanRow struct {
	ID       string
	OwnerIDs []string
}

// CanonicalFiles returns copies of the exact canonical bytes accepted by
// ParseManifestFiles. Mutating the returned buffers cannot mutate the parsed
// result or create a second manifest authority.
func (set ManifestSet) CanonicalFiles() ManifestFiles {
	return ManifestFiles{
		Catalog:  append([]byte(nil), set.canonical.Catalog...),
		Indexes:  append([]byte(nil), set.canonical.Indexes...),
		Surfaces: append([]byte(nil), set.canonical.Surfaces...),
		Residue:  append([]byte(nil), set.canonical.Residue...),
	}
}

// ParseManifestFiles parses exactly the four versioned LF-delimited TSV
// inputs. It does not read the filesystem, choose owners, generate rows, or
// validate any future generated artifact.
func ParseManifestFiles(files ManifestFiles) (ManifestSet, error) {
	set := ManifestSet{}
	catalogLines, err := splitManifestLines(catalogFile, files.Catalog)
	if err != nil {
		return ManifestSet{}, err
	}
	indexesLines, err := splitManifestLines(indexesFile, files.Indexes)
	if err != nil {
		return ManifestSet{}, err
	}
	surfacesLines, err := splitManifestLines(surfacesFile, files.Surfaces)
	if err != nil {
		return ManifestSet{}, err
	}
	residueLines, err := splitManifestLines(residueFile, files.Residue)
	if err != nil {
		return ManifestSet{}, err
	}
	if err := parseCatalogRows(catalogLines[1:], &set.Catalog); err != nil {
		return ManifestSet{}, err
	}
	if err := parseIndexRows(indexesLines[1:], &set.Indexes); err != nil {
		return ManifestSet{}, err
	}
	if err := parseSurfaceRows(surfacesLines[1:], &set.Surfaces); err != nil {
		return ManifestSet{}, err
	}
	if err := parseResidueRows(residueLines[1:], &set.Residue, set.Catalog.Owners); err != nil {
		return ManifestSet{}, err
	}
	set.canonical = ManifestFiles{
		Catalog:  joinManifestLines(catalogLines),
		Indexes:  joinManifestLines(indexesLines),
		Surfaces: joinManifestLines(surfacesLines),
		Residue:  joinManifestLines(residueLines),
	}
	return set, nil
}

func splitManifestLines(name string, raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, manifestError(name, 0, "empty input")
	}
	if bytes.Contains(raw, []byte{0xef, 0xbb, 0xbf}) {
		return nil, manifestError(name, 1, "UTF-8 BOM is forbidden")
	}
	if !utf8.Valid(raw) {
		return nil, manifestError(name, 1, "invalid UTF-8")
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		return nil, manifestError(name, 1, "NUL is forbidden")
	}
	if bytes.IndexByte(raw, '\r') >= 0 {
		return nil, manifestError(name, 1, "CRLF/CR is forbidden; use LF")
	}
	if raw[len(raw)-1] != '\n' {
		return nil, manifestError(name, 1, "input must end with one LF")
	}
	rawLines := strings.Split(string(raw[:len(raw)-1]), "\n")
	if len(rawLines) == 0 || rawLines[0] != name+"\t"+manifestVersion {
		return nil, manifestError(name, 1, "invalid version header")
	}
	for index := 1; index < len(rawLines); index++ {
		if rawLines[index] == "" {
			return nil, manifestError(name, index+1, "blank row")
		}
		if strings.Contains(rawLines[index], "\r") {
			return nil, manifestError(name, index+1, "CR is forbidden")
		}
	}
	for index := 2; index < len(rawLines); index++ {
		if rawLines[index-1] >= rawLines[index] {
			return nil, manifestError(name, index+1, "rows must be strictly sorted and unique")
		}
	}
	return rawLines, nil
}

func joinManifestLines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

func manifestError(name string, line int, message string) error {
	if line > 0 {
		return fmt.Errorf("%w: %s:%d: %s", ErrManifestParse, name, line, message)
	}
	return fmt.Errorf("%w: %s: %s", ErrManifestParse, name, message)
}

func fieldsFor(name string, line int, raw string, want int) ([]string, error) {
	fields := strings.Split(raw, "\t")
	if len(fields) != want {
		return nil, manifestError(name, line, fmt.Sprintf("row has %d columns, want %d", len(fields), want))
	}
	return fields, nil
}

func requiredAtom(name string, line int, field, label string) (string, error) {
	if field == "" {
		return "", manifestError(name, line, label+" is empty")
	}
	if strings.TrimSpace(field) != field || strings.ContainsAny(field, " \t\n\r") {
		return "", manifestError(name, line, label+" has non-canonical whitespace")
	}
	if forbiddenClassification(field) {
		return "", manifestError(name, line, label+" uses forbidden wildcard/default/inferred/regex value")
	}
	return field, nil
}

func optionalAtom(name string, line int, field, label string) (string, error) {
	if field == "" {
		return "", nil
	}
	return requiredAtom(name, line, field, label)
}

// canonicalTextCell is for source-derived text such as go/types signatures.
// Internal spaces are semantic text and are therefore allowed; only control
// separators and non-canonical edge whitespace are rejected.
func canonicalTextCell(name string, line int, field, label string, required bool) (string, error) {
	if field == "" {
		if required {
			return "", manifestError(name, line, label+" is empty")
		}
		return "", nil
	}
	if strings.TrimSpace(field) != field || strings.ContainsAny(field, "\x00\t\n\r") {
		return "", manifestError(name, line, label+" has non-canonical separators/whitespace")
	}
	return field, nil
}

// portableSourceCell admits only repository-relative slash paths, optionally
// followed by a canonical :line suffix for observed provenance. Source paths
// are evidence identities, never host paths or platform-dependent spellings.
func portableSourceCell(name string, line int, field, label string, lineSuffix bool) (string, error) {
	value, err := requiredAtom(name, line, field, label)
	if err != nil {
		return "", err
	}
	path := value
	if lineSuffix {
		if index := strings.LastIndexByte(path, ':'); index > 0 && index+1 < len(path) {
			suffix := path[index+1:]
			if _, err := positiveDecimal(name, line, suffix, label+" line suffix"); err != nil {
				return "", err
			}
			path = path[:index]
		}
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") || strings.HasPrefix(path, "//") || strings.HasPrefix(path, "\\\\") || hasWindowsVolumePath(path) {
		return "", manifestError(name, line, label+" must be a portable repository-relative path")
	}
	if strings.Contains(path, "\\") {
		return "", manifestError(name, line, label+" must use slash separators")
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return "", manifestError(name, line, label+" has non-canonical path traversal/separators")
		}
	}
	return value, nil
}

func hasWindowsVolumePath(value string) bool {
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func forbiddenClassification(value string) bool {
	if strings.ContainsAny(value, "*?") {
		return true
	}
	lower := strings.ToLower(value)
	switch lower {
	case "*", "?", "default", "inferred", "multiple", "parallel", "regex", "glob", "wildcard":
		return true
	}
	return strings.HasPrefix(lower, "parallel:") || strings.HasPrefix(lower, "regex:") || strings.HasPrefix(lower, "glob:") || strings.HasPrefix(lower, "wildcard:")
}

func positiveDecimal(name string, line int, raw, label string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || strconv.Itoa(value) != raw {
		return 0, manifestError(name, line, label+" is not a canonical positive decimal")
	}
	return value, nil
}

func parseCatalogRows(lines []string, result *CatalogManifest) error {
	classified := make(map[string]string)
	owners := make(map[string]struct{})
	edges := make(map[string]struct{})
	for offset, raw := range lines {
		line := offset + 2
		kind := strings.SplitN(raw, "\t", 2)[0]
		switch kind {
		case "owner":
			fields, err := fieldsFor(catalogFile, line, raw, 5)
			if err != nil {
				return err
			}
			id, err := requiredAtom(catalogFile, line, fields[1], "owner ID")
			if err != nil {
				return err
			}
			if _, exists := owners[id]; exists {
				return manifestError(catalogFile, line, "duplicate owner ID")
			}
			owners[id] = struct{}{}
			pkg, err := requiredAtom(catalogFile, line, fields[2], "owner package")
			if err != nil {
				return err
			}
			surface, err := requiredAtom(catalogFile, line, fields[3], "owner surface")
			if err != nil {
				return err
			}
			ownerKind, err := ownerKindValue(catalogFile, line, fields[4])
			if err != nil {
				return err
			}
			result.Owners = append(result.Owners, OwnerRow{ID: id, PackagePath: pkg, Surface: surface, Kind: ownerKind})
		case "decl":
			fields, err := fieldsFor(catalogFile, line, raw, 9)
			if err != nil {
				return err
			}
			factID, err := requiredAtom(catalogFile, line, fields[1], "declaration fact ID")
			if err != nil {
				return err
			}
			if err := claimCatalogFact(classified, factID, "decl", catalogFile, line); err != nil {
				return err
			}
			pkg, err := requiredAtom(catalogFile, line, fields[2], "declaration package")
			if err != nil {
				return err
			}
			declKind, err := requiredAtom(catalogFile, line, fields[3], "declaration kind")
			if err != nil {
				return err
			}
			owner, err := requiredAtom(catalogFile, line, fields[4], "declaration owner")
			if err != nil {
				return err
			}
			surface, err := requiredAtom(catalogFile, line, fields[5], "declaration surface")
			if err != nil {
				return err
			}
			name, err := requiredAtom(catalogFile, line, fields[6], "declaration name")
			if err != nil {
				return err
			}
			typ, err := canonicalTextCell(catalogFile, line, fields[7], "declaration type", true)
			if err != nil {
				return err
			}
			signature, err := canonicalTextCell(catalogFile, line, fields[8], "declaration signature", false)
			if err != nil {
				return err
			}
			result.Declarations = append(result.Declarations, DeclarationRow{FactID: factID, PackagePath: pkg, Kind: declKind, Owner: owner, Surface: surface, Name: name, Type: typ, Signature: signature})
		case "use":
			fields := strings.Split(raw, "\t")
			if len(fields) != 11 {
				return manifestError(catalogFile, line, "use row must have 11 columns")
			}
			factID, err := requiredAtom(catalogFile, line, fields[1], "use fact ID")
			if err != nil {
				return err
			}
			if err := claimCatalogFact(classified, factID, "use", catalogFile, line); err != nil {
				return err
			}
			pkg, err := requiredAtom(catalogFile, line, fields[2], "use package")
			if err != nil {
				return err
			}
			source, err := portableSourceCell(catalogFile, line, fields[3], "use source", false)
			if err != nil {
				return err
			}
			lineNo, err := positiveDecimal(catalogFile, line, fields[4], "use line")
			if err != nil {
				return err
			}
			column, err := positiveDecimal(catalogFile, line, fields[5], "use column")
			if err != nil {
				return err
			}
			symbol, err := requiredAtom(catalogFile, line, fields[6], "use symbol")
			if err != nil {
				return err
			}
			evidence, err := requiredAtom(catalogFile, line, fields[7], "use evidence")
			if err != nil {
				return err
			}
			roleValue, err := requiredAtom(catalogFile, line, fields[8], "use role")
			if err != nil {
				return err
			}
			role := UseRole(roleValue)
			if !role.valid() {
				return manifestError(catalogFile, line, "unknown use role enum "+strconv.Quote(roleValue))
			}
			target, err := requiredAtom(catalogFile, line, fields[9], "use target declaration ID")
			if err != nil {
				return err
			}
			typ, err := canonicalTextCell(catalogFile, line, fields[10], "use type", true)
			if err != nil {
				return err
			}
			result.Uses = append(result.Uses, UseRow{FactID: factID, PackagePath: pkg, SourceFile: source, Line: lineNo, Column: column, Symbol: symbol, Evidence: evidence, TargetDeclID: target, Type: typ, Role: role})
		case "ownership-import-edge":
			fields, err := fieldsFor(catalogFile, line, raw, 6)
			if err != nil {
				return err
			}
			from, err := requiredAtom(catalogFile, line, fields[1], "import-edge source owner")
			if err != nil {
				return err
			}
			to, err := requiredAtom(catalogFile, line, fields[2], "import-edge target owner")
			if err != nil {
				return err
			}
			source, err := portableSourceCell(catalogFile, line, fields[3], "import-edge source file", false)
			if err != nil {
				return err
			}
			lineNo, err := positiveDecimal(catalogFile, line, fields[4], "import-edge line")
			if err != nil {
				return err
			}
			column, err := positiveDecimal(catalogFile, line, fields[5], "import-edge column")
			if err != nil {
				return err
			}
			key := strings.Join([]string{from, to, source, strconv.Itoa(lineNo), strconv.Itoa(column)}, "\x00")
			if _, exists := edges[key]; exists {
				return manifestError(catalogFile, line, "duplicate ownership import edge")
			}
			edges[key] = struct{}{}
			result.ImportEdges = append(result.ImportEdges, OwnershipImportEdgeRow{FromOwner: from, ToOwner: to, SourceFile: source, Line: lineNo, Column: column})
		default:
			return manifestError(catalogFile, line, "unknown row kind/columns")
		}
	}
	return nil
}

func claimCatalogFact(classified map[string]string, factID, kind, name string, line int) error {
	if previous, exists := classified[factID]; exists {
		return classificationError(name, line, fmt.Sprintf("fact %q is classified as both %s and %s", factID, previous, kind))
	}
	classified[factID] = kind
	return nil
}

func parseIndexRows(lines []string, result *IndexManifest) error {
	classified := make(map[string]string)
	for offset, raw := range lines {
		line := offset + 2
		fields := strings.Split(raw, "\t")
		if len(fields) == 0 {
			return manifestError(indexesFile, line, "empty row")
		}
		switch fields[0] {
		case "index":
			if len(fields) != 8 {
				return manifestError(indexesFile, line, "index row must have 8 columns")
			}
			row, err := parseTypedIndexRow(fields, line, classified)
			if err != nil {
				return err
			}
			result.Indexes = append(result.Indexes, row)
		case "hot-ref", "cold-ref", "contextual-ref":
			if len(fields) != 9 {
				return manifestError(indexesFile, line, "reference row must have 9 columns")
			}
			row, err := parseTypedReferenceRow(fields, line, classified)
			if err != nil {
				return err
			}
			switch fields[0] {
			case "hot-ref":
				result.HotReferences = append(result.HotReferences, row.hot)
			case "cold-ref":
				result.ColdReferences = append(result.ColdReferences, row.cold)
			default:
				result.ContextualReferences = append(result.ContextualReferences, row.contextual)
			}
		case "identity":
			if len(fields) != 8 {
				return manifestError(indexesFile, line, "identity row must have 8 columns")
			}
			row, err := parseTypedIdentityRow(fields, line, classified)
			if err != nil {
				return err
			}
			result.Identities = append(result.Identities, row)
		case "index-plan":
			if len(fields) != 6 {
				return manifestError(indexesFile, line, "index-plan row must have 6 columns")
			}
			row, err := parseIndexPlanRow(fields, line, classified)
			if err != nil {
				return err
			}
			result.IndexPlans = append(result.IndexPlans, row)
		case "reference-plan":
			if len(fields) != 7 {
				return manifestError(indexesFile, line, "reference-plan row must have 7 columns")
			}
			row, err := parseReferencePlanRow(fields, line, classified)
			if err != nil {
				return err
			}
			result.ReferencePlans = append(result.ReferencePlans, row)
		case "identity-plan":
			if len(fields) != 7 {
				return manifestError(indexesFile, line, "identity-plan row must have 7 columns")
			}
			row, err := parseIdentityPlanRow(fields, line, classified)
			if err != nil {
				return err
			}
			result.IdentityPlans = append(result.IdentityPlans, row)
		default:
			return manifestError(indexesFile, line, "unknown row kind/columns")
		}
	}
	return nil
}

type typedReferenceRows struct {
	hot        HotReferenceRow
	cold       ColdReferenceRow
	contextual ContextualReferenceRow
}

func parseTypedIndexRow(fields []string, line int, classified map[string]string) (IndexRow, error) {
	id, err := requiredAtom(indexesFile, line, fields[1], "index ID")
	if err != nil {
		return IndexRow{}, err
	}
	if err := claimIndex(classified, id, "index", line); err != nil {
		return IndexRow{}, err
	}
	owner, err := requiredAtom(indexesFile, line, fields[2], "index owner")
	if err != nil {
		return IndexRow{}, err
	}
	query, err := requiredAtom(indexesFile, line, fields[3], "index query declaration FactID")
	if err != nil {
		return IndexRow{}, err
	}
	callers, err := typedFactList("index caller FactIDs", fields[4], true)
	if err != nil {
		return IndexRow{}, manifestError(indexesFile, line, err.Error())
	}
	sources, err := typedFactList("index source FactIDs", fields[5], false)
	if err != nil {
		return IndexRow{}, manifestError(indexesFile, line, err.Error())
	}
	pattern, err := requiredAtom(indexesFile, line, fields[6], "index pattern ID")
	if err != nil {
		return IndexRow{}, err
	}
	benchmark, err := optionalAtom(indexesFile, line, fields[7], "index benchmark receipt digest")
	if err != nil {
		return IndexRow{}, err
	}
	if !canonicalBenchmarkReceiptDigest(benchmark) {
		return IndexRow{}, manifestError(indexesFile, line, "index benchmark receipt digest is not a canonical SHA-256 digest")
	}
	return IndexRow{ID: id, Owner: owner, QueryFactID: query, PatternID: pattern, BenchmarkReceiptDigest: benchmark, CallerUseFactIDs: callers, SourceFactIDs: sources}, nil
}

func parseTypedReferenceRow(fields []string, line int, classified map[string]string) (typedReferenceRows, error) {
	id, err := requiredAtom(indexesFile, line, fields[1], "reference ID")
	if err != nil {
		return typedReferenceRows{}, err
	}
	if err := claimIndex(classified, id, fields[0], line); err != nil {
		return typedReferenceRows{}, err
	}
	issuer, err := requiredAtom(indexesFile, line, fields[2], "reference issuer")
	if err != nil {
		return typedReferenceRows{}, err
	}
	consumer, err := requiredAtom(indexesFile, line, fields[3], "reference consumer")
	if err != nil {
		return typedReferenceRows{}, err
	}
	query, err := requiredAtom(indexesFile, line, fields[4], "reference query declaration FactID")
	if err != nil {
		return typedReferenceRows{}, err
	}
	sources, err := typedFactList("reference source FactIDs", fields[5], false)
	if err != nil {
		return typedReferenceRows{}, manifestError(indexesFile, line, err.Error())
	}
	pattern, err := requiredAtom(indexesFile, line, fields[6], "reference pattern ID")
	if err != nil {
		return typedReferenceRows{}, err
	}
	callers, err := typedFactList("reference caller FactIDs", fields[7], true)
	if err != nil {
		return typedReferenceRows{}, manifestError(indexesFile, line, err.Error())
	}
	benchmark, err := optionalAtom(indexesFile, line, fields[8], "reference benchmark receipt digest")
	if err != nil {
		return typedReferenceRows{}, err
	}
	if !canonicalBenchmarkReceiptDigest(benchmark) {
		return typedReferenceRows{}, manifestError(indexesFile, line, "reference benchmark receipt digest is not a canonical SHA-256 digest")
	}
	return typedReferenceRows{
		hot:        HotReferenceRow{ID: id, Issuer: issuer, Consumer: consumer, QueryFactID: query, PatternID: pattern, BenchmarkReceiptDigest: benchmark, SourceFactIDs: sources, CallerUseFactIDs: callers},
		cold:       ColdReferenceRow{ID: id, Issuer: issuer, Consumer: consumer, QueryFactID: query, PatternID: pattern, BenchmarkReceiptDigest: benchmark, SourceFactIDs: sources, CallerUseFactIDs: callers},
		contextual: ContextualReferenceRow{ID: id, Issuer: issuer, Consumer: consumer, QueryFactID: query, PatternID: pattern, BenchmarkReceiptDigest: benchmark, SourceFactIDs: sources, CallerUseFactIDs: callers},
	}, nil
}

func parseTypedIdentityRow(fields []string, line int, classified map[string]string) (IdentityRow, error) {
	id, err := requiredAtom(indexesFile, line, fields[1], "identity digest ID")
	if err != nil {
		return IdentityRow{}, err
	}
	if err := claimIndex(classified, id, "identity", line); err != nil {
		return IdentityRow{}, err
	}
	owner, err := requiredAtom(indexesFile, line, fields[2], "identity owner")
	if err != nil {
		return IdentityRow{}, err
	}
	declaration, err := requiredAtom(indexesFile, line, fields[3], "identity declaration FactID")
	if err != nil {
		return IdentityRow{}, err
	}
	relation := IdentityRelationKind(fields[4])
	if !relation.valid() {
		return IdentityRow{}, manifestError(indexesFile, line, "unknown identity relation kind")
	}
	direct, err := typedFactList("identity direct FactIDs", fields[5], false)
	if err != nil {
		return IdentityRow{}, manifestError(indexesFile, line, err.Error())
	}
	parents, err := typedFactList("identity parent IDs", fields[6], true)
	if err != nil {
		return IdentityRow{}, manifestError(indexesFile, line, err.Error())
	}
	pattern, err := requiredAtom(indexesFile, line, fields[7], "identity pattern ID")
	if err != nil {
		return IdentityRow{}, err
	}
	return IdentityRow{ID: id, Owner: owner, DeclarationFactID: declaration, PatternID: pattern, RelationKind: relation, DirectFactIDs: direct, ParentIdentityIDs: parents}, nil
}

func parseIndexPlanRow(fields []string, line int, classified map[string]string) (IndexPlanRow, error) {
	id, owner, declaration, err := parsePlanHeader(fields, line, classified, "index-plan")
	if err != nil {
		return IndexPlanRow{}, err
	}
	sources, err := typedFactList("index-plan source FactIDs", fields[4], false)
	if err != nil {
		return IndexPlanRow{}, manifestError(indexesFile, line, err.Error())
	}
	callers, err := typedFactList("index-plan caller FactIDs", fields[5], true)
	if err != nil {
		return IndexPlanRow{}, manifestError(indexesFile, line, err.Error())
	}
	return IndexPlanRow{ID: id, Owner: owner, DeclarationFactID: declaration, SourceFactIDs: sources, CallerUseFactIDs: callers}, nil
}

func parseReferencePlanRow(fields []string, line int, classified map[string]string) (ReferencePlanRow, error) {
	id, err := requiredAtom(indexesFile, line, fields[1], "reference-plan ID")
	if err != nil {
		return ReferencePlanRow{}, err
	}
	if err := claimIndex(classified, id, "reference-plan", line); err != nil {
		return ReferencePlanRow{}, err
	}
	issuer, err := requiredAtom(indexesFile, line, fields[2], "reference-plan issuer")
	if err != nil {
		return ReferencePlanRow{}, err
	}
	consumer, err := requiredAtom(indexesFile, line, fields[3], "reference-plan consumer")
	if err != nil {
		return ReferencePlanRow{}, err
	}
	declaration, err := requiredAtom(indexesFile, line, fields[4], "reference-plan declaration FactID")
	if err != nil {
		return ReferencePlanRow{}, err
	}
	sources, err := typedFactList("reference-plan source FactIDs", fields[5], false)
	if err != nil {
		return ReferencePlanRow{}, manifestError(indexesFile, line, err.Error())
	}
	callers, err := typedFactList("reference-plan caller FactIDs", fields[6], true)
	if err != nil {
		return ReferencePlanRow{}, manifestError(indexesFile, line, err.Error())
	}
	return ReferencePlanRow{ID: id, Issuer: issuer, Consumer: consumer, DeclarationFactID: declaration, SourceFactIDs: sources, CallerUseFactIDs: callers}, nil
}

func parseIdentityPlanRow(fields []string, line int, classified map[string]string) (IdentityPlanRow, error) {
	id, owner, declaration, err := parsePlanHeader(fields, line, classified, "identity-plan")
	if err != nil {
		return IdentityPlanRow{}, err
	}
	relation := IdentityRelationKind(fields[4])
	if !relation.valid() {
		return IdentityPlanRow{}, manifestError(indexesFile, line, "unknown identity-plan relation kind")
	}
	direct, err := typedFactList("identity-plan direct FactIDs", fields[5], false)
	if err != nil {
		return IdentityPlanRow{}, manifestError(indexesFile, line, err.Error())
	}
	parents, err := typedFactList("identity-plan parent IDs", fields[6], true)
	if err != nil {
		return IdentityPlanRow{}, manifestError(indexesFile, line, err.Error())
	}
	return IdentityPlanRow{ID: id, Owner: owner, DeclarationFactID: declaration, RelationKind: relation, DirectFactIDs: direct, ParentIdentityIDs: parents}, nil
}

func parsePlanHeader(fields []string, line int, classified map[string]string, kind string) (string, string, string, error) {
	id, err := requiredAtom(indexesFile, line, fields[1], kind+" ID")
	if err != nil {
		return "", "", "", err
	}
	if err := claimIndex(classified, id, kind, line); err != nil {
		return "", "", "", err
	}
	owner, err := requiredAtom(indexesFile, line, fields[2], kind+" owner/issuer")
	if err != nil {
		return "", "", "", err
	}
	declaration, err := requiredAtom(indexesFile, line, fields[3], kind+" declaration FactID")
	if err != nil {
		return "", "", "", err
	}
	return id, owner, declaration, nil
}

func claimIndex(classified map[string]string, id, kind string, line int) error {
	if previous, exists := classified[id]; exists {
		return classificationError(indexesFile, line, fmt.Sprintf("index fact %q is classified as both %s and %s", id, previous, kind))
	}
	classified[id] = kind
	return nil
}

func parseSurfaceRows(lines []string, result *SurfaceManifest) error {
	seen := make(map[string]string)
	storageSeen := make(map[string]struct{})
	for offset, raw := range lines {
		line := offset + 2
		kind := strings.SplitN(raw, "\t", 2)[0]
		if kind == "storage" {
			fields, err := fieldsFor(surfacesFile, line, raw, 4)
			if err != nil {
				return err
			}
			factID, err := requiredAtom(surfacesFile, line, fields[1], "storage fact ID")
			if err != nil {
				return err
			}
			if _, exists := storageSeen[factID]; exists {
				return classificationError(surfacesFile, line, fmt.Sprintf("storage fact %q is classified more than once", factID))
			}
			storageSeen[factID] = struct{}{}
			ownerSurface, err := requiredAtom(surfacesFile, line, fields[2], "storage owner surface")
			if err != nil {
				return err
			}
			disposition, err := requiredAtom(surfacesFile, line, fields[3], "storage disposition")
			if err != nil {
				return err
			}
			row := StorageRow{FactID: factID, OwnerSurface: ownerSurface, Disposition: StorageDisposition(disposition)}
			if !row.Disposition.valid() {
				return manifestError(surfacesFile, line, "unknown storage disposition enum "+strconv.Quote(disposition))
			}
			result.Storage = append(result.Storage, row)
			continue
		}
		if kind != "field" && kind != "effective-method" && kind != "semantic-package-function" {
			return manifestError(surfacesFile, line, "unknown row kind/columns")
		}
		fields, err := fieldsFor(surfacesFile, line, raw, 4)
		if err != nil {
			return err
		}
		factID, err := requiredAtom(surfacesFile, line, fields[1], "surface fact ID")
		if err != nil {
			return err
		}
		if previous, exists := seen[factID]; exists {
			return classificationError(surfacesFile, line, fmt.Sprintf("surface fact %q is classified as both %s and %s", factID, previous, kind))
		}
		seen[factID] = kind
		ownerSurface, err := requiredAtom(surfacesFile, line, fields[2], "surface owner")
		if err != nil {
			return err
		}
		name, err := requiredAtom(surfacesFile, line, fields[3], "surface member name")
		if err != nil {
			return err
		}
		result.Assignments = append(result.Assignments, SurfaceAssignmentRow{Kind: kind, FactID: factID, OwnerSurface: ownerSurface, Name: name})
	}
	return nil
}

func parseResidueRows(lines []string, result *ResidueManifest, owners []OwnerRow) error {
	seen := make(map[string]struct{})
	ownerIDs := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		ownerIDs[owner.ID] = struct{}{}
	}
	planIDs := make(map[string]struct{})
	planUseCount := make(map[string]int)
	recipientSets := make(map[string]string)
	for offset, raw := range lines {
		line := offset + 2
		kind := strings.SplitN(raw, "\t", 2)[0]
		if kind != "move" && kind != "split" && kind != "delete" && kind != "split-plan" {
			return manifestError(residueFile, line, "unknown row kind/columns")
		}
		fields, err := fieldsFor(residueFile, line, raw, 3)
		if err != nil {
			return err
		}
		if kind == "split-plan" {
			id, err := requiredAtom(residueFile, line, fields[1], "split plan ID")
			if err != nil {
				return err
			}
			if _, duplicate := planIDs[id]; duplicate {
				return classificationError(residueFile, line, "split plan ID is duplicated")
			}
			planIDs[id] = struct{}{}
			ownerValues := strings.Split(fields[2], ",")
			if len(ownerValues) < 2 {
				return manifestError(residueFile, line, "split plan must name at least two OwnerRow IDs")
			}
			for index, ownerID := range ownerValues {
				ownerID, err = requiredAtom(residueFile, line, ownerID, "split plan recipient OwnerRow ID")
				if err != nil {
					return err
				}
				if _, exists := ownerIDs[ownerID]; !exists {
					return manifestError(residueFile, line, "split plan recipient is not an exact Catalog OwnerRow ID")
				}
				if index > 0 && ownerValues[index-1] >= ownerID {
					return manifestError(residueFile, line, "split plan recipient OwnerRow IDs are not sorted and unique")
				}
			}
			if want := splitPlanID(ownerValues); id != want {
				return manifestError(residueFile, line, "split plan ID is not the canonical digest of its sorted recipients")
			}
			recipientKey := strings.Join(ownerValues, "\x00")
			if previous, duplicate := recipientSets[recipientKey]; duplicate && previous != id {
				return classificationError(residueFile, line, "split recipient set has multiple plan IDs")
			}
			recipientSets[recipientKey] = id
			result.SplitPlans = append(result.SplitPlans, SplitPlanRow{ID: id, OwnerIDs: append([]string(nil), ownerValues...)})
			continue
		}
		current, err := requiredAtom(residueFile, line, fields[1], "residue current fact")
		if err != nil {
			return err
		}
		if _, exists := seen[current]; exists {
			return classificationError(residueFile, line, "residue fact has multiple classifications")
		}
		seen[current] = struct{}{}
		destination, err := requiredAtom(residueFile, line, fields[2], "residue destination")
		if err != nil {
			return err
		}
		switch kind {
		case "delete":
			if destination != ResidueDeleteDestination {
				return manifestError(residueFile, line, "delete residue destination must be "+ResidueDeleteDestination)
			}
		case "move":
			if _, exists := ownerIDs[destination]; !exists {
				return manifestError(residueFile, line, "move residue destination is not an exact Catalog OwnerRow ID")
			}
		case "split":
			planUseCount[destination]++
		}
		result.Rows = append(result.Rows, ResidueRow{Kind: kind, CurrentFact: current, Destination: destination})
	}
	for _, plan := range result.SplitPlans {
		if planUseCount[plan.ID] == 0 {
			return manifestError(residueFile, 0, "split plan "+strconv.Quote(plan.ID)+" is unused")
		}
	}
	for planID := range planUseCount {
		if _, exists := planIDs[planID]; !exists {
			return manifestError(residueFile, 0, "split row references unknown plan "+strconv.Quote(planID))
		}
	}
	return nil
}

func classificationError(name string, line int, message string) error {
	return fmt.Errorf("%w: %w: %s:%d: %s", ErrManifestParse, ErrManifestClassification, name, line, message)
}

func ownerKindValue(name string, line int, value string) (string, error) {
	if forbiddenClassification(value) || strings.TrimSpace(value) != value {
		return "", manifestError(name, line, "forbidden or non-canonical owner kind")
	}
	if value != "root" && value != "component" && value != "domain" && value != "artifact" && value != "cold" {
		return "", manifestError(name, line, "unknown owner kind enum "+strconv.Quote(value))
	}
	return value, nil
}

// Report is intentionally scanner-first. Counts are per named production
// surface; there is no aggregate-owner limit in this contract.
type Report struct {
	Mode            Mode
	Scan            ScanResult
	ManifestPresent bool
	GeneratedFresh  bool
	FinalBlockers   []string
}

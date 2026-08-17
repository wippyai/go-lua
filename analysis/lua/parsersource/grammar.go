package parsersource

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Production is one parser-grammar alternative exactly as parser.go.y states
// it: the nonterminal it reduces to, its ordinal among that nonterminal's
// alternatives, the right-hand symbol sequence, and the yacc semantic action in
// both normalized and raw form. It is source evidence, not a Program
// authority: nothing here observes a parse.
type Production struct {
	Nonterminal     string
	Ordinal         int
	RHS             []string
	Action          string
	RawAction       string
	ActionSignature string
}

// Alternative is one parser-grammar production exposed to cold proof
// components. RHS is copied, and Action remains intentionally private: parser
// action implementation is not a second public semantic authority.
type Alternative struct {
	Key         string
	Nonterminal string
	Ordinal     int
	RHS         []string
}

// ActionTemplate is one complete yacc semantic-action law.  It is cold proof
// material: ActionDigest names the normalized parser action which determines
// the legal constructor product for this alternative. Constructors is only an
// index into that law; it is never used to invent a Cartesian product.
//
// The normalized action source deliberately remains private. parser.go.y is
// its sole authority, while this compact projection lets generated proof
// artifacts fail closed when a constructor route changes.
type ActionTemplate struct {
	Key          string
	Nonterminal  string
	ResultTag    string
	Ordinal      int
	RHS          []string
	ActionDigest string
	Constructors []string
}

// HelperTemplate is one parser-header helper callable by a yacc semantic
// action. It is cold proof material only. Parsed helper syntax is available
// through VisitHelperSyntax during cold construction, never as a stored text
// payload.
type HelperTemplate struct {
	Name   string
	Digest string
}

// Alternatives derives the complete parser grammar directly from parser.go.y.
// It is source authority only; it does not observe fixtures or parser output.
func Alternatives(root string) ([]Alternative, error) {
	rows, err := ExtractGrammar(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return nil, err
	}
	result := make([]Alternative, len(rows))
	for index, row := range rows {
		result[index] = Alternative{
			Key:         ProductionKey(row),
			Nonterminal: row.Nonterminal,
			Ordinal:     row.Ordinal,
			RHS:         append([]string(nil), row.RHS...),
		}
	}
	return result, nil
}

// VisitActionSyntax gives cold proof builders the parsed Go body of every
// yacc action.  The raw yacc action never becomes an evidence field: it is
// translated to the legal ArgN/Result vocabulary, parsed, visited, and then
// discarded by the caller.  Keeping this boundary here makes grammarproof the
// sole owner of parser.go.y syntax extraction.
func VisitActionSyntax(root string, visit func(ActionTemplate, *goast.BlockStmt) error) error {
	if visit == nil {
		return fmt.Errorf("grammar action syntax visitor is nil")
	}
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	rows, err := ExtractGrammar(path)
	if err != nil {
		return err
	}
	tags, err := DeclaredResultTags(path)
	if err != nil {
		return err
	}
	for _, row := range rows {
		tag, ok := tags[row.Nonterminal]
		if !ok || tag == "" {
			return fmt.Errorf("parser grammar %s: nonterminal %s has no declared %%type result tag", path, row.Nonterminal)
		}
		action := yaccActionOperands.ReplaceAllStringFunc(row.RawAction, func(operand string) string {
			if operand == "$$" {
				return "Result"
			}
			return "Arg" + strings.TrimPrefix(operand, "$")
		})
		file, parseErr := parser.ParseFile(token.NewFileSet(), "parser-action.go", "package parseraction\nfunc action() "+action, 0)
		if parseErr != nil {
			return fmt.Errorf("parse parser action %s: %w", ProductionKey(row), parseErr)
		}
		if len(file.Decls) != 1 {
			return fmt.Errorf("parse parser action %s: expected one declaration", ProductionKey(row))
		}
		function, ok := file.Decls[0].(*goast.FuncDecl)
		if !ok || function.Body == nil {
			return fmt.Errorf("parse parser action %s: expected function body", ProductionKey(row))
		}
		template := ActionTemplate{
			Key:          ProductionKey(row),
			Nonterminal:  row.Nonterminal,
			ResultTag:    tag,
			Ordinal:      row.Ordinal,
			RHS:          append([]string(nil), row.RHS...),
			ActionDigest: row.ActionSignature,
			Constructors: actionConstructors(row.Action),
		}
		if err := visit(template, function.Body); err != nil {
			return err
		}
	}
	return nil
}

var yaccActionOperands = regexp.MustCompile(`\$\$|\$[0-9]+`)

var yaccType = regexp.MustCompile(`(?m)^\s*%type<([^>]+)>\s+(.+)$`)

// DeclaredResultTags maps every nonterminal to the %type result tag parser.go.y
// declares for it. A nonterminal without a declaration, or with two, fails
// closed rather than defaulting to an untagged result.
func DeclaredResultTags(path string) (map[string]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string)
	for _, match := range yaccType.FindAllStringSubmatch(string(contents), -1) {
		declared := strings.TrimSpace(strings.SplitN(match[2], "//", 2)[0])
		for _, name := range strings.Fields(declared) {
			if tags[name] != "" {
				return nil, fmt.Errorf("parser grammar %s: duplicate %%type declaration for %s", path, name)
			}
			tags[name] = match[1]
		}
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("has no %%type declarations")
	}
	return tags, nil
}

// HelperTemplates derives every Go helper declaration callable by a yacc
// semantic action. yacc permits helpers both in its %{...%} preamble and
// after the second %% delimiter; both regions are parser authority. A
// malformed or duplicate declaration fails closed instead of silently
// omitting an assembly route.
func HelperTemplates(root string) ([]HelperTemplate, error) {
	functions, err := parserHelperSyntax(root)
	if err != nil {
		return nil, err
	}
	result := make([]HelperTemplate, len(functions))
	for index, function := range functions {
		result[index] = function.template
	}
	return result, nil
}

// VisitHelperSyntax gives cold proof builders parsed helper declarations.
// Neither the declaration text nor formal names are stored in the returned
// templates; those ASTs are valid only for the duration of the callback.
func VisitHelperSyntax(root string, visit func(HelperTemplate, *goast.FuncDecl) error) error {
	if visit == nil {
		return fmt.Errorf("grammar helper syntax visitor is nil")
	}
	functions, err := parserHelperSyntax(root)
	if err != nil {
		return err
	}
	for _, function := range functions {
		if err := visit(function.template, function.declaration); err != nil {
			return err
		}
	}
	return nil
}

type parserHelperSyntaxRow struct {
	template    HelperTemplate
	declaration *goast.FuncDecl
}

func parserHelperSyntax(root string) ([]parserHelperSyntaxRow, error) {
	contents, err := os.ReadFile(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return nil, err
	}
	source := string(contents)
	start := strings.Index(source, "%{")
	if start < 0 {
		return nil, fmt.Errorf("parser preamble start is missing")
	}
	endOffset := strings.Index(source[start+2:], "%}")
	if endOffset < 0 {
		return nil, fmt.Errorf("parser preamble end is missing")
	}
	preamble := source[start+2 : start+2+endOffset]
	postamble, err := yaccPostamble(source)
	if err != nil {
		return nil, err
	}
	sections := []struct {
		name   string
		source string
	}{
		{name: "preamble", source: preamble},
		{name: "postamble", source: "package parse\n" + postamble},
	}
	var result []parserHelperSyntaxRow
	for _, section := range sections {
		file, parseErr := parser.ParseFile(token.NewFileSet(), "parser-"+section.name+".go", section.source, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("parse parser %s: %w", section.name, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*goast.FuncDecl)
			if !ok || function.Recv != nil || function.Name == nil {
				continue
			}
			var rendered strings.Builder
			if err := format.Node(&rendered, token.NewFileSet(), function); err != nil {
				return nil, fmt.Errorf("render parser helper %s: %w", function.Name.Name, err)
			}
			source := rendered.String()
			sum := sha256.Sum256([]byte(source))
			result = append(result, parserHelperSyntaxRow{template: HelperTemplate{Name: function.Name.Name, Digest: hex.EncodeToString(sum[:])}, declaration: function})
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].template.Name < result[right].template.Name })
	for index, helper := range result {
		if helper.template.Name == "" || helper.template.Digest == "" || index != 0 && result[index-1].template.Name >= helper.template.Name {
			return nil, fmt.Errorf("parser helpers are not canonical")
		}
	}
	return result, nil
}

func yaccPostamble(source string) (string, error) {
	masked, err := maskQuotedAndComments(source)
	if err != nil {
		return "", err
	}
	markers := 0
	for start := 0; start <= len(masked); {
		end := strings.IndexByte(masked[start:], '\n')
		if end < 0 {
			end = len(masked)
		} else {
			end += start
		}
		if strings.TrimSpace(masked[start:end]) == "%%" {
			markers++
			if markers == 2 {
				after := end
				if after < len(source) {
					after++
				}
				return source[after:], nil
			}
		}
		if end == len(masked) {
			break
		}
		start = end + 1
	}
	return "", fmt.Errorf("parser postamble delimiter is missing")
}

// GrammarActionDigest is the opaque fingerprint of the complete parser
// production/action contract. It is intended for cold profile proofs which
// must be revisited when parser syntax or a yacc semantic action changes. It
// does not include fixtures, parser output, or Program lowering.
func GrammarActionDigest(root string) (string, error) {
	rows, err := ExtractGrammar(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, row := range rows {
		_, _ = hash.Write([]byte(ProductionDigestLine(row)))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ProductionDigestLine is the canonical committed form of one production. It
// exists so every digest that commits parser alternatives - this package's own
// action digest and the cold proof evidence digests that additionally commit a
// source corpus - hashes the same bytes for the same production.
func ProductionDigestLine(production Production) string {
	return fmt.Sprintf("%s\x00%s\x00%s\n", ProductionKey(production), strings.Join(production.RHS, "\x1f"), production.ActionSignature)
}

// ParserSourceDigest commits the entire yacc source, including parser-only
// helper functions that semantic actions can call. GrammarActionDigest tracks
// alternatives compactly; this companion closes the otherwise-hidden helper
// route for cold constructor-product laws. It is intentionally fail-closed on
// any parser.go.y source change and is not a runtime parser input.
func ParserSourceDigest(root string) (string, error) {
	contents, err := os.ReadFile(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), nil
}

// ExtractGrammar reads every alternative of every rule from a parser.go.y
// source, in nonterminal then ordinal order. It is the sole extraction route
// for parser grammar text.
func ExtractGrammar(path string) ([]Production, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	section, err := yaccProductionSection(string(contents))
	if err != nil {
		return nil, fmt.Errorf("parser grammar %s: %w", path, err)
	}
	rows, err := parseGrammarAlternatives(section)
	if err != nil {
		return nil, fmt.Errorf("parser grammar %s: %w", path, err)
	}
	return rows, nil
}

// ProductionKey names one alternative by its nonterminal and ordinal. It is
// the identity every cold proof component joins productions on.
func ProductionKey(production Production) string {
	return fmt.Sprintf("%s#%d", production.Nonterminal, production.Ordinal)
}

type grammarSpan struct{ start, end int }

func yaccProductionSection(source string) (string, error) {
	masked, err := maskQuotedAndComments(source)
	if err != nil {
		return "", err
	}
	var markers []grammarSpan
	for start := 0; start <= len(masked); {
		end := strings.IndexByte(masked[start:], '\n')
		if end < 0 {
			end = len(masked)
		} else {
			end += start
		}
		if strings.TrimSpace(masked[start:end]) == "%%" {
			after := end
			if after < len(masked) {
				after++
			}
			markers = append(markers, grammarSpan{start: start, end: after})
			if len(markers) == 2 {
				return source[markers[0].end:markers[1].start], nil
			}
		}
		if end == len(masked) {
			break
		}
		start = end + 1
	}
	return "", fmt.Errorf("has %d production-section delimiters, want at least 2", len(markers))
}

type grammarRuleHeader struct {
	name string
	line int
	body int
}

type grammarAlternative struct {
	rhs    string
	action string
}

func parseGrammarAlternatives(source string) ([]Production, error) {
	var rows []Production
	position := 0
	for {
		header, found, err := nextGrammarRuleHeader(source, position)
		if err != nil {
			return nil, err
		}
		if !found {
			break
		}
		alternatives, next, err := scanGrammarAlternatives(source, header)
		if err != nil {
			return nil, fmt.Errorf("rule %s: %w", header.name, err)
		}
		for ordinal, alternative := range alternatives {
			rhs := grammarSymbols(alternative.rhs)
			rows = append(rows, Production{
				Nonterminal:     header.name,
				Ordinal:         ordinal + 1,
				RHS:             rhs,
				Action:          normalizeGrammarAction(alternative.action),
				RawAction:       alternative.action,
				ActionSignature: grammarActionSignature(rhs, alternative.action),
			})
		}
		position = next
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("has no grammar productions")
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Nonterminal != rows[right].Nonterminal {
			return rows[left].Nonterminal < rows[right].Nonterminal
		}
		return rows[left].Ordinal < rows[right].Ordinal
	})
	return rows, nil
}

// actionConstructors is a lexical index, not a parser for Go action code.
// The action digest remains the complete law.  Keeping this index tiny and
// deterministic avoids a second semantic-action interpreter in the proof
// layer while still connecting each direct ast.Foo constructor to its exact
// owning parser alternative.
func actionConstructors(action string) []string {
	seen := make(map[string]bool)
	for index := 0; index+4 < len(action); index++ {
		if !strings.HasPrefix(action[index:], "ast.") {
			continue
		}
		start := index + len("ast.")
		if start >= len(action) || !grammarIdentifierPart(action[start]) {
			continue
		}
		end := start + 1
		for end < len(action) && grammarIdentifierPart(action[end]) {
			end++
		}
		seen[action[start:end]] = true
		index = end - 1
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// grammarActionSignature is intentionally based on the normalized full action.
// That includes every constructor and mutation change without reimplementing a
// second AST-source authority inside the proof package.
func grammarActionSignature(rhs []string, action string) string {
	sum := sha256.Sum256([]byte(strings.Join(rhs, "\x1f") + "\x00" + normalizeGrammarAction(action)))
	return hex.EncodeToString(sum[:])
}

func normalizeGrammarAction(action string) string {
	var out strings.Builder
	space := false
	for index := 0; index < len(action); {
		if startsComment(action, index) {
			end, _ := scanComment(action, index)
			index = end
			space = true
			continue
		}
		if whitespace(action[index]) {
			space = true
			index++
			continue
		}
		if space && out.Len() != 0 {
			previous := out.String()[out.Len()-1]
			if grammarIdentifierPart(previous) && grammarIdentifierPart(action[index]) {
				out.WriteByte(' ')
			}
		}
		space = false
		if !isQuote(action[index]) {
			out.WriteByte(action[index])
			index++
			continue
		}
		quote := action[index]
		out.WriteByte(quote)
		index++
		for index < len(action) {
			current := action[index]
			out.WriteByte(current)
			index++
			if current == '\\' && quote != '`' && index < len(action) {
				out.WriteByte(action[index])
				index++
				continue
			}
			if current == quote {
				break
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func nextGrammarRuleHeader(source string, start int) (grammarRuleHeader, bool, error) {
	for index := start; index < len(source); {
		if index == 0 || source[index-1] == '\n' {
			if header, found := grammarRuleHeaderAtLine(source, index); found {
				return header, true, nil
			}
		}
		switch {
		case startsComment(source, index):
			end, err := scanComment(source, index)
			if err != nil {
				return grammarRuleHeader{}, false, err
			}
			index = end
		case isQuote(source[index]):
			end, err := scanQuoted(source, index)
			if err != nil {
				return grammarRuleHeader{}, false, err
			}
			index = end
		case source[index] == '{':
			end, err := scanGoAction(source, index)
			if err != nil {
				return grammarRuleHeader{}, false, err
			}
			index = end
		default:
			index++
		}
	}
	return grammarRuleHeader{}, false, nil
}

func grammarRuleHeaderAtLine(source string, line int) (grammarRuleHeader, bool) {
	index := line
	for index < len(source) && (source[index] == ' ' || source[index] == '\t' || source[index] == '\r') {
		index++
	}
	if index >= len(source) || !grammarIdentifierStart(source[index]) {
		return grammarRuleHeader{}, false
	}
	nameStart := index
	index++
	for index < len(source) && grammarIdentifierPart(source[index]) {
		index++
	}
	name := source[nameStart:index]
	for index < len(source) && (source[index] == ' ' || source[index] == '\t') {
		index++
	}
	if index >= len(source) || source[index] != ':' {
		return grammarRuleHeader{}, false
	}
	return grammarRuleHeader{name: name, line: line, body: index + 1}, true
}

func scanGrammarAlternatives(source string, header grammarRuleHeader) ([]grammarAlternative, int, error) {
	var alternatives []grammarAlternative
	var rhs, action strings.Builder
	segment := header.body
	finish := func(end int) {
		rhs.WriteString(source[segment:end])
		alternatives = append(alternatives, grammarAlternative{rhs: strings.TrimSpace(rhs.String()), action: strings.TrimSpace(action.String())})
		rhs.Reset()
		action.Reset()
	}
	for index := header.body; index < len(source); {
		if index == 0 || source[index-1] == '\n' {
			if next, found := grammarRuleHeaderAtLine(source, index); found {
				finish(next.line)
				return alternatives, next.line, nil
			}
		}
		switch {
		case startsComment(source, index):
			rhs.WriteString(source[segment:index])
			rhs.WriteByte(' ')
			end, err := scanComment(source, index)
			if err != nil {
				return nil, 0, err
			}
			index, segment = end, end
		case isQuote(source[index]):
			end, err := scanQuoted(source, index)
			if err != nil {
				return nil, 0, err
			}
			index = end
		case source[index] == '{':
			rhs.WriteString(source[segment:index])
			end, err := scanGoAction(source, index)
			if err != nil {
				return nil, 0, err
			}
			if action.Len() != 0 {
				action.WriteByte('\n')
			}
			action.WriteString(source[index:end])
			index, segment = end, end
		case source[index] == '|':
			finish(index)
			index++
			segment = index
		default:
			index++
		}
	}
	finish(len(source))
	return alternatives, len(source), nil
}

func scanGoAction(source string, start int) (int, error) {
	depth := 0
	for index := start; index < len(source); {
		switch {
		case startsComment(source, index):
			end, err := scanComment(source, index)
			if err != nil {
				return 0, err
			}
			index = end
		case isQuote(source[index]):
			end, err := scanQuoted(source, index)
			if err != nil {
				return 0, err
			}
			index = end
		case source[index] == '{':
			depth++
			index++
		case source[index] == '}':
			depth--
			index++
			if depth == 0 {
				return index, nil
			}
		default:
			index++
		}
	}
	return 0, fmt.Errorf("action starting at byte %d has no matching closing brace", start)
}

func scanQuoted(source string, start int) (int, error) {
	quote := source[start]
	for index := start + 1; index < len(source); index++ {
		if quote != '`' && source[index] == '\\' {
			index++
			continue
		}
		if source[index] == quote {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("quoted literal starting at byte %d is not terminated", start)
}

func startsComment(source string, index int) bool {
	return index+1 < len(source) && source[index] == '/' && (source[index+1] == '/' || source[index+1] == '*')
}

func scanComment(source string, start int) (int, error) {
	if source[start+1] == '/' {
		if end := strings.IndexByte(source[start+2:], '\n'); end >= 0 {
			return start + 2 + end, nil
		}
		return len(source), nil
	}
	if end := strings.Index(source[start+2:], "*/"); end >= 0 {
		return start + 2 + end + 2, nil
	}
	return 0, fmt.Errorf("block comment starting at byte %d is not terminated", start)
}

func maskQuotedAndComments(source string) (string, error) {
	masked := []byte(source)
	for index := 0; index < len(source); {
		var end int
		var err error
		switch {
		case startsComment(source, index):
			end, err = scanComment(source, index)
		case isQuote(source[index]):
			end, err = scanQuoted(source, index)
		default:
			index++
			continue
		}
		if err != nil {
			return "", err
		}
		for offset := index; offset < end; offset++ {
			if masked[offset] != '\n' && masked[offset] != '\r' {
				masked[offset] = ' '
			}
		}
		index = end
	}
	return string(masked), nil
}

func grammarSymbols(rhs string) []string {
	var symbols []string
	for index := 0; index < len(rhs); {
		for index < len(rhs) && whitespace(rhs[index]) {
			index++
		}
		if index == len(rhs) {
			break
		}
		start := index
		if isQuote(rhs[index]) {
			end, err := scanQuoted(rhs, index)
			if err != nil {
				return append(symbols, rhs[start:])
			}
			index = end
		} else {
			for index < len(rhs) && !whitespace(rhs[index]) {
				index++
			}
		}
		symbols = append(symbols, rhs[start:index])
	}
	return symbols
}

func grammarIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func grammarIdentifierPart(value byte) bool {
	return grammarIdentifierStart(value) || value >= '0' && value <= '9'
}

func isQuote(value byte) bool { return value == '\'' || value == '"' || value == '`' }

func whitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

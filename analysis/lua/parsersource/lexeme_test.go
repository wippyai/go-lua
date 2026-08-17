package parsersource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryScannedTerminalIsAGrammarTerminal states that the two source
// authorities name the same symbols. The lexer contract exists so a consumer
// can join a scanned lexeme against a grammar alternative, and a row naming a
// symbol the grammar never mentions would join against nothing while reading
// like evidence, so the join is checked against parser.go.y itself.
func TestEveryScannedTerminalIsAGrammarTerminal(t *testing.T) {
	root := moduleRoot(t)
	contract, err := DiscoverLexerContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Terminals) == 0 {
		t.Fatal("the scanner emits no terminal")
	}
	grammar := grammarTerminals(t, root)
	seen := make(map[string]bool, len(contract.Terminals))
	previous := ""
	for _, row := range contract.Terminals {
		if row.Terminal == "" {
			t.Fatal("the scanner emits an unnamed terminal")
		}
		if seen[row.Terminal] {
			t.Fatalf("terminal %s carries two lexeme judgements", row.Terminal)
		}
		seen[row.Terminal] = true
		if previous != "" && row.Terminal <= previous {
			t.Fatalf("terminal %s follows %s out of order", row.Terminal, previous)
		}
		previous = row.Terminal
		if !grammar[row.Terminal] {
			t.Fatalf("the scanner emits %s, which parser.go.y never names", row.Terminal)
		}
	}
}

// TestNonEmptyLexemeHoldsExactlyForAnchoredScans states the law the contract
// carries: a scanned lexeme is non-empty exactly when its scanner anchors on the
// character that triggered it. An identifier, a number and every reserved word
// are written starting from that character, so their text can never be empty; a
// string is delimited rather than anchored, so an empty literal scans to empty
// text and no consumer may assume otherwise. The fixture is the shipped scanner
// source, so the law is stated against the lexer the compiler actually runs.
func TestNonEmptyLexemeHoldsExactlyForAnchoredScans(t *testing.T) {
	root := moduleRoot(t)
	contract, err := DiscoverLexerContract(root)
	if err != nil {
		t.Fatal(err)
	}
	judgement := make(map[string]bool, len(contract.Terminals))
	for _, row := range contract.Terminals {
		judgement[row.Terminal] = row.NonEmptyText
	}
	for _, terminal := range []string{"TIdent", "TNumber"} {
		nonEmpty, present := judgement[terminal]
		if !present {
			t.Fatalf("the scanner never emits %s", terminal)
		}
		if !nonEmpty {
			t.Fatalf("%s anchors on its first character but its text is not proven non-empty", terminal)
		}
	}
	nonEmpty, present := judgement["TString"]
	if !present {
		t.Fatal("the scanner never emits TString")
	}
	if nonEmpty {
		t.Fatal("TString is delimited rather than anchored, so an empty literal makes its text empty")
	}
	words := reservedWordTerminals(t, root)
	if len(words) == 0 {
		t.Fatal("the scanner recognizes no reserved word")
	}
	for _, word := range words {
		reserved, present := judgement[word]
		if !present {
			t.Fatalf("reserved word %s is recognized but never emitted", word)
		}
		if !reserved {
			t.Fatalf("reserved word %s is an identifier scan, so its text cannot be empty", word)
		}
	}
	character := 0
	for terminal, proven := range judgement {
		if !strings.HasPrefix(terminal, "'") {
			continue
		}
		character++
		if !proven {
			t.Fatalf("character terminal %s is written from the character it matched, so its text cannot be empty", terminal)
		}
	}
	if character == 0 {
		t.Fatal("the scanner emits no character terminal")
	}
}

// TestScannerStampsNoZeroPosition states that a zero position in a parsed tree
// is a missing stamp. The scanner seeds its line at one and only ever increments
// it or sets it to the end sentinel, so no token it produces can carry the zero
// position, and a consumer may read a zero position as evidence that some other
// stage failed to fill one in.
func TestScannerStampsNoZeroPosition(t *testing.T) {
	root := moduleRoot(t)
	contract, err := DiscoverLexerContract(root)
	if err != nil {
		t.Fatal(err)
	}
	if !contract.NonZeroPositions {
		t.Fatal("the scanner does not prove its stamped positions differ from the zero position")
	}
}

// grammarTerminals reads every symbol parser.go.y names, from its terminal
// declarations and from the right-hand sides of its alternatives. Character
// terminals need no declaration in yacc, so the productions are as much of the
// terminal authority as the declaration lines are.
func grammarTerminals(t *testing.T, root string) map[string]bool {
	t.Helper()
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	named := make(map[string]bool)
	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		declaration := ""
		for _, keyword := range []string{"%token", "%left", "%right", "%nonassoc"} {
			if strings.HasPrefix(trimmed, keyword) {
				declaration = strings.TrimPrefix(trimmed, keyword)
			}
		}
		if declaration == "" {
			continue
		}
		if index := strings.Index(declaration, "/*"); index >= 0 {
			declaration = declaration[:index]
		}
		if index := strings.Index(declaration, ">"); strings.HasPrefix(strings.TrimSpace(declaration), "<") && index >= 0 {
			declaration = declaration[index+1:]
		}
		for _, symbol := range strings.Fields(declaration) {
			named[symbol] = true
		}
	}
	productions, err := Alternatives(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(productions) == 0 {
		t.Fatal("parser.go.y states no alternative")
	}
	for _, production := range productions {
		for _, symbol := range production.RHS {
			named[symbol] = true
		}
	}
	return named
}

// reservedWordTerminals reads the token constants the scanner's reserved-word
// table maps to. The list is read from the shipped scanner rather than written
// here so that a keyword added to the scanner is covered by the law without the
// test being edited.
func reservedWordTerminals(t *testing.T, root string) []string {
	t.Helper()
	path := filepath.Join(root, "compiler", "parse", "lexer.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(contents)
	start := strings.Index(source, "reservedWords = map[string]int{")
	if start < 0 {
		t.Fatal("the scanner declares no reserved-word table")
	}
	body := source[start:]
	end := strings.Index(body, "}")
	if end < 0 {
		t.Fatal("the scanner reserved-word table is unterminated")
	}
	var words []string
	for _, entry := range strings.Split(body[strings.Index(body, "{")+1:end], ",") {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			continue
		}
		constant := strings.TrimSpace(parts[1])
		if constant == "" {
			continue
		}
		words = append(words, constant)
	}
	return words
}

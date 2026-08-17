package frozen

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/census"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// The frozen parser-construction evidence in the grammarproof tree derived the
// same two facts these rows now derive: which whole-constructor field vectors a
// yacc action produces, and which carrier states no action can produce. These
// receipts state that the census reaches the same answers from the same cold
// sources, so the frozen evidence is a second reading of one derivation rather
// than a second derivation.
//
// Where the two disagree the receipt names the disagreement instead of
// absorbing it. A receipt that closed by filtering would be proving that the
// filter was tuned, not that the derivations agree.

// TestProductRowsReproduceFrozenParserProducts is the product receipt: once the
// two grain differences are named, the derived field vectors and the frozen
// ones are the same rows, key for key, with nothing missing and nothing extra.
//
// The grain differences are both real and both stated on the row rather than
// hidden by the comparison. The frozen evidence files a construction inside a
// diagnosed branch under its rejection law and a construction performed once
// per element of an input sequence under its map-summary law; this grain keeps
// both as products, because each is a whole-constructor field vector the parser
// can build, and marks which it is.
func TestProductRowsReproduceFrozenParserProducts(t *testing.T) {
	root := moduleRoot(t)
	value, err := census.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	derived := make(map[string]bool, len(value.Products))
	rejected, elementwise := 0, 0
	for _, product := range value.Products {
		if product.Rejected {
			rejected++
			continue
		}
		if product.Elementwise {
			elementwise++
			continue
		}
		key := productKey(product.Owner, product.Ordinal, product.Constructor, assignedVector(product))
		if derived[key] {
			t.Fatalf("derived product %s is stated twice", key)
		}
		derived[key] = true
	}
	if rejected == 0 || elementwise == 0 {
		t.Fatalf("the receipt names %d rejected and %d elementwise rows, so a grain difference it accounts for is untested", rejected, elementwise)
	}
	frozen := frozenProductKeys(t)
	if len(frozen) == 0 {
		t.Fatal("the frozen parser evidence states no constructor products")
	}
	var missing, extra []string
	for key := range frozen {
		if !derived[key] {
			missing = append(missing, key)
		}
	}
	for key := range derived {
		if !frozen[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("product rows differ from the frozen evidence:\nmissing %v\nextra %v", missing, extra)
	}
	if len(derived) != len(frozen) {
		t.Fatalf("derived %d product rows, the frozen evidence states %d", len(derived), len(frozen))
	}
}

func productKey(owner string, ordinal int, constructor string, vector []string) string {
	return fmt.Sprintf("%s#%d:%s[%s]", owner, ordinal, constructor, strings.Join(vector, " "))
}

func assignedVector(product parsersource.ActionProduct) []string {
	vector := make([]string, 0, len(product.Fields))
	for _, coordinate := range product.Fields {
		state := "zero"
		if coordinate.Assigned {
			state = "term"
		}
		vector = append(vector, coordinate.Field+"="+state)
	}
	return vector
}

func frozenProductKeys(t *testing.T) map[string]bool {
	t.Helper()
	evidence := parserproducts.Generated
	result := make(map[string]bool)
	add := func(owner string, product parserproducts.ConstructorProduct) {
		vector := make([]string, 0, len(product.Fields))
		for _, coordinate := range product.Fields {
			state := "zero"
			if coordinate.Kind == parserproducts.ActionValueTerm {
				state = "term"
			}
			vector = append(vector, coordinate.Field+"="+state)
		}
		key := productKey(owner, product.Ordinal, product.Constructor, vector)
		if result[key] {
			t.Fatalf("frozen product %s is stated twice", key)
		}
		result[key] = true
	}
	for _, law := range evidence.ProductLaws {
		for _, product := range law.Products {
			add(law.Production, product)
		}
	}
	for _, law := range evidence.HelperLaws {
		scope, ok := evidence.ActionTerms.Scope(law.Scope)
		if !ok {
			t.Fatal("frozen helper law names no scope")
		}
		owner, ok := evidence.ActionTerms.Symbol(scope.Owner)
		if !ok {
			t.Fatal("frozen helper scope names no callable")
		}
		for _, product := range law.Products {
			add(owner.Text, product)
		}
	}
	return result
}

// TestDerivedParserImpossibilityIsSoundAgainstFrozenEvidence is the half of the
// state receipt that must never bend. The frozen evidence carries, for every
// carrier state, whether a source witness reached it; a derivation that called
// any of those states impossible would be claiming the parser cannot build
// something it demonstrably built.
func TestDerivedParserImpossibilityIsSoundAgainstFrozenEvidence(t *testing.T) {
	root := moduleRoot(t)
	value, err := census.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	derived := make(map[string]bool)
	for _, coordinate := range census.StateSpace(value).Impossible {
		derived[carrierStateKey(coordinate.Form, coordinate.Field, coordinate.State)] = true
	}
	witnessed := 0
	for _, row := range parserproducts.Generated.Fields {
		if row.Disposition == parserproducts.DispositionImpossible {
			continue
		}
		key := carrierStateKey(row.Form, row.Field, parsersource.FieldState(row.State))
		witnessed++
		if derived[key] {
			t.Fatalf("carrier state %s is derived parser-impossible, the frozen evidence reaches it with disposition %d", key, row.Disposition)
		}
	}
	if witnessed == 0 {
		t.Fatal("the frozen evidence reaches no carrier state, so the soundness receipt is vacuous")
	}
}

// TestDerivedParserImpossibilityAccountsForTheFrozenTable is the other half.
// Every state the derivation calls impossible is one the frozen hand-authored
// table also calls impossible, and the one row the frozen table states and the
// derivation does not is named here with the reason it stands.
//
// LabelStmt.Name empty is a frozen row that is right and this derivation cannot
// see. Every other token-fed name carrier is decided by the scanner's anchored
// buffer scanners, which write the character that triggered them before
// anything conditional; the label lexeme alone is assembled by index arithmetic
// over a peeked byte slice, so its non-emptiness is a property of a loop bound
// rather than of an anchor, and the lexeme contract does not reach it.
//
// FunctionParamExpr.Type absent was the second standing row and is one no
// longer. The frozen table now carries it as parser-reachable and rejected by
// public ingress, which is what the derivation reads, so the two authorities
// agree on it and it leaves this account.
func TestDerivedParserImpossibilityAccountsForTheFrozenTable(t *testing.T) {
	root := moduleRoot(t)
	value, err := census.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	derived := make(map[string]bool)
	for _, coordinate := range census.StateSpace(value).Impossible {
		derived[carrierStateKey(coordinate.Form, coordinate.Field, coordinate.State)] = true
	}
	frozen := make(map[string]bool)
	for _, row := range parserproducts.Generated.Fields {
		if row.Disposition != parserproducts.DispositionImpossible {
			continue
		}
		frozen[carrierStateKey(row.Form, row.Field, parsersource.FieldState(row.State))] = true
	}
	if len(frozen) == 0 {
		t.Fatal("the frozen evidence states no parser-impossible carrier state")
	}
	var claimed []string
	for key := range derived {
		if !frozen[key] {
			claimed = append(claimed, key)
		}
	}
	sort.Strings(claimed)
	if len(claimed) != 0 {
		t.Fatalf("the derivation calls %v parser-impossible, the frozen table does not", claimed)
	}
	standing := map[string]string{
		"LabelStmt.Name@empty": "the frozen row is right: the label lexeme is assembled outside the scanner's anchored buffer scans",
	}
	var residue []string
	for key := range frozen {
		if !derived[key] {
			residue = append(residue, key)
		}
	}
	sort.Strings(residue)
	for _, key := range residue {
		if _, named := standing[key]; !named {
			t.Fatalf("the frozen table states %s parser-impossible with no account for why the derivation does not", key)
		}
	}
	if len(residue) != len(standing) {
		t.Fatalf("the standing account names %d rows, the frozen table leaves %v", len(standing), residue)
	}
}

func carrierStateKey(form, field string, state parsersource.FieldState) string {
	return form + "." + field + "@" + state.String()
}

// TestUntypedInterfaceMethodParameterHasAbsentType is the source evidence that
// the derivation reads FunctionParamExpr.Type absent correctly. It parses the
// exact source that reaches the state and reads the field, so the census-side
// claim that the state is parser-reachable rests on the parser itself rather
// than on this package's own derivation.
func TestUntypedInterfaceMethodParameterHasAbsentType(t *testing.T) {
	const source = "interface I\n  function m(a)\nend\n"
	statements, err := parse.ParseString(source, "census-witness.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	reached := false
	for _, statement := range statements {
		declaration, ok := statement.(*ast.InterfaceDefStmt)
		if !ok {
			continue
		}
		for _, member := range declaration.Members {
			signature, ok := member.Type.(*ast.FunctionTypeExpr)
			if !ok {
				continue
			}
			for _, parameter := range signature.Params {
				if parameter.Type == nil {
					reached = true
				}
			}
		}
	}
	if !reached {
		t.Fatal("an untyped interface method parameter does not reach an absent parameter type")
	}
}

// moduleRoot walks up from this test source until it finds the directory that
// owns go.mod, so the receipt reads the same sources the census does no matter
// where it is run from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate frozen receipt source")
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

package parsersource

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProductScope names the two owners a construction can have. A yacc semantic
// action owns the constructions its own reduction performs; a parser helper
// owns the constructions every call to it performs. They are kept apart because
// a helper is reached from many reductions, so attributing its constructions to
// one of them would state a law about an alternative that does not hold it.
type ProductScope uint8

const (
	ProductScopeInvalid ProductScope = iota
	ProductScopeProduction
	ProductScopeHelper
)

// ProductField is one coordinate of a whole-constructor field vector. Assigned
// separates a coordinate the construction names from one it omits: an omitted
// coordinate is not absent evidence, it is evidence that the field keeps its
// declared zero state. States is the complete set of representation states the
// coordinate can hold once the construction has run.
//
// Member is the discriminant refinement, and it is stated only where a closed
// constant family makes it decidable: the carrier's declared type is an
// admitted DiscriminantEnum and this construction leaves exactly one of that
// family's members in it. Everywhere else it is empty and States is the whole
// account, because a carrier with no closed family has no member to name and a
// construction reaching a carrier through a computed value has not chosen one.
// The column refines the vector, it does not widen the state space: two
// constructions differing only in which member they assign are two rows here
// and one row under States alone.
type ProductField struct {
	Ordinal  int
	Field    string
	Assigned bool
	Member   string
	States   []FieldState
}

// ActionProduct is one whole-constructor field vector one parser action can
// produce. Ordinal orders the constructions of one action by the walk that
// starts at the value the action yields, so a construction that only exists to
// feed another is stated after the one it feeds.
//
// Guarded records that the construction sits behind a branch of its action, and
// Rejected that the branch it sits behind has already raised a parse
// diagnostic. Neither weakens the construction as evidence: a rejected branch
// still assigns the fields it names, so a law about what the parser can build
// reads both, while a law about what a successful parse yields reads only the
// rest.
//
// Elementwise records that the action performs this construction once per
// element of an input sequence rather than once per reduction. The field vector
// is the same law either way, so the row is stated here rather than being left
// to a summary of the loop that drives it: a vector reachable only through such
// a loop is still a vector the parser produces.
// Root records that the action yields this construction rather than reaching it
// through a coordinate of another construction it performs. An action can yield
// more than one, because a branch that assigns the result again yields its own,
// so rootness is a property of how the construction is reached and not of its
// ordinal.
type ActionProduct struct {
	Owner       string
	Scope       ProductScope
	Ordinal     int
	Constructor string
	Guarded     bool
	Rejected    bool
	Elementwise bool
	Root        bool
	Fields      []ProductField
}

// FieldMutation is one field assignment an action performs on an already
// constructed value. It is a separate row from a product coordinate because it
// can move a field into a state no construction of that form ever names, and
// for the same reason it carries its own Member: an edit can put a family
// member in a discriminant no construction of that form ever writes.
type FieldMutation struct {
	Owner       string
	Scope       ProductScope
	Constructor string
	Field       string
	Member      string
	States      []FieldState
}

// ProductAnalysis is the complete parser construction behaviour derived from
// parser.go.y, stated at the grain the action performs it: one row per
// construction and one row per later field assignment. What a carrier can hold
// across every action is a consequence of these rows, and is decided where the
// rows are held rather than a second time here.
type ProductAnalysis struct {
	Products  []ActionProduct
	Mutations []FieldMutation
	// Uses is the dual of Products at the same grain: one row for every
	// coordinate of a construction that receives another AST value, naming the
	// slot it lands in and where the action obtained it. A product states what a
	// reduction builds; a use states where each built value goes.
	Uses []ActionUse
	// Sequences is the list-building grain: one row for every list-valued
	// result carrier every reduction can leave a value in, stating how that
	// reduction assembles the list out of its operands. It is held apart from
	// Products because a list is not a constructed AST value: it is the
	// carrier a construction's coordinate later receives, so a law about how
	// long a list is and where its final member comes from has no product row
	// to be stated on.
	Sequences []ActionSequence
}

// DiscoverProducts derives every whole-constructor field vector the shipped
// parser can build, from parser.go.y semantic actions, the parser helpers those
// actions call, the compiler/ast declarations that give each field its state
// space, the compiler/ast constants that decide whether a discriminant
// assignment is a zero one, and the shipped scanner's own lexeme and position
// contract. It observes no parse and reads no fixture.
func DiscoverProducts(root string) (ProductAnalysis, error) {
	builder, err := newProductBuilder(root)
	if err != nil {
		return ProductAnalysis{}, err
	}
	if err := builder.collect(root); err != nil {
		return ProductAnalysis{}, err
	}
	builder.solve()
	return builder.result()
}

type siteID int

type constructionSite struct {
	id       siteID
	scope    int
	typeName string
	astType  bool
	semantic bool
	literal  *goast.CompositeLit
	elements map[string]goast.Expr
	fields   []Field
}

type bindingKind uint8

const (
	bindingExpr bindingKind = iota
	bindingCallResult
	bindingElement
	bindingAssert
)

type binding struct {
	kind     bindingKind
	expr     goast.Expr
	helper   string
	index    int
	typeName string
}

type helperCall struct {
	scope   int
	helper  string
	actuals []goast.Expr
}

type mutationSite struct {
	scope  int
	target goast.Expr
	field  string
	value  goast.Expr
}

type actionScope struct {
	index int
	kind  ProductScope
	owner string
	// body and resultTag are the reduction's own action block and the %union
	// arm its nonterminal yields. They are stated for a production scope only:
	// a helper has no reduction result, so a list law is never owned by one.
	body        *goast.BlockStmt
	resultTag   string
	symbols     []string
	formals     []string
	results     int
	locals      map[string][]binding
	elements    map[string][]goast.Expr
	roots       []goast.Expr
	returns     [][]goast.Expr
	guarded     map[*goast.CompositeLit]bool
	rejected    map[*goast.CompositeLit]bool
	elementwise map[*goast.CompositeLit]bool
	sites       map[*goast.CompositeLit]siteID
	rejectedAt  map[goast.Node]bool
}

type queryKind uint8

const (
	queryLocal queryKind = iota
	querySymbol
	queryHelperResult
	queryFormal
	queryField
	queryMutation
)

type queryKey struct {
	kind  queryKind
	scope int
	name  string
	index int
}

type productBuilder struct {
	declarations map[string]Declaration
	records      map[string][]Field
	semantic     map[string]bool
	vocabulary   GrammarVocabulary
	constants    map[string]bool
	// enums are the admitted closed constant families, keyed by the named type
	// a carrier declares, and memberTypes the family each constant belongs to.
	// A carrier is refined by a member only when both agree it is the same
	// family, so a constant of one family assigned to a carrier of another
	// refines nothing rather than being read as that carrier's choice.
	enums        map[string]DiscriminantEnum
	memberTypes  map[string]string
	terminalText map[string]bool
	nonZeroPos   bool
	packages     map[string]bool
	scopes       []*actionScope
	helperScopes map[string]int
	nonterminals map[string][]int
	sites        []*constructionSite
	calls        []helperCall
	mutations    []mutationSite
	diagnostics  map[string]bool
	env          map[queryKey]value
	slots        map[string]UseSlot
	carriers     map[string][]SequenceCarrier
}

func newProductBuilder(root string) (*productBuilder, error) {
	schema, err := Discover(root)
	if err != nil {
		return nil, fmt.Errorf("parser products: discover AST declarations: %w", err)
	}
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	vocabulary, err := DiscoverVocabulary(path)
	if err != nil {
		return nil, err
	}
	constants, err := DiscoverConstants(root)
	if err != nil {
		return nil, err
	}
	contract, err := DiscoverLexerContract(root)
	if err != nil {
		return nil, err
	}
	records, err := parserRecordTypes(root)
	if err != nil {
		return nil, err
	}
	packages, err := parserPackages(root)
	if err != nil {
		return nil, err
	}
	builder := &productBuilder{
		declarations: make(map[string]Declaration, len(schema.Declarations)),
		records:      records,
		semantic:     make(map[string]bool, len(schema.Constructors)),
		vocabulary:   vocabulary,
		constants:    make(map[string]bool, len(constants)),
		enums:        make(map[string]DiscriminantEnum),
		memberTypes:  make(map[string]string, len(constants)),
		terminalText: make(map[string]bool, len(contract.Terminals)),
		nonZeroPos:   contract.NonZeroPositions,
		packages:     packages,
		helperScopes: make(map[string]int),
		nonterminals: make(map[string][]int),
		diagnostics:  make(map[string]bool),
		env:          make(map[queryKey]value),
		slots:        make(map[string]UseSlot),
		carriers:     make(map[string][]SequenceCarrier),
	}
	// Sequence carriers consume the same parser-private record table already
	// admitted above. Keeping this derivation on the existing table avoids a
	// second parser.go.y parse during one product census.
	carriers, err := sequenceCarriers(root, records)
	if err != nil {
		return nil, err
	}
	for _, carrier := range carriers {
		builder.carriers[carrier.Tag] = append(builder.carriers[carrier.Tag], carrier)
	}
	slots, err := UseSlots(schema)
	if err != nil {
		return nil, err
	}
	for _, slot := range slots {
		builder.slots[slot.Form+"."+slot.Field] = slot
	}
	for _, declaration := range schema.Declarations {
		builder.declarations[declaration.Name] = declaration
	}
	for _, constructor := range schema.Constructors {
		builder.semantic[constructor.Name] = constructor.Semantic
	}
	for _, constant := range constants {
		builder.constants[constant.Name] = constant.Zero
	}
	for _, family := range DiscriminantEnums(constants) {
		builder.enums[family.Type] = family
		for _, member := range family.Members {
			builder.memberTypes[member] = family.Type
		}
	}
	for _, terminal := range contract.Terminals {
		builder.terminalText[terminal.Terminal] = terminal.NonEmptyText
	}
	return builder, nil
}

// parserRecordTypes reads the parser-only record declarations parser.go.y
// states beside its helpers. They are not AST forms and never become census
// rows, but a semantic action reaches AST values through them, so a proof that
// could not follow one would lose the assignment behaviour behind it.
func parserRecordTypes(root string) (map[string][]Field, error) {
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	sections, err := parserGoSections(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]Field)
	for name, source := range sections {
		file, parseErr := parser.ParseFile(token.NewFileSet(), "parser-"+name+".go", source, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("parser products: parse parser %s: %w", name, parseErr)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*goast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*goast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*goast.StructType)
				if !ok {
					continue
				}
				fields, fieldErr := recordFields(structType)
				if fieldErr != nil {
					return nil, fmt.Errorf("parser products: parser record %s: %w", typeSpec.Name.Name, fieldErr)
				}
				result[typeSpec.Name.Name] = fields
			}
		}
	}
	return result, nil
}

// recordFields keeps unexported members, unlike the AST schema: a parser-only
// record is internal by construction, so excluding its members would hide the
// route a semantic action takes through it.
func recordFields(structType *goast.StructType) ([]Field, error) {
	var result []Field
	for _, declaration := range structType.Fields.List {
		form, err := fieldForm(declaration.Type)
		if err != nil {
			return nil, err
		}
		typeName := sourceExpr(declaration.Type)
		for _, name := range declaration.Names {
			result = append(result, Field{Ordinal: len(result), Name: name.Name, Type: typeName, Form: form})
		}
	}
	return result, nil
}

// parserPackages reads the package qualifiers parser.go.y imports. A selector
// on one of them names a declared constant rather than a field of a value, and
// telling the two apart is the difference between reading a discriminant and
// projecting through nothing.
func parserPackages(root string) (map[string]bool, error) {
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	sections, err := parserGoSections(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	for name, source := range sections {
		file, parseErr := parser.ParseFile(token.NewFileSet(), "parser-"+name+".go", source, parser.ImportsOnly)
		if parseErr != nil {
			return nil, fmt.Errorf("parser products: parse parser %s imports: %w", name, parseErr)
		}
		for _, imported := range file.Imports {
			qualifier := ""
			if imported.Name != nil {
				qualifier = imported.Name.Name
			} else {
				unquoted, unquoteErr := strconv.Unquote(imported.Path.Value)
				if unquoteErr != nil {
					return nil, fmt.Errorf("parser products: parser %s states an unreadable import path: %w", name, unquoteErr)
				}
				qualifier = filepath.Base(unquoted)
			}
			if qualifier == "" || qualifier == "_" {
				continue
			}
			result[qualifier] = true
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("parser products: parser.go.y imports no package")
	}
	return result, nil
}

func parserGoSections(path string) (map[string]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	source := string(contents)
	start := strings.Index(source, "%{")
	if start < 0 {
		return nil, fmt.Errorf("parser products: parser preamble start is missing")
	}
	endOffset := strings.Index(source[start+2:], "%}")
	if endOffset < 0 {
		return nil, fmt.Errorf("parser products: parser preamble end is missing")
	}
	postamble, err := yaccPostamble(source)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"preamble":  source[start+2 : start+2+endOffset],
		"postamble": "package parse\n" + postamble,
	}, nil
}

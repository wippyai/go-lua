package parsersource

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
)

// TerminalLexeme is one terminal the shipped scanner emits, carried together
// with the lexeme judgement the scanner source proves about it. Terminal uses
// the spelling parser.go.y gives the same symbol, so the two source authorities
// join without a translation table: a named token constant appears verbatim and
// a character terminal appears as the quoted character literal the scanner
// writes.
type TerminalLexeme struct {
	// Terminal is the token constant spelling as parser.go.y names it, either a
	// Go identifier such as TIdent or a character literal such as '['.
	Terminal string
	// NonEmptyText states that every scan path emitting this terminal pins the
	// token text to something non-empty. It is a proof rather than an
	// observation, so a terminal whose text the scanner source leaves open
	// stays false.
	NonEmptyText bool
}

// LexerContract is what compiler/parse/lexer.go proves about the tokens it
// hands the parser. The grammar states neither fact: it names terminals but
// never their lexeme content, and it carries no position at all. Both are
// therefore derived from the scanner source here, so that a consumer joining
// grammar rows against token evidence reads one authority for each fact rather
// than assuming either.
type LexerContract struct {
	// Terminals is every terminal the scanner can emit, sorted by Terminal and
	// free of duplicates. A terminal several scan paths reach is non-empty only
	// when every one of those paths proves it.
	Terminals []TerminalLexeme
	// NonZeroPositions states that every position the scanner stamps differs
	// from the zero ast.Position, which makes a zero position in a parsed tree
	// a missing stamp rather than a scanned one.
	NonZeroPositions bool
}

// DiscoverLexerContract derives the lexer contract from compiler/parse/lexer.go
// alone. It is source authority in the same sense as the rest of this package:
// nothing here runs the scanner, tokenizes Lua, or observes a token stream, so
// its rows describe what the scanner can emit rather than what any particular
// input made it emit.
//
// It fails closed. A scanner source it cannot parse, a token type it cannot
// name, or a terminal set that comes out empty is an error rather than a
// shortened row set, because a silently dropped terminal reads to a consumer
// exactly like a terminal the scanner cannot produce. A token text the source
// does not pin is not an error: that is the NonEmptyText: false judgement.
func DiscoverLexerContract(root string) (LexerContract, error) {
	path := filepath.Join(root, "compiler", "parse", "lexer.go")
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return LexerContract{}, fmt.Errorf("parse scanner source %s: %w", path, err)
	}
	source := newLexerSource(path, file)
	terminals, err := source.terminals()
	if err != nil {
		return LexerContract{}, err
	}
	positions, err := source.nonZeroPositions()
	if err != nil {
		return LexerContract{}, err
	}
	return LexerContract{Terminals: terminals, NonZeroPositions: positions}, nil
}

// lexerSource is the indexed scanner source. Declarations are keyed by receiver
// type so that the several Error and TokenError methods in the file stay apart,
// and the constant and package-level map tables are held because a token type
// and a reserved-word class are both stated through them.
type lexerSource struct {
	path     string
	file     *goast.File
	funcs    map[string]*goast.FuncDecl
	consts   map[string]goast.Expr
	tables   map[string]*goast.CompositeLit
	writers  map[string]bool
	emitted  map[string]bool
	visited  map[string]bool
	reached  map[string]bool
	structs  map[string]*goast.StructType
	receiver map[string]string
}

func newLexerSource(path string, file *goast.File) *lexerSource {
	source := &lexerSource{
		path:     path,
		file:     file,
		funcs:    make(map[string]*goast.FuncDecl),
		consts:   make(map[string]goast.Expr),
		tables:   make(map[string]*goast.CompositeLit),
		emitted:  make(map[string]bool),
		visited:  make(map[string]bool),
		reached:  make(map[string]bool),
		structs:  make(map[string]*goast.StructType),
		receiver: make(map[string]string),
	}
	for _, declaration := range file.Decls {
		switch node := declaration.(type) {
		case *goast.FuncDecl:
			key := functionKey(node)
			source.funcs[key] = node
			if name, ok := receiverName(node); ok {
				source.receiver[key] = name
			}
		case *goast.GenDecl:
			for _, spec := range node.Specs {
				switch typed := spec.(type) {
				case *goast.ValueSpec:
					for index, name := range typed.Names {
						if index >= len(typed.Values) {
							continue
						}
						switch node.Tok {
						case token.CONST:
							source.consts[name.Name] = typed.Values[index]
						case token.VAR:
							if literal, ok := typed.Values[index].(*goast.CompositeLit); ok {
								source.tables[name.Name] = literal
							}
						}
					}
				case *goast.TypeSpec:
					if structure, ok := typed.Type.(*goast.StructType); ok {
						source.structs[typed.Name.Name] = structure
					}
				}
			}
		}
	}
	source.computeWriters()
	return source
}

// functionKey names a declaration by receiver type and method name, because
// method names in this file repeat across the scanner, the lexer and the error
// type and a bare name would conflate them.
func functionKey(declaration *goast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return declaration.Name.Name
	}
	return receiverType(declaration) + "." + declaration.Name.Name
}

func receiverType(declaration *goast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return ""
	}
	expression := declaration.Recv.List[0].Type
	if star, ok := expression.(*goast.StarExpr); ok {
		expression = star.X
	}
	if ident, ok := expression.(*goast.Ident); ok {
		return ident.Name
	}
	return ""
}

func receiverName(declaration *goast.FuncDecl) (string, bool) {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return "", false
	}
	names := declaration.Recv.List[0].Names
	if len(names) == 0 {
		return "", false
	}
	return names[0].Name, true
}

// computeWriters is the least fixed point of definite buffer writing over the
// declarations that take a *bytes.Buffer. A declaration definitely writes when
// its own statement list, at the level that always runs, either writes bytes
// into that buffer or hands it to a declaration already known to write. A write
// reached only through a loop or a branch body is not a definite write, which
// is what separates an identifier scan from a string scan.
func (s *lexerSource) computeWriters() {
	s.writers = make(map[string]bool)
	for changed := true; changed; {
		changed = false
		for key, declaration := range s.funcs {
			if s.writers[key] || declaration.Body == nil {
				continue
			}
			buffer, ok := bufferParam(declaration)
			if !ok {
				continue
			}
			if s.listWritesBuffer(declaration.Body.List, key, buffer) {
				s.writers[key] = true
				changed = true
			}
		}
	}
}

func bufferParam(declaration *goast.FuncDecl) (string, bool) {
	index, name := bufferParamIndex(declaration)
	if index < 0 {
		return "", false
	}
	return name, true
}

// bufferParamIndex reports which parameter carries the buffer a declaration
// fills, so that a call site can be matched against the parameter the callee
// actually writes rather than against any buffer-shaped argument.
func bufferParamIndex(declaration *goast.FuncDecl) (int, string) {
	if declaration.Type.Params == nil {
		return -1, ""
	}
	position := 0
	for _, field := range declaration.Type.Params.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if isBufferPointer(field.Type) {
			if len(field.Names) == 0 {
				return position, ""
			}
			return position, field.Names[0].Name
		}
		position += count
	}
	return -1, ""
}

func isBufferPointer(expression goast.Expr) bool {
	star, ok := expression.(*goast.StarExpr)
	if !ok {
		return false
	}
	return isSelector(star.X, "bytes", "Buffer")
}

func isTokenPointer(expression goast.Expr) bool {
	star, ok := expression.(*goast.StarExpr)
	if !ok {
		return false
	}
	return isSelector(star.X, "ast", "Token")
}

func isSelector(expression goast.Expr, qualifier string, name string) bool {
	selector, ok := expression.(*goast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	ident, ok := selector.X.(*goast.Ident)
	return ok && ident.Name == qualifier
}

// listWritesBuffer runs the definite-write test over one statement list. An if
// initialiser counts as part of that list because it always runs before the
// branch is taken; the branch bodies themselves do not.
func (s *lexerSource) listWritesBuffer(list []goast.Stmt, owner string, buffer string) bool {
	scope := newLexerScope()
	scope.buffers[buffer] = buffer
	scope.receiver = s.receiver[owner]
	scope.receiverType = ownerType(owner)
	filled := make(map[string]bool)
	for _, statement := range list {
		s.applyFills(statement, scope, filled)
	}
	return filled[buffer]
}

func ownerType(key string) string {
	for index := 0; index < len(key); index++ {
		if key[index] == '.' {
			return key[:index]
		}
	}
	return ""
}

// lexerScope is what a statement list knows about its own names: which of them
// hold the token being built, which name the byte buffer that backs a lexeme,
// which came out of a package-level table lookup, and which the enclosing
// switch has pinned to a case value.
type lexerScope struct {
	tokens       map[string]bool
	buffers      map[string]string
	values       map[string]bool
	locals       map[string]bool
	tableLookups map[string]string
	caseValues   map[string][]goast.Expr
	receiver     string
	receiverType string
}

func newLexerScope() *lexerScope {
	return &lexerScope{
		tokens:       make(map[string]bool),
		buffers:      make(map[string]string),
		values:       make(map[string]bool),
		locals:       make(map[string]bool),
		tableLookups: make(map[string]string),
		caseValues:   make(map[string][]goast.Expr),
	}
}

func (s *lexerScope) clone() *lexerScope {
	copied := &lexerScope{
		tokens:       make(map[string]bool, len(s.tokens)),
		buffers:      make(map[string]string, len(s.buffers)),
		values:       make(map[string]bool, len(s.values)),
		locals:       make(map[string]bool, len(s.locals)),
		tableLookups: make(map[string]string, len(s.tableLookups)),
		caseValues:   make(map[string][]goast.Expr, len(s.caseValues)),
		receiver:     s.receiver,
		receiverType: s.receiverType,
	}
	for name := range s.tokens {
		copied.tokens[name] = true
	}
	for name, canonical := range s.buffers {
		copied.buffers[name] = canonical
	}
	for name := range s.values {
		copied.values[name] = true
	}
	for name := range s.locals {
		copied.locals[name] = true
	}
	for name, table := range s.tableLookups {
		copied.tableLookups[name] = table
	}
	for name, values := range s.caseValues {
		copied.caseValues[name] = values
	}
	return copied
}

// lexerFrame is one statement list together with the buffer state that holds
// before each of its statements. Keeping the state per position is what lets a
// token text stated after a nested branch be judged against the writes that
// already ran on the way to it.
type lexerFrame struct {
	list   []goast.Stmt
	before []map[string]bool
}

// terminals walks the scanner from its single entry point, following the token
// pointer into every scanner method that receives it, and returns one row per
// terminal any of those methods can assign.
func (s *lexerSource) terminals() ([]TerminalLexeme, error) {
	entry, ok := s.scanEntry()
	if !ok {
		return nil, fmt.Errorf("scanner source %s: no scan entry point returns an ast.Token", s.path)
	}
	if err := s.walkFunction(entry, nil); err != nil {
		return nil, err
	}
	for key, declaration := range s.funcs {
		if s.reached[key] || !statesTokenType(declaration) {
			continue
		}
		return nil, fmt.Errorf("scanner source %s: %s states a token type but no scan path reaches it", s.path, key)
	}
	if len(s.emitted) == 0 {
		return nil, fmt.Errorf("scanner source %s: no terminal emission found", s.path)
	}
	rows := make([]TerminalLexeme, 0, len(s.emitted))
	for terminal, nonEmpty := range s.emitted {
		rows = append(rows, TerminalLexeme{Terminal: terminal, NonEmptyText: nonEmpty})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Terminal < rows[j].Terminal })
	return rows, nil
}

// scanEntry is the scanner method that produces tokens: the one method whose
// results contain ast.Token. Naming it structurally keeps the entry point tied
// to what the method does rather than to what it is called.
func (s *lexerSource) scanEntry() (string, bool) {
	entry := ""
	for key, declaration := range s.funcs {
		if declaration.Type.Results == nil || declaration.Recv == nil {
			continue
		}
		produces := false
		for _, field := range declaration.Type.Results.List {
			if isSelector(field.Type, "ast", "Token") {
				produces = true
			}
		}
		if !produces {
			continue
		}
		if entry != "" {
			return "", false
		}
		entry = key
	}
	return entry, entry != ""
}

func tokenPointerParam(declaration *goast.FuncDecl) bool {
	if declaration.Type.Params == nil {
		return false
	}
	for _, field := range declaration.Type.Params.List {
		if isTokenPointer(field.Type) {
			return true
		}
	}
	return false
}

// statesTokenType reports whether a declaration assigns the type of a token it
// was handed. It is the closure condition of the terminal walk: a declaration
// that only receives a token, such as one that returns it to a pool, states no
// terminal and losing it drops nothing, while one that assigns a token type and
// is never reached would drop a terminal silently.
func statesTokenType(declaration *goast.FuncDecl) bool {
	if declaration.Body == nil || declaration.Type.Params == nil {
		return false
	}
	holders := make(map[string]bool)
	for _, field := range declaration.Type.Params.List {
		if !isTokenPointer(field.Type) {
			continue
		}
		for _, name := range field.Names {
			holders[name.Name] = true
		}
	}
	if len(holders) == 0 {
		return false
	}
	states := false
	goast.Inspect(declaration.Body, func(node goast.Node) bool {
		assign, ok := node.(*goast.AssignStmt)
		if !ok {
			return true
		}
		for _, target := range assign.Lhs {
			selector, ok := target.(*goast.SelectorExpr)
			if !ok || selector.Sel.Name != "Type" {
				continue
			}
			if holder, named := selector.X.(*goast.Ident); named && holders[holder.Name] {
				states = true
			}
		}
		return true
	})
	return states
}

// walkFunction visits one declaration under a stated entry buffer state. The
// state is part of the visit key so that a method reached with a filled buffer
// and the same method reached with an empty one are judged apart, and so that
// the walk terminates: the key set is finite and each key runs once.
func (s *lexerSource) walkFunction(key string, entryFilled map[string]bool) error {
	declaration, ok := s.funcs[key]
	if !ok || declaration.Body == nil {
		return fmt.Errorf("scanner source %s: %s has no body to derive terminals from", s.path, key)
	}
	s.reached[key] = true
	visitKey := key + "|" + filledKey(entryFilled)
	if s.visited[visitKey] {
		return nil
	}
	s.visited[visitKey] = true

	scope := newLexerScope()
	scope.receiver = s.receiver[key]
	scope.receiverType = receiverType(declaration)
	if declaration.Type.Params != nil {
		for _, field := range declaration.Type.Params.List {
			for _, name := range field.Names {
				scope.locals[name.Name] = true
				if isTokenPointer(field.Type) {
					scope.tokens[name.Name] = true
				}
				if isBufferPointer(field.Type) {
					scope.buffers[name.Name] = name.Name
				}
			}
		}
	}
	filled := make(map[string]bool)
	for name, state := range entryFilled {
		filled[name] = state
	}
	return s.walkList(declaration.Body.List, scope, filled, nil)
}

func filledKey(filled map[string]bool) string {
	names := make([]string, 0, len(filled))
	for name, state := range filled {
		if state {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	key := ""
	for _, name := range names {
		key += name + ","
	}
	return key
}

// walkList walks one statement list in order. The buffer state before each
// position is computed first, so that a token text stated at any position in
// this list can be judged from any nested branch that reaches it.
func (s *lexerSource) walkList(list []goast.Stmt, scope *lexerScope, entryFilled map[string]bool, frames []lexerFrame) error {
	before := make([]map[string]bool, len(list)+1)
	before[0] = copyFilled(entryFilled)
	staging := scope.clone()
	for index, statement := range list {
		next := copyFilled(before[index])
		s.applyFills(statement, staging, next)
		before[index+1] = next
		s.applyBindings(statement, staging)
	}
	frame := lexerFrame{list: list, before: before}
	inner := make([]lexerFrame, 0, len(frames)+1)
	inner = append(inner, frames...)
	inner = append(inner, frame)

	current := scope.clone()
	for index, statement := range list {
		if err := s.walkStmt(statement, current, before[index], inner); err != nil {
			return err
		}
		s.applyBindings(statement, current)
	}
	return nil
}

func copyFilled(filled map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(filled))
	for name, state := range filled {
		copied[name] = state
	}
	return copied
}

// walkStmt handles one statement: it records any token type the statement
// states, follows any call that hands the token onward, and descends into the
// nested statement lists the statement owns.
func (s *lexerSource) walkStmt(statement goast.Stmt, scope *lexerScope, filled map[string]bool, frames []lexerFrame) error {
	switch node := statement.(type) {
	case *goast.AssignStmt:
		if err := s.recordAssign(node, scope, frames); err != nil {
			return err
		}
		for _, expression := range node.Rhs {
			if err := s.followCall(expression, scope, filled); err != nil {
				return err
			}
		}
		return nil
	case *goast.ExprStmt:
		return s.followCall(node.X, scope, filled)
	case *goast.BlockStmt:
		return s.walkList(node.List, scope, filled, frames)
	case *goast.LabeledStmt:
		return s.walkStmt(node.Stmt, scope, filled, frames)
	case *goast.IfStmt:
		local := scope.clone()
		bodyFilled := copyFilled(filled)
		if node.Init != nil {
			if err := s.walkStmt(node.Init, local, filled, frames); err != nil {
				return err
			}
			s.applyFills(node.Init, local, bodyFilled)
			s.applyBindings(node.Init, local)
		}
		if err := s.walkList(node.Body.List, local, bodyFilled, frames); err != nil {
			return err
		}
		if node.Else != nil {
			return s.walkStmt(node.Else, local, bodyFilled, frames)
		}
		return nil
	case *goast.SwitchStmt:
		local := scope.clone()
		bodyFilled := copyFilled(filled)
		if node.Init != nil {
			if err := s.walkStmt(node.Init, local, filled, frames); err != nil {
				return err
			}
			s.applyFills(node.Init, local, bodyFilled)
			s.applyBindings(node.Init, local)
		}
		tag, tagged := node.Tag.(*goast.Ident)
		for _, clause := range node.Body.List {
			caseClause, ok := clause.(*goast.CaseClause)
			if !ok {
				return fmt.Errorf("scanner source %s: switch body holds %T", s.path, clause)
			}
			branch := local.clone()
			if tagged && len(caseClause.List) > 0 {
				branch.caseValues[tag.Name] = caseClause.List
			}
			if err := s.walkList(caseClause.Body, branch, copyFilled(bodyFilled), frames); err != nil {
				return err
			}
		}
		return nil
	case *goast.ForStmt:
		return s.walkList(node.Body.List, scope.clone(), copyFilled(filled), frames)
	case *goast.RangeStmt:
		return s.walkList(node.Body.List, scope.clone(), copyFilled(filled), frames)
	case *goast.TypeSwitchStmt:
		for _, clause := range node.Body.List {
			caseClause, ok := clause.(*goast.CaseClause)
			if !ok {
				return fmt.Errorf("scanner source %s: type switch body holds %T", s.path, clause)
			}
			if err := s.walkList(caseClause.Body, scope.clone(), copyFilled(filled), frames); err != nil {
				return err
			}
		}
		return nil
	case *goast.SelectStmt:
		for _, clause := range node.Body.List {
			commClause, ok := clause.(*goast.CommClause)
			if !ok {
				return fmt.Errorf("scanner source %s: select body holds %T", s.path, clause)
			}
			if err := s.walkList(commClause.Body, scope.clone(), copyFilled(filled), frames); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

// recordAssign turns a token type assignment into terminal rows. The text
// judgement comes from the token text stated on the same path, which is the
// innermost enclosing statement list that states it at all, read at the
// position where it is stated.
func (s *lexerSource) recordAssign(node *goast.AssignStmt, scope *lexerScope, frames []lexerFrame) error {
	for _, target := range node.Lhs {
		holder, field, ok := tokenField(target, scope)
		if !ok {
			continue
		}
		if node.Tok != token.ASSIGN {
			return fmt.Errorf("scanner source %s: token %s is stated by %s, which the lexeme model does not cover", s.path, field, node.Tok)
		}
		if field != "Type" {
			continue
		}
		if len(node.Lhs) != len(node.Rhs) {
			return fmt.Errorf("scanner source %s: token Type is stated by a multi-value assignment", s.path)
		}
		names, err := s.terminalNames(node.Rhs[indexOf(node.Lhs, target)], scope)
		if err != nil {
			return err
		}
		nonEmpty, err := s.textIsNonEmpty(holder, scope, frames)
		if err != nil {
			return err
		}
		for _, name := range names {
			s.emit(name, nonEmpty)
		}
	}
	return nil
}

func indexOf(list []goast.Expr, target goast.Expr) int {
	for index, expression := range list {
		if expression == target {
			return index
		}
	}
	return 0
}

func (s *lexerSource) emit(terminal string, nonEmpty bool) {
	if existing, seen := s.emitted[terminal]; seen {
		s.emitted[terminal] = existing && nonEmpty
		return
	}
	s.emitted[terminal] = nonEmpty
}

func tokenField(expression goast.Expr, scope *lexerScope) (string, string, bool) {
	selector, ok := expression.(*goast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	holder, ok := selector.X.(*goast.Ident)
	if !ok || !scope.tokens[holder.Name] {
		return "", "", false
	}
	return holder.Name, selector.Sel.Name, true
}

// terminalNames resolves a token type expression to the terminal spellings
// parser.go.y uses. A named constant is a terminal under its own name unless it
// resolves to a negative value in this file, which is the scanner's own end
// sentinel and never reaches the parser as a symbol. A character literal is a
// terminal under its literal text. A name the enclosing switch pins to case
// values stands for each of those values, and a name bound from a package-level
// table lookup stands for each value that table holds.
func (s *lexerSource) terminalNames(expression goast.Expr, scope *lexerScope) ([]string, error) {
	switch node := expression.(type) {
	case *goast.ParenExpr:
		return s.terminalNames(node.X, scope)
	case *goast.BasicLit:
		if node.Kind != token.CHAR {
			return nil, fmt.Errorf("scanner source %s: token type is stated by the %s literal %s", s.path, node.Kind, node.Value)
		}
		return []string{node.Value}, nil
	case *goast.Ident:
		if values, pinned := scope.caseValues[node.Name]; pinned {
			return s.caseTerminals(values)
		}
		if table, looked := scope.tableLookups[node.Name]; looked {
			return s.tableTerminals(table)
		}
		if scope.locals[node.Name] {
			return nil, fmt.Errorf("scanner source %s: token type is stated by the unpinned local %s", s.path, node.Name)
		}
		if definition, declared := s.consts[node.Name]; declared {
			value, resolved := s.constantValue(definition, 0)
			if !resolved {
				return nil, fmt.Errorf("scanner source %s: token type constant %s does not resolve", s.path, node.Name)
			}
			if value < 0 {
				return nil, nil
			}
			return nil, fmt.Errorf("scanner source %s: token type constant %s is scanner-local but not an end sentinel", s.path, node.Name)
		}
		return []string{node.Name}, nil
	default:
		return nil, fmt.Errorf("scanner source %s: token type is stated by %T, which the lexeme model does not cover", s.path, expression)
	}
}

func (s *lexerSource) caseTerminals(values []goast.Expr) ([]string, error) {
	names := make([]string, 0, len(values))
	for _, value := range values {
		literal, ok := value.(*goast.BasicLit)
		if !ok || literal.Kind != token.CHAR {
			return nil, fmt.Errorf("scanner source %s: token type follows a case value of %T, which the lexeme model does not cover", s.path, value)
		}
		names = append(names, literal.Value)
	}
	return names, nil
}

func (s *lexerSource) tableTerminals(table string) ([]string, error) {
	literal, ok := s.tables[table]
	if !ok {
		return nil, fmt.Errorf("scanner source %s: token type is looked up in %s, which is not a package-level table", s.path, table)
	}
	names := make([]string, 0, len(literal.Elts))
	for _, element := range literal.Elts {
		pair, ok := element.(*goast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf("scanner source %s: table %s holds %T rather than a key-value pair", s.path, table, element)
		}
		value, ok := pair.Value.(*goast.Ident)
		if !ok {
			return nil, fmt.Errorf("scanner source %s: table %s maps to %T rather than a token constant", s.path, table, pair.Value)
		}
		names = append(names, value.Name)
	}
	return names, nil
}

// textIsNonEmpty judges the token text stated for this token on this path. The
// text is looked for in the innermost enclosing statement list that states it,
// and read against the buffer state holding where it is stated.
func (s *lexerSource) textIsNonEmpty(holder string, scope *lexerScope, frames []lexerFrame) (bool, error) {
	for index := len(frames) - 1; index >= 0; index-- {
		frame := frames[index]
		for position, statement := range frame.list {
			assign, ok := statement.(*goast.AssignStmt)
			if !ok {
				continue
			}
			for _, target := range assign.Lhs {
				owner, field, ok := tokenField(target, scope)
				if !ok || owner != holder || field != "Str" {
					continue
				}
				if assign.Tok != token.ASSIGN || len(assign.Lhs) != len(assign.Rhs) {
					return false, fmt.Errorf("scanner source %s: token text is stated by %s, which the lexeme model does not cover", s.path, assign.Tok)
				}
				return s.definitelyNonEmpty(assign.Rhs[indexOf(assign.Lhs, target)], scope, frame.before[position]), nil
			}
		}
	}
	return false, nil
}

// definitelyNonEmpty is the text proof. A literal proves itself, a conversion
// of a scanned character proves one byte, and a buffer proves itself only where
// a definite write already ran on this path. Everything else is unproven, which
// is a judgement rather than a failure: a terminal whose text the scanner may
// leave empty is exactly what the false row states.
func (s *lexerSource) definitelyNonEmpty(expression goast.Expr, scope *lexerScope, filled map[string]bool) bool {
	switch node := expression.(type) {
	case *goast.ParenExpr:
		return s.definitelyNonEmpty(node.X, scope, filled)
	case *goast.BasicLit:
		if node.Kind != token.STRING {
			return false
		}
		text, err := strconv.Unquote(node.Value)
		return err == nil && text != ""
	case *goast.CallExpr:
		return s.callIsNonEmpty(node, scope, filled)
	default:
		return false
	}
}

func (s *lexerSource) callIsNonEmpty(call *goast.CallExpr, scope *lexerScope, filled map[string]bool) bool {
	switch fun := call.Fun.(type) {
	case *goast.Ident:
		if fun.Name != "string" || len(call.Args) != 1 {
			return false
		}
		return s.conversionIsNonEmpty(call.Args[0])
	case *goast.SelectorExpr:
		if fun.Sel.Name != "String" || len(call.Args) != 0 {
			return false
		}
		holder, ok := fun.X.(*goast.Ident)
		if !ok {
			return false
		}
		canonical, tracked := scope.buffers[holder.Name]
		return tracked && filled[canonical]
	default:
		return false
	}
}

func (s *lexerSource) conversionIsNonEmpty(argument goast.Expr) bool {
	switch node := argument.(type) {
	case *goast.ParenExpr:
		return s.conversionIsNonEmpty(node.X)
	case *goast.CallExpr:
		fun, ok := node.Fun.(*goast.Ident)
		return ok && (fun.Name == "rune" || fun.Name == "byte")
	case *goast.CompositeLit:
		return len(node.Elts) > 0
	default:
		return false
	}
}

// applyBindings records what a statement adds to the name environment: the
// token the scan builds, the buffer that backs a lexeme, and the name a
// package-level table lookup binds.
func (s *lexerSource) applyBindings(statement goast.Stmt, scope *lexerScope) {
	switch node := statement.(type) {
	case *goast.DeclStmt:
		declaration, ok := node.Decl.(*goast.GenDecl)
		if !ok || declaration.Tok != token.VAR {
			return
		}
		for _, spec := range declaration.Specs {
			value, ok := spec.(*goast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				scope.locals[name.Name] = true
				if value.Type != nil && isSelector(value.Type, "bytes", "Buffer") {
					scope.values[name.Name] = true
				}
				if value.Type != nil && isTokenPointer(value.Type) {
					scope.tokens[name.Name] = true
				}
				if value.Type != nil && isBufferPointer(value.Type) {
					scope.buffers[name.Name] = name.Name
				}
			}
		}
	case *goast.AssignStmt:
		for index, target := range node.Lhs {
			name, ok := target.(*goast.Ident)
			if !ok {
				continue
			}
			if node.Tok == token.DEFINE {
				scope.locals[name.Name] = true
			}
			if index >= len(node.Rhs) {
				continue
			}
			s.bindSingle(name.Name, node.Rhs[index], scope)
		}
		if len(node.Rhs) == 1 && len(node.Lhs) > 1 {
			s.bindMulti(node.Lhs, node.Rhs[0], scope)
		}
	}
}

func (s *lexerSource) bindSingle(name string, value goast.Expr, scope *lexerScope) {
	switch node := value.(type) {
	case *goast.UnaryExpr:
		if node.Op != token.AND {
			return
		}
		target, ok := node.X.(*goast.Ident)
		if ok && scope.values[target.Name] {
			scope.buffers[name] = target.Name
			scope.buffers[target.Name] = target.Name
		}
	case *goast.CallExpr:
		fun, ok := node.Fun.(*goast.Ident)
		if !ok {
			return
		}
		declaration, declared := s.funcs[fun.Name]
		if !declared || declaration.Type.Results == nil {
			return
		}
		for _, field := range declaration.Type.Results.List {
			if isTokenPointer(field.Type) {
				scope.tokens[name] = true
			}
		}
	}
}

func (s *lexerSource) bindMulti(targets []goast.Expr, value goast.Expr, scope *lexerScope) {
	index, ok := value.(*goast.IndexExpr)
	if !ok {
		return
	}
	table, ok := index.X.(*goast.Ident)
	if !ok {
		return
	}
	if _, known := s.tables[table.Name]; !known {
		return
	}
	name, ok := targets[0].(*goast.Ident)
	if !ok {
		return
	}
	scope.tableLookups[name.Name] = table.Name
}

// applyFills advances the buffer state across one statement at the level where
// that statement always runs. An if or switch initialiser is part of that level
// because it runs before any branch is chosen; a branch body is not.
func (s *lexerSource) applyFills(statement goast.Stmt, scope *lexerScope, filled map[string]bool) {
	switch node := statement.(type) {
	case *goast.ExprStmt:
		s.applyCallFills(node.X, scope, filled)
	case *goast.AssignStmt:
		for _, expression := range node.Rhs {
			s.applyCallFills(expression, scope, filled)
		}
	case *goast.IfStmt:
		if node.Init != nil {
			s.applyFills(node.Init, scope, filled)
		}
	case *goast.SwitchStmt:
		if node.Init != nil {
			s.applyFills(node.Init, scope, filled)
		}
	case *goast.LabeledStmt:
		s.applyFills(node.Stmt, scope, filled)
	}
}

func (s *lexerSource) applyCallFills(expression goast.Expr, scope *lexerScope, filled map[string]bool) {
	call, ok := expression.(*goast.CallExpr)
	if !ok {
		return
	}
	switch fun := call.Fun.(type) {
	case *goast.SelectorExpr:
		holder, ok := fun.X.(*goast.Ident)
		if !ok {
			return
		}
		if canonical, tracked := scope.buffers[holder.Name]; tracked {
			if bufferWriteIsNonEmpty(fun.Sel.Name, call.Args) {
				filled[canonical] = true
			}
			return
		}
		if holder.Name != scope.receiver {
			return
		}
		s.applyCalleeFills(scope.receiverType+"."+fun.Sel.Name, call, scope, filled)
	case *goast.Ident:
		s.applyCalleeFills(fun.Name, call, scope, filled)
	}
}

func bufferWriteIsNonEmpty(method string, args []goast.Expr) bool {
	switch method {
	case "WriteByte", "WriteRune":
		return true
	case "WriteString":
		if len(args) != 1 {
			return false
		}
		literal, ok := args[0].(*goast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return false
		}
		text, err := strconv.Unquote(literal.Value)
		return err == nil && text != ""
	default:
		return false
	}
}

func (s *lexerSource) applyCalleeFills(key string, call *goast.CallExpr, scope *lexerScope, filled map[string]bool) {
	if !s.writers[key] {
		return
	}
	declaration, ok := s.funcs[key]
	if !ok {
		return
	}
	position, _ := bufferParamIndex(declaration)
	if position < 0 || position >= len(call.Args) {
		return
	}
	name, ok := call.Args[position].(*goast.Ident)
	if !ok {
		return
	}
	if canonical, tracked := scope.buffers[name.Name]; tracked {
		filled[canonical] = true
	}
}

// followCall walks into a scanner method that receives the token being built,
// carrying the buffer state the call site holds so that the callee judges its
// own text statements against what already ran.
func (s *lexerSource) followCall(expression goast.Expr, scope *lexerScope, filled map[string]bool) error {
	call, ok := expression.(*goast.CallExpr)
	if !ok {
		return nil
	}
	key := ""
	switch fun := call.Fun.(type) {
	case *goast.SelectorExpr:
		holder, named := fun.X.(*goast.Ident)
		if !named || holder.Name != scope.receiver {
			return nil
		}
		key = scope.receiverType + "." + fun.Sel.Name
	case *goast.Ident:
		key = fun.Name
	default:
		return nil
	}
	declaration, declared := s.funcs[key]
	if !declared || !tokenPointerParam(declaration) {
		return nil
	}
	carries := false
	for _, argument := range call.Args {
		name, ok := argument.(*goast.Ident)
		if ok && scope.tokens[name.Name] {
			carries = true
		}
	}
	if !carries {
		return nil
	}
	entryFilled := make(map[string]bool)
	position, parameter := bufferParamIndex(declaration)
	if position >= 0 && position < len(call.Args) {
		if name, ok := call.Args[position].(*goast.Ident); ok {
			if canonical, tracked := scope.buffers[name.Name]; tracked && filled[canonical] {
				entryFilled[parameter] = true
			}
		}
	}
	return s.walkFunction(key, entryFilled)
}

// nonZeroPositions proves that the scanner's own line number never reaches
// zero. The proof is the initial value the scanner is constructed with together
// with every write the scanner makes to that line: an increment cannot reach
// the initial value's sign from above, and a constant write is only accepted
// when the constant resolves to something other than zero. A write that cannot
// be shown non-zero makes the fact false rather than making the derivation
// guess.
func (s *lexerSource) nonZeroPositions() (bool, error) {
	scanner, err := s.scannerType()
	if err != nil {
		return false, err
	}
	initialised := false
	for _, declaration := range s.file.Decls {
		function, ok := declaration.(*goast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		provable := true
		goast.Inspect(function.Body, func(node goast.Node) bool {
			literal, ok := node.(*goast.CompositeLit)
			if ok {
				if ident, named := literal.Type.(*goast.Ident); named && ident.Name == scanner {
					initialised = true
					if !s.compositeLineIsNonZero(literal) {
						provable = false
					}
				}
				return true
			}
			if !s.statementKeepsLineNonZero(node, function, scanner) {
				provable = false
			}
			return true
		})
		if !provable {
			return false, nil
		}
	}
	if !initialised {
		return false, fmt.Errorf("scanner source %s: %s is never constructed, so its position has no initial line", s.path, scanner)
	}
	return true, nil
}

// scannerType is the struct in this file that carries a position and writes to
// it. The error type carries a position too but never writes one, so the write
// is what tells the scanner apart from the reporter.
func (s *lexerSource) scannerType() (string, error) {
	writers := make(map[string]bool)
	for key, declaration := range s.funcs {
		owner := ownerType(key)
		if owner == "" || declaration.Body == nil {
			continue
		}
		name, ok := s.receiver[key]
		if !ok {
			continue
		}
		structure, declared := s.structs[owner]
		if !declared || !structHasField(structure, "Pos") {
			continue
		}
		goast.Inspect(declaration.Body, func(node goast.Node) bool {
			switch typed := node.(type) {
			case *goast.AssignStmt:
				for _, target := range typed.Lhs {
					if _, ok := positionField(target, name); ok {
						writers[owner] = true
					}
				}
			case *goast.IncDecStmt:
				if _, ok := positionField(typed.X, name); ok {
					writers[owner] = true
				}
			}
			return true
		})
	}
	if len(writers) != 1 {
		return "", fmt.Errorf("scanner source %s: %d position-writing types declared, want exactly one", s.path, len(writers))
	}
	for owner := range writers {
		return owner, nil
	}
	return "", fmt.Errorf("scanner source %s: no position-writing type declared", s.path)
}

func structHasField(structure *goast.StructType, field string) bool {
	if structure.Fields == nil {
		return false
	}
	for _, entry := range structure.Fields.List {
		for _, name := range entry.Names {
			if name.Name == field {
				return true
			}
		}
	}
	return false
}

// positionField reports the position member a selector names on the given
// receiver, so that a write to the scanner's own line is told apart from a
// write to a token's line.
func positionField(expression goast.Expr, receiver string) (string, bool) {
	selector, ok := expression.(*goast.SelectorExpr)
	if !ok {
		return "", false
	}
	if holder, direct := selector.X.(*goast.Ident); direct {
		if holder.Name == receiver && selector.Sel.Name == "Pos" {
			return "", true
		}
		return "", false
	}
	inner, ok := selector.X.(*goast.SelectorExpr)
	if !ok || inner.Sel.Name != "Pos" {
		return "", false
	}
	holder, ok := inner.X.(*goast.Ident)
	if !ok || holder.Name != receiver {
		return "", false
	}
	return selector.Sel.Name, true
}

func (s *lexerSource) compositeLineIsNonZero(literal *goast.CompositeLit) bool {
	for _, element := range literal.Elts {
		pair, ok := element.(*goast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*goast.Ident)
		if !ok || key.Name != "Pos" {
			continue
		}
		position, ok := pair.Value.(*goast.CompositeLit)
		if !ok {
			return false
		}
		for _, member := range position.Elts {
			field, ok := member.(*goast.KeyValueExpr)
			if !ok {
				return false
			}
			name, ok := field.Key.(*goast.Ident)
			if !ok || name.Name != "Line" {
				continue
			}
			value, resolved := s.constantValue(field.Value, 0)
			return resolved && value != 0
		}
		return false
	}
	return false
}

// statementKeepsLineNonZero checks one write against the line rule: an
// increment keeps the line away from zero from above, and a plain write is only
// accepted for a constant that resolves to something other than zero. A write
// of the whole position replaces the line wholesale and is never accepted.
func (s *lexerSource) statementKeepsLineNonZero(node goast.Node, function *goast.FuncDecl, scanner string) bool {
	if receiverType(function) != scanner {
		return true
	}
	receiver, ok := receiverName(function)
	if !ok {
		return true
	}
	switch typed := node.(type) {
	case *goast.IncDecStmt:
		field, ok := positionField(typed.X, receiver)
		if !ok || (field != "Line" && field != "") {
			return true
		}
		return field == "Line" && typed.Tok == token.INC
	case *goast.AssignStmt:
		for index, target := range typed.Lhs {
			field, ok := positionField(target, receiver)
			if !ok {
				continue
			}
			if field == "" {
				return false
			}
			if field != "Line" {
				continue
			}
			if typed.Tok == token.ADD_ASSIGN {
				if index >= len(typed.Rhs) {
					return false
				}
				value, resolved := s.constantValue(typed.Rhs[index], 0)
				if !resolved || value <= 0 {
					return false
				}
				continue
			}
			if typed.Tok != token.ASSIGN || index >= len(typed.Rhs) {
				return false
			}
			value, resolved := s.constantValue(typed.Rhs[index], 0)
			if !resolved || value == 0 {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// constantValue resolves the integer a source expression denotes, following
// constant names declared in this same file. It resolves only the forms a
// scanner sentinel or a line seed is written in, so an expression it cannot
// resolve is reported as unresolved rather than assumed.
func (s *lexerSource) constantValue(expression goast.Expr, depth int) (int64, bool) {
	if depth > 8 {
		return 0, false
	}
	switch node := expression.(type) {
	case *goast.ParenExpr:
		return s.constantValue(node.X, depth+1)
	case *goast.BasicLit:
		switch node.Kind {
		case token.INT:
			value, err := strconv.ParseInt(node.Value, 0, 64)
			if err != nil {
				return 0, false
			}
			return value, true
		case token.CHAR:
			if len(node.Value) < 3 {
				return 0, false
			}
			character, _, _, err := strconv.UnquoteChar(node.Value[1:len(node.Value)-1], '\'')
			if err != nil {
				return 0, false
			}
			return int64(character), true
		default:
			return 0, false
		}
	case *goast.UnaryExpr:
		value, ok := s.constantValue(node.X, depth+1)
		if !ok {
			return 0, false
		}
		switch node.Op {
		case token.SUB:
			return -value, true
		case token.ADD:
			return value, true
		default:
			return 0, false
		}
	case *goast.Ident:
		definition, declared := s.consts[node.Name]
		if !declared {
			return 0, false
		}
		return s.constantValue(definition, depth+1)
	default:
		return 0, false
	}
}

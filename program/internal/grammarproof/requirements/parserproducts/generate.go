package parserproducts

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/program/internal/grammarproof"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/recursion"
)

// Current proves the checked-in parser-products artifact against one fresh
// grammar snapshot. The caller supplies the repository root; this authority
// never guesses a working directory or retains a second source location.
func Current(root string) (Evidence, error) {
	snapshot, err := grammarproof.Collect(root)
	if err != nil {
		return Evidence{}, fmt.Errorf("parser products: collect grammar proof: %w", err)
	}
	if err := Generated.Validate(root, snapshot); err != nil {
		return Evidence{}, err
	}
	rendered, err := renderFiles(Generated)
	if err != nil {
		return Evidence{}, err
	}
	if err := checkRendered(generatedDirectory(), rendered); err != nil {
		return Evidence{}, err
	}
	return clone(Generated), nil
}

// Generate rebuilds or checks the checked-in parser-products authority from
// one coherent grammar snapshot. It is deliberately the only renderer for
// this artifact, so canonical bytes and generated rows cannot drift apart.
func Generate(root, out string, check bool) error {
	if filepath.Base(out) != "evidence_gen.go" {
		return fmt.Errorf("parser products: out must name evidence_gen.go, got %s", filepath.Base(out))
	}
	snapshot, err := grammarproof.Collect(root)
	if err != nil {
		return fmt.Errorf("parser products: collect grammar proof: %w", err)
	}
	evidence, err := Build(root, snapshot)
	if err != nil {
		return err
	}
	rendered, err := renderFiles(evidence)
	if err != nil {
		return err
	}
	directory := filepath.Dir(out)
	if check {
		return checkRendered(directory, rendered)
	}
	return writeRendered(directory, rendered)
}

const generatedPrefix = "evidence_gen"

func expectedGeneratedFiles() []string {
	return []string{
		"evidence_gen.go",
		"evidence_gen_carriers.go",
		"evidence_gen_fields.go",
		"evidence_gen_helpers.go",
		"evidence_gen_mutations.go",
		"evidence_gen_products.go",
		"evidence_gen_recursion.go",
		"evidence_gen_sequences.go",
		"evidence_gen_terms.go",
		"evidence_gen_terms_control.go",
		"evidence_gen_terms_expression.go",
		"evidence_gen_terms_type.go",
	}
}

func checkRendered(directory string, rendered map[string][]byte) error {
	for _, name := range expectedGeneratedFiles() {
		current, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || !bytes.Equal(current, rendered[name]) {
			return fmt.Errorf("parser products: generated evidence %s is stale", name)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("parser products: read generated directory: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), generatedPrefix) && strings.HasSuffix(entry.Name(), ".go") {
			if _, expected := rendered[entry.Name()]; !expected {
				return fmt.Errorf("parser products: stale generated evidence %s", entry.Name())
			}
		}
	}
	return nil
}

func writeRendered(directory string, rendered map[string][]byte) error {
	if err := checkNoUnexpected(directory, rendered); err != nil {
		return err
	}
	for _, name := range expectedGeneratedFiles() {
		if err := os.WriteFile(filepath.Join(directory, name), rendered[name], 0o644); err != nil {
			return fmt.Errorf("parser products: write generated evidence %s: %w", name, err)
		}
	}
	return nil
}

func checkNoUnexpected(directory string, rendered map[string][]byte) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("parser products: read generated directory: %w", err)
	}
	var stale []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), generatedPrefix) && strings.HasSuffix(entry.Name(), ".go") {
			if _, expected := rendered[entry.Name()]; !expected {
				stale = append(stale, entry.Name())
			}
		}
	}
	if len(stale) != 0 {
		sort.Strings(stale)
		return fmt.Errorf("parser products: refuse to overwrite with stale generated evidence %s", strings.Join(stale, ", "))
	}
	return nil
}

// render is a compact review projection. renderFiles is the checked-in
// authority and contains every typed arena coordinate.
func render(e Evidence) ([]byte, error) {
	var out strings.Builder
	out.WriteString(generatedHeader())
	out.WriteString("\nfunc init() {\n\tGenerated = Evidence{\n")
	fmt.Fprintf(&out, "\t\tGrammarDigest: %q,\n", e.GrammarDigest)
	fmt.Fprintf(&out, "\t\tParserSourceDigest: %q,\n", e.ParserSourceDigest)
	fmt.Fprintf(&out, "\t\tSchemaDigest: %q,\n", e.SchemaDigest)
	fmt.Fprintf(&out, "\t\tIngressDigest: %q,\n", e.IngressDigest)
	fmt.Fprintf(&out, "\t\tDigest: %q,\n", e.Digest)
	out.WriteString("\t\tFields: generatedFields(),\n\t\tProducts: generatedProducts(),\n")
	out.WriteString("\t\tProductLaws: generatedProductLaws(),\n\t\tHelperLaws: generatedHelperLaws(),\n")
	out.WriteString("\t\tSequences: generatedSequences(),\n\t\tMutations: generatedMutations(),\n")
	out.WriteString("\t\tActionTerms: generatedActionTerms(),\n\t\tCarriers: generatedCarriers(),\n\t\tRecursion: generatedRecursion(),\n")
	out.WriteString("\t}\n}\n")
	return format.Source([]byte(out.String()))
}

func generatedHeader(imports ...string) string {
	var out strings.Builder
	out.WriteString("// Code generated by parser-products evidence; DO NOT EDIT.\n\npackage parserproducts\n")
	if len(imports) == 0 {
		return out.String()
	}
	out.WriteString("\nimport (\n")
	for _, value := range imports {
		fmt.Fprintf(&out, "\t%q\n", value)
	}
	out.WriteString(")\n")
	return out.String()
}

func formatGenerated(name string, source string) ([]byte, error) {
	result, err := format.Source([]byte(source))
	if err != nil {
		return nil, fmt.Errorf("parser products: format %s: %w", name, err)
	}
	for number, line := range strings.Split(string(result), "\n") {
		if len(line) > 320 {
			return nil, fmt.Errorf("parser products: generated %s line %d exceeds 320 bytes", name, number+1)
		}
	}
	if lines := len(strings.Split(string(result), "\n")); lines > 2000 {
		return nil, fmt.Errorf("parser products: generated %s has %d lines, want at most 2000", name, lines)
	}
	return result, nil
}

func renderFiles(e Evidence) (map[string][]byte, error) {
	raw := make(map[string]string, len(expectedGeneratedFiles()))
	raw["evidence_gen.go"] = renderAssembly(e)
	raw["evidence_gen_fields.go"] = renderFields(e.Fields)
	raw["evidence_gen_products.go"] = renderProductsFile(e.Products)
	raw["evidence_gen_helpers.go"] = renderHelpersFile(e.HelperLaws)
	raw["evidence_gen_sequences.go"] = renderSequencesFile(e.Sequences)
	raw["evidence_gen_mutations.go"] = renderMutationsFile(e.Mutations)
	raw["evidence_gen_carriers.go"] = renderCarriersFile(e.Carriers)
	raw["evidence_gen_recursion.go"] = renderRecursionFile(e.Recursion)

	lawParts, err := splitProductLaws(e.ProductLaws, []int{1600, 1600, 1600, 950})
	if err != nil {
		return nil, err
	}
	parts := splitActionTerms(e.ActionTerms.Terms, 3)
	raw["evidence_gen_terms.go"] = renderTermsAssembly(e.ActionTerms, lawParts[3])
	raw["evidence_gen_terms_control.go"] = renderTermFamily("generatedControlProductLaws", "generatedActionTermNodesControl", lawParts[0], parts[0])
	raw["evidence_gen_terms_expression.go"] = renderTermFamily("generatedExpressionProductLaws", "generatedActionTermNodesExpression", lawParts[1], parts[1])
	raw["evidence_gen_terms_type.go"] = renderTermFamily("generatedTypeProductLaws", "generatedActionTermNodesType", lawParts[2], parts[2])

	result := make(map[string][]byte, len(raw))
	for _, name := range expectedGeneratedFiles() {
		rendered, err := formatGenerated(name, raw[name])
		if err != nil {
			return nil, err
		}
		result[name] = rendered
	}
	return result, nil
}

func renderAssembly(e Evidence) string {
	var out strings.Builder
	out.WriteString(generatedHeader())
	out.WriteString("\nfunc init() {\n\tGenerated = Evidence{\n")
	fmt.Fprintf(&out, "\t\tGrammarDigest: %q,\n", e.GrammarDigest)
	fmt.Fprintf(&out, "\t\tParserSourceDigest: %q,\n", e.ParserSourceDigest)
	fmt.Fprintf(&out, "\t\tSchemaDigest: %q,\n", e.SchemaDigest)
	fmt.Fprintf(&out, "\t\tIngressDigest: %q,\n", e.IngressDigest)
	fmt.Fprintf(&out, "\t\tDigest: %q,\n", e.Digest)
	out.WriteString("\t\tFields: generatedFields(),\n\t\tProducts: generatedProducts(),\n")
	out.WriteString("\t\tProductLaws: generatedProductLaws(),\n\t\tHelperLaws: generatedHelperLaws(),\n")
	out.WriteString("\t\tSequences: generatedSequences(),\n\t\tMutations: generatedMutations(),\n")
	out.WriteString("\t\tActionTerms: generatedActionTerms(),\n\t\tCarriers: generatedCarriers(),\n\t\tRecursion: generatedRecursion(),\n")
	out.WriteString("\t}\n}\n")
	return out.String()
}

func renderFields(rows []FieldState) string {
	var out strings.Builder
	out.WriteString(generatedHeader(
		"github.com/wippyai/go-lua/program/internal/grammarproof",
		"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/occurrence",
	))
	out.WriteString("\nfunc generatedFields() []FieldState { return []FieldState{\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "{Form:%q, Field:%q, State:grammarproof.FieldState(%d), Context:occurrence.Context(%d), Disposition:Disposition(%d), Source:%q,\nParserLaw:occurrence.ParserLaw(%d), SemanticLaw:occurrence.SemanticLaw(%d), IngressLaw:occurrence.IngressLaw(%d)},\n", row.Form, row.Field, row.State, row.Context, row.Disposition, row.Source, row.ParserLaw, row.SemanticLaw, row.IngressLaw)
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderProductsFile(rows []Product) string {
	var out strings.Builder
	out.WriteString(generatedHeader(
		"github.com/wippyai/go-lua/program/internal/grammarproof",
		"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/occurrence",
	))
	out.WriteString("\nfunc generatedProducts() []Product { return []Product{\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "{\nForm: %q,\nContext: occurrence.Context(%d),\nStates: ", row.Form, row.Context)
		renderFieldStates(&out, row.States)
		fmt.Fprintf(&out, ",\nSource: %q,\n},\n", row.Source)
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderHelpersFile(rows []HelperLaw) string {
	var out strings.Builder
	out.WriteString(generatedHeader())
	out.WriteString("\nfunc generatedHelperLaws() []HelperLaw { return []HelperLaw{\n")
	for _, row := range rows {
		renderHelperLaw(&out, row)
		out.WriteString(",\n")
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderSequencesFile(rows []SequenceLaw) string {
	var out strings.Builder
	out.WriteString(generatedHeader())
	out.WriteString("\nfunc generatedSequences() []SequenceLaw { return []SequenceLaw{\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "{Production:%q, Scope:ActionScopeID(%d), Destination:SequenceDestination{Tag:%q, Field:%q}, Construction:SequenceConstruction(%d), Segments:", row.Production, row.Scope, row.Destination.Tag, row.Destination.Field, row.Construction)
		renderSegments(&out, row.Segments)
		out.WriteString("},\n")
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderMutationsFile(rows []FieldMutation) string {
	var out strings.Builder
	out.WriteString(generatedHeader())
	out.WriteString("\nfunc generatedMutations() []FieldMutation { return []FieldMutation{\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "{Production:%q, Edit:", row.Production)
		renderEdit(&out, row.Edit)
		out.WriteString("},\n")
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderCarriersFile(rows []Carrier) string {
	var out strings.Builder
	out.WriteString(generatedHeader(
		"github.com/wippyai/go-lua/program/internal/grammarproof",
		"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/grammar",
	))
	out.WriteString("\nfunc generatedCarriers() []Carrier { return []Carrier{\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "{Form:%q, Field:%q, Class:grammar.ConstructorClass(%d), ChildType:%q, Cardinality:grammarproof.FieldState(%d)},\n", row.Form, row.Field, row.Class, row.ChildType, row.Cardinality)
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderRecursionFile(rows []recursion.Obligation) string {
	var out strings.Builder
	out.WriteString(generatedHeader("github.com/wippyai/go-lua/program/internal/grammarproof/requirements/recursion"))
	out.WriteString("\nfunc generatedRecursion() []recursion.Obligation { return []recursion.Obligation{\n")
	for _, row := range rows {
		fmt.Fprintf(&out, "{Family:recursion.Family(%d), Stage:recursion.Stage(%d), Nonterminal:%q},\n", row.Family, row.Stage, row.Nonterminal)
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderTermsAssembly(table ActionTerms, overflow []ProductLaw) string {
	var out strings.Builder
	out.WriteString(generatedHeader())
	out.WriteString("\nfunc generatedProductLaws() []ProductLaw {\n")
	out.WriteString("\trows := make([]ProductLaw, 0)\n")
	out.WriteString("\trows = append(rows, generatedControlProductLaws()...)\n")
	out.WriteString("\trows = append(rows, generatedExpressionProductLaws()...)\n")
	out.WriteString("\trows = append(rows, generatedTypeProductLaws()...)\n")
	out.WriteString("\trows = append(rows, generatedOverflowProductLaws()...)\n")
	out.WriteString("\tif err := validateProductLawOrder(rows); err != nil { panic(err) }\n\treturn rows\n}\n")
	out.WriteString("\nfunc generatedActionTerms() ActionTerms { return ActionTerms{\n")
	out.WriteString("Symbols: generatedActionSymbols(),\nScopes: generatedActionScopes(),\n")
	out.WriteString("Terms: append(append(generatedActionTermNodesControl(), generatedActionTermNodesExpression()...), generatedActionTermNodesType()...),\n")
	out.WriteString("Edges: generatedActionEdges(),\nChainTails: generatedActionChainTails(),\nPlaceSteps: generatedActionPlaceSteps(),\nGuardSymbols: generatedActionGuardSymbols(),\n} }\n")

	out.WriteString("\nfunc generatedActionSymbols() []ActionSymbol { return []ActionSymbol{\n")
	for _, row := range table.Symbols {
		fmt.Fprintf(&out, "{Kind:ActionSymbolKind(%d), Text:%q},\n", row.Kind, row.Text)
	}
	out.WriteString("} }\n")
	out.WriteString("\nfunc generatedOverflowProductLaws() []ProductLaw { return []ProductLaw{\n")
	for _, row := range overflow {
		renderProductLaw(&out, row)
		out.WriteString(",\n")
	}
	out.WriteString("} }\n")
	out.WriteString("\nfunc generatedActionScopes() []ActionScope { return []ActionScope{\n")
	for _, row := range table.Scopes {
		fmt.Fprintf(&out, "{Kind:ActionScopeKind(%d), Owner:ActionSymbolID(%d), Inputs:%d, Formals:%d, Locals:%d, Results:%d},\n", row.Kind, row.Owner, row.Inputs, row.Formals, row.Locals, row.Results)
	}
	out.WriteString("} }\n")
	out.WriteString("\nfunc generatedActionEdges() []ActionEdge { return []ActionEdge{\n")
	for _, row := range table.Edges {
		fmt.Fprintf(&out, "{Term:ActionTermID(%d), Label:ActionSymbolID(%d), Flags:ActionEdgeFlags(%d)},\n", row.Term, row.Label, row.Flags)
	}
	out.WriteString("} }\n")
	out.WriteString("\nfunc generatedActionChainTails() []ChainTail { return []ChainTail{\n")
	for _, row := range table.ChainTails {
		fmt.Fprintf(&out, "{Field:ActionSymbolID(%d), Value:ActionTermID(%d)},\n", row.Field, row.Value)
	}
	out.WriteString("} }\n")
	out.WriteString("\nfunc generatedActionPlaceSteps() []PlaceStep { return []PlaceStep{\n")
	for _, row := range table.PlaceSteps {
		fmt.Fprintf(&out, "{Kind:PlaceStepKind(%d), Field:ActionSymbolID(%d), Index:ActionTermID(%d)},\n", row.Kind, row.Field, row.Index)
	}
	out.WriteString("} }\n")
	out.WriteString("\nfunc generatedActionGuardSymbols() []ActionSymbolID { return []ActionSymbolID{\n")
	for _, row := range table.GuardSymbols {
		fmt.Fprintf(&out, "ActionSymbolID(%d),\n", row)
	}
	out.WriteString("} }\n")
	return out.String()
}

func renderTermFamily(lawFunction, nodeFunction string, laws []ProductLaw, nodes []ActionTerm) string {
	var out strings.Builder
	out.WriteString(generatedHeader())
	fmt.Fprintf(&out, "\nfunc %s() []ProductLaw { return []ProductLaw{\n", lawFunction)
	for _, row := range laws {
		renderProductLaw(&out, row)
		out.WriteString(",\n")
	}
	out.WriteString("} }\n")
	fmt.Fprintf(&out, "\nfunc %s() []ActionTerm { return []ActionTerm{\n", nodeFunction)
	for _, row := range nodes {
		fmt.Fprintf(&out, "{Scope:ActionScopeID(%d), Kind:ActionTermKind(%d), Slot:%d, Symbol:ActionSymbolID(%d), EdgeStart:%d, EdgeCount:%d},\n", row.Scope, row.Kind, row.Slot, row.Symbol, row.EdgeStart, row.EdgeCount)
	}
	out.WriteString("} }\n")
	return out.String()
}

func splitActionTerms(rows []ActionTerm, parts int) [][]ActionTerm {
	result := make([][]ActionTerm, parts)
	for index := range result {
		start := len(rows) * index / parts
		end := len(rows) * (index + 1) / parts
		result[index] = rows[start:end]
	}
	return result
}

func splitProductLaws(rows []ProductLaw, budgets []int) ([][]ProductLaw, error) {
	result := make([][]ProductLaw, len(budgets))
	next := 0
	for part, budget := range budgets {
		used := 0
		for next < len(rows) {
			lines := productLawRenderLines(rows[next])
			if len(result[part]) != 0 && used+lines > budget {
				break
			}
			if len(result[part]) == 0 && lines > budget {
				return nil, fmt.Errorf("parser products: one generated product law has %d lines, budget %d", lines, budget)
			}
			result[part] = append(result[part], rows[next])
			used += lines
			next++
		}
	}
	if next != len(rows) {
		return nil, fmt.Errorf("parser products: generated product-law chunks exceed vertical budgets")
	}
	return result, nil
}

func productLawRenderLines(row ProductLaw) int {
	var out strings.Builder
	renderProductLaw(&out, row)
	return strings.Count(out.String(), "\n") + 1
}

func productLawFamily(nonterminal string) (string, error) {
	switch nonterminal {
	case "typeexpr", "simpletypeexpr", "primarytypeexpr", "typeexprlist", "typeexprlist2", "calltypeargs", "returntypeannot", "typeparams", "optionaltypeparams", "typeparamlist", "typeparam", "typefieldlist", "typefield", "typefieldtype", "typednamelist", "typedname", "interfacebody", "interfacemethod", "interfaceextends", "interfaceref", "qualifiedtyperef", "funcparam", "funcparamlist", "annotation", "annotations":
		return "type", nil
	case "var", "varlist", "namelist", "expr", "exprlist", "string", "prefixexp", "functioncall", "afunctioncall", "args", "function", "funcbody", "parlist", "tableconstructor", "fieldlist", "field", "fieldsep", "fieldname", "staticfieldname", "methodname":
		return "expression", nil
	case "chunk", "chunk1", "block", "stat", "elseifs", "laststat", "funcname", "funcname1", "closegt":
		return "control", nil
	default:
		return "", fmt.Errorf("parser products: no generated product-law family for %s", nonterminal)
	}
}

func generatedDirectory() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(file)
}

func renderFieldStates(out *strings.Builder, rows []grammarproof.FieldState) {
	out.WriteString("[]grammarproof.FieldState{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "grammarproof.FieldState(%d),\n", row)
	}
	out.WriteByte('}')
}

func renderStrings(out *strings.Builder, rows []string) {
	out.WriteString("[]string{")
	for _, row := range rows {
		fmt.Fprintf(out, "%q,", row)
	}
	out.WriteByte('}')
}

func renderTermIDs(out *strings.Builder, rows []ActionTermID) {
	out.WriteString("[]ActionTermID{")
	for _, row := range rows {
		fmt.Fprintf(out, "ActionTermID(%d),", row)
	}
	out.WriteByte('}')
}

func renderPlaces(out *strings.Builder, rows []Place) {
	if len(rows) == 0 {
		out.WriteString("[]Place{}")
		return
	}
	out.WriteString("[]Place{\n")
	for _, row := range rows {
		renderPlace(out, row)
		out.WriteString(",\n")
	}
	out.WriteByte('}')
}

func renderGuard(out *strings.Builder, guard Guard) {
	if len(guard.Atoms) == 0 {
		out.WriteString("Guard{Atoms: []GuardAtom{}}")
		return
	}
	out.WriteString("Guard{Atoms: []GuardAtom{\n")
	for _, atom := range guard.Atoms {
		fmt.Fprintf(out, "{Kind: GuardAtomKind(%d), Negated: %t, Term: ActionTermID(%d), Constant: ActionSymbolID(%d), SetStart: %d, SetCount: %d, ParseClass: NumberParseClass(%d)},\n", atom.Kind, atom.Negated, atom.Term, atom.Constant, atom.SetStart, atom.SetCount, atom.ParseClass)
	}
	out.WriteString("}}")
}

func renderPlace(out *strings.Builder, place Place) {
	fmt.Fprintf(out, "Place{Scope: ActionScopeID(%d), Root: PlaceRoot(%d), Slot: %d, StepStart: %d, StepCount: %d}", place.Scope, place.Root, place.Slot, place.StepStart, place.StepCount)
}

func renderEdit(out *strings.Builder, row Edit) {
	fmt.Fprintf(out, "Edit{\nKind: EditKind(%d),\nGuard: ", row.Kind)
	renderGuard(out, row.Guard)
	out.WriteString(",\nPlace: ")
	renderPlace(out, row.Place)
	fmt.Fprintf(out, ",\nValue: ActionTermID(%d),\n}", row.Value)
}

func renderProducts(out *strings.Builder, rows []ConstructorProduct) {
	if len(rows) == 0 {
		out.WriteString("[]ConstructorProduct{}")
		return
	}
	out.WriteString("[]ConstructorProduct{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "{\nOrdinal: %d,\nGuard: ", row.Ordinal)
		renderGuard(out, row.Guard)
		fmt.Fprintf(out, ",\nConstructor: %q,\nFields: []ProductField{\n", row.Constructor)
		for _, field := range row.Fields {
			fmt.Fprintf(out, "{Field: %q, Kind: ActionValueKind(%d), Term: ActionTermID(%d)},\n", field.Field, field.Kind, field.Term)
		}
		out.WriteString("},\n},\n")
	}
	out.WriteByte('}')
}

func renderApplications(out *strings.Builder, rows []HelperApplication) {
	if len(rows) == 0 {
		out.WriteString("[]HelperApplication{}")
		return
	}
	out.WriteString("[]HelperApplication{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "{\nHelper: ActionSymbolID(%d),\nScope: ActionScopeID(%d),\nGuard: ", row.Helper, row.Scope)
		renderGuard(out, row.Guard)
		out.WriteString(",\nActuals: ")
		renderTermIDs(out, row.Actuals)
		out.WriteString(",\nResults: ")
		renderPlaces(out, row.Results)
		out.WriteString(",\n},\n")
	}
	out.WriteByte('}')
}

func renderEdits(out *strings.Builder, rows []Edit) {
	if len(rows) == 0 {
		out.WriteString("[]Edit{}")
		return
	}
	out.WriteString("[]Edit{\n")
	for _, row := range rows {
		renderEdit(out, row)
		out.WriteString(",\n")
	}
	out.WriteByte('}')
}

func renderRejects(out *strings.Builder, rows []Reject) {
	if len(rows) == 0 {
		out.WriteString("[]Reject{}")
		return
	}
	out.WriteString("[]Reject{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "{\nOrdinal: %d,\nCondition: RejectCondition(%d),\nGuard: ", row.Ordinal, row.Condition)
		renderGuard(out, row.Guard)
		fmt.Fprintf(out, ",\nDiagnostic: ActionSymbolID(%d),\n},\n", row.Diagnostic)
	}
	out.WriteByte('}')
}

func renderChains(out *strings.Builder, rows []ChainLaw) {
	if len(rows) == 0 {
		out.WriteString("[]ChainLaw{}")
		return
	}
	out.WriteString("[]ChainLaw{\n")
	for _, row := range rows {
		fmt.Fprintf(out, "{\nScope: ActionScopeID(%d),\nGuard: ", row.Scope)
		renderGuard(out, row.Guard)
		fmt.Fprintf(out, ",\nInput: ActionTermID(%d),\nSeed: ActionTermID(%d),\nLinkField: ActionSymbolID(%d),\nTailStart: %d,\nTailCount: %d,\n},\n", row.Input, row.Seed, row.LinkField, row.TailStart, row.TailCount)
	}
	out.WriteByte('}')
}

func renderSegments(out *strings.Builder, rows []SequenceSegment) {
	if len(rows) == 0 {
		out.WriteString("[]SequenceSegment{}")
		return
	}
	out.WriteString("[]SequenceSegment{")
	for _, row := range rows {
		fmt.Fprintf(out, "{Kind: SequenceSegmentKind(%d), Term: ActionTermID(%d)},", row.Kind, row.Term)
	}
	out.WriteByte('}')
}

func renderProductLaw(out *strings.Builder, row ProductLaw) {
	fmt.Fprintf(out, "{\nProduction: %q,\nNonterminal: %q,\nRHS: ", row.Production, row.Nonterminal)
	renderStrings(out, row.RHS)
	fmt.Fprintf(out, ",\nActionDigest: %q,\nScope: ActionScopeID(%d),\nForm: ActionForm(%d),\nForward: %d,\nProducts: ", row.ActionDigest, row.Scope, row.Form, row.Forward)
	renderProducts(out, row.Products)
	out.WriteString(",\nHelpers: ")
	renderApplications(out, row.Helpers)
	out.WriteString(",\nEdits: ")
	renderEdits(out, row.Edits)
	out.WriteString(",\nRejects: ")
	renderRejects(out, row.Rejects)
	out.WriteString(",\nChains: ")
	renderChains(out, row.Chains)
	out.WriteString(",\n}")
}

func renderHelperLaw(out *strings.Builder, row HelperLaw) {
	fmt.Fprintf(out, "{\nScope: ActionScopeID(%d),\nDisposition: HelperDisposition(%d),\nDigest: %q,\nReturns: []GuardedReturn{\n", row.Scope, row.Disposition, row.Digest)
	for _, returned := range row.Returns {
		fmt.Fprintf(out, "{Ordinal: %d, Guard: ", returned.Ordinal)
		renderGuard(out, returned.Guard)
		out.WriteString(", Values: ")
		renderTermIDs(out, returned.Values)
		out.WriteString("},\n")
	}
	out.WriteString("},\nRejects: ")
	renderRejects(out, row.Rejects)
	out.WriteString(",\nProducts: ")
	renderProducts(out, row.Products)
	out.WriteString(",\nHelpers: ")
	renderApplications(out, row.Helpers)
	out.WriteString(",\nEdits: ")
	renderEdits(out, row.Edits)
	out.WriteString(",\nSummary: HelperSummary{\nMaps: []MapIndex{\n")
	for _, mapRow := range row.Summary.Maps {
		fmt.Fprintf(out, "{Scope: ActionScopeID(%d), ItemScope: ActionScopeID(%d), Input: %d, Output: %d, Element: ActionTermID(%d)},\n", mapRow.Scope, mapRow.ItemScope, mapRow.Input, mapRow.Output, mapRow.Element)
	}
	out.WriteString("},\nPresence: []ConditionalPresence{\n")
	for _, presence := range row.Summary.Presence {
		fmt.Fprintf(out, "{Scope: ActionScopeID(%d), Output: %d, Predicate: PresencePredicateKind(%d), Input: %d, ItemField: ActionSymbolID(%d)},\n", presence.Scope, presence.Output, presence.Predicate, presence.Input, presence.ItemField)
	}
	out.WriteString("},\n},\n}")
}

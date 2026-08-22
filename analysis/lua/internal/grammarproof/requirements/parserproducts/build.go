package parserproducts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/occurrence"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/recursion"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
	"github.com/wippyai/go-lua/internal/framing"
)

// Build derives every parser construction coordinate from fresh cold syntax.
func Build(root string, snapshot grammarproof.Snapshot) (Evidence, error) {
	return deriveSealed(root, snapshot)
}

// deriveSealed performs one exact cold derivation and seals its canonical
// digest. Structural validation of an already-sealed value belongs to
// validateStructural; Build must not rederive its own result just to validate
// that result against the same source snapshot.
func deriveSealed(root string, snapshot grammarproof.Snapshot) (Evidence, error) {
	evidence, err := derive(root, snapshot)
	if err != nil {
		return Evidence{}, err
	}
	normalizeEvidenceSlices(&evidence)
	if err := validateEvidenceRows(evidence); err != nil {
		return Evidence{}, err
	}
	evidence.Digest = digest(evidence)
	return evidence, nil
}

func derive(root string, snapshot grammarproof.Snapshot) (Evidence, error) {
	if err := snapshot.ValidateGenerated(); err != nil {
		return Evidence{}, err
	}
	schema, err := parsersource.Discover(root)
	if err != nil {
		return Evidence{}, err
	}
	required, err := occurrence.Derive(schema)
	if err != nil {
		return Evidence{}, err
	}
	report, err := occurrence.Observe(required, schema, snapshot.Traces)
	if err != nil {
		return Evidence{}, err
	}
	dispositions, err := occurrence.ClassifyResidue(root, report, schema)
	if err != nil {
		return Evidence{}, err
	}
	fields, err := buildFields(schema, report, dispositions, snapshot)
	if err != nil {
		return Evidence{}, err
	}
	products, err := buildProducts(schema, snapshot, fields)
	if err != nil {
		return Evidence{}, err
	}
	laws, helpers, mutations, terms, err := deriveTypedRelations(root, schema)
	if err != nil {
		return Evidence{}, err
	}
	carriers, err := buildCarriers(schema)
	if err != nil {
		return Evidence{}, err
	}
	requiredRecursion, err := recursion.Discover(root)
	if err != nil {
		return Evidence{}, err
	}
	if err := requiredRecursion.Validate(); err != nil {
		return Evidence{}, err
	}
	parserSourceDigest, err := parsersource.ParserSourceDigest(root)
	if err != nil {
		return Evidence{}, fmt.Errorf("parser products: parser source digest: %w", err)
	}
	return Evidence{
		GrammarDigest: snapshot.Evidence.Digest, ParserSourceDigest: parserSourceDigest,
		SchemaDigest: schema.Digest(), IngressDigest: snapshot.Evidence.IngressDigest,
		Fields: fields, Products: products, ProductLaws: laws, HelperLaws: helpers,
		Mutations: mutations, ActionTerms: terms, Carriers: carriers,
		Recursion: append([]recursion.Obligation(nil), requiredRecursion.Required...),
	}, nil
}

func (e Evidence) Validate(root string, snapshot grammarproof.Snapshot) error {
	if err := e.validateStructural(); err != nil {
		return err
	}
	expected, err := deriveSealed(root, snapshot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(e, expected) {
		return fmt.Errorf("parser products: evidence differs from exact source denominator")
	}
	return nil
}

func (e Evidence) validateStructural() error {
	if e.Digest != digest(e) {
		return fmt.Errorf("parser products: invalid evidence digest")
	}
	return validateEvidenceRows(e)
}

func normalizeEvidenceSlices(evidence *Evidence) {
	if evidence.Fields == nil {
		evidence.Fields = []FieldState{}
	}
	if evidence.Products == nil {
		evidence.Products = []Product{}
	}
	if evidence.ProductLaws == nil {
		evidence.ProductLaws = []ProductLaw{}
	}
	if evidence.HelperLaws == nil {
		evidence.HelperLaws = []HelperLaw{}
	}
	if evidence.Mutations == nil {
		evidence.Mutations = []FieldMutation{}
	}
	if evidence.ActionTerms.Symbols == nil {
		evidence.ActionTerms.Symbols = []ActionSymbol{}
	}
	if evidence.ActionTerms.Scopes == nil {
		evidence.ActionTerms.Scopes = []ActionScope{}
	}
	if evidence.ActionTerms.Terms == nil {
		evidence.ActionTerms.Terms = []ActionTerm{}
	}
	if evidence.ActionTerms.Edges == nil {
		evidence.ActionTerms.Edges = []ActionEdge{}
	}
	if evidence.ActionTerms.ChainTails == nil {
		evidence.ActionTerms.ChainTails = []ChainTail{}
	}
	if evidence.ActionTerms.PlaceSteps == nil {
		evidence.ActionTerms.PlaceSteps = []PlaceStep{}
	}
	if evidence.ActionTerms.GuardSymbols == nil {
		evidence.ActionTerms.GuardSymbols = []ActionSymbolID{}
	}
	if evidence.Carriers == nil {
		evidence.Carriers = []Carrier{}
	}
	if evidence.Recursion == nil {
		evidence.Recursion = []recursion.Obligation{}
	}
	for index := range evidence.Products {
		if evidence.Products[index].States == nil {
			evidence.Products[index].States = []astcodec.FieldState{}
		}
	}
	for index := range evidence.ProductLaws {
		law := &evidence.ProductLaws[index]
		if law.RHS == nil {
			law.RHS = []string{}
		}
		normalizeProducts(law.Products)
		law.Products = nonNilProducts(law.Products)
		normalizeApplications(law.Helpers)
		law.Helpers = nonNilApplications(law.Helpers)
		normalizeEdits(law.Edits)
		law.Edits = nonNilEdits(law.Edits)
		if law.Rejects == nil {
			law.Rejects = []Reject{}
		}
		if law.Chains == nil {
			law.Chains = []ChainLaw{}
		}
		normalizeRejects(law.Rejects)
		normalizeChains(law.Chains)
	}
	for index := range evidence.HelperLaws {
		law := &evidence.HelperLaws[index]
		if law.Returns == nil {
			law.Returns = []GuardedReturn{}
		}
		if law.Rejects == nil {
			law.Rejects = []Reject{}
		}
		normalizeReturns(law.Returns)
		normalizeRejects(law.Rejects)
		normalizeProducts(law.Products)
		law.Products = nonNilProducts(law.Products)
		normalizeApplications(law.Helpers)
		law.Helpers = nonNilApplications(law.Helpers)
		normalizeEdits(law.Edits)
		law.Edits = nonNilEdits(law.Edits)
		if law.Summary.Maps == nil {
			law.Summary.Maps = []MapIndex{}
		}
		if law.Summary.Presence == nil {
			law.Summary.Presence = []ConditionalPresence{}
		}
	}
	for index := range evidence.Mutations {
		normalizeGuard(&evidence.Mutations[index].Edit.Guard)
	}
}

func nonNilProducts(rows []ConstructorProduct) []ConstructorProduct {
	if rows == nil {
		return []ConstructorProduct{}
	}
	return rows
}
func nonNilApplications(rows []HelperApplication) []HelperApplication {
	if rows == nil {
		return []HelperApplication{}
	}
	return rows
}
func nonNilEdits(rows []Edit) []Edit {
	if rows == nil {
		return []Edit{}
	}
	return rows
}
func normalizeProducts(rows []ConstructorProduct) {
	for index := range rows {
		if rows[index].Fields == nil {
			rows[index].Fields = []ProductField{}
		}
		normalizeGuard(&rows[index].Guard)
	}
}
func normalizeApplications(rows []HelperApplication) {
	for index := range rows {
		if rows[index].Actuals == nil {
			rows[index].Actuals = []ActionTermID{}
		}
		if rows[index].Results == nil {
			rows[index].Results = []Place{}
		}
		normalizeGuard(&rows[index].Guard)
	}
}
func normalizeEdits(rows []Edit) {
	for index := range rows {
		normalizeGuard(&rows[index].Guard)
	}
}
func normalizeReturns(rows []GuardedReturn) {
	for index := range rows {
		if rows[index].Values == nil {
			rows[index].Values = []ActionTermID{}
		}
		normalizeGuard(&rows[index].Guard)
	}
}
func normalizeRejects(rows []Reject) {
	for index := range rows {
		normalizeGuard(&rows[index].Guard)
	}
}
func normalizeChains(rows []ChainLaw) {
	for index := range rows {
		normalizeGuard(&rows[index].Guard)
	}
}
func normalizeGuard(guard *Guard) {
	if guard.Atoms == nil {
		guard.Atoms = []GuardAtom{}
	}
}

type evidenceUsage struct {
	evidence     Evidence
	terms        []bool
	scopes       []bool
	symbols      []bool
	steps        []bool
	chainTails   []bool
	guardSymbols []bool
}

func validateEvidenceRows(evidence Evidence) error {
	if err := evidence.ActionTerms.Validate(); err != nil {
		return err
	}
	use := evidenceUsage{
		evidence:     evidence,
		terms:        make([]bool, len(evidence.ActionTerms.Terms)),
		scopes:       make([]bool, len(evidence.ActionTerms.Scopes)),
		symbols:      make([]bool, len(evidence.ActionTerms.Symbols)),
		steps:        make([]bool, len(evidence.ActionTerms.PlaceSteps)),
		chainTails:   make([]bool, len(evidence.ActionTerms.ChainTails)),
		guardSymbols: make([]bool, len(evidence.ActionTerms.GuardSymbols)),
	}
	if err := use.fields(); err != nil {
		return err
	}
	if err := use.productLaws(); err != nil {
		return err
	}
	if err := use.helperLaws(); err != nil {
		return err
	}
	if err := use.mutations(); err != nil {
		return err
	}
	return use.finish()
}

func (use *evidenceUsage) fields() error {
	for index, row := range use.evidence.Fields {
		if row.Form == "" || row.Field == "" || row.Disposition == DispositionInvalid || index != 0 && !fieldLess(use.evidence.Fields[index-1], row) {
			return fmt.Errorf("parser products: fields are not canonical")
		}
	}
	for index, row := range use.evidence.Products {
		if row.Form == "" || row.Source == "" || index != 0 && !productLess(use.evidence.Products[index-1], row) {
			return fmt.Errorf("parser products: products are not canonical")
		}
	}
	return nil
}

func (use *evidenceUsage) productLaws() error {
	if err := validateProductLawOrder(use.evidence.ProductLaws); err != nil {
		return err
	}
	for _, law := range use.evidence.ProductLaws {
		scope, ok := use.evidence.ActionTerms.Scope(law.Scope)
		if !ok || scope.Kind != ActionScopeProduction {
			return fmt.Errorf("parser products: product law has invalid scope")
		}
		use.markScope(law.Scope)
		if law.Form == ActionFormInvalid || law.Form == ActionFormForward && (law.Forward <= 0 || law.Forward > int(scope.Inputs)) {
			return fmt.Errorf("parser products: invalid product action form")
		}
		if err := use.products(law.Products, law.Scope); err != nil {
			return err
		}
		if err := use.applications(law.Helpers, law.Scope); err != nil {
			return err
		}
		if err := use.edits(law.Edits, law.Scope); err != nil {
			return err
		}
		if err := use.rejects(law.Rejects, law.Scope); err != nil {
			return err
		}
		if err := use.chains(law.Chains, law.Scope); err != nil {
			return err
		}
	}
	return nil
}

func (use *evidenceUsage) helperLaws() error {
	semantic, metadata, diagnostic := 0, 0, 0
	returns, rejects := 0, 0
	for index, law := range use.evidence.HelperLaws {
		scope, ok := use.evidence.ActionTerms.Scope(law.Scope)
		if !ok || scope.Kind != ActionScopeHelper || index != 0 && use.evidence.HelperLaws[index-1].Scope >= law.Scope {
			return fmt.Errorf("parser products: helper laws are not canonical")
		}
		use.markScope(law.Scope)
		switch law.Disposition {
		case HelperDispositionSemantic:
			semantic++
		case HelperDispositionMetadata:
			metadata++
		case HelperDispositionDiagnostic:
			diagnostic++
		default:
			return fmt.Errorf("parser products: unclassified helper")
		}
		if law.Digest == "" {
			return fmt.Errorf("parser products: helper lacks digest")
		}
		if law.Disposition != HelperDispositionSemantic && (len(law.Returns) != 0 || len(law.Rejects) != 0 || len(law.Products) != 0 || len(law.Helpers) != 0 || len(law.Edits) != 0 || len(law.Summary.Maps) != 0 || len(law.Summary.Presence) != 0) {
			return fmt.Errorf("parser products: nonsemantic helper carries semantic rows")
		}
		for returnIndex, row := range law.Returns {
			if row.Ordinal != returnIndex+1 {
				return fmt.Errorf("parser products: noncanonical helper return ordinal")
			}
			if err := use.guard(row.Guard, law.Scope); err != nil {
				return err
			}
			for _, value := range row.Values {
				if err := use.term(value, law.Scope); err != nil {
					return err
				}
			}
		}
		if err := use.rejects(law.Rejects, law.Scope); err != nil {
			return err
		}
		if err := use.products(law.Products, law.Scope); err != nil {
			return err
		}
		if err := use.applications(law.Helpers, law.Scope); err != nil {
			return err
		}
		if err := use.edits(law.Edits, law.Scope); err != nil {
			return err
		}
		if err := use.summary(law.Summary, law.Scope); err != nil {
			return err
		}
		if law.Disposition == HelperDispositionSemantic {
			if err := validateHelperPartition(law); err != nil {
				return err
			}
		}
		returns += len(law.Returns)
		rejects += len(law.Rejects)
	}
	if semantic != 15 || metadata != 3 || diagnostic != 1 || returns != 18 || rejects != 5 {
		return fmt.Errorf("parser products: invalid helper ledger semantic=%d metadata=%d diagnostic=%d returns=%d rejects=%d", semantic, metadata, diagnostic, returns, rejects)
	}
	return nil
}

// validateHelperPartition checks the finite control relation of a semantic
// helper. Every valuation of its typed guard atoms reaches exactly one return
// or one reject, and every recorded row remains reachable. This deliberately
// works over the closed Guard atom language rather than over source syntax.
func validateHelperPartition(law HelperLaw) error {
	if len(law.Returns) == 0 || len(law.Rejects) > 1 {
		return fmt.Errorf("parser products: malformed semantic helper partition")
	}
	for _, row := range law.Returns {
		if len(row.Values) == 0 {
			return fmt.Errorf("parser products: semantic helper return has no values")
		}
	}

	atoms := make(map[GuardAtom]int)
	collect := func(guard Guard) {
		for _, atom := range guard.Atoms {
			base := atom
			base.Negated = false
			if _, known := atoms[base]; !known {
				atoms[base] = len(atoms)
			}
		}
	}
	for _, row := range law.Returns {
		collect(row.Guard)
	}
	for _, row := range law.Rejects {
		collect(row.Guard)
	}
	if len(atoms) > 12 {
		return fmt.Errorf("parser products: semantic helper partition has too many guard atoms")
	}

	returnReachable := make([]bool, len(law.Returns))
	rejectReachable := make([]bool, len(law.Rejects))
	for assignment := 0; assignment < 1<<len(atoms); assignment++ {
		accepted := 0
		for index, row := range law.Returns {
			if guardMatches(row.Guard, atoms, assignment) {
				accepted++
				returnReachable[index] = true
			}
		}
		rejected := 0
		for index, row := range law.Rejects {
			matches := guardMatches(row.Guard, atoms, assignment)
			if row.Condition == RejectUnlessAll {
				matches = !matches
			}
			if matches {
				rejected++
				rejectReachable[index] = true
			}
		}
		if accepted+rejected != 1 {
			return fmt.Errorf("parser products: semantic helper branches do not partition control")
		}
	}
	for _, reachable := range returnReachable {
		if !reachable {
			return fmt.Errorf("parser products: unreachable semantic helper return")
		}
	}
	for _, reachable := range rejectReachable {
		if !reachable {
			return fmt.Errorf("parser products: unreachable semantic helper reject")
		}
	}
	return nil
}

func guardMatches(guard Guard, atoms map[GuardAtom]int, assignment int) bool {
	for _, atom := range guard.Atoms {
		base := atom
		base.Negated = false
		matches := assignment&(1<<atoms[base]) != 0
		if atom.Negated {
			matches = !matches
		}
		if !matches {
			return false
		}
	}
	return true
}

func (use *evidenceUsage) products(rows []ConstructorProduct, scope ActionScopeID) error {
	for index, row := range rows {
		if row.Ordinal != index+1 || row.Constructor == "" {
			return fmt.Errorf("parser products: invalid constructor product")
		}
		if err := use.guard(row.Guard, scope); err != nil {
			return err
		}
		for _, field := range row.Fields {
			if field.Field == "" || field.Kind == ActionValueInvalid || field.Kind == ActionValueZero && field.Term != 0 {
				return fmt.Errorf("parser products: invalid product field")
			}
			if field.Kind == ActionValueTerm {
				if err := use.term(field.Term, scope); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (use *evidenceUsage) applications(rows []HelperApplication, scope ActionScopeID) error {
	for _, row := range rows {
		symbol, ok := use.evidence.ActionTerms.Symbol(row.Helper)
		if !ok || symbol.Kind != ActionSymbolCallable || row.Scope != scope {
			return fmt.Errorf("parser products: invalid helper application")
		}
		use.markSymbol(row.Helper)
		if err := use.guard(row.Guard, scope); err != nil {
			return err
		}
		for _, actual := range row.Actuals {
			if err := use.term(actual, scope); err != nil {
				return err
			}
		}
		for _, place := range row.Results {
			if place.Scope != scope || use.evidence.ActionTerms.ValidatePlace(place) != nil {
				return fmt.Errorf("parser products: invalid helper result place")
			}
			if err := use.markPlace(place); err != nil {
				return err
			}
		}
	}
	return nil
}

func (use *evidenceUsage) edits(rows []Edit, scope ActionScopeID) error {
	for _, row := range rows {
		if err := use.evidence.ActionTerms.ValidateEdit(row, scope); err != nil {
			return err
		}
		if err := use.guard(row.Guard, scope); err != nil {
			return err
		}
		if err := use.markPlace(row.Place); err != nil {
			return err
		}
		if err := use.term(row.Value, scope); err != nil {
			return err
		}
	}
	return nil
}

func (use *evidenceUsage) rejects(rows []Reject, scope ActionScopeID) error {
	for index, row := range rows {
		if row.Ordinal != index+1 {
			return fmt.Errorf("parser products: noncanonical reject ordinal")
		}
		if err := use.evidence.ActionTerms.ValidateReject(row, scope); err != nil {
			return err
		}
		if err := use.guard(row.Guard, scope); err != nil {
			return err
		}
		use.markSymbol(row.Diagnostic)
	}
	return nil
}

func (use *evidenceUsage) chains(rows []ChainLaw, scope ActionScopeID) error {
	for index, row := range rows {
		if row.Scope != scope || row.Input == 0 || row.Seed == 0 || row.LinkField == 0 {
			return fmt.Errorf("parser products: malformed chain law %d", index+1)
		}
		if err := use.guard(row.Guard, scope); err != nil {
			return err
		}
		input, ok := use.evidence.ActionTerms.Term(row.Input)
		if !ok || input.Scope != scope || input.Kind != ActionTermInput {
			return fmt.Errorf("parser products: chain input is not an action input")
		}
		if err := use.term(row.Input, scope); err != nil {
			return err
		}
		if err := use.term(row.Seed, scope); err != nil {
			return err
		}
		field, ok := use.evidence.ActionTerms.Symbol(row.LinkField)
		if !ok || field.Kind != ActionSymbolField {
			return fmt.Errorf("parser products: invalid chain link field")
		}
		use.markSymbol(row.LinkField)
		end := uint64(row.TailStart) + uint64(row.TailCount)
		if end > uint64(len(use.evidence.ActionTerms.ChainTails)) {
			return fmt.Errorf("parser products: invalid chain tail span")
		}
		previous := ActionSymbolID(0)
		for tailIndex := row.TailStart; tailIndex < row.TailStart+uint32(row.TailCount); tailIndex++ {
			tail := use.evidence.ActionTerms.ChainTails[tailIndex]
			if tail.Field <= previous {
				return fmt.Errorf("parser products: noncanonical chain tail fields")
			}
			previous = tail.Field
			use.chainTails[tailIndex] = true
			use.markSymbol(tail.Field)
			if err := use.term(tail.Value, scope); err != nil {
				return err
			}
		}
	}
	return nil
}

func (use *evidenceUsage) summary(summary HelperSummary, scope ActionScopeID) error {
	for _, row := range summary.Maps {
		helper, ok := use.evidence.ActionTerms.Scope(row.Scope)
		item, itemOK := use.evidence.ActionTerms.Scope(row.ItemScope)
		term, termOK := use.evidence.ActionTerms.Term(row.Element)
		if !ok || row.Scope != scope || !itemOK || item.Kind != ActionScopeMapItem || item.Owner != helper.Owner || row.Input >= helper.Formals || row.Output >= helper.Results || !termOK || term.Scope != row.ItemScope {
			return fmt.Errorf("parser products: invalid map summary")
		}
		use.markScope(row.ItemScope)
		use.markScope(row.Scope)
		if err := use.term(row.Element, row.ItemScope); err != nil {
			return err
		}
		if err := use.exactMapItemInput(row.ItemScope); err != nil {
			return err
		}
	}
	for _, row := range summary.Presence {
		helper, ok := use.evidence.ActionTerms.Scope(row.Scope)
		symbol, symbolOK := use.evidence.ActionTerms.Symbol(row.ItemField)
		if !ok || row.Scope != scope || row.Predicate != PresencePredicateAnyNonNilField || row.Input >= helper.Formals || row.Output >= helper.Results || !symbolOK || symbol.Kind != ActionSymbolField {
			return fmt.Errorf("parser products: invalid conditional presence")
		}
		use.markScope(row.Scope)
		use.markSymbol(row.ItemField)
	}
	return nil
}

func (use *evidenceUsage) exactMapItemInput(scope ActionScopeID) error {
	found := 0
	for _, term := range use.evidence.ActionTerms.Terms {
		if term.Scope == scope && term.Kind == ActionTermInput && term.Slot == 0 {
			found++
		}
	}
	if found != 1 {
		return fmt.Errorf("parser products: map item scope must bind exactly one input")
	}
	return nil
}

func (use *evidenceUsage) mutations() error {
	for index, row := range use.evidence.Mutations {
		if row.Production == "" || index != 0 && (use.evidence.Mutations[index-1].Production > row.Production || use.evidence.Mutations[index-1].Production == row.Production && !editLess(use.evidence.Mutations[index-1].Edit, row.Edit)) {
			return fmt.Errorf("parser products: mutations are not canonical")
		}
		if err := use.edits([]Edit{row.Edit}, row.Edit.Place.Scope); err != nil {
			return err
		}
	}
	return nil
}

func (use *evidenceUsage) guard(guard Guard, scope ActionScopeID) error {
	if err := use.evidence.ActionTerms.ValidateGuard(guard, scope); err != nil {
		return fmt.Errorf("parser products: guard in scope %d: %w", scope, err)
	}
	for _, atom := range guard.Atoms {
		if err := use.term(atom.Term, scope); err != nil {
			return err
		}
		if atom.Constant != 0 {
			use.markSymbol(atom.Constant)
		}
		for index := atom.SetStart; index < atom.SetStart+uint32(atom.SetCount); index++ {
			use.guardSymbols[index] = true
			use.markSymbol(use.evidence.ActionTerms.GuardSymbols[index])
		}
	}
	return nil
}

func (use *evidenceUsage) term(id ActionTermID, scope ActionScopeID) error {
	term, ok := use.evidence.ActionTerms.Term(id)
	if !ok || term.Scope != scope {
		return fmt.Errorf("parser products: term crosses row scope")
	}
	if use.terms[id-1] {
		return nil
	}
	use.terms[id-1] = true
	use.markScope(term.Scope)
	if term.Symbol != 0 {
		use.markSymbol(term.Symbol)
	}
	for _, edge := range use.evidence.ActionTerms.Edges[term.EdgeStart : term.EdgeStart+uint32(term.EdgeCount)] {
		if edge.Label != 0 {
			use.markSymbol(edge.Label)
		}
		if err := use.term(edge.Term, scope); err != nil {
			return err
		}
	}
	return nil
}

func (use *evidenceUsage) markPlace(place Place) error {
	use.markScope(place.Scope)
	for index := place.StepStart; index < place.StepStart+uint32(place.StepCount); index++ {
		use.steps[index] = true
		step := use.evidence.ActionTerms.PlaceSteps[index]
		if step.Field != 0 {
			use.markSymbol(step.Field)
		}
		if step.Index != 0 {
			if err := use.term(step.Index, place.Scope); err != nil {
				return err
			}
		}
	}
	return nil
}
func (use *evidenceUsage) markScope(id ActionScopeID) {
	use.scopes[id-1] = true
	scope := use.evidence.ActionTerms.Scopes[id-1]
	use.markSymbol(scope.Owner)
}
func (use *evidenceUsage) markSymbol(id ActionSymbolID) {
	if id != 0 {
		use.symbols[id-1] = true
	}
}
func (use *evidenceUsage) finish() error {
	for index, used := range use.terms {
		if !used {
			return fmt.Errorf("parser products: disconnected action term %d", index+1)
		}
	}
	for index, used := range use.scopes {
		if !used {
			return fmt.Errorf("parser products: disconnected action scope %d", index+1)
		}
	}
	for index, used := range use.symbols {
		if !used {
			return fmt.Errorf("parser products: disconnected action symbol %d", index+1)
		}
	}
	for index, used := range use.steps {
		if !used {
			return fmt.Errorf("parser products: disconnected place step %d", index)
		}
	}
	for index, used := range use.chainTails {
		if !used {
			return fmt.Errorf("parser products: disconnected chain tail %d", index)
		}
	}
	for index, used := range use.guardSymbols {
		if !used {
			return fmt.Errorf("parser products: disconnected guard type %d", index)
		}
	}
	return nil
}

func schemaConstructorFields(schema parsersource.Schema) map[string]constructorFields {
	known := make(map[string]constructorFields, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		if !constructor.Semantic {
			continue
		}
		fields := make([]string, len(constructor.Fields))
		for _, field := range constructor.Fields {
			fields[field.Ordinal] = field.Name
		}
		known[constructor.Name] = constructorFields{fields: fields}
	}
	return known
}

type constructorFields struct {
	class    parsersource.ConstructorClass
	semantic bool
	fields   []string
}

func buildFields(schema parsersource.Schema, report occurrence.Report, dispositions []occurrence.Disposition, snapshot grammarproof.Snapshot) ([]FieldState, error) {
	fields, err := schemaFieldNames(schema)
	if err != nil {
		return nil, err
	}
	ingress, corpus := ingressSources(snapshot), corpusSources(snapshot)
	observed := make(map[occurrence.Requirement]string, len(report.Witness))
	for _, witness := range report.Witness {
		if witness.Source == "" || observed[witness.Requirement] != "" || !ingress[witness.Source] {
			return nil, fmt.Errorf("parser products: invalid observed witness")
		}
		observed[witness.Requirement] = witness.Source
	}
	residue := make(map[occurrence.Requirement]occurrence.Disposition, len(dispositions))
	for _, disposition := range dispositions {
		if _, exists := residue[disposition.Requirement]; exists {
			return nil, fmt.Errorf("parser products: duplicate residue disposition")
		}
		residue[disposition.Requirement] = disposition
	}
	rows := make([]FieldState, 0, len(report.Required))
	for _, requirement := range report.Required {
		name, nameErr := requirementField(fields, requirement)
		if nameErr != nil {
			return nil, nameErr
		}
		row := FieldState{Form: requirement.Constructor, Field: name, State: requirement.State, Context: requirement.Context}
		if source := observed[requirement]; source != "" {
			row.Disposition, row.Source = DispositionObserved, source
			rows = append(rows, row)
			continue
		}
		disposition, exists := residue[requirement]
		if !exists {
			return nil, fmt.Errorf("parser products: unclassified field-state %s.%s", row.Form, row.Field)
		}
		switch disposition.Kind {
		case occurrence.DispositionParserImpossible:
			if disposition.Parser == occurrence.ParserLawInvalid || disposition.Semantic != occurrence.SemanticLawInvalid {
				return nil, fmt.Errorf("parser products: invalid parser-impossible disposition")
			}
			row.Disposition, row.ParserLaw = DispositionImpossible, disposition.Parser
		case occurrence.DispositionSourceReachable:
			source, ok := occurrence.SemanticWitnessSource(disposition.Semantic)
			if !ok || !ingress[source] {
				return nil, fmt.Errorf("parser products: invalid semantic witness")
			}
			input, exists := corpus[source]
			if !exists {
				return nil, fmt.Errorf("parser products: semantic witness outside corpus")
			}
			observations, observeErr := grammarproof.ObserveSource(input.Text, source+".lua")
			if observeErr != nil {
				return nil, observeErr
			}
			if observeErr := grammarproof.RequireObservedState(observations, row.Form, row.Field, row.State); observeErr != nil {
				return nil, observeErr
			}
			row.Disposition, row.Source, row.SemanticLaw = DispositionSemanticWitness, source, disposition.Semantic
		case occurrence.DispositionPublicIngressRejected:
			if disposition.Ingress == occurrence.IngressLawInvalid || disposition.Parser != occurrence.ParserLawInvalid || disposition.Semantic != occurrence.SemanticLawInvalid {
				return nil, fmt.Errorf("parser products: invalid public-ingress-rejected disposition")
			}
			row.Disposition, row.IngressLaw = DispositionPublicIngressRejected, disposition.Ingress
		default:
			return nil, fmt.Errorf("parser products: invalid residue disposition")
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool { return fieldLess(rows[left], rows[right]) })
	return rows, nil
}

func buildProducts(schema parsersource.Schema, snapshot grammarproof.Snapshot, fieldRows []FieldState) ([]Product, error) {
	fields, err := schemaFieldNames(schema)
	if err != nil {
		return nil, err
	}
	ingress := ingressSources(snapshot)
	products := make([]Product, 0)
	add := func(source string, occurrences []astcodec.Occurrence) error {
		if !ingress[source] {
			return fmt.Errorf("parser products: product source bypasses public ingress")
		}
		for _, observed := range occurrences {
			constructor, exists := fields[observed.Type]
			if !exists || !constructor.semantic {
				continue
			}
			states, stateErr := productStates(constructor, observed)
			if stateErr != nil {
				return stateErr
			}
			candidate := Product{
				Form:    observed.Type,
				Context: ContextFor(constructor.class),
				States:  states,
				Source:  source,
			}
			for index := range products {
				if !sameProductShape(products[index], candidate) {
					continue
				}
				if candidate.Source < products[index].Source {
					products[index] = candidate
				}
				candidate = Product{}
				break
			}
			if candidate.Form != "" {
				products = append(products, candidate)
			}
		}
		return nil
	}
	for _, trace := range snapshot.Traces {
		if err := add(trace.Source, trace.Occurrences); err != nil {
			return nil, err
		}
	}
	corpus := corpusSources(snapshot)
	witnessSources := make(map[string]bool)
	for _, row := range fieldRows {
		if row.Disposition == DispositionSemanticWitness {
			witnessSources[row.Source] = true
		}
	}
	for source := range witnessSources {
		input, exists := corpus[source]
		if !exists {
			return nil, fmt.Errorf("parser products: missing semantic product source")
		}
		observations, observeErr := grammarproof.ObserveSource(input.Text, source+".lua")
		if observeErr != nil {
			return nil, observeErr
		}
		if err := add(source, observations); err != nil {
			return nil, err
		}
	}
	sort.Slice(products, func(left, right int) bool { return productLess(products[left], products[right]) })
	return products, nil
}

func schemaFieldNames(schema parsersource.Schema) (map[string]constructorFields, error) {
	result := make(map[string]constructorFields, len(schema.Constructors))
	for _, constructor := range schema.Constructors {
		if constructor.Name == "" || constructor.Class == 0 {
			return nil, fmt.Errorf("parser products: invalid constructor schema")
		}
		item := constructorFields{class: constructor.Class, semantic: constructor.Semantic, fields: make([]string, len(constructor.Fields))}
		for _, field := range constructor.Fields {
			if field.Ordinal < 0 || field.Ordinal >= len(item.fields) || field.Name == "" || item.fields[field.Ordinal] != "" {
				return nil, fmt.Errorf("parser products: malformed field schema")
			}
			item.fields[field.Ordinal] = field.Name
		}
		result[constructor.Name] = item
	}
	return result, nil
}
func requirementField(fields map[string]constructorFields, requirement occurrence.Requirement) (string, error) {
	constructor, exists := fields[requirement.Constructor]
	if !exists || requirement.Field < 0 || requirement.Field >= len(constructor.fields) {
		return "", fmt.Errorf("parser products: invalid field requirement")
	}
	return constructor.fields[requirement.Field], nil
}
func productStates(constructor constructorFields, observed astcodec.Occurrence) ([]astcodec.FieldState, error) {
	byName := make(map[string]astcodec.FieldState, len(observed.Fields))
	for _, field := range observed.Fields {
		if _, exists := byName[field.Name]; exists {
			return nil, fmt.Errorf("parser products: duplicate observed field")
		}
		byName[field.Name] = field.State
	}
	states := make([]astcodec.FieldState, len(constructor.fields))
	for index, name := range constructor.fields {
		state, exists := byName[name]
		if !exists {
			return nil, fmt.Errorf("parser products: AST omits schema field")
		}
		states[index] = state
	}
	return states, nil
}
func ingressSources(snapshot grammarproof.Snapshot) map[string]bool {
	result := make(map[string]bool, len(snapshot.Evidence.Ingress))
	for _, row := range snapshot.Evidence.Ingress {
		result[row.Source] = true
	}
	return result
}
func corpusSources(snapshot grammarproof.Snapshot) map[string]grammarproof.CorpusSource {
	result := make(map[string]grammarproof.CorpusSource, len(snapshot.Corpus))
	for _, source := range snapshot.Corpus {
		result[source.ID] = source
	}
	return result
}
func ContextFor(class parsersource.ConstructorClass) occurrence.Context {
	switch class {
	case parsersource.ConstructorStatement:
		return occurrence.ContextStatement
	case parsersource.ConstructorExpression:
		return occurrence.ContextExpression
	case parsersource.ConstructorTypeExpression:
		return occurrence.ContextStaticType
	default:
		return occurrence.ContextStructural
	}
}
func sameProductShape(left, right Product) bool {
	if left.Form != right.Form || left.Context != right.Context || len(left.States) != len(right.States) {
		return false
	}
	for index := range left.States {
		if left.States[index] != right.States[index] {
			return false
		}
	}
	return true
}
func fieldLess(left, right FieldState) bool {
	if left.Form != right.Form {
		return left.Form < right.Form
	}
	if left.Field != right.Field {
		return left.Field < right.Field
	}
	if left.Context != right.Context {
		return left.Context < right.Context
	}
	return left.State < right.State
}
func productLess(left, right Product) bool {
	if left.Form != right.Form {
		return left.Form < right.Form
	}
	if left.Context != right.Context {
		return left.Context < right.Context
	}
	for index := 0; index < len(left.States) && index < len(right.States); index++ {
		if left.States[index] != right.States[index] {
			return left.States[index] < right.States[index]
		}
	}
	return len(left.States) < len(right.States)
}
func sortProductLaws(rows []ProductLaw) {
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Production < rows[right].Production
	})
}

func validateProductLawOrder(rows []ProductLaw) error {
	for index, law := range rows {
		if law.Production == "" || law.ActionDigest == "" {
			return fmt.Errorf("parser products: incomplete parser action law")
		}
		if index != 0 && rows[index-1].Production >= law.Production {
			return fmt.Errorf("parser products: parser action laws are not a total canonical relation")
		}
	}
	return nil
}

func digest(e Evidence) string {
	sum := sha256.Sum256(e.Canonical())
	return hex.EncodeToString(sum[:])
}

// Canonical is a strict framed encoding of every public evidence coordinate.
func (e Evidence) Canonical() []byte {
	var out bytes.Buffer
	var w framing.Writer
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(w.Reset(&out, "program.parserproducts.evidence", 5))
	stringValue := func(value string) { must(w.String(value)) }
	uintValue := func(value uint64) { must(w.Uint(value)) }
	stringValue(e.GrammarDigest)
	stringValue(e.ParserSourceDigest)
	stringValue(e.SchemaDigest)
	stringValue(e.IngressDigest)
	must(w.Record(1))
	must(w.Count(uint64(len(e.Fields))))
	for _, row := range e.Fields {
		stringValue(row.Form)
		stringValue(row.Field)
		stringValue(row.Source)
		uintValue(uint64(row.State))
		uintValue(uint64(row.Context))
		uintValue(uint64(row.Disposition))
		uintValue(uint64(row.ParserLaw))
		uintValue(uint64(row.SemanticLaw))
		uintValue(uint64(row.IngressLaw))
	}
	must(w.Record(2))
	must(w.Count(uint64(len(e.Products))))
	for _, row := range e.Products {
		stringValue(row.Form)
		stringValue(row.Source)
		uintValue(uint64(row.Context))
		statesCanonical(&w, row.States, must)
	}
	must(w.Record(3))
	must(w.Count(uint64(len(e.ProductLaws))))
	for _, row := range e.ProductLaws {
		productLawCanonical(&w, row, must)
	}
	must(w.Record(4))
	must(w.Count(uint64(len(e.HelperLaws))))
	for _, row := range e.HelperLaws {
		helperLawCanonical(&w, row, must)
	}
	must(w.Record(5))
	must(w.Count(uint64(len(e.Mutations))))
	for _, row := range e.Mutations {
		stringValue(row.Production)
		editCanonical(&w, row.Edit, must)
	}
	must(w.Record(7))
	must(w.Count(uint64(len(e.ActionTerms.Symbols))))
	for _, row := range e.ActionTerms.Symbols {
		uintValue(uint64(row.Kind))
		stringValue(row.Text)
	}
	must(w.Record(8))
	must(w.Count(uint64(len(e.ActionTerms.Scopes))))
	for _, row := range e.ActionTerms.Scopes {
		uintValue(uint64(row.Kind))
		uintValue(uint64(row.Owner))
		uintValue(uint64(row.Inputs))
		uintValue(uint64(row.Formals))
		uintValue(uint64(row.Locals))
		uintValue(uint64(row.Results))
	}
	must(w.Record(9))
	must(w.Count(uint64(len(e.ActionTerms.Terms))))
	for _, row := range e.ActionTerms.Terms {
		uintValue(uint64(row.Scope))
		uintValue(uint64(row.Kind))
		uintValue(uint64(row.Slot))
		uintValue(uint64(row.Symbol))
		uintValue(uint64(row.EdgeStart))
		uintValue(uint64(row.EdgeCount))
	}
	must(w.Record(10))
	must(w.Count(uint64(len(e.ActionTerms.Edges))))
	for _, row := range e.ActionTerms.Edges {
		uintValue(uint64(row.Term))
		uintValue(uint64(row.Label))
		uintValue(uint64(row.Flags))
	}
	must(w.Record(11))
	must(w.Count(uint64(len(e.ActionTerms.ChainTails))))
	for _, row := range e.ActionTerms.ChainTails {
		uintValue(uint64(row.Field))
		uintValue(uint64(row.Value))
	}
	must(w.Record(12))
	must(w.Count(uint64(len(e.ActionTerms.PlaceSteps))))
	for _, row := range e.ActionTerms.PlaceSteps {
		uintValue(uint64(row.Kind))
		uintValue(uint64(row.Field))
		uintValue(uint64(row.Index))
	}
	must(w.Record(13))
	must(w.Count(uint64(len(e.ActionTerms.GuardSymbols))))
	for _, row := range e.ActionTerms.GuardSymbols {
		uintValue(uint64(row))
	}
	must(w.Record(14))
	must(w.Count(uint64(len(e.Carriers))))
	for _, row := range e.Carriers {
		stringValue(row.Form)
		stringValue(row.Field)
		stringValue(row.ChildType)
		uintValue(uint64(row.Class))
		uintValue(uint64(row.Cardinality))
	}
	must(w.Record(15))
	must(w.Count(uint64(len(e.Recursion))))
	for _, row := range e.Recursion {
		stringValue(row.Nonterminal)
		uintValue(uint64(row.Family))
		uintValue(uint64(row.Stage))
	}
	must(w.Finish())
	return out.Bytes()
}

func statesCanonical(w *framing.Writer, rows []astcodec.FieldState, must func(error)) {
	must(w.Count(uint64(len(rows))))
	for _, row := range rows {
		must(w.Uint(uint64(row)))
	}
}

func guardCanonical(w *framing.Writer, guard Guard, must func(error)) {
	must(w.Count(uint64(len(guard.Atoms))))
	for _, atom := range guard.Atoms {
		must(w.Uint(uint64(atom.Kind)))
		if atom.Negated {
			must(w.Uint(1))
		} else {
			must(w.Uint(0))
		}
		must(w.Uint(uint64(atom.Term)))
		must(w.Uint(uint64(atom.Constant)))
		must(w.Uint(uint64(atom.SetStart)))
		must(w.Uint(uint64(atom.SetCount)))
		must(w.Uint(uint64(atom.ParseClass)))
	}
}

func placeCanonical(w *framing.Writer, place Place, must func(error)) {
	must(w.Uint(uint64(place.Scope)))
	must(w.Uint(uint64(place.Root)))
	must(w.Uint(uint64(place.Slot)))
	must(w.Uint(uint64(place.StepStart)))
	must(w.Uint(uint64(place.StepCount)))
}

func editCanonical(w *framing.Writer, edit Edit, must func(error)) {
	must(w.Uint(uint64(edit.Kind)))
	guardCanonical(w, edit.Guard, must)
	placeCanonical(w, edit.Place, must)
	must(w.Uint(uint64(edit.Value)))
}

func productCanonical(w *framing.Writer, row ConstructorProduct, must func(error)) {
	must(w.Uint(uint64(row.Ordinal)))
	guardCanonical(w, row.Guard, must)
	must(w.String(row.Constructor))
	must(w.Count(uint64(len(row.Fields))))
	for _, field := range row.Fields {
		must(w.String(field.Field))
		must(w.Uint(uint64(field.Kind)))
		must(w.Uint(uint64(field.Term)))
	}
}

func applicationCanonical(w *framing.Writer, row HelperApplication, must func(error)) {
	must(w.Uint(uint64(row.Helper)))
	must(w.Uint(uint64(row.Scope)))
	guardCanonical(w, row.Guard, must)
	must(w.Count(uint64(len(row.Actuals))))
	for _, actual := range row.Actuals {
		must(w.Uint(uint64(actual)))
	}
	must(w.Count(uint64(len(row.Results))))
	for _, result := range row.Results {
		placeCanonical(w, result, must)
	}
}

func productLawCanonical(w *framing.Writer, law ProductLaw, must func(error)) {
	must(w.String(law.Production))
	must(w.String(law.Nonterminal))
	must(w.String(law.ActionDigest))
	must(w.Uint(uint64(law.Scope)))
	must(w.Uint(uint64(law.Form)))
	must(w.Uint(uint64(law.Forward)))
	must(w.Count(uint64(len(law.RHS))))
	for _, item := range law.RHS {
		must(w.String(item))
	}
	must(w.Count(uint64(len(law.Products))))
	for _, row := range law.Products {
		productCanonical(w, row, must)
	}
	must(w.Count(uint64(len(law.Helpers))))
	for _, row := range law.Helpers {
		applicationCanonical(w, row, must)
	}
	must(w.Count(uint64(len(law.Edits))))
	for _, row := range law.Edits {
		editCanonical(w, row, must)
	}
	must(w.Count(uint64(len(law.Rejects))))
	for _, row := range law.Rejects {
		must(w.Uint(uint64(row.Ordinal)))
		must(w.Uint(uint64(row.Condition)))
		guardCanonical(w, row.Guard, must)
		must(w.Uint(uint64(row.Diagnostic)))
	}
	must(w.Count(uint64(len(law.Chains))))
	for _, row := range law.Chains {
		chainCanonical(w, row, must)
	}
}

func chainCanonical(w *framing.Writer, row ChainLaw, must func(error)) {
	must(w.Uint(uint64(row.Scope)))
	guardCanonical(w, row.Guard, must)
	must(w.Uint(uint64(row.Input)))
	must(w.Uint(uint64(row.Seed)))
	must(w.Uint(uint64(row.LinkField)))
	must(w.Uint(uint64(row.TailStart)))
	must(w.Uint(uint64(row.TailCount)))
}

func helperLawCanonical(w *framing.Writer, law HelperLaw, must func(error)) {
	must(w.Uint(uint64(law.Scope)))
	must(w.Uint(uint64(law.Disposition)))
	must(w.String(law.Digest))
	must(w.Count(uint64(len(law.Returns))))
	for _, row := range law.Returns {
		must(w.Uint(uint64(row.Ordinal)))
		guardCanonical(w, row.Guard, must)
		must(w.Count(uint64(len(row.Values))))
		for _, value := range row.Values {
			must(w.Uint(uint64(value)))
		}
	}
	must(w.Count(uint64(len(law.Rejects))))
	for _, row := range law.Rejects {
		must(w.Uint(uint64(row.Ordinal)))
		must(w.Uint(uint64(row.Condition)))
		guardCanonical(w, row.Guard, must)
		must(w.Uint(uint64(row.Diagnostic)))
	}
	must(w.Count(uint64(len(law.Products))))
	for _, row := range law.Products {
		productCanonical(w, row, must)
	}
	must(w.Count(uint64(len(law.Helpers))))
	for _, row := range law.Helpers {
		applicationCanonical(w, row, must)
	}
	must(w.Count(uint64(len(law.Edits))))
	for _, row := range law.Edits {
		editCanonical(w, row, must)
	}
	must(w.Count(uint64(len(law.Summary.Maps))))
	for _, row := range law.Summary.Maps {
		must(w.Uint(uint64(row.Scope)))
		must(w.Uint(uint64(row.ItemScope)))
		must(w.Uint(uint64(row.Input)))
		must(w.Uint(uint64(row.Output)))
		must(w.Uint(uint64(row.Element)))
	}
	must(w.Count(uint64(len(law.Summary.Presence))))
	for _, row := range law.Summary.Presence {
		must(w.Uint(uint64(row.Scope)))
		must(w.Uint(uint64(row.Output)))
		must(w.Uint(uint64(row.Predicate)))
		must(w.Uint(uint64(row.Input)))
		must(w.Uint(uint64(row.ItemField)))
	}
}

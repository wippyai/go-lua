package parseruses

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
)

const (
	canonicalDomain  = "program.grammarproof.requirements.parseruses"
	canonicalVersion = 3

	recordProducts = 1
	recordSlot     = 2
	recordDirect   = 3
	recordHelper   = 4
	recordMutation = 5
	recordTail     = 6
	recordLValue   = 7
)

// Build derives every parser-consumption coordinate from one already sealed
// parser-products proof. The products proof is injected: this package never
// reopens parser source, grammar declarations, or action templates.
func Build(products parserproducts.Evidence) (Evidence, error) {
	if err := validateProducts(products); err != nil {
		return Evidence{}, err
	}
	evidence, err := derive(products)
	if err != nil {
		return Evidence{}, err
	}
	evidence.Digest = digest(evidence)
	if err := evidence.Validate(products); err != nil {
		return Evidence{}, err
	}
	return evidence, nil
}

// Current validates generated parser-use evidence against the supplied sealed
// parser-products artifact. Gate code obtains and validates that artifact from
// its owner before calling this function.
func Current(products parserproducts.Evidence) (Evidence, error) {
	if err := Generated.Validate(products); err != nil {
		return Evidence{}, err
	}
	return clone(Generated), nil
}

func derive(products parserproducts.Evidence) (Evidence, error) {
	carriers, err := carrierIndex(products.Carriers)
	if err != nil {
		return Evidence{}, err
	}
	slots, err := buildUseSlots(products.Fields, carriers)
	if err != nil {
		return Evidence{}, err
	}
	sequences, err := sequenceIndex(products.ProductLaws, products.Sequences)
	if err != nil {
		return Evidence{}, err
	}
	paths, err := buildUsePaths(products.ProductLaws, carriers, products.ActionTerms)
	if err != nil {
		return Evidence{}, err
	}
	helperPaths, err := buildHelperUsePaths(products.ProductLaws, products.HelperLaws, carriers, products.ActionTerms)
	if err != nil {
		return Evidence{}, err
	}
	mutationPaths, err := buildMutationUsePaths(products.ProductLaws, products.HelperLaws, products.Mutations, carriers, products.ActionTerms)
	if err != nil {
		return Evidence{}, err
	}
	if err := verifyRoutes(products.ProductLaws, products.HelperLaws, products.Mutations, carriers, slots, paths, helperPaths, mutationPaths, products.ActionTerms); err != nil {
		return Evidence{}, err
	}
	tails, err := buildValuesTails(products.ProductLaws, sequences)
	if err != nil {
		return Evidence{}, err
	}
	lvalues, err := buildLValuePaths(products.ProductLaws, paths, sequences, products.ActionTerms)
	if err != nil {
		return Evidence{}, err
	}
	return Evidence{
		ProductsDigest:   products.Digest,
		UseSlots:         slots,
		UsePaths:         paths,
		HelperUsePaths:   helperPaths,
		MutationUsePaths: mutationPaths,
		ValuesTails:      tails,
		LValuePaths:      lvalues,
	}, nil
}

// Validate requires structural equality with a fresh derivation. Cardinality
// is evidence, never an existential completion proxy.
func (e Evidence) Validate(products parserproducts.Evidence) error {
	if err := validateProducts(products); err != nil {
		return err
	}
	if e.ProductsDigest != products.Digest {
		return fmt.Errorf("parser uses: stale parser-products identity")
	}
	expected, err := derive(products)
	if err != nil {
		return err
	}
	expected.Digest = digest(expected)
	if e.Digest != digest(e) {
		return fmt.Errorf("parser uses: invalid evidence digest")
	}
	if !reflect.DeepEqual(e, expected) {
		return fmt.Errorf("parser uses: evidence differs from exact parser-products denominator")
	}
	return nil
}

// Canonical returns detached package-owned bytes. Digest is the SHA-256 of
// these bytes; the digest is not encoded into its own preimage.
func (e Evidence) Canonical() []byte {
	encoded, err := encodeCanonical(e)
	if err != nil {
		return nil
	}
	return append([]byte(nil), encoded...)
}

func digest(e Evidence) string {
	encoded, err := encodeCanonical(e)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func encodeCanonical(e Evidence) ([]byte, error) {
	var out bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&out, canonicalDomain, canonicalVersion); err != nil {
		return nil, err
	}
	if err := writer.Record(recordProducts); err != nil {
		return nil, err
	}
	if err := writer.String(e.ProductsDigest); err != nil {
		return nil, err
	}
	if err := writeSlots(&writer, e.UseSlots); err != nil {
		return nil, err
	}
	if err := writeDirect(&writer, e.UsePaths); err != nil {
		return nil, err
	}
	if err := writeHelpers(&writer, e.HelperUsePaths); err != nil {
		return nil, err
	}
	if err := writeMutations(&writer, e.MutationUsePaths); err != nil {
		return nil, err
	}
	if err := writeTails(&writer, e.ValuesTails); err != nil {
		return nil, err
	}
	if err := writeLValues(&writer, e.LValuePaths); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeSlots(w *framing.Writer, rows []UseSlot) error {
	if err := w.Record(recordSlot); err != nil {
		return err
	}
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.String(r.ParentForm); err != nil {
			return err
		}
		if err := w.String(r.ParentField); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.ParentContext)); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Role)); err != nil {
			return err
		}
		if err := w.String(r.ChildType); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Cardinality)); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Target)); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Disposition)); err != nil {
			return err
		}
		if err := w.String(r.Source); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.ParserLaw)); err != nil {
			return err
		}
	}
	return nil
}

func writeDirect(w *framing.Writer, rows []UsePath) error {
	if err := w.Record(recordDirect); err != nil {
		return err
	}
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.String(r.ParentProduction); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.ParentOrdinal)); err != nil {
			return err
		}
		if err := w.String(r.ParentForm); err != nil {
			return err
		}
		if err := w.String(r.ParentField); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Term)); err != nil {
			return err
		}
		if err := writeAxes(w, r.Role, r.Child, r.Target, r.Open, r.Table, r.LValue, r.Values); err != nil {
			return err
		}
	}
	return nil
}

func writeHelpers(w *framing.Writer, rows []HelperUsePath) error {
	if err := w.Record(recordHelper); err != nil {
		return err
	}
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.String(r.Production); err != nil {
			return err
		}
		if err := writeApplications(w, r.Applications); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Helper)); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Ordinal)); err != nil {
			return err
		}
		if err := w.String(r.ParentForm); err != nil {
			return err
		}
		if err := w.String(r.ParentField); err != nil {
			return err
		}
		if err := writeInstance(w, r.Instance); err != nil {
			return err
		}
		if err := writeAxes(w, r.Role, r.Child, r.Target, r.Open, r.Table, r.LValue, r.Values); err != nil {
			return err
		}
	}
	return nil
}

func writeMutations(w *framing.Writer, rows []MutationUsePath) error {
	if err := w.Record(recordMutation); err != nil {
		return err
	}
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.String(r.Production); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Ordinal)); err != nil {
			return err
		}
		if err := writeEdit(w, r.Edit); err != nil {
			return err
		}
		if err := writeAxes(w, r.Role, r.Child, r.Target, r.Open, r.Table, r.LValue, ValuesPositionNotApplicable); err != nil {
			return err
		}
	}
	return nil
}

func writeAxes(w *framing.Writer, role UseRole, child ChildClass, target ProgramUseClass, open OpenAxis, table TableAxis, lvalue LValueAxis, values ValuesPosition) error {
	for _, value := range []uint64{uint64(role), uint64(child), uint64(target), uint64(open), uint64(table), uint64(lvalue), uint64(values)} {
		if err := w.Uint(value); err != nil {
			return err
		}
	}
	return nil
}

func writeApplications(w *framing.Writer, rows []uint16) error {
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Uint(uint64(row)); err != nil {
			return err
		}
	}
	return nil
}

func writeInstance(w *framing.Writer, instance parserproducts.TermInstance) error {
	for _, value := range []uint64{uint64(instance.CallerScope), uint64(instance.HelperScope), uint64(instance.Root)} {
		if err := w.Uint(value); err != nil {
			return err
		}
	}
	if err := w.Count(uint64(len(instance.Actuals))); err != nil {
		return err
	}
	for _, actual := range instance.Actuals {
		if err := w.Uint(uint64(actual)); err != nil {
			return err
		}
	}
	return nil
}

func writeEdit(w *framing.Writer, edit parserproducts.Edit) error {
	if err := w.Uint(uint64(edit.Kind)); err != nil {
		return err
	}
	if err := w.Count(uint64(len(edit.Guard.Atoms))); err != nil {
		return err
	}
	for _, atom := range edit.Guard.Atoms {
		if err := w.Uint(uint64(atom.Kind)); err != nil {
			return err
		}
		if err := w.Bool(atom.Negated); err != nil {
			return err
		}
		if err := w.Uint(uint64(atom.Term)); err != nil {
			return err
		}
		if err := w.Uint(uint64(atom.Constant)); err != nil {
			return err
		}
		if err := w.Uint(uint64(atom.SetStart)); err != nil {
			return err
		}
		if err := w.Uint(uint64(atom.SetCount)); err != nil {
			return err
		}
		if err := w.Uint(uint64(atom.ParseClass)); err != nil {
			return err
		}
	}
	for _, value := range []uint64{uint64(edit.Place.Scope), uint64(edit.Place.Root), uint64(edit.Place.Slot), uint64(edit.Place.StepStart), uint64(edit.Place.StepCount), uint64(edit.Value)} {
		if err := w.Uint(value); err != nil {
			return err
		}
	}
	return nil
}

func writeTails(w *framing.Writer, rows []ValuesTail) error {
	if err := w.Record(recordTail); err != nil {
		return err
	}
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, r := range rows {
		if err := writeSequenceCoordinate(w, r.Sequence); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Position)); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.Successor)); err != nil {
			return err
		}
	}
	return nil
}

func writeSequenceCoordinate(w *framing.Writer, coordinate SequenceCoordinate) error {
	for _, value := range []string{coordinate.Production, coordinate.Tag, coordinate.Field} {
		if err := w.String(value); err != nil {
			return err
		}
	}
	return w.Uint(uint64(coordinate.Segment))
}

func writeLValues(w *framing.Writer, rows []LValuePath) error {
	if err := w.Record(recordLValue); err != nil {
		return err
	}
	if err := w.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, r := range rows {
		if err := w.String(r.SeedProduction); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.SeedOrdinal)); err != nil {
			return err
		}
		if err := writeSequenceCoordinate(w, r.Sequence); err != nil {
			return err
		}
		if err := w.String(r.TerminalProduction); err != nil {
			return err
		}
		if err := w.Uint(uint64(r.TerminalOrdinal)); err != nil {
			return err
		}
		if err := w.String(r.TerminalForm); err != nil {
			return err
		}
	}
	return nil
}

func clone(e Evidence) Evidence {
	copy := e
	copy.UseSlots = append([]UseSlot(nil), e.UseSlots...)
	copy.UsePaths = append([]UsePath(nil), e.UsePaths...)
	copy.HelperUsePaths = make([]HelperUsePath, len(e.HelperUsePaths))
	for index, row := range e.HelperUsePaths {
		copy.HelperUsePaths[index] = row
		copy.HelperUsePaths[index].Applications = append([]uint16(nil), row.Applications...)
		copy.HelperUsePaths[index].Instance.Actuals = append([]parserproducts.ActionTermID(nil), row.Instance.Actuals...)
	}
	copy.MutationUsePaths = make([]MutationUsePath, len(e.MutationUsePaths))
	for index, row := range e.MutationUsePaths {
		copy.MutationUsePaths[index] = row
		copy.MutationUsePaths[index].Edit = cloneEdit(row.Edit)
	}
	copy.ValuesTails = append([]ValuesTail(nil), e.ValuesTails...)
	copy.LValuePaths = append([]LValuePath(nil), e.LValuePaths...)
	return copy
}

// validateProducts verifies the minimum sealed-artifact invariants that this
// package consumes. Source freshness is the parser-products owner's concern;
// the gate establishes it before injecting this value.
func validateProducts(products parserproducts.Evidence) error {
	if products.Digest == "" || products.ParserSourceDigest == "" || products.SchemaDigest == "" || products.GrammarDigest == "" {
		return fmt.Errorf("parser uses: unsealed parser-products evidence")
	}
	canonical := products.Canonical()
	if len(canonical) == 0 {
		return fmt.Errorf("parser uses: parser-products has no canonical authority")
	}
	sum := sha256.Sum256(canonical)
	if products.Digest != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("parser uses: invalid parser-products digest")
	}
	if len(products.Carriers) == 0 || len(products.ProductLaws) == 0 || len(products.Sequences) == 0 {
		return fmt.Errorf("parser uses: incomplete parser-products denominator")
	}
	if err := products.ActionTerms.Validate(); err != nil {
		return fmt.Errorf("parser uses: invalid parser-products action arena: %w", err)
	}
	return nil
}

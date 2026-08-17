package census

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// RowKind is the grain one census row is stated at. The first three grains are
// the three ways the language can grow: a new parser alternative, a new AST
// form, and a new carrier on an existing form. A denominator that dropped any of
// them would let one of those three changes enter unaccounted. The fourth grain
// is finer than growth: a carrier that already exists still has a state space,
// and a disposition stated about a carrier as a whole cannot say which of its
// states it means. The fifth grain is finer than the first in the same way: a
// production row states which forms a reduction builds, and a disposition
// stated about the alternative as a whole cannot say what that reduction puts
// in each carrier of the form it builds.
type RowKind uint8

const (
	RowInvalid RowKind = iota
	RowProduction
	RowForm
	RowCarrier
	RowFieldState
	RowProduct
	RowUseSlot
)

// Row is one census row in the neutral vocabulary a seal-time disposition join
// consumes. It carries the containment edges that join needs and nothing else:
// the parser and AST types this package reads from source stay inside it.
type Row struct {
	// Key is the stable row identity, prefixed by grain.
	Key  string
	Kind RowKind
	// Builds is, for a production row, the form row keys its reduction
	// constructs.
	Builds []string
	// Owner is, for a carrier row, the form row key that declares it.
	Owner string
	// Coordinate marks a carrier that holds a source position rather than a
	// parsed value.
	Coordinate bool
	// Class is, for a form row, the semantic class the AST itself declares for
	// that form. It is the context a disposition about the form is stated in:
	// a structural form reaches the analyzer only inside the form declaring it,
	// while the other classes cross the boundary in their own right.
	Class parsersource.ConstructorClass
	// State is, for a field-state row, the exact carrier state the row stands
	// for.
	State parsersource.FieldState
	// Constructs is, for a product row, the form row the construction builds.
	Constructs string
	// Accepts is, for a use-slot row, the declared type of the child the slot
	// carries, and AcceptsClass the class of material that type is. It is a
	// declared type rather than a row key because an abstract child stands for
	// every form of its class, and the census states forms, not classes. Role is
	// the role the parent gives that child.
	Accepts      string
	AcceptsClass parsersource.ConstructorClass
	Role         parsersource.UseRole
	// FilledBy is, for a use-slot row, the product rows whose construction puts
	// a value in this slot. An empty column is evidence in its own right: it
	// names a slot the declarations admit and no action fills.
	FilledBy []string
}

// ProductionRow, FormRow, CarrierRow, and FieldStateRow are the row key
// spellings. They are functions rather than a format left to each caller so
// that one prefix change cannot silently split the key space in two.
func ProductionRow(key string) string { return "production:" + key }

func FormRow(name string) string { return "form:" + name }

func CarrierRow(form, field string) string { return "carrier:" + form + "." + field }

func FieldStateRow(form, field string, state parsersource.FieldState) string {
	return "state:" + form + "." + field + "@" + state.String()
}

// ProductRow names one construction of one action. The owner and ordinal are
// both needed: an action can build two values of the same form, and an ordinal
// alone would not say which action built either.
func ProductRow(owner string, ordinal int, constructor string) string {
	return "product:" + owner + "#" + strconv.Itoa(ordinal) + ":" + constructor
}

// UseSlotRow names one typed parent slot. The carrier it refines is named by
// the same form and field, so a reader holding a slot key can reach the carrier
// row it belongs to without a second index.
func UseSlotRow(form, field string) string { return "use:" + form + "." + field }

// coordinateTypes are the compiler's source-coordinate aliases. A carrier of
// one of these holds a position in the source text, not a parsed value, so no
// rule can own it and the join states that rather than leaving it to a caller's
// name heuristic.
var coordinateTypes = map[string]bool{"Position": true, "[]Position": true}

// Projection is the census as a seal-time join consumes it: the row
// denominator plus the census identity that produced it. The identity travels
// with the rows because a row key alone cannot express an action rewritten in
// place, and a join that pins only the row set would close over a parser it no
// longer describes.
//
// Rows, States and Products are separate because a join accounts for one grain
// at a time.
// Rows is the growth denominator - alternatives, forms, carriers - and a join
// over it is total. States is the finer carrier-state denominator: it is
// derived from the same declarations and is complete in its own right, but a
// disposition stated over it is a different judgment from a disposition stated
// over the carrier, so it is published beside Rows rather than folded into it
// where a join total over Rows would silently absorb it.
// Uses is the consumption denominator, and it is published beside the others
// for the same reason States is: which slots a language admits is a different
// judgment from which carriers it declares, and a join total over Rows would
// absorb it without ever accounting for a slot.
type Projection struct {
	Digest   string
	Rows     []Row
	States   []Row
	Products []Row
	Uses     []Row
}

// Project derives the complete projection of a census value.
func Project(value Census) Projection {
	return Projection{Digest: value.Digest, Rows: rows(value), States: states(value), Products: products(value), Uses: uses(value)}
}

// uses projects a census into the typed parent-slot denominator. The slot rows
// are the declarations' own denominator, and the fills are the actions that
// reach it, so one row carries both what the language admits at a coordinate
// and every construction that has ever put something there.
func uses(value Census) []Row {
	filled := make(map[string][]string, len(value.Slots))
	for _, use := range value.Uses {
		key := UseSlotRow(use.Form, use.Field)
		filled[key] = append(filled[key], ProductRow(use.Owner, use.Ordinal, use.Form))
	}
	result := make([]Row, 0, len(value.Slots))
	for _, slot := range value.Slots {
		key := UseSlotRow(slot.Form, slot.Field)
		fills := filled[key]
		sort.Strings(fills)
		result = append(result, Row{
			Key:          key,
			Kind:         RowUseSlot,
			Owner:        CarrierRow(slot.Form, slot.Field),
			Class:        slot.Class,
			State:        slot.Cardinality,
			Accepts:      slot.ChildType,
			AcceptsClass: slot.ChildClass,
			Role:         slot.Role,
			FilledBy:     dedupe(fills),
		})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result
}

// dedupe keeps one edge per filling construction. A single construction can
// reach one slot through more than one action route, and the slot is filled by
// that construction once either way.
func dedupe(rows []string) []string {
	if len(rows) == 0 {
		return nil
	}
	result := rows[:1]
	for _, row := range rows[1:] {
		if row != result[len(result)-1] {
			result = append(result, row)
		}
	}
	return result
}

// products projects a census into the per-construction denominator. Builds
// carries the carrier rows the construction assigns, so a join over this grain
// reads a construction's own field vector rather than re-deriving it from the
// form it names.
func products(value Census) []Row {
	result := make([]Row, 0, len(value.Products))
	for _, product := range value.Products {
		row := Row{
			Key:        ProductRow(product.Owner, product.Ordinal, product.Constructor),
			Kind:       RowProduct,
			Constructs: FormRow(product.Constructor),
		}
		for _, field := range product.Fields {
			if !field.Assigned {
				continue
			}
			row.Builds = append(row.Builds, CarrierRow(product.Constructor, field.Field))
		}
		result = append(result, row)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result
}

// rows projects a census into its growth row set, sorted by key so the result
// is a stable denominator rather than a map walk.
func rows(value Census) []Row {
	result := make([]Row, 0, len(value.Productions)+len(value.Constructors))
	for _, production := range value.Productions {
		builds := make([]string, 0, len(production.Constructors))
		for _, name := range production.Constructors {
			builds = append(builds, FormRow(name))
		}
		result = append(result, Row{Key: ProductionRow(production.Key), Kind: RowProduction, Builds: builds})
	}
	for _, constructor := range value.Constructors {
		result = append(result, Row{
			Key:   FormRow(constructor.Name),
			Kind:  RowForm,
			Class: constructor.Class,
		})
		for _, field := range constructor.Fields {
			result = append(result, Row{
				Key:        CarrierRow(constructor.Name, field.Name),
				Kind:       RowCarrier,
				Owner:      FormRow(constructor.Name),
				Coordinate: coordinateTypes[field.Type],
			})
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result
}

// states projects a census into the complete carrier-state denominator: one row
// per state the carrier's declared form admits. The state space is a property of
// the declaration, so this derives from the same AST source the carrier rows do
// and observes no parse. What a given state means - unreachable from any parse,
// reachable but refused at ingress, ordinary - is a separate judgment and is not
// stated here.
func states(value Census) []Row {
	result := make([]Row, 0, len(value.Constructors))
	for _, constructor := range value.Constructors {
		for _, field := range constructor.Fields {
			carrier := CarrierRow(constructor.Name, field.Name)
			for _, state := range field.Form.States() {
				result = append(result, Row{
					Key:   FieldStateRow(constructor.Name, field.Name, state),
					Kind:  RowFieldState,
					Owner: carrier,
					State: state,
				})
			}
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Key < result[right].Key })
	return result
}

// CurrentProjection validates the checked-in census against the current parser
// and AST source and projects it. It is the one call a seal-time join needs: a
// stale census fails here rather than joining against yesterday's language.
func CurrentProjection(root string) (Projection, error) {
	value, err := Current(root)
	if err != nil {
		return Projection{}, err
	}
	projection := Project(value)
	if len(projection.Rows) == 0 || len(projection.States) == 0 || len(projection.Products) == 0 || len(projection.Uses) == 0 || projection.Digest == "" {
		return Projection{}, fmt.Errorf("parser census: projection is empty")
	}
	return projection, nil
}

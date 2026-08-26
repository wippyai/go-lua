package relbind

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// KeyedDestination is the address a family declares when the owner names each
// destination row by its own content identity. It is relbindgen's own constant
// restated at declaration altitude so a family states an address without
// importing the substrate it is emitted against.
const KeyedDestination = -1

// NoDestination is the address a family declares when it publishes no fact.
// Its signature declares no output column, so there is no row to address.
const NoDestination = -2

// Axis is one owner's emission target. Produced artifacts live beside the
// owner whose mathematics they carry, never inside the generic substrate, so
// the substrate keeps no dependency on any domain and an owner keeps its own
// binding surface.
type Axis struct {
	// Key is the stable name payloads and families name this axis by.
	Key string
	// Dir is the module-relative directory the artifacts are written into.
	Dir string
	// Package is the Go package clause the artifacts carry.
	Package string
}

// Available reports whether the axis states an emission target.
func (axis Axis) Available() bool {
	return axis.Key != "" && axis.Dir != "" && axis.Package != ""
}

// Path is the import path of the axis's emitted package.
func (axis Axis) Path() string { return module + "/" + axis.Dir }

// module is the import prefix every emitted package resolves under.
const module = "github.com/wippyai/go-lua"

// Payload is one owner payload type: the Go type a column carries, the import
// that spells it, and the ascent authority its TypeID resolves to.
//
// A payload with no Lattice is a decode-only column. It carries an owner value
// into the frame and never states an order, so no algebra is emitted for it
// and the state layer resolves no ascent authority under its TypeID.
type Payload struct {
	// Key is the stable name slots and results reference this payload by.
	Key string
	// Axis is the owner whose package publishes this column.
	Axis string
	// Field is the exported Go identifier this payload occupies on the emitted
	// column structures.
	Field string
	// Type is the Go type expression of the owner value.
	Type string
	// Alias and Path spell the import the type expression resolves through. A
	// payload declared by the emitted package itself leaves both empty.
	Alias string
	Path  string
	// Lattice is the Go type expression of the owner's Lattice witness, empty
	// for a decode-only payload.
	Lattice string
}

// Available reports whether the payload states a referenceable owner type.
func (payload Payload) Available() bool {
	if payload.Key == "" || payload.Axis == "" || payload.Field == "" || payload.Type == "" {
		return false
	}
	if (payload.Alias == "") != (payload.Path == "") {
		return false
	}
	return true
}

// Ascends reports whether this payload's TypeID carries ascent authority.
func (payload Payload) Ascends() bool { return payload.Lattice != "" }

// Slot is one ordered input the sealed signature declares. Delivery is the
// signature's own frame-shape vocabulary, so the emitted decoder borrows a
// span exactly where the contract delivers one.
type Slot struct {
	// Field is the exported Go identifier this slot occupies on the argument.
	Field string
	// Payload names the owner column this slot decodes through.
	Payload string
	// Delivery is the sealed frame shape of this slot.
	Delivery signature.DeliveryKind
}

// Available reports whether the slot states a decodable input.
func (slot Slot) Available() bool {
	if slot.Field == "" || slot.Payload == "" {
		return false
	}
	switch slot.Delivery {
	case signature.ScalarDelivery, signature.BoundedSpanDelivery, signature.CompleteSpanDelivery:
		return true
	default:
		return false
	}
}

// Column is one declared output column of the produced row. Field selects the
// part of the result the column carries; an empty Field carries the whole
// result, which is the shape of every single-column family.
type Column struct {
	Field   string
	Payload string
}

// Available reports whether the column states a publishable destination.
func (column Column) Available() bool { return column.Payload != "" }

// Family is one census row's binding declaration: everything the two emitted
// artifacts need that the sealed signature does not already carry, which is
// exactly the Go spelling of the owner types.
type Family struct {
	// Census and Rule are the census row this family answers for. A family
	// that answers no census row states the ABI class arm it proves instead.
	Census string
	Rule   string
	// Axis is the owner whose package publishes this binding.
	Axis string
	// Stem is the Go identifier stem every emitted name is built from.
	Stem string
	// Judgment is the hand-authored operation type that carries the owner
	// mathematics. The generator names it and never states its body.
	Judgment string
	// Inputs are the sealed signature's ordered input slots.
	Inputs []Slot
	// Result is the payload the owner judgment produces.
	Result string
	// Outputs are the signature's ordered output columns.
	Outputs []Column
	// Cardinality and Bound are the sealed output row contract.
	Cardinality model.CardinalityKind
	Bound       uint32
	// Address is the scalar input slot whose row every proposal is published
	// at, or KeyedDestination when the owner names each row itself.
	Address int
	// Arm names the ABI class this family proves when it answers no census
	// row. Delivery and cardinality already derive the class, so this is what
	// the row is called and never how it is treated.
	Arm string
	// Pending names the identity a census row still owes when its family
	// cannot be bound yet. A pending family is declared, reported and not
	// emitted; it is never replaced by an invented token.
	Pending string
}

// Label is what a comment in the emitted artifact calls this family: the
// census row it answers, or the ABI class arm it proves when it answers none.
func (family Family) Label() string {
	if family.Census != "" {
		return family.Census
	}
	return family.Arm
}

// Publishes reports whether this family publishes a fact. A family that
// declares no output column answers with its disposition alone, which the ABI
// reads off the sealed contract rather than off a label.
func (family Family) Publishes() bool { return len(family.Outputs) != 0 }

// Emitted reports whether this family's artifacts are emitted. A family that
// owes an identity is carried by the corpus as a named row and nothing else.
func (family Family) Emitted() bool { return family.Pending == "" }

// Corpus is the whole binding declaration: the emission targets, the owner
// payloads the families share, and one row per family.
type Corpus struct {
	Axes     []Axis
	Payloads []Payload
	Families []Family
}

// Axis resolves one axis key.
func (corpus Corpus) Axis(key string) (Axis, bool) {
	for _, axis := range corpus.Axes {
		if axis.Key == key {
			return axis, true
		}
	}
	return Axis{}, false
}

// AxisPayloads returns the payloads one axis publishes, in declaration order.
func (corpus Corpus) AxisPayloads(key string) []Payload {
	owned := make([]Payload, 0, len(corpus.Payloads))
	for _, payload := range corpus.Payloads {
		if payload.Axis == key {
			owned = append(owned, payload)
		}
	}
	return owned
}

// Payload resolves one payload key.
func (corpus Corpus) Payload(key string) (Payload, bool) {
	for _, payload := range corpus.Payloads {
		if payload.Key == key {
			return payload, true
		}
	}
	return Payload{}, false
}

// Validate proves the declaration is emittable before a byte is written. Every
// refusal names the family and the declaration that is wrong, because a
// generator that guesses a missing declaration writes a lie into checked-in
// code.
func (corpus Corpus) Validate() error {
	axes := map[string]bool{}
	for _, axis := range corpus.Axes {
		if !axis.Available() {
			return fmt.Errorf("axis %q is not an emission target", axis.Key)
		}
		if axes[axis.Key] {
			return fmt.Errorf("axis %q is declared twice", axis.Key)
		}
		axes[axis.Key] = true
	}
	seen := map[string]bool{}
	fields := map[string]bool{}
	for _, payload := range corpus.Payloads {
		if !payload.Available() {
			return fmt.Errorf("payload %q is not a complete declaration", payload.Key)
		}
		if seen[payload.Key] {
			return fmt.Errorf("payload %q is declared twice", payload.Key)
		}
		if !axes[payload.Axis] {
			return fmt.Errorf("payload %q names undeclared axis %q", payload.Key, payload.Axis)
		}
		if fields[payload.Axis+"."+payload.Field] {
			return fmt.Errorf("payload field %q is declared twice on axis %q", payload.Field, payload.Axis)
		}
		seen[payload.Key] = true
		fields[payload.Axis+"."+payload.Field] = true
	}
	stems := map[string]bool{}
	for _, family := range corpus.Families {
		if family.Stem == "" {
			return fmt.Errorf("a family states no stem")
		}
		if family.Census == "" && family.Arm == "" {
			return fmt.Errorf("family %s answers no census row and names no class arm", family.Stem)
		}
		if !axes[family.Axis] {
			return fmt.Errorf("family %s names undeclared axis %q", family.Stem, family.Axis)
		}
		if stems[family.Stem] {
			return fmt.Errorf("family stem %q is declared twice", family.Stem)
		}
		stems[family.Stem] = true
		if !family.Emitted() {
			continue
		}
		if err := corpus.validateFamily(family); err != nil {
			return err
		}
	}
	return nil
}

func (corpus Corpus) validateFamily(family Family) error {
	if family.Judgment == "" {
		return fmt.Errorf("family %s names no owner judgment", family.Stem)
	}
	if len(family.Inputs) == 0 {
		return fmt.Errorf("family %s declares no input slot", family.Stem)
	}
	names := map[string]bool{}
	for index, slot := range family.Inputs {
		if !slot.Available() {
			return fmt.Errorf("family %s input %d is not a complete slot", family.Stem, index)
		}
		if names[slot.Field] {
			return fmt.Errorf("family %s declares input field %q twice", family.Stem, slot.Field)
		}
		names[slot.Field] = true
		if _, ok := corpus.Payload(slot.Payload); !ok {
			return fmt.Errorf("family %s input %d names undeclared payload %q", family.Stem, index, slot.Payload)
		}
	}
	if family.Publishes() {
		if _, ok := corpus.Payload(family.Result); !ok {
			return fmt.Errorf("family %s produces undeclared payload %q", family.Stem, family.Result)
		}
	} else if family.Result != "" || len(family.Outputs) != 0 {
		return fmt.Errorf("family %s publishes no fact and still names one", family.Stem)
	}
	for index, column := range family.Outputs {
		if !column.Available() {
			return fmt.Errorf("family %s output %d is not a complete column", family.Stem, index)
		}
		if _, ok := corpus.Payload(column.Payload); !ok {
			return fmt.Errorf("family %s output %d names undeclared payload %q", family.Stem, index, column.Payload)
		}
		if len(family.Outputs) > 1 && column.Field == "" {
			return fmt.Errorf("family %s output %d does not select a part of the result", family.Stem, index)
		}
	}
	return corpus.validateAddress(family)
}

// validateAddress restates the substrate's own admission rule at generation
// time: a bounded-many family names its rows, and a single-row family
// addresses them by a declared scalar slot.
func (corpus Corpus) validateAddress(family Family) error {
	cardinality, ok := model.NewCardinality(family.Cardinality, family.Bound)
	if !ok {
		return fmt.Errorf("family %s declares an unavailable cardinality", family.Stem)
	}
	single := cardinality.Kind() == model.ExactlyOne || cardinality.Kind() == model.Optional
	if bound, bounded := cardinality.Bound(); bounded && bound == 1 {
		single = true
	}
	if !family.Publishes() {
		if family.Address != NoDestination {
			return fmt.Errorf("family %s publishes no fact and still addresses a row", family.Stem)
		}
		return nil
	}
	if family.Address == KeyedDestination {
		if single {
			return fmt.Errorf("family %s names its rows but publishes at most one", family.Stem)
		}
		return nil
	}
	if !single {
		return fmt.Errorf("family %s addresses one slot but publishes many rows", family.Stem)
	}
	if family.Address < 0 || family.Address >= len(family.Inputs) {
		return fmt.Errorf("family %s addresses input %d, which it does not declare", family.Stem, family.Address)
	}
	if family.Inputs[family.Address].Delivery != signature.ScalarDelivery {
		return fmt.Errorf("family %s addresses input %d, which is delivered as a span", family.Stem, family.Address)
	}
	return nil
}

// imports collects the import lines one emitted file needs, ordered.
func imports(base []string, payloads []Payload, axis Axis) []string {
	set := map[string]bool{}
	for _, line := range base {
		set[line] = true
	}
	for _, payload := range payloads {
		if payload.Path == "" || payload.Path == axis.Path() {
			continue
		}
		set[fmt.Sprintf("%s %q", payload.Alias, payload.Path)] = true
	}
	lines := make([]string, 0, len(set))
	for line := range set {
		lines = append(lines, line)
	}
	sort.Slice(lines, func(left, right int) bool {
		return importPath(lines[left]) < importPath(lines[right])
	})
	return lines
}

func importPath(line string) string {
	if index := strings.IndexByte(line, '"'); index >= 0 {
		return line[index:]
	}
	return line
}

// lower renders one exported stem as the unexported spelling the emitted
// decoder and encoder types carry.
func lower(stem string) string {
	if stem == "" {
		return stem
	}
	return strings.ToLower(stem[:1]) + stem[1:]
}

// snake renders one stem as the file name its artifact is written under.
func snake(stem string) string {
	var rendered strings.Builder
	for index, letter := range stem {
		if letter >= 'A' && letter <= 'Z' {
			if index != 0 {
				rendered.WriteByte('_')
			}
			rendered.WriteRune(letter + ('a' - 'A'))
			continue
		}
		rendered.WriteRune(letter)
	}
	return rendered.String()
}

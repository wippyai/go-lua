// Package equation defines the Stage-2 parameterized transfer artifact.
//
// It is deliberately below transformer: a compiler supplies opaque, already
// sealed operand terms and contract content IDs.  The artifact contains no
// State value, callback, version, or alternate transfer implementation.
package equation

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

// ContentID identifies canonical semantic bytes.  It is intentionally a
// package-local wire type so this package can remain independent of the
// transformer implementation that owns OperatorContract.
type ContentID [sha256.Size]byte

func contentID(bytes []byte) ContentID { return sha256.Sum256(bytes) }
func (id ContentID) Valid() bool {
	return id != (ContentID{})
}

// CanonicalContent is the cache/collision boundary for equation artifacts.
type CanonicalContent interface {
	CanonicalBytes() []byte
	ContentID() ContentID
}

// BodyID is the full stable lexical-body identity.  It is never a dense
// relation variable, source path, or solve generation.
type BodyID [sha256.Size]byte

func (b BodyID) Valid() bool { return b != (BodyID{}) }

// Coordinate is one body-owned equation coordinate.
type Coordinate struct {
	Body BodyID
	Name string
}

func (c Coordinate) valid() bool { return c.Body.Valid() && c.Name != "" }
func (c Coordinate) less(other Coordinate) bool {
	if c.Body != other.Body {
		return string(c.Body[:]) < string(other.Body[:])
	}
	return c.Name < other.Name
}

// EntryParameter is the formal input to a body equation system.  Its name is
// semantic syntax, while its body prevents a caller entry from becoming a
// body-owned coordinate.
type EntryParameter struct {
	Body BodyID
	Name string
}

func (p EntryParameter) valid() bool { return p.Body.Valid() && p.Name != "" }

// Term is a sealed operand.  Encoding is supplied by the owner of the source
// relation syntax and must already be closed, except that an entry operand may
// name the enclosing formal parameter exactly.
type Term struct {
	Encoding []byte
	Entry    bool
}

func ClosedTerm(encoding []byte) Term { return Term{Encoding: append([]byte(nil), encoding...)} }
func EntryTerm(parameter EntryParameter) Term {
	return Term{Encoding: []byte(parameter.Name), Entry: true}
}
func (t Term) validFor(entry EntryParameter) bool {
	if len(t.Encoding) == 0 {
		return false
	}
	return !t.Entry || string(t.Encoding) == entry.Name
}

// Guard is canonical, body-owned guard syntax.  It is never a State predicate
// fallback; the bound kernel remains the sole evaluator of its semantics.
type Guard struct {
	Body     BodyID
	Encoding []byte
}

func (g Guard) valid() bool { return g.Body.Valid() && len(g.Encoding) != 0 }
func (g Guard) less(other Guard) bool {
	if g.Body != other.Body {
		return string(g.Body[:]) < string(other.Body[:])
	}
	return string(g.Encoding) < string(other.Encoding)
}

// Occurrence names the content-addressed OperatorContract instance bound by
// an equation. ContractID is the only contract identity retained here.
type Occurrence struct {
	Kind       string
	ContractID ContentID
}

func (o Occurrence) valid() bool { return o.Kind != "" && o.ContractID.Valid() }

// Operand binds one mandatory contract role to a closed term.
type Operand struct {
	Role string
	Term Term
}

func (o Operand) validFor(entry EntryParameter) bool { return o.Role != "" && o.Term.validFor(entry) }
func (o Operand) less(other Operand) bool            { return o.Role < other.Role }

// Equation is one guarded equation over a body coordinate.  Every equation
// explicitly retains its body's formal entry parameter; it has no concrete
// entry value and no implicit State operand.
type Equation struct {
	Target Coordinate
	Entry  EntryParameter
	Guards []Guard
	// Dependencies are readiness edges to already-complete equation targets.
	// They are not operand values: operands remain closed when a transaction
	// is invoked.  The Stage-3 evaluator uses these edges solely to reject
	// cyclic bodies and select its deterministic acyclic schedule.
	Dependencies []Coordinate
	Occurrence   Occurrence
	Operands     []Operand
	KernelID     string
}

func (e Equation) valid() error {
	if !e.Target.valid() || !e.Entry.valid() || e.Target.Body != e.Entry.Body || !e.Occurrence.valid() || e.KernelID == "" {
		return fmt.Errorf("equation: malformed equation identity")
	}
	for _, guard := range e.Guards {
		if !guard.valid() || guard.Body != e.Target.Body {
			return fmt.Errorf("equation: foreign or malformed guard")
		}
	}
	for _, dependency := range e.Dependencies {
		if !dependency.valid() || dependency.Body != e.Target.Body || dependency == e.Target {
			return fmt.Errorf("equation: foreign, malformed, or self dependency")
		}
	}
	for _, operand := range e.Operands {
		if !operand.validFor(e.Entry) {
			return fmt.Errorf("equation: malformed operand")
		}
	}
	return nil
}

func canonicalEquation(in Equation) (Equation, error) {
	if err := in.valid(); err != nil {
		return Equation{}, err
	}
	out := in
	out.Guards = append([]Guard(nil), in.Guards...)
	out.Dependencies = append([]Coordinate(nil), in.Dependencies...)
	out.Operands = append([]Operand(nil), in.Operands...)
	sort.Slice(out.Guards, func(i, j int) bool { return out.Guards[i].less(out.Guards[j]) })
	sort.Slice(out.Dependencies, func(i, j int) bool { return out.Dependencies[i].less(out.Dependencies[j]) })
	sort.Slice(out.Operands, func(i, j int) bool { return out.Operands[i].less(out.Operands[j]) })
	for i := 1; i < len(out.Guards); i++ {
		if !out.Guards[i-1].less(out.Guards[i]) {
			return Equation{}, fmt.Errorf("equation: duplicate guard")
		}
	}
	for i := 1; i < len(out.Dependencies); i++ {
		if !out.Dependencies[i-1].less(out.Dependencies[i]) {
			return Equation{}, fmt.Errorf("equation: duplicate dependency")
		}
	}
	for i := 1; i < len(out.Operands); i++ {
		if out.Operands[i-1].Role == out.Operands[i].Role {
			return Equation{}, fmt.Errorf("equation: duplicate operand role %q", out.Operands[i].Role)
		}
	}
	return out, nil
}

// Artifact is a complete, canonical parameterized equation program.
type Artifact struct{ Equations []Equation }

func (a Artifact) CanonicalBytes() []byte {
	equations := append([]Equation(nil), a.Equations...)
	for i := range equations {
		canonical, err := canonicalEquation(equations[i])
		if err != nil {
			return nil
		}
		equations[i] = canonical
	}
	sort.Slice(equations, func(i, j int) bool { return equations[i].Target.less(equations[j].Target) })
	for i := 1; i < len(equations); i++ {
		if !equations[i-1].Target.less(equations[i].Target) {
			return nil
		}
	}
	encoded := appendText(nil, "parameterized-equation-artifact/content-v1")
	encoded = appendU64(encoded, uint64(len(equations)))
	for _, equation := range equations {
		encoded = appendEquation(encoded, equation)
	}
	return encoded
}

func (a Artifact) ContentID() ContentID {
	encoded := a.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

func appendEquation(out []byte, equation Equation) []byte {
	out = append(out, equation.Target.Body[:]...)
	out = appendText(out, equation.Target.Name)
	out = append(out, equation.Entry.Body[:]...)
	out = appendText(out, equation.Entry.Name)
	out = appendText(out, equation.Occurrence.Kind)
	out = append(out, equation.Occurrence.ContractID[:]...)
	out = appendText(out, equation.KernelID)
	out = appendU64(out, uint64(len(equation.Guards)))
	for _, guard := range equation.Guards {
		out = append(out, guard.Body[:]...)
		out = appendBytes(out, guard.Encoding)
	}
	out = appendU64(out, uint64(len(equation.Dependencies)))
	for _, dependency := range equation.Dependencies {
		out = append(out, dependency.Body[:]...)
		out = appendText(out, dependency.Name)
	}
	out = appendU64(out, uint64(len(equation.Operands)))
	for _, operand := range equation.Operands {
		out = appendText(out, operand.Role)
		if operand.Term.Entry {
			out = append(out, 1)
		} else {
			out = append(out, 0)
		}
		out = appendBytes(out, operand.Term.Encoding)
	}
	return out
}

func appendU64(out []byte, value uint64) []byte {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	return append(out, raw[:]...)
}
func appendBytes(out, value []byte) []byte {
	out = appendU64(out, uint64(len(value)))
	return append(out, value...)
}
func appendText(out []byte, value string) []byte { return appendBytes(out, []byte(value)) }

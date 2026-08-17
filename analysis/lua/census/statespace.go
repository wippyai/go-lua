package census

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// CarrierState names one exact carrier state of one form. It is the coordinate
// the field-state grain is keyed at, stated as a value so a law can hand back
// the states it decided rather than a set of row keys a caller has to reparse.
type CarrierState struct {
	Form  string
	Field string
	State parsersource.FieldState
}

// Key is the field-state row this coordinate names.
func (c CarrierState) Key() string { return FieldStateRow(c.Form, c.Field, c.State) }

// StateSpaceReport is the complete account of the carrier-state denominator
// against the parser's own construction behaviour. Judged is the part of the
// denominator this law speaks about, Reachable the states some construction or
// mutation produces, and Impossible the rest: states a carrier's declared form
// admits and no parser action can put it in.
//
// Unjudged is the remainder of the denominator, and it is published rather than
// dropped. A form the parser assembles for itself carries whatever the scanner
// stamped on it, so its states are not decided by any action's assignment
// behaviour, and a law that silently counted them as impossible would be
// claiming a parser fact from the absence of one.
type StateSpaceReport struct {
	Judged     []CarrierState
	Reachable  []CarrierState
	Impossible []CarrierState
	Unjudged   []CarrierState
}

// StateSpace decides, for every carrier state the census denominator holds,
// whether the parser can produce it. The premises are census rows: the
// constructor declarations give each carrier its state space, and the product
// and mutation rows give the states each action leaves it in. Nothing here
// observes a parse, so a state is impossible because no action assigns it, not
// because no corpus happened to exercise it.
//
// The law speaks about the forms that cross a semantic boundary. A form the
// parser builds only as its own assembly - the lexical token - holds values the
// scanner produced, and the construction rows describe the few the parser
// synthesises rather than the many it receives, so those states are returned
// Unjudged.
func StateSpace(value Census) StateSpaceReport {
	reachable := make(map[CarrierState]bool, len(value.Products)*2)
	for _, product := range value.Products {
		for _, field := range product.Fields {
			for _, state := range field.States {
				reachable[CarrierState{Form: product.Constructor, Field: field.Field, State: state}] = true
			}
		}
	}
	for _, mutation := range value.Mutations {
		for _, state := range mutation.States {
			reachable[CarrierState{Form: mutation.Constructor, Field: mutation.Field, State: state}] = true
		}
	}
	report := StateSpaceReport{}
	for _, constructor := range value.Constructors {
		for _, field := range constructor.Fields {
			for _, state := range field.Form.States() {
				coordinate := CarrierState{Form: constructor.Name, Field: field.Name, State: state}
				if !constructor.Semantic {
					report.Unjudged = append(report.Unjudged, coordinate)
					continue
				}
				report.Judged = append(report.Judged, coordinate)
				if reachable[coordinate] {
					report.Reachable = append(report.Reachable, coordinate)
					continue
				}
				report.Impossible = append(report.Impossible, coordinate)
			}
		}
	}
	sortCarrierStates(report.Judged)
	sortCarrierStates(report.Reachable)
	sortCarrierStates(report.Impossible)
	sortCarrierStates(report.Unjudged)
	return report
}

// Validate makes the report usable as a gate without letting the inventory pass
// for the judgement: an empty denominator and an empty judged set are both
// failures, and a form whose every state is impossible means the census lost
// the constructions of that form rather than that the parser cannot build it.
func (r StateSpaceReport) Validate(value Census) error {
	if len(r.Judged) == 0 {
		return fmt.Errorf("parser census: empty carrier-state judgement")
	}
	if len(r.Reachable) == 0 {
		return fmt.Errorf("parser census: no carrier state is parser-reachable")
	}
	reachable := make(map[string]bool, len(r.Reachable))
	for _, coordinate := range r.Reachable {
		reachable[coordinate.Form] = true
	}
	for _, constructor := range value.Constructors {
		if !constructor.Semantic || len(constructor.Fields) == 0 {
			continue
		}
		if !reachable[constructor.Name] {
			return fmt.Errorf("parser census: form %s has no reachable carrier state", constructor.Name)
		}
	}
	return nil
}

func sortCarrierStates(rows []CarrierState) {
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Form != rows[right].Form {
			return rows[left].Form < rows[right].Form
		}
		if rows[left].Field != rows[right].Field {
			return rows[left].Field < rows[right].Field
		}
		return rows[left].State < rows[right].State
	})
}

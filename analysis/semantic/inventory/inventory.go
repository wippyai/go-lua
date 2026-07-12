// Package inventory parses and seals the declarative inventory of semantic
// state lanes, sparse value axes, and reduced-product rules.
//
// Inventory documents use the JSON subset of YAML 1.2. Keeping the accepted
// syntax deliberately narrow makes parsing hermetic and canonicalization
// independent of a third-party YAML implementation.
package inventory

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

const Schema = "go-lua.semantic.inventory/v1"

//go:embed builtins.yaml
var baseSource []byte

var ErrInvalid = errors.New("semantic inventory: invalid")

// StateLane declares one lane in the State product. Order is an explicit
// layout ordinal; source array order is never semantic.
type StateLane struct {
	ID    string `json:"id"`
	Order uint16 `json:"order"`
}

// ValueAxis declares one registered sparse product axis.
type ValueAxis struct {
	ID       string `json:"id"`
	Order    uint16 `json:"order"`
	Boundary string `json:"boundary"`
}

// Reducer declares the dependency surface of one reduced-product rule. It
// inventories ownership and dependencies only; executable behavior remains in
// the semantic implementation selected by the runtime universe.
type Reducer struct {
	ID        string   `json:"id"`
	OwnerAxis string   `json:"owner_axis"`
	Reads     []string `json:"reads"`
	Writes    []string `json:"writes"`
}

type document struct {
	Schema     string      `json:"schema"`
	StateLanes []StateLane `json:"state_lanes"`
	ValueAxes  []ValueAxis `json:"value_axes"`
	Reducers   []Reducer   `json:"reducers"`
}

// Inventory is a validated, normalized inventory. Its canonical bytes are
// stable across permutations of source arrays because every ordered family
// carries explicit ordinals and reducers/dependency sets are sorted by ID.
type Inventory struct {
	doc       document
	canonical []byte
	digest    [sha256.Size]byte
}

// Parse accepts a single JSON-syntax YAML 1.2 document, rejects unknown fields
// and trailing input, validates all references, and seals canonical bytes.
func Parse(source []byte) (Inventory, error) {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	var input document
	if err := decoder.Decode(&input); err != nil {
		return Inventory{}, invalid("decode JSON-syntax YAML", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple documents")
		}
		return Inventory{}, invalid("trailing input", err)
	}
	normalized, err := normalize(input)
	if err != nil {
		return Inventory{}, err
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return Inventory{}, invalid("canonical encoding", err)
	}
	return Inventory{
		doc:       normalized,
		canonical: canonical,
		digest:    sha256.Sum256(canonical),
	}, nil
}

// Base parses and validates the embedded built-in inventory. It intentionally
// does not mutate or derive the production registries.
func Base() (Inventory, error) {
	return Parse(baseSource)
}

// CanonicalBytes returns a detached canonical JSON encoding.
func (i Inventory) CanonicalBytes() []byte {
	return append([]byte(nil), i.canonical...)
}

// Digest returns the SHA-256 digest of CanonicalBytes.
func (i Inventory) Digest() [sha256.Size]byte { return i.digest }

// DigestString returns Digest as lowercase hexadecimal.
func (i Inventory) DigestString() string { return hex.EncodeToString(i.digest[:]) }

// StateLanes returns the canonical explicit-layout order.
func (i Inventory) StateLanes() []StateLane {
	return append([]StateLane(nil), i.doc.StateLanes...)
}

// ValueAxes returns the canonical explicit-layout order.
func (i Inventory) ValueAxes() []ValueAxis {
	return append([]ValueAxis(nil), i.doc.ValueAxes...)
}

// Reducers returns reducers in stable-ID order with detached dependencies.
func (i Inventory) Reducers() []Reducer {
	out := make([]Reducer, len(i.doc.Reducers))
	for index, reducer := range i.doc.Reducers {
		out[index] = reducer
		out[index].Reads = append([]string(nil), reducer.Reads...)
		out[index].Writes = append([]string(nil), reducer.Writes...)
	}
	return out
}

func normalize(input document) (document, error) {
	if input.Schema != Schema {
		return document{}, invalid("schema", fmt.Errorf("got %q, want %q", input.Schema, Schema))
	}
	if len(input.StateLanes) == 0 {
		return document{}, invalid("state_lanes", errors.New("must not be empty"))
	}
	if len(input.ValueAxes) == 0 {
		return document{}, invalid("value_axes", errors.New("must not be empty"))
	}
	out := document{Schema: input.Schema}
	out.StateLanes = append([]StateLane(nil), input.StateLanes...)
	if err := normalizeOrdered("state lane", out.StateLanes,
		func(lane StateLane) string { return lane.ID },
		func(lane StateLane) uint16 { return lane.Order }); err != nil {
		return document{}, err
	}
	out.ValueAxes = append([]ValueAxis(nil), input.ValueAxes...)
	if err := normalizeOrdered("value axis", out.ValueAxes,
		func(value ValueAxis) string { return value.ID },
		func(value ValueAxis) uint16 { return value.Order }); err != nil {
		return document{}, err
	}
	axisIDs := make(map[string]struct{}, len(out.ValueAxes))
	for _, valueAxis := range out.ValueAxes {
		switch valueAxis.Boundary {
		case "local-only", "portable-identity", "projected":
		default:
			return document{}, invalid("value axis "+valueAxis.ID, fmt.Errorf("unknown boundary policy %q", valueAxis.Boundary))
		}
		axisIDs[valueAxis.ID] = struct{}{}
	}
	out.Reducers = make([]Reducer, len(input.Reducers))
	for index, reducer := range input.Reducers {
		out.Reducers[index] = reducer
		out.Reducers[index].Reads = append([]string(nil), reducer.Reads...)
		out.Reducers[index].Writes = append([]string(nil), reducer.Writes...)
	}
	sort.Slice(out.Reducers, func(left, right int) bool { return out.Reducers[left].ID < out.Reducers[right].ID })
	seenReducers := make(map[string]struct{}, len(out.Reducers))
	for index := range out.Reducers {
		reducer := &out.Reducers[index]
		if err := validateID("reducer", reducer.ID); err != nil {
			return document{}, err
		}
		if _, duplicate := seenReducers[reducer.ID]; duplicate {
			return document{}, invalid("reducer", fmt.Errorf("duplicate id %q", reducer.ID))
		}
		seenReducers[reducer.ID] = struct{}{}
		if _, ok := axisIDs[reducer.OwnerAxis]; !ok {
			return document{}, invalid("reducer "+reducer.ID, fmt.Errorf("dangling owner axis %q", reducer.OwnerAxis))
		}
		if len(reducer.Reads) == 0 || len(reducer.Writes) == 0 {
			return document{}, invalid("reducer "+reducer.ID, errors.New("reads and writes must not be empty"))
		}
		if err := normalizeAxisReferences(reducer.ID, "read", reducer.Reads, axisIDs); err != nil {
			return document{}, err
		}
		if err := normalizeAxisReferences(reducer.ID, "write", reducer.Writes, axisIDs); err != nil {
			return document{}, err
		}
	}
	return out, nil
}

func normalizeOrdered[T any](kind string, values []T, id func(T) string, order func(T) uint16) error {
	seenIDs := make(map[string]struct{}, len(values))
	seenOrders := make(map[uint16]string, len(values))
	for _, value := range values {
		stableID := id(value)
		if err := validateID(kind, stableID); err != nil {
			return err
		}
		if _, duplicate := seenIDs[stableID]; duplicate {
			return invalid(kind, fmt.Errorf("duplicate id %q", stableID))
		}
		seenIDs[stableID] = struct{}{}
		ordinal := order(value)
		if previous, duplicate := seenOrders[ordinal]; duplicate {
			return invalid(kind, fmt.Errorf("duplicate order %d for %q and %q", ordinal, previous, stableID))
		}
		seenOrders[ordinal] = stableID
	}
	sort.Slice(values, func(left, right int) bool { return order(values[left]) < order(values[right]) })
	for index, value := range values {
		if order(value) != uint16(index) {
			return invalid(kind, fmt.Errorf("orders must be contiguous from zero; %q has %d, want %d", id(value), order(value), index))
		}
	}
	return nil
}

func normalizeAxisReferences(reducerID, role string, references []string, axes map[string]struct{}) error {
	sort.Strings(references)
	for index, reference := range references {
		if _, ok := axes[reference]; !ok {
			return invalid("reducer "+reducerID, fmt.Errorf("dangling %s axis %q", role, reference))
		}
		if index > 0 && references[index-1] == reference {
			return invalid("reducer "+reducerID, fmt.Errorf("duplicate %s axis %q", role, reference))
		}
	}
	return nil
}

func validateID(kind, id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return invalid(kind, fmt.Errorf("invalid id %q", id))
	}
	separator := false
	for index := 0; index < len(id); index++ {
		character := id[index]
		if character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' {
			separator = false
			continue
		}
		if index > 0 && index < len(id)-1 && !separator && (character == '-' || character == '.') {
			separator = true
			continue
		}
		return invalid(kind, fmt.Errorf("invalid id %q", id))
	}
	return nil
}

func invalid(field string, cause error) error {
	return fmt.Errorf("%w: %s: %v", ErrInvalid, field, cause)
}

// Equal reports canonical inventory equality.
func (i Inventory) Equal(other Inventory) bool {
	return slices.Equal(i.canonical, other.canonical)
}

package compiler

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// freezeSubedges retains only the authored neutral declaration. Values and
// exact keys still cross Target's domain adapter here because their portable
// handles are needed by the operation query boundary. Structural construction,
// canonical ordering, route resolution, and every subedge law belong to
// operation.Core.
func (d *operationDraft) freezeSubedges(input []vocabulary.SubedgeSpec) ([]vocabulary.SubedgeSpec, error) {
	if _, err := vocabulary.CheckedStoredLength("subedge table", len(input)); err != nil {
		return nil, err
	}
	out := make([]vocabulary.SubedgeSpec, len(input))
	d.subedgeReadRoots = make([]vocabulary.InitialRoot, len(input))
	for index, item := range input {
		out[index] = cloneSubedgeSpec(item)
		if err := d.freezeSubedgeValues(out[index]); err != nil {
			return nil, fmt.Errorf("target: subedge %d: %w", index, err)
		}
		// Exact-key normalization is the one portable authoring operation that
		// must happen before the contract-wide exact-key table is compiled.
		// The operation owner receives only its resulting ExactKey handle.
		switch out[index].Callee.Kind {
		case vocabulary.SubedgeCalleeCapturedInitialRead:
			key, err := normalizeRequiredExactKey(out[index].Callee.Read.Key)
			if err != nil {
				return nil, fmt.Errorf("target: subedge %d captured initial read key: %w", index, err)
			}
			out[index].Callee.Read.Key = key
		case vocabulary.SubedgeCalleeMetaKey:
			key, err := normalizeRequiredExactKey(out[index].Callee.MetaKey)
			if err != nil {
				return nil, fmt.Errorf("target: subedge %d meta key: %w", index, err)
			}
			out[index].Callee.MetaKey = key
		}
	}
	return out, nil
}

// freezeSubedgeValues is intentionally limited to Target's typed Values
// adapter. It does not decide whether an endpoint is legal for a route or
// callee; operation.Core makes those decisions after Values handles exist.
func (d *operationDraft) freezeSubedgeValues(edge vocabulary.SubedgeSpec) error {
	freeze := func(values vocabulary.ValuesSpec, label string) error {
		if _, err := d.freezeValues(values, false); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}
	if edge.Callee.Kind != vocabulary.SubedgeCalleeCallback {
		if err := freeze(edge.Arguments, "arguments"); err != nil {
			return err
		}
	}
	if _, err := vocabulary.CheckedStoredLength("subedge terminal table", len(edge.Outcomes)); err != nil {
		return err
	}
	for index, terminal := range edge.Outcomes {
		if edge.Callee.Kind == vocabulary.SubedgeCalleeCallback {
			return errors.New("callback subedge carries authored terminals")
		}
		if err := freeze(terminal.Values, fmt.Sprintf("terminal %d", index)); err != nil {
			return err
		}
	}
	if err := freeze(edge.AdmissionFailure.Values, "admission failure Values"); err != nil {
		return err
	}
	if err := freeze(edge.AdmissionFailure.Route.Result, "admission failure Result"); err != nil {
		return err
	}
	if _, err := vocabulary.CheckedStoredLength("subedge route table", len(edge.Routes)); err != nil {
		return err
	}
	for index, route := range edge.Routes {
		if err := freeze(route.Result, fmt.Sprintf("route %d result", index)); err != nil {
			return err
		}
	}
	return nil
}

func cloneSubedgeSpec(input vocabulary.SubedgeSpec) vocabulary.SubedgeSpec {
	out := input
	out.ArgumentOrigins = append([]vocabulary.ArgumentOrigin(nil), input.ArgumentOrigins...)
	out.Outcomes = append([]vocabulary.TerminalSpec(nil), input.Outcomes...)
	out.Routes = append([]vocabulary.SubedgeRouteSpec(nil), input.Routes...)
	return out
}

func zeroLiteral(value keyspace.LiteralValue) bool { return value == (keyspace.LiteralValue{}) }

func normalizeRequiredExactKey(value keyspace.LiteralValue) (keyspace.LiteralValue, error) {
	normalized, ok := scalar.Normalize(value)
	if !ok {
		return keyspace.LiteralValue{}, errors.New("not an exact Lua key")
	}
	return normalized, nil
}

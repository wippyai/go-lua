package generator

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/domain/memberroster"
)

func rosterSource(t *testing.T, name string) definition.Source {
	t.Helper()
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		t.Fatal("member definition roster is not admissible")
	}
	source, sourceOK := roster.Source(name)
	if !sourceOK {
		t.Fatalf("member definition source %q is not registered", name)
	}
	return source
}

func renderSource(t *testing.T, source definition.Source) Artifact {
	t.Helper()
	composed, composedOK := source.Compose()
	if !composedOK {
		t.Fatalf("member definition source %q does not compose", source.Name)
	}
	artifact, err := Render(source.Package, composed)
	if err != nil {
		t.Fatalf("render %q: %v", source.Name, err)
	}
	return artifact
}

func withoutContribution(source definition.Source, position int) definition.Source {
	reduced := source
	reduced.Contributions = nil
	for index, contribution := range source.Contributions {
		if index == position {
			continue
		}
		reduced.Contributions = append(reduced.Contributions, contribution)
	}
	return reduced
}

// sourceDeclaresRelation and sourceDeclaresProjection distinguish a stale row
// from a row another retained rule independently declares. Source.Compose
// permits only exact repeated declarations, so a shared row remains owned by
// each contributor that states it; removing one such contributor must not
// make the other contributor's row disappear.
func sourceDeclaresRelation(source definition.Source, key schema.Key) bool {
	for _, relation := range source.Base.Relations {
		if relation.Key == key {
			return true
		}
	}
	for _, contribution := range source.Contributions {
		for _, relation := range contribution.Relations {
			if relation.Key == key {
				return true
			}
		}
	}
	return false
}

func sourceDeclaresProjection(source definition.Source, key schema.Key) bool {
	for _, projection := range source.Base.Projections {
		if projection.Key == key {
			return true
		}
	}
	for _, contribution := range source.Contributions {
		for _, projection := range contribution.Projections {
			if projection.Key == key {
				return true
			}
		}
	}
	return false
}

// reducerBlocks splits the rendered reducer vocabulary into one text block per
// reducer, keyed by the constant name the row is emitted under. Comparing
// blocks rather than whole files keeps the law about the rows a rule owns: the
// surrounding constant block is column-aligned by gofmt, so a name changing
// length moves other lines without changing any row.
func reducerBlocks(cold []byte) map[string]string {
	const opening = "[]member.Reducer{"
	text := string(cold)
	start := strings.Index(text, opening)
	if start < 0 {
		return map[string]string{}
	}
	blocks := make(map[string]string, 4)
	name := ""
	var current []string
	flush := func() {
		if name != "" {
			blocks[name] = strings.Join(current, "\n")
		}
		name, current = "", nil
	}
	for _, line := range strings.Split(text[start+len(opening):], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{Key: ") {
			flush()
			rest := strings.TrimPrefix(trimmed, "{Key: ")
			if comma := strings.Index(rest, ","); comma >= 0 {
				name = rest[:comma]
			}
		}
		if strings.HasPrefix(line, "\t\t}") {
			break
		}
		if name == "" {
			continue
		}
		current = append(current, trimmed)
	}
	flush()
	return blocks
}

// TestGeneratedOutputDiffIsConfinedToTheDroppedRulesRows is the containment
// half of the composition law: removing one rule's contribution from the
// roster removes that rule's rows from the generated catalog and touches no
// other row. A rule whose arrival or departure moved another rule's row would
// mean the axis numbered its vocabulary against a hand-kept list rather than
// against the composition.
func TestGeneratedOutputDiffIsConfinedToTheDroppedRulesRows(t *testing.T) {
	source := rosterSource(t, "value")
	if len(source.Contributions) < 2 {
		t.Fatalf("value contributes %d reducers, want at least two for a containment law", len(source.Contributions))
	}
	full := renderSource(t, source)
	fullBlocks := reducerBlocks(full.Cold)
	for position, contribution := range source.Contributions {
		reduced := renderSource(t, withoutContribution(source, position))
		reducedBlocks := reducerBlocks(reduced.Cold)
		dropped := make(map[string]struct{}, len(contribution.Reducers))
		for _, reducer := range contribution.Reducers {
			dropped[reducer.Name] = struct{}{}
			if _, survived := reducedBlocks[reducer.Name]; survived {
				t.Fatalf("dropping %q left its reducer row %q behind", contribution.Rule, reducer.Name)
			}
		}
		if len(reducedBlocks)+len(dropped) != len(fullBlocks) {
			t.Fatalf("dropping %q changed the row count by more than its own rows: %d + %d != %d",
				contribution.Rule, len(reducedBlocks), len(dropped), len(fullBlocks))
		}
		for name, block := range fullBlocks {
			if _, isDropped := dropped[name]; isDropped {
				continue
			}
			if reducedBlocks[name] != block {
				t.Fatalf("dropping %q rewrote the unrelated row %q:\nwant %s\ngot  %s", contribution.Rule, name, block, reducedBlocks[name])
			}
		}
		for _, relation := range contribution.Relations {
			if sourceDeclaresRelation(withoutContribution(source, position), relation.Key) {
				continue
			}
			if strings.Contains(string(reduced.Cold), string(relation.Key)) || strings.Contains(string(reduced.Relations), string(relation.Key)) {
				t.Fatalf("dropping %q left its relation row %q behind", contribution.Rule, relation.Key)
			}
		}
		for _, projection := range contribution.Projections {
			if sourceDeclaresProjection(withoutContribution(source, position), projection.Key) {
				continue
			}
			if strings.Contains(string(reduced.Cold), string(projection.Key)) || strings.Contains(string(reduced.Relations), string(projection.Key)) {
				t.Fatalf("dropping %q left its projection row %q behind", contribution.Rule, projection.Key)
			}
		}
	}
}

// TestAxisWithNoContributionsRendersNoReducerRows is the empty case at the
// generated-output level: an axis whose rules declare no fold emits an empty
// reducer vocabulary rather than a stale one.
func TestAxisWithNoContributionsRendersNoReducerRows(t *testing.T) {
	source := rosterSource(t, "value")
	declared := source.Contributions
	source.Contributions = nil
	cold := string(renderSource(t, source).Cold)
	if !strings.Contains(cold, "[]member.Reducer{}") {
		t.Fatalf("an axis with no contributions did not render an empty reducer vocabulary:\n%s", cold)
	}
	for _, contribution := range declared {
		for _, reducer := range contribution.Reducers {
			if strings.Contains(cold, string(reducer.Key)) {
				t.Fatalf("reducer %q survived the removal of its contribution", reducer.Key)
			}
		}
	}
}

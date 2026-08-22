package diagnostic

import "testing"

// TestSubjectTemplateTokenGrammar states the closed grammar a finding may name
// its subject by. The accepted set is one identifier followed by authored
// access steps; the refused set is everything a report would otherwise
// interpolate as prose. The earlier declaration-name grammar is the dotted
// subset, so its members are listed here as accepted rather than restated in a
// second law.
func TestSubjectTemplateTokenGrammar(t *testing.T) {
	accepted := []string{
		"x",
		"_private",
		"LocalPoint",
		"missing_count",
		"module.Type",
		"c.hook",
		"result.value.id",
		"opened.id",
		"spec.tools[1].id",
		"tags[0]",
		"store.state.flags[\"did_index\"]",
		"a[\"meow\"]",
		"email_one.meta.tags[\"source\"]",
		"encode(...)",
		"component.singleton_component_id(...)",
		"protocol.meta(...).tags[\"source\"]",
		"store:lookup_policy(...).tags[\"source\"]",
		"items[-1]",
		"escaped[\"a\\\"b\"]",
	}
	for _, value := range accepted {
		if !diagnosticTemplateTokenValid(value) {
			t.Errorf("subject token %q is refused by the access-path grammar", value)
		}
	}

	refused := []string{
		"",
		".",
		".leading",
		"trailing.",
		"has space",
		"1leading",
		"a..b",
		"a[",
		"a]",
		"a[]",
		"a[1",
		"a[\"open]",
		"a[\"a\nb\"]",
		"a[1x]",
		"a[-]",
		"call()",
		"call(x)",
		"call(...",
		"store:lookup_policy",
		"store:(...)",
		"cannot assign x because it may be nil",
		"a.b, c.d",
		"a`b",
		"a[\"a\\nb\"]",
	}
	for _, value := range refused {
		if diagnosticTemplateTokenValid(value) {
			t.Errorf("subject token %q is admitted by the access-path grammar", value)
		}
	}
}

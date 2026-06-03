package lua

import "testing"

func TestCanonicalFamilyScopeDoesNotLeakAcrossFixtureChecks(t *testing.T) {
	contaminator := suiteByNameForScopeRegression(t, "narrowing/union-method-after-narrowing")
	target := suiteByNameForScopeRegression(t, "realworld/index-presence-laws")

	fixtureDiagnostics(contaminator)
	diags, entry := fixtureDiagnostics(target)
	verdict := judgeAgainstCuratedExpectations(target, diags, entry)
	if !verdict.passed {
		t.Fatalf("canonical family scope leaked across checks: missing=%v unexpected=%v", verdict.missing, verdict.unexpected)
	}
}

func suiteByNameForScopeRegression(t *testing.T, name string) namedSuite {
	t.Helper()
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discovering fixtures: %v", err)
	}
	for _, s := range suites {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("suite %s not found", name)
	return namedSuite{}
}

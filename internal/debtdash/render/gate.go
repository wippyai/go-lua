package render

import (
	"fmt"
	"strings"
)

// GateStatus is the outcome of one gate criterion.
type GateStatus string

const (
	GatePass GateStatus = "PASS"
	GateFail GateStatus = "FAIL"
)

// GateCriterion is one row of the debt gate (journal seq 6656).
type GateCriterion struct {
	Name   string
	Status GateStatus
	Detail string
}

// GateReport is the full gate verdict: every criterion plus the overall
// status, which is FAIL if any criterion fails.
type GateReport struct {
	Criteria []GateCriterion
	Overall  GateStatus
}

// Debt-gate ceilings from journal seq 6656, pinned at commit A
// (7a7f57cb88): the cutover is not complete until every authored-debt
// metric is strictly below its commit-A value.
const (
	domainNonTestCeiling  = 110827
	domainTestCeiling     = 79060
	engineExportedCeiling = 2041
	schemaExportedCeiling = 2413
)

// Gate evaluates the debt gate thresholds against r and returns a PASS/FAIL
// verdict per criterion plus the overall result.
func Gate(r Report) GateReport {
	criteria := []GateCriterion{
		lessThan("domain non-test LOC", r.DomainTotal.NonTest, domainNonTestCeiling),
		lessThan("domain test LOC", r.DomainTotal.Test, domainTestCeiling),
		lessThan("exported symbols analysis/engine", r.ExportedEngine, engineExportedCeiling),
		lessThan("exported symbols analysis/schema", r.ExportedSchema, schemaExportedCeiling),
		equalsZero("legacy residue files (domain)", r.ResidueFiles),
		equalsZero("hot_rule.go count (domain)", r.HotRuleFiles),
		equalsZero("registration.go count (domain)", r.RegistrationFiles),
		equalsZero("schema_fragment.go count (domain)", r.SchemaFragmentFiles),
		equalsZero("scheduled-death ledger rows", r.ScheduledDeathRows),
	}

	overall := GatePass
	for _, c := range criteria {
		if c.Status == GateFail {
			overall = GateFail
			break
		}
	}
	return GateReport{Criteria: criteria, Overall: overall}
}

func lessThan(name string, value, ceiling int) GateCriterion {
	status := GatePass
	if value >= ceiling {
		status = GateFail
	}
	return GateCriterion{Name: name, Status: status, Detail: fmt.Sprintf("%d < %d", value, ceiling)}
}

func equalsZero(name string, value int) GateCriterion {
	status := GatePass
	if value != 0 {
		status = GateFail
	}
	return GateCriterion{Name: name, Status: status, Detail: fmt.Sprintf("%d == 0", value)}
}

// FormatGate renders a GateReport as a plain-text table followed by the
// overall "GATE: PASS"/"GATE: FAIL" line.
func FormatGate(g GateReport) string {
	nameWidth := len("CRITERION")
	statusWidth := len("STATUS")
	for _, c := range g.Criteria {
		if len(c.Name) > nameWidth {
			nameWidth = len(c.Name)
		}
		if len(c.Status) > statusWidth {
			statusWidth = len(c.Status)
		}
	}

	var b strings.Builder
	writeRow := func(name, status, detail string) {
		fmt.Fprintf(&b, "%-*s  %-*s  %s\n", nameWidth, name, statusWidth, status, detail)
	}
	writeRow("CRITERION", "STATUS", "DETAIL")
	writeRow(strings.Repeat("-", nameWidth), strings.Repeat("-", statusWidth), strings.Repeat("-", len("DETAIL")))
	for _, c := range g.Criteria {
		writeRow(c.Name, string(c.Status), c.Detail)
	}
	fmt.Fprintf(&b, "GATE: %s\n", g.Overall)
	return b.String()
}

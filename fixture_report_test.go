package lua

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	fixtureImpactReportEnv = "FIXTURE_IMPACT_REPORT"
	fixtureReportPathEnv   = "FIXTURE_REPORT_PATH"
)

type fixtureImpactRecorder struct {
	mu     sync.Mutex
	suites map[string]*fixtureImpactSuite
}

type fixtureImpactReport struct {
	Schema          string               `json:"schema"`
	GeneratedAt     string               `json:"generated_at"`
	DeadlineSeconds int64                `json:"deadline_seconds"`
	TotalFixtures   int                  `json:"total_fixtures"`
	PassedFixtures  int                  `json:"passed_fixtures"`
	FailedFixtures  int                  `json:"failed_fixtures"`
	SkippedFixtures int                  `json:"skipped_fixtures"`
	PassedSteps     int                  `json:"passed_steps"`
	FailedSteps     int                  `json:"failed_steps"`
	SkippedSteps    int                  `json:"skipped_steps"`
	Fixtures        []fixtureImpactSuite `json:"fixtures"`
}

type fixtureImpactSuite struct {
	Name       string              `json:"name"`
	Status     string              `json:"status"`
	DurationMS int64               `json:"duration_ms"`
	Message    string              `json:"message,omitempty"`
	Steps      []fixtureImpactStep `json:"steps"`
}

type fixtureImpactStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Message    string `json:"message,omitempty"`
}

func newFixtureImpactRecorder() *fixtureImpactRecorder {
	return &fixtureImpactRecorder{suites: make(map[string]*fixtureImpactSuite)}
}

func (r *fixtureImpactRecorder) recordSuite(name, status string, duration time.Duration, message string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	suite := r.suiteLocked(name)
	suite.Status = status
	suite.DurationMS = duration.Milliseconds()
	if status == "skipped" {
		suite.Message = message
	}
}

func (r *fixtureImpactRecorder) recordStep(suiteName, stepName, status string, duration time.Duration, message string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	suite := r.suiteLocked(suiteName)
	step := fixtureImpactStep{
		Name:       stepName,
		Status:     status,
		DurationMS: duration.Milliseconds(),
	}
	if status == "skipped" {
		step.Message = message
	}
	suite.Steps = append(suite.Steps, step)
}

func (r *fixtureImpactRecorder) suiteLocked(name string) *fixtureImpactSuite {
	suite, ok := r.suites[name]
	if !ok {
		suite = &fixtureImpactSuite{Name: name, Status: "passed"}
		r.suites[name] = suite
	}
	return suite
}

func (r *fixtureImpactRecorder) finish(t *testing.T) {
	t.Helper()
	report := r.snapshot()
	t.Logf("FIXTURE IMPACT: %d/%d fixtures passed, %d failed, %d skipped; steps %d passed, %d failed, %d skipped",
		report.PassedFixtures, report.TotalFixtures, report.FailedFixtures, report.SkippedFixtures,
		report.PassedSteps, report.FailedSteps, report.SkippedSteps)

	path := fixtureReportPath()
	if path == "" {
		return
	}
	if err := writeFixtureImpactReport(path, report); err != nil {
		t.Errorf("writing fixture impact report: %v", err)
	}
}

func (r *fixtureImpactRecorder) snapshot() fixtureImpactReport {
	if r == nil {
		return fixtureImpactReport{Schema: "go-lua.fixture-impact.v1"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	names := make([]string, 0, len(r.suites))
	for name := range r.suites {
		names = append(names, name)
	}
	sort.Strings(names)

	report := fixtureImpactReport{
		Schema:          "go-lua.fixture-impact.v1",
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		DeadlineSeconds: int64(fixtureDeadline().Seconds()),
		Fixtures:        make([]fixtureImpactSuite, 0, len(names)),
	}
	for _, name := range names {
		suite := *r.suites[name]
		suite.Steps = append([]fixtureImpactStep(nil), suite.Steps...)
		report.Fixtures = append(report.Fixtures, suite)
		report.TotalFixtures++
		switch suite.Status {
		case "failed":
			report.FailedFixtures++
		case "skipped":
			report.SkippedFixtures++
		default:
			report.PassedFixtures++
		}
		for _, step := range suite.Steps {
			switch step.Status {
			case "failed":
				report.FailedSteps++
			case "skipped":
				report.SkippedSteps++
			default:
				report.PassedSteps++
			}
		}
	}
	return report
}

func fixtureTestStatus(t *testing.T) string {
	switch {
	case t.Skipped():
		return "skipped"
	case t.Failed():
		return "failed"
	default:
		return "passed"
	}
}

func fixtureReportPath() string {
	if path := os.Getenv(fixtureImpactReportEnv); path != "" {
		return path
	}
	return os.Getenv(fixtureReportPathEnv)
}

func writeFixtureImpactReport(path string, report fixtureImpactReport) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

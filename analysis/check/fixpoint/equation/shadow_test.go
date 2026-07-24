package equation

import "fmt"

// ShadowCase has a production publication and its independently bound
// equation artifact. Shadow mode is test-only: it never routes production
// results to callers.
type ShadowCase struct {
	Name       string
	Artifact   Artifact
	Entry      EntryBinding
	Production func() (OutputClosure, error)
}

// RunShadow requires exact published equality for every supplied acyclic
// case, including values, outcomes, diagnostic candidates, and rekeys.
func RunShadow(vm *AcyclicVM, cases []ShadowCase) (ShadowReport, error) {
	report := ShadowReport{Cases: len(cases)}
	for _, shadow := range cases {
		if shadow.Name == "" || shadow.Production == nil {
			return report, fmt.Errorf("equation: malformed shadow case")
		}
		production, err := shadow.Production()
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s production: %w", shadow.Name, err)
		}
		production, err = joinClosure(production)
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s production output: %w", shadow.Name, err)
		}
		bound, err := BindEntry(shadow.Artifact, shadow.Entry)
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s binding: %w", shadow.Name, err)
		}
		evaluation, err := vm.Evaluate(bound)
		if err != nil {
			return report, fmt.Errorf("equation: shadow %s bound evaluation: %w", shadow.Name, err)
		}
		if !production.Equal(evaluation.Closure) {
			return report, fmt.Errorf("equation: shadow %s published output differs", shadow.Name)
		}
		report.Passed++
	}
	return report, nil
}

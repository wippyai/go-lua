package workbench

import "fmt"

type configError struct{ detail string }

func (err configError) Error() string { return "flashrefactor workbench configuration: " + err.detail }

func errConfig(format string, args ...any) error {
	return configError{detail: fmt.Sprintf(format, args...)}
}

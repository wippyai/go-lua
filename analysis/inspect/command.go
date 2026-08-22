package inspect

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
)

// Command renders one inspector verb over this session. Printed facts name
// their source accessors. Unknown verbs are refused.
func (session *Session) Command(name string, args ...string) (string, error) {
	if session == nil || !session.Available() {
		return "", fmt.Errorf("inspect: unavailable session")
	}
	switch name {
	case "target":
		if len(args) != 0 {
			return "", fmt.Errorf("inspect: target takes no arguments")
		}
		return formatTarget(session), nil
	case "rows":
		if len(args) != 0 {
			return "", fmt.Errorf("inspect: rows takes no arguments")
		}
		return formatRows(session), nil
	case "row":
		if len(args) != 1 {
			return "", fmt.Errorf("inspect: row requires one identity")
		}
		id, ok := ParseContentID(args[0])
		if !ok {
			return "", fmt.Errorf("inspect: unavailable identity")
		}
		return formatRow(session, id), nil
	case "why":
		if len(args) > 1 {
			return "", fmt.Errorf("inspect: why takes at most one identity")
		}
		if len(args) == 0 {
			return formatWhy(session, identity.ContentID{}, false), nil
		}
		id, ok := ParseContentID(args[0])
		if !ok {
			return "", fmt.Errorf("inspect: unavailable identity")
		}
		return formatWhy(session, id, true), nil
	case "publish":
		if len(args) != 0 {
			return "", fmt.Errorf("inspect: publish takes no arguments")
		}
		return formatPublish(session), nil
	case "diff":
		if len(args) != 1 {
			return "", fmt.Errorf("inspect: diff requires one fixture")
		}
		if session.repository == "" {
			return "", fmt.Errorf("inspect: unavailable repository")
		}
		other, err := Open(session.repository, args[0])
		if err != nil {
			return "", err
		}
		defer other.Close()
		return formatDiff(session, other), nil
	default:
		return "", fmt.Errorf("inspect: unknown command %q", name)
	}
}

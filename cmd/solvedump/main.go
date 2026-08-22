package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/wippyai/go-lua/analysis/inspect"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: solvedump <fixture> [command [args...]]\n")
		os.Exit(2)
	}
	root, err := testfixture.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	session, err := inspect.Open(root, os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer session.Close()
	if len(os.Args) == 2 {
		if err := repl(session); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	output, err := session.Command(os.Args[2], os.Args[3:]...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if output != "" {
		fmt.Println(output)
	}
}

func repl(session *inspect.Session) error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		output, err := session.Command(fields[0], fields[1:]...)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if output != "" {
			fmt.Println(output)
		}
	}
	return scanner.Err()
}

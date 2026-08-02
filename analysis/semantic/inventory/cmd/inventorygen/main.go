package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/semantic/inventory"
	"github.com/wippyai/go-lua/analysis/semantic/inventory/internal/gen"
)

func main() {
	inventoryPath := flag.String("inventory", "", "reviewed inventory input")
	bindingsPath := flag.String("bindings", "", "owner-local generator bindings")
	productPath := flag.String("product", "", "generated analyzer value-registry output")
	check := flag.Bool("check", false, "verify outputs without writing")
	flag.Parse()
	if *inventoryPath == "" || *bindingsPath == "" || *productPath == "" {
		fail(fmt.Errorf("inventory, bindings, and product paths are required"))
	}
	source, err := os.ReadFile(*inventoryPath)
	if err != nil {
		fail(err)
	}
	in, err := inventory.Parse(source)
	if err != nil {
		fail(err)
	}
	bindingFile, err := os.Open(*bindingsPath)
	if err != nil {
		fail(err)
	}
	bindings, err := gen.DecodeBindings(bindingFile)
	if closeErr := bindingFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fail(err)
	}
	productSource, err := gen.RenderProduct(in, bindings)
	if err != nil {
		fail(err)
	}
	if err := publish(*productPath, productSource, *check); err != nil {
		fail(err)
	}
}

func publish(path string, content []byte, check bool) error {
	if check {
		existing, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(existing, content) {
			return fmt.Errorf("generated output %s is stale; run go generate ./analysis/semantic/inventory", path)
		}
		return nil
	}
	return os.WriteFile(path, content, 0o644)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "inventorygen:", err)
	os.Exit(1)
}

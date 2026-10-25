// Command stresscheck renders and checks Go's reported test execution.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	showJSON := flag.Bool("json", false, "retain JSON test output")
	flag.Parse()
	if err := check(os.Stdin, os.Stdout, *showJSON); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func check(input io.Reader, output io.Writer, showJSON bool) error {
	type testID struct{ pkg, name string }
	passed, parents := map[testID]bool{}, map[testID]bool{}
	emptyPackages := map[string]bool{}
	failed := false
	decoder := json.NewDecoder(input)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("decode Go test event: %w", err)
		}
		var event struct{ Action, Package, Test, Output string }
		if err := json.Unmarshal(raw, &event); err != nil {
			return err
		}
		if showJSON {
			if _, err := fmt.Fprintln(output, string(raw)); err != nil {
				return err
			}
		} else if _, err := io.WriteString(output, event.Output); err != nil {
			return err
		}
		switch event.Action {
		case "run":
			for name := event.Test; strings.Contains(name, "/"); {
				name = name[:strings.LastIndex(name, "/")]
				parents[testID{event.Package, name}] = true
			}
		case "pass":
			if event.Test != "" {
				passed[testID{event.Package, event.Test}] = true
			}
		case "fail":
			failed = true
		case "output":
			// Go can pass a parent while reporting that none of its subtests ran.
			if strings.TrimSpace(event.Output) == "testing: warning: no tests to run" {
				emptyPackages[event.Package] = true
			}
		}
	}
	if failed {
		return errors.New("Go test reported failing tests")
	}
	// shortcut: Go reports empty execution per package, revisit when per-test execution metadata is available.
	for test := range passed {
		if !parents[test] && !emptyPackages[test.pkg] {
			return nil
		}
	}
	return errors.New("no passing leaf tests (empty selection or all skipped)")
}

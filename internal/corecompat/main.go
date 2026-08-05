// Command corecompat verifies Commerce against an explicit Spice core and tool line.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

const requiredGoVersion = "go1.26.5"

var output = log.New(os.Stdout, "", 0)

func main() {
	os.Exit(execute(os.Args[1:])) // Entrypoint exception: propagate compatibility failure.
}

func execute(arguments []string) int {
	flags := flag.NewFlagSet("corecompat", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	line := flags.String("line", "all", "Spice compatibility line: minimum, current, or all")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		output.Printf("compatibility failed: unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
		return 2
	}
	if runtime.Version() != requiredGoVersion {
		output.Printf(
			"compatibility failed: go version is %s; require exactly %s",
			runtime.Version(),
			requiredGoVersion,
		)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	root, err := repositoryRoot()
	if err == nil {
		err = run(ctx, root, *line)
	}
	if err != nil {
		output.Printf("compatibility failed: %v", err)
		return 1
	}
	return 0
}

func invalidLine(line string) error {
	return fmt.Errorf("compatibility line %q is invalid; require minimum, current, or all", line)
}

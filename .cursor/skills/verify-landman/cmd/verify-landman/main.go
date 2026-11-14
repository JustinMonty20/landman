package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: verify-landman check-schema|check-loaded|probe|classify")
		return 2
	}
	switch args[0] {
	case "check-schema":
		return cmdCheckSchema(args[1:])
	case "check-loaded":
		return cmdCheckLoaded(args[1:])
	case "probe":
		return cmdProbe(args[1:])
	case "classify":
		return cmdClassify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %s\n", args[0])
		return 2
	}
}

func cmdCheckSchema(args []string) int {
	container, gooseURL, seedPath, err := dbFlags("check-schema", args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-schema: %v\n", err)
		return 2
	}
	text, code := checkSchema(container, gooseURL, seedPath, dockerPsql)
	if text != "" {
		fmt.Fprint(os.Stdout, text)
	}
	return code
}

func cmdCheckLoaded(args []string) int {
	container, gooseURL, seedPath, err := dbFlags("check-loaded", args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-loaded: %v\n", err)
		return 2
	}
	text, code := checkLoaded(container, gooseURL, seedPath, dockerPsql)
	if text != "" {
		fmt.Fprint(os.Stdout, text)
	}
	return code
}

func cmdProbe(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	evidence := fs.String("evidence", "", "directory for page-timings.json")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "probe: %v\n", err)
		return 2
	}
	if *evidence == "" {
		fmt.Fprintln(os.Stderr, "probe: evidence is required")
		return 2
	}
	return runProbe(*evidence, httpGet)
}

func cmdClassify(args []string) int {
	fs := flag.NewFlagSet("classify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	container := fs.String("container", "", "verify container name")
	gooseURL := fs.String("goose-url", "", "goose database URL inside the container")
	seedPath := fs.String("seed", "", "path to seed.sql")
	evidence := fs.String("evidence", "", "directory with the drive evidence")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "classify: %v\n", err)
		return 2
	}
	if *container == "" || *gooseURL == "" || *seedPath == "" || *evidence == "" {
		fmt.Fprintln(os.Stderr, "classify: container, goose-url, seed, and evidence are required")
		return 2
	}
	text, code := runClassify(*container, *gooseURL, *seedPath, *evidence, classifyDeps{query: dockerPsql, get: httpGet})
	if text != "" {
		fmt.Fprint(os.Stdout, text)
	}
	return code
}

func dbFlags(name string, args []string) (string, string, string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	container := fs.String("container", "", "verify container name")
	gooseURL := fs.String("goose-url", "", "goose database URL inside the container")
	seedPath := fs.String("seed", "", "path to seed.sql")
	if err := fs.Parse(args); err != nil {
		return "", "", "", err
	}
	if *container == "" || *gooseURL == "" || *seedPath == "" {
		return "", "", "", fmt.Errorf("container, goose-url, and seed are required")
	}
	return *container, *gooseURL, *seedPath, nil
}

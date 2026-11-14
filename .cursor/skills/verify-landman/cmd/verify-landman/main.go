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
		fmt.Fprintln(os.Stderr, "usage: verify-landman check-schema|check-loaded")
		return 2
	}
	switch args[0] {
	case "check-schema":
		return cmdCheckSchema(args[1:])
	case "check-loaded":
		return cmdCheckLoaded(args[1:])
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type classifyDeps struct {
	query sqlQuery
	get   httpGetter
}

// runClassify reads the drive evidence, and reads sales only when a full
// parcel scan published. It writes drive-outcome.json and returns the verdict line.
func runClassify(container, gooseURL, seedPath, evidence string, deps classifyDeps) (string, int) {
	if err := refuseContainer(container); err != nil {
		fmt.Fprintf(os.Stderr, "classify: %v\n", err)
		return "", 1
	}
	in, err := readEvidence(evidence)
	if err != nil {
		fmt.Fprintf(os.Stderr, "classify: %v\n", err)
		return "", 1
	}
	published, err := parcelPublished(context.Background(), deps.query, container, gooseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "classify: %v\n", err)
		return "", 1
	}
	in.committed = published
	if published {
		text, err := os.ReadFile(seedPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "classify: %v\n", err)
			return "", 1
		}
		snap, err := parseSeed(string(text))
		if err != nil {
			fmt.Fprintf(os.Stderr, "classify: %v\n", err)
			return "", 1
		}
		expected, actual, err := readSaleComparison(context.Background(), deps.get, deps.query, container, gooseURL, snap.parcelIDs())
		if err != nil {
			fmt.Fprintf(os.Stderr, "classify: %v\n", err)
			return "", 1
		}
		in.salesRead = true
		in.expected = expected
		in.actual = actual
		if in.processExit == 0 {
			post, err := readPostImport(context.Background(), deps.query, container, gooseURL, snap)
			if err != nil {
				fmt.Fprintf(os.Stderr, "classify: %v\n", err)
				return "", 1
			}
			in.post = &post
		}
	}
	out := classify(in)
	if err := writeJSON(filepath.Join(evidence, "drive-outcome.json"), out); err != nil {
		fmt.Fprintf(os.Stderr, "classify: %v\n", err)
		return "", 1
	}
	if len(out.Problems) > 0 {
		fmt.Fprintf(os.Stderr, "classify: %s\n", strings.Join(out.Problems, "; "))
	}
	return string(out.Verdict) + "\n", out.code()
}

func readEvidence(dir string) (classifyInput, error) {
	stdout, err := os.ReadFile(filepath.Join(dir, "import.stdout.txt"))
	if err != nil {
		return classifyInput{}, err
	}
	stderr, err := os.ReadFile(filepath.Join(dir, "import.stderr.txt"))
	if err != nil {
		return classifyInput{}, err
	}
	exitText, err := os.ReadFile(filepath.Join(dir, "import.exit.txt"))
	if err != nil {
		return classifyInput{}, err
	}
	processExit, err := strconv.Atoi(strings.TrimSpace(string(exitText)))
	if err != nil {
		return classifyInput{}, fmt.Errorf("import exit: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "page-timings.json"))
	if err != nil {
		return classifyInput{}, err
	}
	var probe pageTimings
	if err := json.Unmarshal(raw, &probe); err != nil {
		return classifyInput{}, fmt.Errorf("page timings: %w", err)
	}
	return classifyInput{stdout: string(stdout), stderr: string(stderr), processExit: processExit, probe: probe}, nil
}

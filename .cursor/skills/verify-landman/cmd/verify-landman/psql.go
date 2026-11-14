package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const verifyContainerName = "landman-verify-postgis"

type sqlQuery func(ctx context.Context, container, databaseURL, sql string) (string, error)

func refuseContainer(name string) error {
	if name != verifyContainerName {
		return fmt.Errorf("refusing container %s", name)
	}
	return nil
}

func dockerExec(ctx context.Context, container, databaseURL, sql string) error {
	_, err := dockerPsqlInput(ctx, container, databaseURL, sql)
	return err
}

// dockerPsql runs a short query inside the verify container.
// It refuses any other container before it starts Docker.
func dockerPsql(ctx context.Context, container, databaseURL, sql string) (string, error) {
	if err := refuseContainer(container); err != nil {
		return "", err
	}
	return runPsql(ctx, container, databaseURL, sql, true)
}

func dockerPsqlInput(ctx context.Context, container, databaseURL, sql string) (string, error) {
	if err := refuseContainer(container); err != nil {
		return "", err
	}
	return runPsql(ctx, container, databaseURL, sql, false)
}

func runPsql(ctx context.Context, container, databaseURL, sql string, commandArg bool) (string, error) {
	args := []string{"exec", "-i", container, "psql", databaseURL, "-v", "ON_ERROR_STOP=1", "-t", "-A", "-F", "\t"}
	if commandArg {
		args = append(args, "-c", sql)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	if !commandArg {
		cmd.Stdin = strings.NewReader(sql)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("psql: %w: %s", err, detail)
	}
	return stdout.String(), nil
}

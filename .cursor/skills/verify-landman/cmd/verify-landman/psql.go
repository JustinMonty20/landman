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

// dockerPsql runs psql inside the verify container.
// It refuses any other container before it starts Docker.
func dockerPsql(ctx context.Context, container, databaseURL, sql string) (string, error) {
	if err := refuseContainer(container); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container,
		"psql", databaseURL, "-v", "ON_ERROR_STOP=1", "-t", "-A", "-F", "\t", "-c", sql)
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

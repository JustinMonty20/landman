package parceljob_test

import (
	"context"
	"os"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
)

func TestPublicJobRejectsIncompleteSnapshotAndUnboundedRepair(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	job := parceljob.Job{Root: root}
	if _, err := job.Replay(context.Background(), "00000000000000000000000000000000"); err == nil {
		t.Fatal("incomplete snapshot accepted")
	}
	for _, limit := range []string{"0", "101", "broken"} {
		if _, err := parceljob.ReprocessLimit(limit); err == nil {
			t.Fatalf("unbounded repair %q accepted", limit)
		}
	}
	if n, err := parceljob.ReprocessLimit("100"); err != nil || n != 100 {
		t.Fatalf("repair budget %d %v", n, err)
	}
}

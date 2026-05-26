package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDialogDirectoryCreatesMissingPath(t *testing.T) {
	target := filepath.Join(t.TempDir(), "简传", "uploads")

	dir, err := resolveDialogDirectory(target, "")
	if err != nil {
		t.Fatal(err)
	}
	if dir != target {
		t.Fatalf("dir = %q, want %q", dir, target)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("target directory was not created: %v", err)
	}
}

func TestResolveDialogDirectoryFallsBackFromFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir, err := resolveDialogDirectory(file, "")
	if err != nil {
		t.Fatal(err)
	}
	if dir != root {
		t.Fatalf("dir = %q, want %q", dir, root)
	}
}

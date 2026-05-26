package storage

import (
	"path/filepath"
	"testing"
)

func TestSafeName(t *testing.T) {
	tests := map[string]string{
		"report.pdf":          "report.pdf",
		" ../secret.txt ":     "secret.txt",
		"folder/document.txt": "document.txt",
		"...":                 "",
	}
	for input, want := range tests {
		if got := SafeName(input); got != want {
			t.Fatalf("SafeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDecodeIDRejectsUnsafeName(t *testing.T) {
	id := EncodeID("../secret.txt")
	if _, err := DecodeID(id); err == nil {
		t.Fatal("DecodeID should reject path traversal")
	}
}

func TestStoreListSkipsPartialFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "done.txt")); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "upload.txt.part")); err != nil {
		t.Fatal(err)
	}
	files, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "done.txt" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func writeFile(path string) error {
	return osWriteFile(path, []byte("ok"), 0o644)
}

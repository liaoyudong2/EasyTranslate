package main

import (
	"os"
	"testing"
)

func TestEnsureWindowsResource(t *testing.T) {
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	path, err := ensureWindowsResource(root, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("Windows resource file should not be empty")
	}
}

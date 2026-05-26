package main

import (
	"os"
	"testing"

	"github.com/tc-hib/winres"
)

func TestNewWindowsResourceSetUsesWailsIconID(t *testing.T) {
	rs, err := newWindowsResourceSet()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rs.GetIcon(winres.ID(windowsWailsAppIconID)); err != nil {
		t.Fatalf("missing Windows title bar icon resource: %v", err)
	}
}

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

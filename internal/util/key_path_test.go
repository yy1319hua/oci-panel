package util

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyFilePathRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../secret", "../../etc/passwd", "/etc/passwd", `..\\secret`, "", ".", ".."} {
		if _, err := KeyFilePath(name); err == nil {
			t.Fatalf("KeyFilePath(%q) accepted an unsafe filename", name)
		}
	}
}

func TestKeyFilePathStaysBelowKeys(t *testing.T) {
	got, err := KeyFilePath("oci-key.pem")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("./keys")
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, got)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "oci-key.pem" || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("path escaped keys directory: root=%q path=%q rel=%q", root, got, rel)
	}
}

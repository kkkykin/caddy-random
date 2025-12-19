package randomfile

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchInclude_CaseInsensitive(t *testing.T) {
	rf := &RandomFile{Include: []string{"*.JPG"}}
	rf.includeLower = []string{"*.jpg"}


	if !rf.matchInclude("a.jpg") {
		t.Fatalf("expected match")
	}
	if !rf.matchInclude("A.JPG") {
		t.Fatalf("expected match")
	}
	if rf.matchInclude("a.png") {
		t.Fatalf("expected no match")
	}
}

func TestResolveTargetDir_ReturnsRoot(t *testing.T) {
	root := t.TempDir()
	rf := &RandomFile{Root: root}

	u := &url.URL{Path: "/anything"}
	r := &http.Request{URL: u}

	dir, err := rf.resolveTargetDir(root, r)
	if err != nil {
		t.Fatalf("resolveTargetDir: %v", err)
	}

	want, _ := filepath.Abs(root)
	if dir != want {
		t.Fatalf("expected %q, got %q", want, dir)
	}
}

func TestPickRandomFile_Recursive_FindsNested(t *testing.T) {
	root := t.TempDir()

	nestedDir := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	nestedFile := filepath.Join(nestedDir, "x.jpg")
	if err := os.WriteFile(nestedFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rf := &RandomFile{Root: root, Recursive: true, Include: []string{"*.jpg"}}
	rf.includeLower = []string{"*.jpg"}

	selected, err := rf.pickRandomFile(root)
	if err != nil {
		t.Fatalf("pickRandomFile: %v", err)
	}
	if selected != nestedFile {
		t.Fatalf("expected %q, got %q", nestedFile, selected)
	}
}

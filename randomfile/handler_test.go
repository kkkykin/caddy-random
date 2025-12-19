package randomfile

import (
	"net/http"
	"net/url"
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

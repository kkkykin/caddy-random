package randomfile

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
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
	// Ensure cache condition variable is initialized for tests that call pickRandomFile directly.
	rf.cacheCond = sync.NewCond(&rf.cacheMu)
	rf.cacheReady = true

	selected, err := rf.pickRandomFile(root)
	if err != nil {
		t.Fatalf("pickRandomFile: %v", err)
	}
	if selected != nestedFile {
		t.Fatalf("expected %q, got %q", nestedFile, selected)
	}
}

func TestPickRandomFile_Cache_TTL(t *testing.T) {
	root := t.TempDir()

	file1 := filepath.Join(root, "a.jpg")
	if err := os.WriteFile(file1, []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rf := &RandomFile{Root: root, Include: []string{"*.jpg"}, Cache: caddy.Duration(200 * time.Millisecond)}
	rf.includeLower = []string{"*.jpg"}
	rf.cacheCond = sync.NewCond(&rf.cacheMu)
	rf.cacheReady = true

	selected1, err := rf.pickRandomFile(root)
	if err != nil {
		t.Fatalf("pickRandomFile#1: %v", err)
	}
	if selected1 != file1 {
		t.Fatalf("expected %q, got %q", file1, selected1)
	}

	file2 := filepath.Join(root, "b.jpg")
	if err := os.WriteFile(file2, []byte("b"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	selected2, err := rf.pickRandomFile(root)
	if err != nil {
		t.Fatalf("pickRandomFile#2: %v", err)
	}
	// Should still come from cached list (only a.jpg) since TTL not expired.
	if selected2 != file1 {
		t.Fatalf("expected cached %q, got %q", file1, selected2)
	}

	time.Sleep(250 * time.Millisecond)

	selected3, err := rf.pickRandomFile(root)
	if err != nil {
		t.Fatalf("pickRandomFile#3: %v", err)
	}
	if selected3 != file1 && selected3 != file2 {
		t.Fatalf("expected %q or %q, got %q", file1, file2, selected3)
	}
}

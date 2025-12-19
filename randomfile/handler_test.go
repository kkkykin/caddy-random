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

func TestSanitizeRelativeSubdir(t *testing.T) {
	cases := []struct {
		in      string
		ok      bool
		expected string
	}{
		{in: "foo", ok: true, expected: "foo"},
		{in: "foo/bar", ok: true, expected: "foo/bar"},
		{in: "foo/../bar", ok: true, expected: "bar"},
		{in: "../etc", ok: false},
		{in: "/abs", ok: false},
		{in: "..", ok: false},
	}

	for _, tc := range cases {
		got, err := sanitizeRelativeSubdir(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("%q: expected ok, got err=%v", tc.in, err)
			}
			if got != tc.expected {
				t.Fatalf("%q: expected %q, got %q", tc.in, tc.expected, got)
			}
		} else {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
		}
	}
}

func TestResolveTargetDir_QueryParamOverridesURLPath(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "a"), 0o755)
	_ = os.MkdirAll(filepath.Join(root, "b"), 0o755)

	rf := &RandomFile{Root: root, SubdirParam: "subdir", UseURLPathSubdir: true}

	u := &url.URL{Path: "/a"}
	q := u.Query()
	q.Set("subdir", "b")
	u.RawQuery = q.Encode()
	r := &http.Request{URL: u}

	dir, err := rf.resolveTargetDir(root, r)
	if err != nil {
		t.Fatalf("resolveTargetDir: %v", err)
	}

	want, _ := filepath.Abs(filepath.Join(root, "b"))
	if dir != want {
		t.Fatalf("expected %q, got %q", want, dir)
	}
}

func TestResolveTargetDir_PreventTraversal(t *testing.T) {
	root := t.TempDir()
	rf := &RandomFile{Root: root, SubdirParam: "subdir"}

	u := &url.URL{Path: "/"}
	q := u.Query()
	q.Set("subdir", "../etc")
	u.RawQuery = q.Encode()
	r := &http.Request{URL: u}

	_, err := rf.resolveTargetDir(root, r)
	if err == nil {
		t.Fatalf("expected error")
	}
}

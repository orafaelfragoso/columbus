package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitBlobOIDMatchesKnownValue(t *testing.T) {
	// `printf '' | git hash-object --stdin` => empty blob oid.
	if got := gitBlobOID([]byte("")); got != "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391" {
		t.Errorf("empty blob oid = %s", got)
	}
	// `printf 'hello\n' | git hash-object --stdin`.
	if got := gitBlobOID([]byte("hello\n")); got != "ce013625030ba8dba906f756967f9e9ca394464a" {
		t.Errorf("hello blob oid = %s", got)
	}
}

func TestIsBinary(t *testing.T) {
	if isBinary([]byte("plain text\n")) {
		t.Error("text classified as binary")
	}
	if !isBinary([]byte("has\x00nul")) {
		t.Error("nul content should be binary")
	}
}

func TestDeriveRole(t *testing.T) {
	cases := map[string]string{
		"internal/svc/svc.go":      "impl",
		"internal/svc/svc_test.go": "test",
		"src/foo.test.ts":          "test",
		"src/foo.spec.tsx":         "test",
		"src/__tests__/foo.ts":     "test",
		"README.md":                "doc",
		"app.py":                   "impl",
	}
	for path, want := range cases {
		if got := deriveRole(path); got != want {
			t.Errorf("deriveRole(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPackageResolverNearestManifest(t *testing.T) {
	root := t.TempDir()
	// root/go.mod, root/sub/pkg/file.go and root/web/package.json
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "sub", "pkg"), 0o755)
	os.MkdirAll(filepath.Join(root, "web"), 0o755)
	os.WriteFile(filepath.Join(root, "web", "package.json"), []byte("{}"), 0o644)

	r := newPackageResolver(root)
	if got := r.pkgFor(filepath.Join("sub", "pkg", "file.go")); got != filepath.Base(root) {
		t.Errorf("go file package = %q, want %q (root module dir)", got, filepath.Base(root))
	}
	if got := r.pkgFor(filepath.Join("web", "index.ts")); got != "web" {
		t.Errorf("web file package = %q, want web", got)
	}
}

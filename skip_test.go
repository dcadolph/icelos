package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// skipPatterns are the calls and names that let a test pass without running.
var skipPatterns = []string{ //nolint:gochecknoglobals // Not modified, simplifies testing.
	".Skip(", ".Skipf(", ".SkipNow(", "testing.Short(", "ICELOS_SWEEP",
}

// skipCalls walks root and returns "path:line: text" for every line of a _test.go file that holds
// a skip pattern. It passes over directories whose names start with a dot and the file at self,
// a slash-separated path relative to root.
func skipCalls(root, self string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)
		if rel == self {
			return nil
		}
		lines, err := skipLines(path)
		if err != nil {
			return err
		}
		for _, l := range lines {
			found = append(found, rel+":"+l)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("skip scan failed: %w", err)
	}
	return found, nil
}

// skipLines returns "line: text" for every line of the file at path that holds a skip pattern.
func skipLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open test file: %w", err)
	}
	defer f.Close()
	var found []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		for _, p := range skipPatterns {
			if strings.Contains(line, p) {
				found = append(found, fmt.Sprintf("%d: %s", n, strings.TrimSpace(line)))
				break
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return found, nil
}

// TestNoSkips fails for every skip in the module's tests, so the real-ffmpeg tests run or fail.
func TestNoSkips(t *testing.T) {
	t.Parallel()
	found, err := skipCalls(".", "skip_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		t.Errorf("%s: tests run or fail, never skip", f)
	}
}

func TestSkipCalls(t *testing.T) {
	t.Parallel()
	const head = "package x\n\n"
	tests := []struct {
		Files     map[string]string
		WantFound []string
	}{{ // Test 0: A clean test file.
		Files:     map[string]string{"a_test.go": head + "func TestA(t *testing.T) {}\n"},
		WantFound: nil,
	}, { // Test 1: Every skip call in a test file.
		Files: map[string]string{"a_test.go": head + "\tt.Skip(\"x\")\n\tt.Skipf(\"%d\", 1)\n" +
			"\tt.SkipNow()\n"},
		WantFound: []string{`a_test.go:3: t.Skip("x")`, `a_test.go:4: t.Skipf("%d", 1)`,
			"a_test.go:5: t.SkipNow()"},
	}, { // Test 2: Short mode and the old sweep gate in a nested package.
		Files: map[string]string{"pkg/b_test.go": head + "\tif testing.Short() {\n" +
			"const env = \"ICELOS_SWEEP\"\n"},
		WantFound: []string{"pkg/b_test.go:3: if testing.Short() {",
			`pkg/b_test.go:4: const env = "ICELOS_SWEEP"`},
	}, { // Test 3: A skip outside a test file is not a test skip.
		Files:     map[string]string{"a.go": head + "\tt.Skip(\"x\")\n"},
		WantFound: nil,
	}, { // Test 4: Dot directories are passed over.
		Files:     map[string]string{".build/c_test.go": head + "\tt.Skip(\"x\")\n"},
		WantFound: nil,
	}, { // Test 5: Only the scanner's own file at the root is passed over.
		Files: map[string]string{"skip_test.go": head + "\tt.Skip(\"x\")\n",
			"pkg/skip_test.go": head + "\tt.Skip(\"y\")\n"},
		WantFound: []string{`pkg/skip_test.go:3: t.Skip("y")`},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for name, body := range test.Files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := skipCalls(root, "skip_test.go")
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.WantFound, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

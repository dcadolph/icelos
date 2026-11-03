package preset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/dcadolph/icelos/internal/format"
)

// testCustom returns a valid custom preset with the given ID and name.
func testCustom(id, name string) Preset {
	return Preset{ID: id, Name: name, Target: format.Target{Container: "mkv",
		Video: format.Video{Codec: "hevc", RateControl: format.RateQuality, Quality: format.QualityHigh},
		Audio: format.Audio{Codec: "opus", Bitrate: 128_000}}}
}

func TestStoreSave(t *testing.T) {
	t.Parallel()
	bad := testCustom("bad", "Bad")
	bad.Video.Codec = "vp8"
	bad.Container = "mp4"
	tests := []struct {
		In   Preset
		Want error
	}{{ // Test 0: Valid custom preset.
		In: testCustom("my-hevc", "My HEVC"),
	}, { // Test 1: Built-in ID is refused.
		In: testCustom("apple", "Apple"), Want: ErrBuiltin,
	}, { // Test 2: ID that cannot be a file name.
		In: testCustom("../x", "X"), Want: ErrInvalid,
	}, { // Test 3: Reserved recent ID.
		In: testCustom("recent", "Recent"), Want: ErrInvalid,
	}, { // Test 4: Empty name.
		In: testCustom("noname", " "), Want: ErrInvalid,
	}, { // Test 5: Fails catalog validation.
		In: bad, Want: ErrInvalid,
	}, { // Test 6: Uppercase ID.
		In: testCustom("Mine", "Mine"), Want: ErrInvalid,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			s := WithDir(filepath.Join(t.TempDir(), "presets"))
			err := s.Save(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if err != nil {
				return
			}
			got, err := s.Get(test.In.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := test.In
			want.Custom = true
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStoreListAndLookup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := WithDir(dir)
	for _, p := range []Preset{testCustom("zed", "zebra"), testCustom("abe", "Apple pie"),
		testCustom("bob", "apple pie")} {
		if err := s.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "junk.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("zed"); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if !errors.Is(err, ErrStore) {
		t.Errorf("List error = %v, want %v for junk.json", err, ErrStore)
	}
	var ids []string
	for _, p := range got {
		if !p.Custom {
			t.Errorf("%s: Custom not set", p.ID)
		}
		ids = append(ids, p.ID)
	}
	if diff := cmp.Diff([]string{"abe", "bob", "zed"}, ids); diff != "" {
		t.Errorf("list mismatch (-want +got):\n%s", diff)
	}
	tests := []struct {
		In         string
		WantCustom bool
		Want       error
	}{{ // Test 0: Built-in wins.
		In: "apple",
	}, { // Test 1: Custom found after built-ins.
		In: "zed", WantCustom: true,
	}, { // Test 2: Unknown.
		In: "nope", Want: ErrUnknown,
	}, { // Test 3: Malformed file.
		In: "junk", Want: ErrStore,
	}, { // Test 4: Path-like ID never reaches the disk.
		In: "../zed", Want: ErrUnknown,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			p, err := s.Lookup(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if err == nil && (p.ID != test.In || p.Custom != test.WantCustom) {
				t.Errorf("Lookup(%q) = %s custom=%v", test.In, p.ID, p.Custom)
			}
		})
	}
}

func TestStoreMissingDir(t *testing.T) {
	t.Parallel()
	s := WithDir(filepath.Join(t.TempDir(), "absent"))
	got, err := s.List()
	if err != nil || len(got) != 0 {
		t.Errorf("List = %v, %v", got, err)
	}
	recent, err := s.Recent()
	if err != nil || len(recent) != 0 {
		t.Errorf("Recent = %v, %v", recent, err)
	}
	if err := s.Delete("ghost"); !errors.Is(err, ErrUnknown) {
		t.Errorf("Delete error = %v, want %v", err, ErrUnknown)
	}
}

func TestStoreDelete(t *testing.T) {
	t.Parallel()
	s := WithDir(t.TempDir())
	if err := s.Save(testCustom("mine", "Mine")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"web", "mine"} {
		if err := s.Use(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete("apple"); !errors.Is(err, ErrBuiltin) {
		t.Errorf("Delete built-in error = %v, want %v", err, ErrBuiltin)
	}
	if err := s.Delete("mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("mine"); !errors.Is(err, ErrUnknown) {
		t.Errorf("Get after delete error = %v, want %v", err, ErrUnknown)
	}
	recent, err := s.Recent()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"web"}, recent); diff != "" {
		t.Errorf("recent mismatch (-want +got):\n%s", diff)
	}
}

func TestStoreRecent(t *testing.T) {
	t.Parallel()
	s := WithDir(t.TempDir())
	for i := range 12 {
		if err := s.Use(fmt.Sprintf("p%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Use("p5"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Recent()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"p5", "p11", "p10", "p9", "p8", "p7", "p6", "p4", "p3", "p2"}
	if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
	entries, err := os.ReadDir(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("store holds %d files, want only %s with no temp files left", len(entries), recentFile)
	}
}

func TestWithDirPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	WithDir("")
}

func TestNewStore(t *testing.T) {
	t.Parallel()
	s, err := NewStore()
	if err != nil {
		t.Skipf("no config dir: %v", err)
	}
	if filepath.Base(s.Dir()) != "presets" || filepath.Base(filepath.Dir(s.Dir())) != "icelos" {
		t.Errorf("Dir = %q, want .../icelos/presets", s.Dir())
	}
}

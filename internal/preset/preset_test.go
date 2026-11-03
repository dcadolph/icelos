package preset

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/format"
)

func TestBuiltinValid(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, p := range All() {
		if seen[p.ID] {
			t.Errorf("duplicate id %q", p.ID)
		}
		seen[p.ID] = true
		if p.Name == "" || p.Description == "" {
			t.Errorf("%s: missing name or description", p.ID)
		}
		if p.ID == TargetSizeID {
			p = p.WithTargetSize(10_000_000)
		}
		if err := p.Validate(); err != nil {
			t.Errorf("%s: %v", p.ID, err)
		}
	}
	if !seen[TargetSizeID] {
		t.Errorf("missing %q preset", TargetSizeID)
	}
}

func TestAllReturnsCopy(t *testing.T) {
	t.Parallel()
	got := All()
	got[0].ID = "changed"
	if All()[0].ID == "changed" {
		t.Error("All exposed the internal table")
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want   error
		In     string
		WantID string
	}{{ // Test 0: Known preset.
		In: "apple", WantID: "apple",
	}, { // Test 1: Unknown preset.
		In: "betamax", Want: ErrUnknown,
	}, { // Test 2: Empty id.
		In: "", Want: ErrUnknown,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := Lookup(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantID, got.ID); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	base := Preset{ID: "x", Target: format.Target{Container: "mp4",
		Video: format.Video{Codec: "h264", RateControl: format.RateQuality, Quality: format.QualityHigh},
		Audio: format.Audio{Codec: "aac"}}}
	tests := []struct {
		Want error
		In   func(Preset) Preset
	}{{ // Test 0: Base is valid.
		In: func(p Preset) Preset { return p },
	}, { // Test 1: Empty id.
		In: func(p Preset) Preset { p.ID = ""; return p }, Want: ErrInvalid,
	}, { // Test 2: Unknown container.
		In: func(p Preset) Preset { p.Container = ""; return p }, Want: format.ErrUnknownContainer,
	}, { // Test 3: No streams at all.
		In: func(p Preset) Preset { p.Video.Codec, p.Audio.Codec = "", ""; return p }, Want: ErrInvalid,
	}, { // Test 4: Negative target size.
		In: func(p Preset) Preset { return p.WithTargetSize(-1) }, Want: ErrInvalid,
	}, { // Test 5: Target size with copied video.
		In: func(p Preset) Preset {
			p.Video = format.Video{Codec: format.Copy}
			return p.WithTargetSize(1)
		},
		Want: ErrInvalid,
	}, { // Test 6: Target size on audio-only output.
		In:   func(p Preset) Preset { p.Video = format.Video{}; return p.WithTargetSize(1) },
		Want: ErrInvalid,
	}, { // Test 7: Resize with copied video.
		In: func(p Preset) Preset {
			p.Video = format.Video{Codec: format.Copy, MaxWidth: 640}
			return p
		},
		Want: ErrInvalid,
	}, { // Test 8: Downmix with copied audio.
		In:   func(p Preset) Preset { p.Audio = format.Audio{Codec: format.Copy, Channels: 2}; return p },
		Want: ErrInvalid,
	}, { // Test 9: Target size with HEVC is valid.
		In: func(p Preset) Preset { p.Video.Codec = "hevc"; return p.WithTargetSize(1) },
	}, { // Test 10: Catalog refusals are invalid presets too.
		In: func(p Preset) Preset { p.Video.Width = 1919; return p }, Want: ErrInvalid,
	}, { // Test 11: Catalog refusals keep their own error.
		In: func(p Preset) Preset { p.Video.Width = 1919; return p }, Want: format.ErrSetting,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if err := test.In(base).Validate(); !errors.Is(err, test.Want) {
				t.Errorf("error = %v, want %v", err, test.Want)
			}
		})
	}
}

func TestWithTargetSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantRate format.RateControl
		ID       string
		In       int64
	}{{ // Test 0: A quality preset switches to two-pass.
		ID: "web", In: 8_000_000, WantRate: format.RateTwoPass,
	}, { // Test 1: Zero leaves the rate control alone.
		ID: "web", In: 0, WantRate: format.RateQuality,
	}, { // Test 2: Copied video is left for Validate to refuse.
		ID: "remux", In: 8_000_000, WantRate: "",
	}, { // Test 3: Audio-only output gains no video rate control.
		ID: "audio", In: 8_000_000, WantRate: "",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			p, err := Lookup(test.ID)
			if err != nil {
				t.Fatal(err)
			}
			got := p.WithTargetSize(test.In)
			if diff := cmp.Diff(test.WantRate, got.Video.RateControl); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.In, got.TargetBytes); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExt(t *testing.T) {
	t.Parallel()
	want := map[string]string{"apple": "mp4", "compatible": "mp4", "web": "mp4", "tiny": "mp4",
		TargetSizeID: "mp4", "remux": "mp4", "audio": "m4a", "mp3": "mp3"}
	got := map[string]string{}
	for _, p := range All() {
		got[p.ID] = p.Ext()
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()
	p, err := Lookup("web")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "name", "description", "container", "ext", "video", "audio"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("JSON lacks %q: %s", key, b)
		}
	}
	var back Preset
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(p, back); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

func TestHasStreams(t *testing.T) {
	t.Parallel()
	audio, err := Lookup("audio")
	if err != nil {
		t.Fatal(err)
	}
	if audio.HasVideo() || !audio.HasAudio() {
		t.Errorf("audio preset: HasVideo=%v HasAudio=%v", audio.HasVideo(), audio.HasAudio())
	}
}

func TestParseSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want      error
		In        string
		WantBytes int64
	}{{ // Test 0: Decimal megabytes.
		In: "25MB", WantBytes: 25_000_000,
	}, { // Test 1: Bare number is megabytes.
		In: "8", WantBytes: 8_000_000,
	}, { // Test 2: Binary unit with space and mixed case.
		In: " 8 MiB ", WantBytes: 8 << 20,
	}, { // Test 3: Fractional gigabytes.
		In: "1.5g", WantBytes: 1_500_000_000,
	}, { // Test 4: Plain bytes.
		In: "2048b", WantBytes: 2048,
	}, { // Test 5: Kilobytes.
		In: "500kb", WantBytes: 500_000,
	}, { // Test 6: Garbage.
		In: "lots", Want: ErrSize,
	}, { // Test 7: Empty.
		In: "", Want: ErrSize,
	}, { // Test 8: Zero.
		In: "0", Want: ErrSize,
	}, { // Test 9: Negative.
		In: "-5MB", Want: ErrSize,
	}, { // Test 10: Too large.
		In: "1e30", Want: ErrSize,
	}, { // Test 11: Not a number.
		In: "NaN", Want: ErrSize,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := ParseSize(test.In)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantBytes, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

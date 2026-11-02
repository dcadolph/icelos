package preset

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
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
	base := Preset{ID: "x", Ext: "mp4", Video: Video{Codec: VideoH264}, Audio: Audio{Codec: AudioAAC}}
	tests := []struct {
		Want error
		In   func(Preset) Preset
	}{{ // Test 0: Base is valid.
		In: func(p Preset) Preset { return p },
	}, { // Test 1: Empty id.
		In: func(p Preset) Preset { p.ID = ""; return p }, Want: ErrInvalid,
	}, { // Test 2: Empty extension.
		In: func(p Preset) Preset { p.Ext = ""; return p }, Want: ErrInvalid,
	}, { // Test 3: No streams at all.
		In: func(p Preset) Preset { p.Video.Codec, p.Audio.Codec = VideoNone, AudioNone; return p }, Want: ErrInvalid,
	}, { // Test 4: Negative target size.
		In: func(p Preset) Preset { return p.WithTargetSize(-1) }, Want: ErrInvalid,
	}, { // Test 5: Target size with copied video.
		In: func(p Preset) Preset { p.Video.Codec = VideoCopy; return p.WithTargetSize(1) }, Want: ErrInvalid,
	}, { // Test 6: Target size on audio-only output.
		In: func(p Preset) Preset { p.Video.Codec = VideoNone; return p.WithTargetSize(1) }, Want: ErrInvalid,
	}, { // Test 7: Resize with copied video.
		In: func(p Preset) Preset { p.Video = Video{Codec: VideoCopy, MaxWidth: 640}; return p }, Want: ErrInvalid,
	}, { // Test 8: Downmix with copied audio.
		In: func(p Preset) Preset { p.Audio = Audio{Codec: AudioCopy, Channels: 2}; return p }, Want: ErrInvalid,
	}, { // Test 9: Target size with HEVC is valid.
		In: func(p Preset) Preset { p.Video.Codec = VideoHEVC; return p.WithTargetSize(1) },
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

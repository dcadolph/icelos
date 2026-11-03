package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
)

// yes is a verdict that allows copying.
func yes() *Verdict {
	return &Verdict{Copy: true}
}

// noFor is a verdict that rules copying out for reason.
func noFor(reason string) *Verdict {
	return &Verdict{Reason: reason}
}

func TestCopyable(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	with := func(id string, edit func(*preset.Preset)) preset.Preset {
		p := testPreset(t, id)
		if edit != nil {
			edit(&p)
		}
		return p
	}
	audioOnly := probe.Info{Streams: []probe.Stream{
		{Index: 0, Kind: "audio", Codec: "aac", Channels: 2, SampleRate: 48000}}}
	tests := []struct {
		Want         error
		WantCopy     Copy
		Preset       preset.Preset
		Info         probe.Info
		Extra        []probe.Info
		Streams      Streams
		WantLossless bool
	}{{ // Test 0: Remux of streams mp4 carries.
		Preset: with("remux", nil), Info: streamsInfo(),
		WantCopy: Copy{Video: yes(), Audio: yes()}, WantLossless: true,
	}, { // Test 1: Same codecs inside the fit box and the channel cap.
		Preset: with("compatible", nil), Info: streamsInfo(),
		WantCopy: Copy{Video: yes(), Audio: yes()}, WantLossless: true,
	}, { // Test 2: A resize rules video out.
		Preset: with("compatible", func(p *preset.Preset) {
			p.Video.Width, p.Video.MaxWidth, p.Video.MaxHeight = 1280, 0, 0
		}), Info: streamsInfo(),
		WantCopy: Copy{Video: noFor(ReasonResize), Audio: yes()},
	}, { // Test 3: The fit box shrinks a 4K source.
		Preset: with("compatible", nil), Info: testInfo(),
		WantCopy: Copy{Video: noFor(ReasonResize), Audio: noFor(ReasonChannels)},
	}, { // Test 4: Another video codec.
		Preset: with("tiny", nil), Info: streamsInfo(),
		WantCopy: Copy{Video: noFor(ReasonCodec), Audio: yes()},
	}, { // Test 5: A frame rate change.
		Preset: with("web", func(p *preset.Preset) { p.Video.FPS = 24 }), Info: streamsInfo(),
		WantCopy: Copy{Video: noFor(ReasonFrameRate), Audio: yes()},
	}, { // Test 6: A pinned profile.
		Preset: with("web", func(p *preset.Preset) { p.Video.Profile = "high" }), Info: streamsInfo(),
		WantCopy: Copy{Video: noFor(ReasonProfile), Audio: yes()},
	}, { // Test 7: A target size rules both out.
		Preset: with(preset.TargetSizeID, nil).WithTargetSize(5_000_000), Info: streamsInfo(),
		WantCopy: Copy{Video: noFor(ReasonTargetSize), Audio: noFor(ReasonTargetSize)},
	}, { // Test 8: Burned subtitles.
		Preset: with("web", nil), Info: streamsInfo(),
		Streams:  Streams{Subtitles: Subtitles{Mode: SubtitlesBurn}},
		WantCopy: Copy{Video: noFor(ReasonBurn), Audio: yes()},
	}, { // Test 9: A container that cannot carry the source codecs.
		Preset: with("remux", func(p *preset.Preset) { p.Container = "webm" }), Info: streamsInfo(),
		WantCopy: Copy{Video: noFor(ReasonContainer), Audio: noFor(ReasonContainer)},
	}, { // Test 10: Every track, one of them another codec.
		Preset: with("compatible", nil), Info: streamsInfo(), Streams: Streams{AudioTracks: []int{AllTracks}},
		WantCopy: Copy{Video: yes(), Audio: noFor(ReasonCodec)},
	}, { // Test 11: Audio only, same codec.
		Preset: with("audio", nil), Info: streamsInfo(),
		WantCopy: Copy{Audio: yes()}, WantLossless: true,
	}, { // Test 12: Audio only, another codec.
		Preset: with("mp3", nil), Info: streamsInfo(),
		WantCopy: Copy{Audio: noFor(ReasonCodec)},
	}, { // Test 13: A sample rate change.
		Preset: with("audio", func(p *preset.Preset) { p.Audio.SampleRate = 44100 }), Info: streamsInfo(),
		WantCopy: Copy{Audio: noFor(ReasonSampleRate)},
	}, { // Test 14: An external track that was not probed.
		Preset: with("compatible", nil), Info: streamsInfo(), Streams: Streams{ExternalAudio: []string{"a.m4a"}},
		WantCopy: Copy{Video: yes(), Audio: noFor(ReasonUnknown)},
	}, { // Test 15: An external track that was probed.
		Preset: with("compatible", nil), Info: streamsInfo(), Streams: Streams{ExternalAudio: []string{"a.m4a"}},
		Extra:    []probe.Info{audioOnly},
		WantCopy: Copy{Video: yes(), Audio: yes()}, WantLossless: true,
	}, { // Test 16: A video target with a silent source has no audio verdict.
		Preset: with("compatible", nil), Info: probe.Info{Streams: streamsInfo().Streams[:1]},
		WantCopy: Copy{Video: yes()}, WantLossless: true,
	}, { // Test 17: Video target, audio-only source.
		Preset: with("web", nil), Info: audioOnly, Want: ErrNoVideo,
	}, { // Test 18: Audio target, silent source.
		Preset: with("audio", nil), Info: probe.Info{Streams: streamsInfo().Streams[:1]}, Want: ErrNoAudio,
	}, { // Test 19: A track the source lacks.
		Preset: with("web", nil), Info: streamsInfo(), Streams: Streams{AudioTracks: []int{5}}, Want: ErrNoAudio,
	}, { // Test 20: An invalid preset.
		Preset: with("web", func(p *preset.Preset) { p.Video.Width = 1279 }), Info: streamsInfo(),
		Want: preset.ErrInvalid,
	}, { // Test 21: Copied video into a container that refuses it.
		Preset:   with("remux", func(p *preset.Preset) { p.Audio = format.Audio{Codec: "aac"} }),
		Info:     probe.Info{Streams: []probe.Stream{{Index: 0, Kind: "video", Codec: "wmv2"}}},
		WantCopy: Copy{Video: noFor(ReasonContainer)},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			job := Job{Input: "in.mov", Preset: test.Preset, Streams: test.Streams}
			got, err := Copyable(job, test.Info, streamsAvail(), test.Extra...)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantCopy, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
			if got.Lossless() != test.WantLossless {
				t.Errorf("Lossless = %v, want %v", got.Lossless(), test.WantLossless)
			}
		})
	}
}

func TestCopyLossless(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   Copy
		Want bool
	}{
		{In: Copy{}},                                         // Test 0: No streams.
		{In: Copy{Video: yes()}, Want: true},                 // Test 1: Video only.
		{In: Copy{Video: yes(), Audio: noFor(ReasonCodec)}},  // Test 2: Audio encodes.
		{In: Copy{Video: noFor(ReasonResize), Audio: yes()}}, // Test 3: Video encodes.
		{In: Copy{Video: yes(), Audio: yes()}, Want: true},   // Test 4: Both copy.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.In.Lossless(); got != test.Want {
				t.Errorf("Lossless = %v, want %v", got, test.Want)
			}
		})
	}
}

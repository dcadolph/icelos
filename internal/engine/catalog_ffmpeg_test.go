package engine

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/preset"
	"github.com/dcadolph/icelos/internal/probe"
	"github.com/dcadolph/icelos/internal/util/tooltest"
)

// sweepBitrate is the video bitrate of bitrate rate controls. ffmpeg's built-in two-pass rate
// control refuses a bitrate its coarsest quantizer cannot reach, and the noisy test pattern at
// 16CIF needs about 3 Mb/s.
const sweepBitrate = 4_000_000

// sweepCase is one target the catalog sweep encodes with the real ffmpeg.
type sweepCase struct {
	// Name labels the subtest.
	Name string
	// Target is the conversion target.
	Target format.Target
	// Hardware asks for VideoToolbox.
	Hardware bool
	// WantRefused is true when the catalog must refuse the target.
	WantRefused bool
}

// sweepVideo returns video settings for codec under rate control rc.
func sweepVideo(codec format.Codec, rc format.RateControl, profile string) format.Video {
	v := format.Video{Codec: codec.ID, RateControl: rc, Profile: profile}
	switch rc {
	case format.RateQuality:
		v.Quality = format.QualityBalanced
	case format.RateCBR, format.RateVBR, format.RateTwoPass:
		v.Bitrate = sweepBitrate
	}
	return v
}

// sweepCases lists every container with each video codec it carries and each audio codec it
// carries, every rate control and profile once per codec, and hardware where the codec has it.
func sweepCases(avail EncoderSet) []sweepCase {
	usable := func(c format.Codec) bool {
		return slices.ContainsFunc(c.Encoders, func(e format.Encoder) bool { return avail.Has(e.Name) })
	}
	var cases []sweepCase
	seen := map[string]bool{}
	for _, box := range format.Containers() {
		firstAudio := ""
		for _, id := range box.Audio {
			if c, _ := format.CodecByID(id); usable(c) {
				firstAudio = id
				break
			}
		}
		for _, id := range box.Video {
			codec, _ := format.CodecByID(id)
			if !usable(codec) {
				continue
			}
			rcs := []format.RateControl{""}
			if len(codec.RateControls) != 0 {
				rcs = codec.RateControls[:1]
				if !seen[id] {
					rcs = codec.RateControls
				}
			}
			needsProfile := slices.ContainsFunc(codec.Modes, func(m format.Mode) bool {
				return m.Profile != ""
			})
			profiles := []string{""}
			if needsProfile {
				profiles = nil
			}
			if !seen[id] || needsProfile {
				for _, p := range codec.Profiles {
					profiles = append(profiles, p.ID)
				}
			}
			seen[id] = true
			for _, rc := range rcs {
				for _, profile := range profiles {
					t := format.Target{Container: box.ID, Video: sweepVideo(codec, rc, profile),
						Audio: format.Audio{Codec: firstAudio}}
					name := fmt.Sprintf("%s/%s/%s/%s", box.ID, id, rc, profile)
					cases = append(cases, sweepCase{Name: name, Target: t})
					if slices.ContainsFunc(codec.Encoders, func(e format.Encoder) bool { return e.Hardware }) &&
						rc != format.RateTwoPass {
						cases = append(cases, sweepCase{Name: name + "/hw", Target: t, Hardware: true})
					}
				}
			}
		}
		firstVideo := ""
		for _, id := range box.Video {
			if c, _ := format.CodecByID(id); usable(c) {
				firstVideo = id
				break
			}
		}
		if box.Kind == format.KindVideo && firstAudio != "" {
			t := format.Target{Container: box.ID, Audio: format.Audio{Codec: firstAudio}}
			cases = append(cases, sweepCase{Name: box.ID + "/audio-only", Target: t,
				WantRefused: box.Flags.NeedsVideo})
		}
		for _, id := range box.Audio {
			codec, _ := format.CodecByID(id)
			if !usable(codec) {
				continue
			}
			t := format.Target{Container: box.ID, Audio: format.Audio{Codec: id}}
			if firstVideo != "" {
				vc, _ := format.CodecByID(firstVideo)
				t.Video = sweepVideo(vc, firstRate(vc), "")
			}
			cases = append(cases, sweepCase{Name: fmt.Sprintf("%s/audio/%s", box.ID, id), Target: t})
			for _, depth := range codec.BitDepths {
				if len(codec.BitDepths) > 1 {
					d := t
					d.Audio.BitDepth = depth
					name := fmt.Sprintf("%s/audio/%s/%d", box.ID, id, depth)
					cases = append(cases, sweepCase{Name: name, Target: d})
				}
			}
		}
	}
	return cases
}

// firstRate returns the codec's first rate control, or none.
func firstRate(c format.Codec) format.RateControl {
	if len(c.RateControls) == 0 {
		return ""
	}
	return c.RateControls[0]
}

// TestEncodeCatalog encodes a short clip into every catalog combination the local ffmpeg can
// produce, and checks that ffprobe reads back the codecs the target asked for.
func TestEncodeCatalog(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	tooltest.Require(t, "ffmpeg", "ffprobe")
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mov")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30000/1001:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1",
		"-filter_complex", "[1:a]pan=5.1|c0=c0|c1=c0|c2=c0|c3=c0|c4=c0|c5=c0[a]",
		"-map", "0:v", "-map", "[a]", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("make source: %v: %s", err, out)
	}
	info, err := probe.File(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	avail, err := NewFFmpeg().Encoders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, test := range sweepCases(avail) {
		t.Run(test.Name, func(t *testing.T) {
			t.Parallel()
			p := preset.Preset{ID: "sweep", Target: test.Target}
			switch err := p.Validate(); {
			case err != nil && test.WantRefused:
				return
			case test.WantRefused:
				t.Fatal("catalog accepts a target its muxer refuses")
			case err != nil:
				t.Fatalf("catalog refuses a sweep case: %v", err)
			}
			dest := filepath.Join(dir, fmt.Sprintf("out%d.%s", i, p.Ext()))
			job := Job{Input: src, Output: dest, Preset: p, Hardware: test.Hardware}
			plan, err := Build(job, info, avail, dest, filepath.Join(dir, fmt.Sprintf("pass%d", i)))
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if test.Hardware && !plan.Hardware {
				t.Logf("no hardware encoder for these settings, ran %s", plan.Encoder)
			}
			for _, args := range plan.Passes {
				var stderr bytes.Buffer
				cmd := exec.CommandContext(ctx, "ffmpeg", args...)
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("ffmpeg %s\n%v: %s", strings.Join(args, " "), err, lastLines(stderr.Bytes()))
				}
			}
			got, err := probe.File(ctx, dest)
			if err != nil {
				t.Fatalf("probe output: %v", err)
			}
			if v, ok := got.FirstVideo(); p.HasVideo() && (!ok || v.Codec != p.Video.Codec) {
				t.Errorf("video codec = %q, want %q", v.Codec, p.Video.Codec)
			}
			if a, ok := got.FirstAudio(); p.HasAudio() && (!ok || a.Codec != p.Audio.Codec) {
				t.Errorf("audio codec = %q, want %q", a.Codec, p.Audio.Codec)
			}
		})
	}
}

// lastLines returns the last few lines of ffmpeg output.
func lastLines(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.Join(lines[max(0, len(lines)-4):], "\n")
}

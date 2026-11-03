package format

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// idPattern is the form every codec and container ID must take.
var idPattern = regexp.MustCompile(`^[a-z0-9_]+$`) //nolint:gochecknoglobals // Not modified, simplifies testing.

// TestContainerTable checks the structural rules every container must follow.
func TestContainerTable(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, c := range Containers() {
		if seen[c.ID] {
			t.Errorf("container %q appears twice", c.ID)
		}
		seen[c.ID] = true
		if !idPattern.MatchString(c.ID) || !idPattern.MatchString(c.Ext) {
			t.Errorf("container %q has a malformed ID or extension %q", c.ID, c.Ext)
		}
		if c.Name == "" || c.Muxer == "" {
			t.Errorf("container %q needs a name and a muxer", c.ID)
		}
		switch c.Kind {
		case KindVideo:
			if len(c.Video) == 0 {
				t.Errorf("video container %q allows no video codec", c.ID)
			}
		case KindAudio:
			if len(c.Video) != 0 || len(c.Subtitles) != 0 {
				t.Errorf("audio container %q allows video or subtitle codecs", c.ID)
			}
		default:
			t.Errorf("container %q has kind %q", c.ID, c.Kind)
		}
		if len(c.Audio) == 0 {
			t.Errorf("container %q allows no audio codec", c.ID)
		}
		for _, list := range [][]string{c.Video, c.Audio, c.Subtitles} {
			ids := map[string]bool{}
			for _, id := range list {
				if !idPattern.MatchString(id) {
					t.Errorf("container %q lists malformed codec ID %q", c.ID, id)
				}
				if ids[id] {
					t.Errorf("container %q lists codec %q twice", c.ID, id)
				}
				ids[id] = true
			}
		}
	}
}

// TestChecklistContainers checks that every container the parity checklist names is present.
func TestChecklistContainers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantIDs []string
		Kind    Kind
	}{{ // Test 0: The 22 video outputs.
		Kind: KindVideo,
		WantIDs: []string{"mp4", "mov", "mkv", "m4v", "avi", "wmv", "webm", "flv", "f4v", "mpg", "vob",
			"ts", "trp", "m2ts", "3gp", "3g2", "mxf", "asf", "dv", "amv", "swf", "ogv"},
	}, { // Test 1: The audio outputs.
		Kind: KindAudio,
		WantIDs: []string{"mp3", "m4a", "m4r", "m4b", "aac", "wav", "aiff", "flac", "alac", "ac3",
			"ogg", "opus", "wma", "mka", "caf", "au", "amr", "mp2"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, c := range Containers() {
				if c.Kind == test.Kind {
					got = append(got, c.ID)
				}
			}
			if diff := cmp.Diff(test.WantIDs, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestContainerByID checks lookups, unknown IDs, and that results do not alias the catalog.
func TestContainerByID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want      error
		ID        string
		WantMuxer string
	}{{ // Test 0: A video container.
		ID: "mkv", WantMuxer: "matroska",
	}, { // Test 1: Apple's M4V uses the ipod muxer, not ffmpeg's raw m4v muxer.
		ID: "m4v", WantMuxer: "ipod",
	}, { // Test 2: An audio container that shares its extension with another.
		ID: "alac", WantMuxer: "ipod",
	}, { // Test 3: Unknown ID.
		ID: "divx", Want: ErrUnknownContainer,
	}, { // Test 4: Empty ID.
		ID: "", Want: ErrUnknownContainer,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := ContainerByID(test.ID)
			if !errors.Is(err, test.Want) {
				t.Fatalf("got error %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantMuxer, got.Muxer); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLookupsCopy checks that changing a returned container or codec leaves the catalog intact.
func TestLookupsCopy(t *testing.T) {
	t.Parallel()
	c, err := ContainerByID("mp4")
	if err != nil {
		t.Fatal(err)
	}
	c.Video[0] = "changed"
	all := Containers()
	all[0].Audio[0] = "changed"
	again, err := ContainerByID("mp4")
	if err != nil {
		t.Fatal(err)
	}
	if again.Video[0] == "changed" || again.Audio[0] == "changed" {
		t.Error("a returned container aliases the catalog")
	}
}

// TestCodecByID checks that an unknown codec is refused.
func TestCodecByID(t *testing.T) {
	t.Parallel()
	if _, err := CodecByID("not-a-codec"); !errors.Is(err, ErrUnknownCodec) {
		t.Errorf("got error %v, want %v", err, ErrUnknownCodec)
	}
}

// TestAllows checks codec membership by kind.
func TestAllows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Container string
		Kind      Kind
		Codec     string
		WantOK    bool
	}{{ // Test 0: HEVC in MP4.
		Container: "mp4", Kind: KindVideo, Codec: "hevc", WantOK: true,
	}, { // Test 1: VP8 is not allowed in MP4.
		Container: "mp4", Kind: KindVideo, Codec: "vp8",
	}, { // Test 2: A video codec asked as audio is refused.
		Container: "mkv", Kind: KindAudio, Codec: "h264",
	}, { // Test 3: Soft subtitles in MP4 are mov_text.
		Container: "mp4", Kind: KindSubtitle, Codec: "mov_text", WantOK: true,
	}, { // Test 4: DV forces DV video.
		Container: "dv", Kind: KindVideo, Codec: "h264",
	}, { // Test 5: Unknown kind.
		Container: "mp4", Kind: "data", Codec: "h264",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := ContainerByID(test.Container)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.WantOK, c.Allows(test.Kind, test.Codec)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMuxersExist checks every container's muxer against the local ffmpeg when one is installed.
func TestMuxersExist(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	out, err := exec.Command(bin, "-hide_banner", "-muxers").Output()
	if err != nil {
		t.Fatal(err)
	}
	muxers := map[string]bool{}
	for line := range strings.Lines(string(out)) {
		if fields := strings.Fields(line); len(fields) >= 2 && strings.Contains(fields[0], "E") {
			for name := range strings.SplitSeq(fields[1], ",") {
				muxers[name] = true
			}
		}
	}
	for _, c := range Containers() {
		if !muxers[c.Muxer] {
			t.Errorf("container %q uses muxer %q, which ffmpeg does not list", c.ID, c.Muxer)
		}
	}
}

// encoderPattern is the form every encoder name must take.
var encoderPattern = regexp.MustCompile(`^[a-z0-9_-]+$`) //nolint:gochecknoglobals // Not modified, simplifies testing.

// TestCodecTable checks the structural rules every codec must follow.
func TestCodecTable(t *testing.T) { //nolint:gocognit // Test function.
	t.Parallel()
	seen := map[string]bool{}
	for _, c := range Codecs() {
		if seen[c.ID] {
			t.Errorf("codec %q appears twice", c.ID)
		}
		seen[c.ID] = true
		if !idPattern.MatchString(c.ID) || c.Name == "" {
			t.Errorf("codec %q has a malformed ID or no name", c.ID)
		}
		if len(c.Encoders) == 0 {
			t.Errorf("codec %q has no encoder", c.ID)
		}
		for _, e := range c.Encoders {
			if !encoderPattern.MatchString(e.Name) {
				t.Errorf("codec %q lists malformed encoder %q", c.ID, e.Name)
			}
		}
		audioOnly := len(c.SampleRates) != 0 || c.MaxChannels != 0 || len(c.Bitrates) != 0 ||
			len(c.BitrateCaps) != 0 || len(c.BitDepths) != 0
		videoOnly := len(c.Profiles) != 0 || len(c.Levels) != 0 || len(c.Modes) != 0 ||
			len(c.FrameRates) != 0 || c.PixFmt != "" || len(c.RateControls) != 0
		switch c.Kind {
		case KindVideo:
			if audioOnly {
				t.Errorf("video codec %q sets audio rules", c.ID)
			}
		case KindAudio:
			if videoOnly {
				t.Errorf("audio codec %q sets video rules", c.ID)
			}
			if c.MaxChannels == 0 {
				t.Errorf("audio codec %q has no channel limit", c.ID)
			}
			if c.Lossless != (len(c.BitDepths) != 0) {
				t.Errorf("audio codec %q must list bit depths exactly when lossless", c.ID)
			}
		case KindSubtitle:
			if audioOnly || videoOnly {
				t.Errorf("subtitle codec %q sets audio or video rules", c.ID)
			}
		default:
			t.Errorf("codec %q has kind %q", c.ID, c.Kind)
		}
		profiles := map[string]bool{}
		for _, p := range c.Profiles {
			if profiles[p.ID] || p.ID == "" || p.Name == "" {
				t.Errorf("codec %q has a duplicate or blank profile %q", c.ID, p.ID)
			}
			profiles[p.ID] = true
		}
		for _, m := range c.Modes {
			if m.Profile != "" && !profiles[m.Profile] {
				t.Errorf("codec %q mode %q names unknown profile %q", c.ID, m.Name, m.Profile)
			}
			if m.Width%2 != 0 || m.Height%2 != 0 || m.Width == 0 || m.Height == 0 {
				t.Errorf("codec %q mode %q has size %dx%d", c.ID, m.Name, m.Width, m.Height)
			}
		}
	}
}

// TestContainerCodecsExist checks that every codec a container lists is in the codec table with
// the kind the container lists it under.
func TestContainerCodecsExist(t *testing.T) {
	t.Parallel()
	for _, c := range Containers() {
		for kind, list := range map[Kind][]string{
			KindVideo: c.Video, KindAudio: c.Audio, KindSubtitle: c.Subtitles,
		} {
			for _, id := range list {
				codec, err := CodecByID(id)
				if err != nil {
					t.Errorf("container %q lists %s codec %q: %v", c.ID, kind, id, err)
					continue
				}
				if codec.Kind != kind {
					t.Errorf("container %q lists %q as %s, but it is %s", c.ID, id, kind, codec.Kind)
				}
			}
		}
	}
}

// TestEncodersExist checks every encoder name against the local ffmpeg when one is installed.
// Encoders that need the bundled ffmpeg build are allowed to be missing.
func TestEncodersExist(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	out, err := exec.Command(bin, "-hide_banner", "-encoders").Output()
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for line := range strings.Lines(string(out)) {
		if fields := strings.Fields(line); len(fields) >= 2 && len(fields[0]) == 6 {
			have[fields[1]] = true
		}
	}
	bundled := map[string]bool{"libtheora": true, "libvorbis": true, "libopencore_amrnb": true}
	for _, c := range Codecs() {
		for _, e := range c.Encoders {
			if !have[e.Name] && !bundled[e.Name] {
				t.Errorf("codec %q uses encoder %q, which ffmpeg does not list", c.ID, e.Name)
			}
		}
	}
}

// TestMatch checks the fixed frame shapes of DV, H.263, AMV, and DNxHD.
func TestMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want     error
		Codec    string
		Profile  string
		WantMode string
		Width    int
		Height   int
		FPS      float64
		Bitrate  int
	}{{ // Test 0: DV NTSC.
		Codec: "dvvideo", Width: 720, Height: 480, FPS: 29.97, WantMode: "NTSC",
	}, { // Test 1: DV PAL.
		Codec: "dvvideo", Width: 720, Height: 576, FPS: 25, WantMode: "PAL",
	}, { // Test 2: DV refuses HD.
		Codec: "dvvideo", Width: 1920, Height: 1080, Want: ErrMode,
	}, { // Test 3: DV PAL forces 25 fps.
		Codec: "dvvideo", Width: 720, Height: 576, FPS: 29.97, Want: ErrMode,
	}, { // Test 4: Same as source picks the first mode.
		Codec: "dvvideo", WantMode: "NTSC",
	}, { // Test 5: H.263 refuses a size outside the CIF family.
		Codec: "h263", Width: 320, Height: 240, Want: ErrMode,
	}, { // Test 6: H.263 CIF at any rate.
		Codec: "h263", Width: 352, Height: 288, FPS: 15, WantMode: "CIF",
	}, { // Test 7: AMV refuses a height that is not a multiple of 16.
		Codec: "amv", Width: 160, Height: 120, Want: ErrMode,
	}, { // Test 8: Legacy DNxHD at a listed bitrate.
		Codec: "dnxhd", Profile: "dnxhd", Width: 1920, Height: 1080, Bitrate: 145_000_000,
		WantMode: "1080p",
	}, { // Test 9: Legacy DNxHD refuses an unlisted bitrate.
		Codec: "dnxhd", Profile: "dnxhd", Width: 1920, Height: 1080, Bitrate: 50_000_000, Want: ErrMode,
	}, { // Test 10: Legacy DNxHD refuses UHD.
		Codec: "dnxhd", Profile: "dnxhd", Width: 3840, Height: 2160, Want: ErrMode,
	}, { // Test 11: DNxHR has no fixed sizes.
		Codec: "dnxhd", Profile: "dnxhr_sq", Width: 3840, Height: 2160,
	}, { // Test 12: H.264 has no fixed sizes.
		Codec: "h264", Width: 1922, Height: 1080, FPS: 7,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := CodecByID(test.Codec)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Match(test.Profile, test.Width, test.Height, test.FPS, test.Bitrate)
			if !errors.Is(err, test.Want) {
				t.Fatalf("got error %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantMode, got.Name); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestMaxBitrate checks audio bitrate ceilings by sample rate and channel count.
func TestMaxBitrate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Codec       string
		SampleRate  int
		Channels    int
		WantBitrate int
	}{{ // Test 0: AAC allows six bits per sample per channel.
		Codec: "aac", SampleRate: 48000, Channels: 2, WantBitrate: 576_000,
	}, { // Test 1: MP3 at MPEG-1 rates caps at 320k.
		Codec: "mp3", SampleRate: 44100, Channels: 2, WantBitrate: 320_000,
	}, { // Test 2: MP3 at MPEG-2 rates caps at 160k.
		Codec: "mp3", SampleRate: 22050, Channels: 2, WantBitrate: 160_000,
	}, { // Test 3: MP3 at MPEG-2.5 rates caps at 64k.
		Codec: "mp3", SampleRate: 8000, Channels: 1, WantBitrate: 64_000,
	}, { // Test 4: Opus allows 256k per channel.
		Codec: "opus", SampleRate: 48000, Channels: 6, WantBitrate: 1_536_000,
	}, { // Test 5: AC-3 caps at 640k.
		Codec: "ac3", SampleRate: 48000, Channels: 6, WantBitrate: 640_000,
	}, { // Test 6: MP2 at MPEG-1 rates caps at 384k.
		Codec: "mp2", SampleRate: 48000, Channels: 2, WantBitrate: 384_000,
	}, { // Test 7: Lossless codecs have no cap.
		Codec: "flac", SampleRate: 96000, Channels: 2,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := CodecByID(test.Codec)
			if err != nil {
				t.Fatal(err)
			}
			got := c.MaxBitrate(test.SampleRate, test.Channels)
			if diff := cmp.Diff(test.WantBitrate, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAllowsFrameRate checks codecs limited to a list of frame rates.
func TestAllowsFrameRate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Codec  string
		FPS    float64
		WantOK bool
	}{{ // Test 0: MPEG-1 at an NTSC rate.
		Codec: "mpeg1video", FPS: 29.97, WantOK: true,
	}, { // Test 1: MPEG-1 refuses 20 fps.
		Codec: "mpeg1video", FPS: 20,
	}, { // Test 2: Same as source is always allowed.
		Codec: "mpeg1video", WantOK: true,
	}, { // Test 3: H.264 takes any rate.
		Codec: "h264", FPS: 20, WantOK: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := CodecByID(test.Codec)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.WantOK, c.AllowsFrameRate(test.FPS)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestProfileByID checks profile lookups and their pixel formats.
func TestProfileByID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Codec      string
		Profile    string
		WantPixFmt string
		WantOK     bool
	}{{ // Test 0: ProRes 4444 encodes from 4:4:4.
		Codec: "prores", Profile: "4444", WantPixFmt: "yuv444p10le", WantOK: true,
	}, { // Test 1: HEVC Main uses the codec's pixel format.
		Codec: "hevc", Profile: "main", WantOK: true,
	}, { // Test 2: Unknown profile.
		Codec: "h264", Profile: "high10",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c, err := CodecByID(test.Codec)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := c.ProfileByID(test.Profile)
			if ok != test.WantOK {
				t.Fatalf("got ok %v, want %v", ok, test.WantOK)
			}
			if diff := cmp.Diff(test.WantPixFmt, got.PixFmt); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestRateArg checks that NTSC rates become exact fractions.
func TestRateArg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		WantArg string
		FPS     float64
	}{{ // Test 0: 29.97.
		FPS: 29.97, WantArg: "30000/1001",
	}, { // Test 1: 23.976.
		FPS: 23.976, WantArg: "24000/1001",
	}, { // Test 2: 59.94.
		FPS: 59.94, WantArg: "60000/1001",
	}, { // Test 3: A whole rate.
		FPS: 25, WantArg: "25",
	}, { // Test 4: A fractional rate that is not NTSC.
		FPS: 12.5, WantArg: "12.5",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(test.WantArg, RateArg(test.FPS)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestOptions checks the domain lists and that the result does not alias the catalog.
func TestOptions(t *testing.T) {
	t.Parallel()
	d := Options()
	if d.Resolutions[0].ID != "same" || d.FrameRates[0].FPS != 0 {
		t.Error("same as source must lead the resolution and frame rate lists")
	}
	for name, list := range map[string][]int{
		"video bitrates": d.VideoBitrates, "audio bitrates": d.AudioBitrates,
		"sample rates": d.SampleRates,
	} {
		if !slices.IsSorted(list) {
			t.Errorf("%s are not sorted", name)
		}
	}
	for _, c := range Codecs() {
		for _, r := range c.SampleRates {
			if !slices.Contains(d.SampleRates, r) {
				t.Errorf("codec %q allows sample rate %d, which the domain lacks", c.ID, r)
			}
		}
	}
	d.Resolutions[0].ID = "changed"
	d.SampleRates[0] = 1
	again := Options()
	if again.Resolutions[0].ID == "changed" || again.SampleRates[0] == 1 {
		t.Error("Options aliases the catalog")
	}
}

// TestCodecLookupsCopy checks that changing a returned codec leaves the catalog intact.
func TestCodecLookupsCopy(t *testing.T) {
	t.Parallel()
	c, err := CodecByID("dnxhd")
	if err != nil {
		t.Fatal(err)
	}
	c.Modes[0].Bitrates[0] = 1
	c.Profiles[0].ID = "changed"
	again, err := CodecByID("dnxhd")
	if err != nil {
		t.Fatal(err)
	}
	if again.Modes[0].Bitrates[0] == 1 || again.Profiles[0].ID == "changed" {
		t.Error("a returned codec aliases the catalog")
	}
}

// TestNeedsVideoContainersAreVideo checks that only video containers demand a video stream.
func TestNeedsVideoContainersAreVideo(t *testing.T) {
	t.Parallel()
	for _, c := range Containers() {
		if c.Flags.NeedsVideo && c.Kind != KindVideo {
			t.Errorf("container %q needs video but has kind %q", c.ID, c.Kind)
		}
	}
}

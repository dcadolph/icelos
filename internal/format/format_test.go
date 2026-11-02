package format

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
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

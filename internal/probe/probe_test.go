package probe

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want     error
		In       string
		WantInfo Info
	}{{ // Test 0: Typical mp4 with video and audio.
		In: `{"format":{"format_name":"mov,mp4","duration":"12.5","size":"1000"},` +
			`"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":1920,"height":1080},` +
			`{"index":1,"codec_type":"audio","codec_name":"aac","channels":2}]}`,
		WantInfo: Info{
			Format: "mov,mp4", Duration: 12500 * time.Millisecond, Size: 1000,
			Streams: []Stream{
				{Index: 0, Kind: "video", Codec: "h264", Width: 1920, Height: 1080},
				{Index: 1, Kind: "audio", Codec: "aac", Channels: 2},
			},
		},
	}, { // Test 1: Invalid JSON.
		In: `{`, Want: ErrProbe,
	}, { // Test 2: Bad duration.
		In: `{"format":{"duration":"abc"}}`, Want: ErrProbe,
	}, { // Test 3: Missing duration and size are zero.
		In: `{"format":{"format_name":"wav"}}`, WantInfo: Info{Format: "wav"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := Parse([]byte(test.In))
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantInfo, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStreamHelpers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantVideo  Stream
		WantAudio  Stream
		WantWidth  int
		WantHeight int
		WantFound  [2]bool
	}{{ // Test 0: Portrait phone video stored landscape with a rotation.
		In: `{"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","width":1920,"height":1080,` +
			`"side_data_list":[{"side_data_type":"Display Matrix","rotation":-90}]},` +
			`{"index":1,"codec_type":"audio","codec_name":"aac","channels":2}]}`,
		WantVideo: Stream{Index: 0, Kind: "video", Codec: "hevc", Width: 1920, Height: 1080,
			SideData: []SideData{{Rotation: -90}}},
		WantAudio: Stream{Index: 1, Kind: "audio", Codec: "aac", Channels: 2},
		WantWidth: 1080, WantHeight: 1920, WantFound: [2]bool{true, true},
	}, { // Test 1: MP3 with cover art has no motion video.
		In: `{"streams":[{"index":0,"codec_type":"audio","codec_name":"mp3","channels":2},` +
			`{"index":1,"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,` +
			`"disposition":{"attached_pic":1}}]}`,
		WantAudio: Stream{Index: 0, Kind: "audio", Codec: "mp3", Channels: 2},
		WantFound: [2]bool{false, true},
	}, { // Test 2: Half-turn rotation keeps the size.
		In: `{"streams":[{"index":0,"codec_type":"video","codec_name":"h264","width":640,"height":480,` +
			`"side_data_list":[{"rotation":180}]}]}`,
		WantVideo: Stream{Index: 0, Kind: "video", Codec: "h264", Width: 640, Height: 480,
			SideData: []SideData{{Rotation: 180}}},
		WantWidth: 640, WantHeight: 480, WantFound: [2]bool{true, false},
	}, { // Test 3: No streams.
		In: `{}`,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			info, err := Parse([]byte(test.In))
			if err != nil {
				t.Fatal(err)
			}
			v, vok := info.FirstVideo()
			a, aok := info.FirstAudio()
			if diff := cmp.Diff(test.WantFound, [2]bool{vok, aok}); diff != "" {
				t.Errorf("found mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantVideo, v); diff != "" {
				t.Errorf("video mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(test.WantAudio, a); diff != "" {
				t.Errorf("audio mismatch (-want +got):\n%s", diff)
			}
			w, h := v.DisplaySize()
			if diff := cmp.Diff([2]int{test.WantWidth, test.WantHeight}, [2]int{w, h}); diff != "" {
				t.Errorf("size mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRunMissingFile(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	_, err := File(t.Context(), filepath.Join(t.TempDir(), "missing.mp4"))
	if !errors.Is(err, ErrProbe) {
		t.Errorf("error = %v, want %v", err, ErrProbe)
	}
}

package engine

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/probe"
)

// unbuiltEncoders are catalog encoders the Homebrew ffmpeg lacks, so icelos has no builder for
// them yet and reports them missing.
var unbuiltEncoders = []string{ //nolint:gochecknoglobals // Not modified, simplifies testing.
	"libtheora", "libvorbis", "libopencore_amrnb"}

func TestEncoderTables(t *testing.T) {
	t.Parallel()
	inCatalog := map[string]bool{}
	for _, c := range format.Codecs() {
		for _, e := range c.Encoders {
			inCatalog[e.Name] = true
			if c.Kind == format.KindSubtitle || slices.Contains(unbuiltEncoders, e.Name) {
				continue
			}
			_, video := videoEncoders[e.Name]
			_, audio := audioEncoders[e.Name]
			if (c.Kind == format.KindVideo && !video) || (c.Kind == format.KindAudio && !audio) {
				t.Errorf("%s encoder %s has no argument builder", c.ID, e.Name)
			}
		}
	}
	for name := range videoEncoders {
		if !inCatalog[name] {
			t.Errorf("video builder %s is not a catalog encoder", name)
		}
	}
	for name := range audioEncoders {
		if !inCatalog[name] {
			t.Errorf("audio builder %s is not a catalog encoder", name)
		}
	}
}

func TestVideoEncoderArgs(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	tests := []struct {
		Encoder  string
		Spec     videoSpec
		WantArgs []string
	}{{ // Test 0: x264 quality with profile and level.
		Encoder: "libx264",
		Spec: videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualityHigh,
			Profile: "high", Level: "4.1"}},
		WantArgs: []string{"-profile:v", "high", "-level:v", "4.1", "-crf", "20"},
	}, { // Test 1: x264 constant bitrate fills the rate window and buffer.
		Encoder: "libx264", Spec: videoSpec{Video: format.Video{RateControl: format.RateCBR}, Bitrate: 4_000_000},
		WantArgs: []string{"-b:v", "4000000", "-minrate", "4000000", "-maxrate", "4000000",
			"-bufsize", "8000000"},
	}, { // Test 2: x264 average bitrate.
		Encoder: "libx264", Spec: videoSpec{Video: format.Video{RateControl: format.RateVBR}, Bitrate: 3_000_000},
		WantArgs: []string{"-b:v", "3000000"},
	}, { // Test 3: x264 second pass.
		Encoder: "libx264",
		Spec: videoSpec{Video: format.Video{RateControl: format.RateTwoPass}, Bitrate: 1_000_000, Pass: 2,
			PassLog: "plog"},
		WantArgs: []string{"-b:v", "1000000", "-pass", "2", "-passlogfile", "plog"},
	}, { // Test 4: x265 level and pass statistics share one parameter list.
		Encoder: "libx265",
		Spec: videoSpec{Video: format.Video{RateControl: format.RateTwoPass, Profile: "main10", Level: "5.1"},
			Bitrate: 1_000_000, Pass: 1, PassLog: "plog"},
		WantArgs: []string{"-profile:v", "main10", "-b:v", "1000000",
			"-x265-params", "pass=1:stats=plog:level-idc=5.1:log-level=error"},
	}, { // Test 5: x265 constant bitrate.
		Encoder: "libx265", Spec: videoSpec{Video: format.Video{RateControl: format.RateCBR}, Bitrate: 2_000_000},
		WantArgs: []string{"-b:v", "2000000", "-minrate", "2000000", "-maxrate", "2000000",
			"-bufsize", "4000000", "-x265-params", "log-level=error"},
	}, { // Test 6: VideoToolbox constant bitrate uses its own switch.
		Encoder:  "h264_videotoolbox",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateCBR, Profile: "main"}, Bitrate: 5_000_000},
		WantArgs: []string{"-profile:v", "main", "-b:v", "5000000", "-constant_bit_rate", "1"},
	}, { // Test 7: VideoToolbox HEVC quality.
		Encoder:  "hevc_videotoolbox",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualitySmall}},
		WantArgs: []string{"-q:v", "42"},
	}, { // Test 8: VP9 quality needs a zero bitrate.
		Encoder:  "libvpx-vp9",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualityBalanced}},
		WantArgs: []string{"-crf", "33", "-b:v", "0"},
	}, { // Test 9: VP8 quality needs a bitrate ceiling.
		Encoder:  "libvpx",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualityHigh}},
		WantArgs: []string{"-crf", "6", "-b:v", "30M"},
	}, { // Test 10: SVT-AV1 quality.
		Encoder:  "libsvtav1",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualitySmall}},
		WantArgs: []string{"-crf", "40"},
	}, { // Test 11: MPEG-4 quality is a quantizer.
		Encoder:  "mpeg4",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualityHigh}},
		WantArgs: []string{"-q:v", "3"},
	}, { // Test 12: Bitrate-only encoder.
		Encoder: "wmv2", Spec: videoSpec{Video: format.Video{RateControl: format.RateVBR}, Bitrate: 1_500_000},
		WantArgs: []string{"-b:v", "1500000"},
	}, { // Test 13: ProRes in software marks the file as Apple's.
		Encoder: "prores_ks", Spec: videoSpec{Video: format.Video{Profile: "4444xq"}},
		WantArgs: []string{"-profile:v", "4444xq", "-vendor", "apl0"},
	}, { // Test 14: VideoToolbox spells 4444 XQ differently.
		Encoder: "prores_videotoolbox", Spec: videoSpec{Video: format.Video{Profile: "4444xq"}},
		WantArgs: []string{"-profile:v", "xq"},
	}, { // Test 15: Legacy DNxHD carries its mode bitrate.
		Encoder: "dnxhd", Spec: videoSpec{Video: format.Video{Profile: "dnxhd"}, Bitrate: 120_000_000},
		WantArgs: []string{"-profile:v", "dnxhd", "-b:v", "120000000"},
	}, { // Test 16: DNxHR has no bitrate.
		Encoder: "dnxhd", Spec: videoSpec{Video: format.Video{Profile: "dnxhr_hq"}},
		WantArgs: []string{"-profile:v", "dnxhr_hq"},
	}, { // Test 17: CineForm quality.
		Encoder:  "cfhd",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualityHigh}},
		WantArgs: []string{"-quality", "3"},
	}, { // Test 18: DV needs top-left chroma.
		Encoder: "dvvideo", WantArgs: []string{"-chroma_sample_location", "topleft"},
	}, { // Test 19: FFV1 version 3.
		Encoder: "ffv1", WantArgs: []string{"-level", "3"},
	}, { // Test 20: Theora quality, where higher is better.
		Encoder:  "libtheora",
		Spec:     videoSpec{Video: format.Video{RateControl: format.RateQuality, Quality: format.QualityHigh}},
		WantArgs: []string{"-q:v", "8"},
	}, { // Test 21: Theora variable bitrate.
		Encoder: "libtheora", Spec: videoSpec{Video: format.Video{RateControl: format.RateVBR}, Bitrate: 2_000_000},
		WantArgs: []string{"-b:v", "2000000"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := videoEncoders[test.Encoder].Args(test.Spec)
			if diff := cmp.Diff(test.WantArgs, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPickVideoEncoder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Want         error
		Codec        string
		Container    string
		Video        format.Video
		Hardware     bool
		Avail        EncoderSet
		WantEncoder  string
		WantHardware bool
	}{{ // Test 0: Hardware H.264 at a level VideoToolbox signals.
		Codec: "h264", Container: "mp4", Video: format.Video{Level: "5.2"}, Hardware: true,
		Avail: testAvail(), WantEncoder: "h264_videotoolbox", WantHardware: true,
	}, { // Test 1: A level above VideoToolbox's range falls to software.
		Codec: "h264", Container: "mp4", Video: format.Video{Level: "6.0"}, Hardware: true,
		Avail: testAvail(), WantEncoder: "libx264",
	}, { // Test 2: VideoToolbox HEVC cannot set a level.
		Codec: "hevc", Container: "mp4", Video: format.Video{Level: "5.1"}, Hardware: true,
		Avail: testAvail(), WantEncoder: "libx265",
	}, { // Test 3: MXF cannot carry VideoToolbox ProRes.
		Codec: "prores", Container: "mxf", Hardware: true,
		Avail: EncoderSet{"prores_ks": true, "prores_videotoolbox": true}, WantEncoder: "prores_ks",
	}, { // Test 4: MOV takes VideoToolbox ProRes.
		Codec: "prores", Container: "mov", Hardware: true,
		Avail:       EncoderSet{"prores_ks": true, "prores_videotoolbox": true},
		WantEncoder: "prores_videotoolbox", WantHardware: true,
	}, { // Test 5: An encoder the ffmpeg build lacks is missing.
		Codec: "theora", Container: "ogv", Avail: EncoderSet{"libx264": true}, Want: ErrMissingEncoder,
	}, { // Test 6: Software off never uses hardware.
		Codec: "h264", Container: "mp4", Avail: EncoderSet{"h264_videotoolbox": true},
		Want: ErrMissingEncoder,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			codec, err := format.CodecByID(test.Codec)
			if err != nil {
				t.Fatal(err)
			}
			box, err := format.ContainerByID(test.Container)
			if err != nil {
				t.Fatal(err)
			}
			got, err := pickVideoEncoder(box, codec, test.Video, test.Avail, test.Hardware)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			want := [2]any{test.WantEncoder, test.WantHardware}
			if diff := cmp.Diff(want, [2]any{got.Name, got.Hardware}); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// testVideoStream returns a landscape video stream at the given size and frame rate.
func testVideoStream(w, h int, rate string) probe.Stream {
	return probe.Stream{Index: 0, Kind: "video", Codec: "h264", Width: w, Height: h, AvgFrameRate: rate}
}

func TestFrameFor(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	tests := []struct {
		Want      error
		Container string
		Video     format.Video
		Source    probe.Stream
		WantFrame frame
	}{{ // Test 0: DV from 29.97 fps HD picks NTSC with its pixel format and rate.
		Container: "dv", Video: format.Video{Codec: "dvvideo"}, Source: testVideoStream(1920, 1080, "30000/1001"),
		WantFrame: frame{Scale: "scale=720:480", FPS: 29.97, PixFmt: "yuv411p"},
	}, { // Test 1: DV from 25 fps picks PAL.
		Container: "dv", Video: format.Video{Codec: "dvvideo"}, Source: testVideoStream(1280, 720, "25/1"),
		WantFrame: frame{Scale: "scale=720:576", FPS: 25, PixFmt: "yuv420p"},
	}, { // Test 2: DVD in VOB from a small 50 fps source picks the nearest PAL size.
		Container: "vob", Video: format.Video{Codec: "mpeg2video", RateControl: format.RateVBR, Bitrate: 5_000_000},
		Source:    testVideoStream(352, 288, "50/1"),
		WantFrame: frame{FPS: 25, PixFmt: "yuv420p"},
	}, { // Test 3: A pinned mode wins over the source family.
		Container: "vob",
		Video: format.Video{Codec: "mpeg2video", RateControl: format.RateVBR, Bitrate: 5_000_000,
			Width: 720, Height: 480},
		Source:    testVideoStream(1920, 1080, "25/1"),
		WantFrame: frame{Scale: "scale=720:480", FPS: 29.97, PixFmt: "yuv420p"},
	}, { // Test 4: Legacy DNxHD takes the middle bitrate of the nearest size.
		Container: "mov", Video: format.Video{Codec: "dnxhd", Profile: "dnxhd"},
		Source:    testVideoStream(1280, 720, "30000/1001"),
		WantFrame: frame{PixFmt: "yuv422p", Bitrate: 120_000_000},
	}, { // Test 5: DNxHR 10-bit has no modes and fits the box.
		Container: "mov", Video: format.Video{Codec: "dnxhd", Profile: "dnxhr_hqx", MaxWidth: 1920},
		Source:    testVideoStream(3840, 2160, "24/1"),
		WantFrame: frame{Scale: "scale=1920:1080", PixFmt: "yuv422p10le"},
	}, { // Test 6: Width alone keeps the aspect ratio.
		Container: "mp4", Video: format.Video{Codec: "h264", Width: 1280},
		Source:    testVideoStream(1920, 1080, "30/1"),
		WantFrame: frame{Scale: "scale=1280:-2", PixFmt: "yuv420p"},
	}, { // Test 7: Height alone keeps the aspect ratio.
		Container: "mp4", Video: format.Video{Codec: "h264", Height: 720},
		Source:    testVideoStream(1920, 1080, "30/1"),
		WantFrame: frame{Scale: "scale=-2:720", PixFmt: "yuv420p"},
	}, { // Test 8: Exact size equal to the source needs no filter.
		Container: "mp4", Video: format.Video{Codec: "h264", Width: 1920, Height: 1080, FPS: 60},
		Source:    testVideoStream(1920, 1080, "30/1"),
		WantFrame: frame{FPS: 60, PixFmt: "yuv420p"},
	}, { // Test 9: HEVC Main 10 encodes from 10-bit frames.
		Container: "mp4", Video: format.Video{Codec: "hevc", Profile: "main10"},
		Source:    testVideoStream(1920, 1080, "30/1"),
		WantFrame: frame{PixFmt: "yuv420p10le"},
	}, { // Test 10: AMV moves 29.97 fps to the nearest rate its audio blocks divide.
		Container: "amv", Video: format.Video{Codec: "amv", RateControl: format.RateVBR, Bitrate: 500_000},
		Source:    testVideoStream(1280, 720, "30000/1001"),
		WantFrame: frame{Scale: "scale=320:240", FPS: 30, PixFmt: "yuvj420p"},
	}, { // Test 11: AMV with an unknown source rate.
		Container: "amv", Video: format.Video{Codec: "amv", RateControl: format.RateVBR, Bitrate: 500_000},
		Source:    testVideoStream(320, 240, ""),
		WantFrame: frame{FPS: 30, PixFmt: "yuvj420p"},
	}, { // Test 12: A size no mode has.
		Container: "dv", Video: format.Video{Codec: "dvvideo", Width: 640, Height: 480},
		Source: testVideoStream(1920, 1080, "30/1"), Want: format.ErrMode,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			box, err := format.ContainerByID(test.Container)
			if err != nil {
				t.Fatal(err)
			}
			codec, err := format.CodecByID(test.Video.Codec)
			if err != nil {
				t.Fatal(err)
			}
			got, err := frameFor(box, codec, test.Video, test.Source)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantFrame, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPlanAudio(t *testing.T) { //nolint:funlen // Test function.
	t.Parallel()
	full := EncoderSet{"aac_at": true, "aac": true, "libopus": true, "flac": true, "alac": true,
		"alac_at": true, "pcm_s16le": true, "adpcm_ima_amv": true, "libmp3lame": true,
		"libvorbis": true, "libopencore_amrnb": true}
	tests := []struct {
		Want      error
		Container string
		Audio     format.Audio
		Source    probe.Stream
		FPS       float64
		Avail     EncoderSet
		WantArgs  []string
	}{{ // Test 0: AudioToolbox AAC at a rate it takes.
		Container: "m4a", Audio: format.Audio{Codec: "aac", Bitrate: 256_000},
		Source:   probe.Stream{Channels: 2, SampleRate: 44100},
		WantArgs: []string{"-c:a", "aac_at", "-b:a", "256000"},
	}, { // Test 1: A 96 kHz source skips AudioToolbox, which stops at 48 kHz.
		Container: "m4a", Audio: format.Audio{Codec: "aac"}, Source: probe.Stream{Channels: 2, SampleRate: 96000},
		WantArgs: []string{"-c:a", "aac"},
	}, { // Test 2: A 96 kHz source asked down to 48 kHz keeps AudioToolbox.
		Container: "m4a", Audio: format.Audio{Codec: "aac", SampleRate: 48000},
		Source:   probe.Stream{Channels: 2, SampleRate: 96000},
		WantArgs: []string{"-c:a", "aac_at", "-ar", "48000"},
	}, { // Test 3: Opus moves 44.1 kHz to 48 kHz.
		Container: "webm", Audio: format.Audio{Codec: "opus"}, Source: probe.Stream{Channels: 2, SampleRate: 44100},
		WantArgs: []string{"-c:a", "libopus", "-ar", "48000"},
	}, { // Test 4: DV forces stereo at 48 kHz.
		Container: "dv", Audio: format.Audio{Codec: "pcm_s16le"}, Source: probe.Stream{Channels: 1, SampleRate: 44100},
		WantArgs: []string{"-c:a", "pcm_s16le", "-ac", "2", "-ar", "48000"},
	}, { // Test 5: MP3 caps a surround source at stereo.
		Container: "mp3", Audio: format.Audio{Codec: "mp3"}, Source: probe.Stream{Channels: 6, SampleRate: 48000},
		WantArgs: []string{"-c:a", "libmp3lame", "-ac", "2"},
	}, { // Test 6: 24-bit FLAC.
		Container: "flac", Audio: format.Audio{Codec: "flac", BitDepth: 24}, Source: probe.Stream{Channels: 2},
		WantArgs: []string{"-c:a", "flac", "-sample_fmt", "s32", "-bits_per_raw_sample", "24"},
	}, { // Test 7: 16-bit ALAC.
		Container: "alac", Audio: format.Audio{Codec: "alac", BitDepth: 16}, Source: probe.Stream{Channels: 2},
		WantArgs: []string{"-c:a", "alac", "-sample_fmt", "s16p"},
	}, { // Test 8: 24-bit ALAC skips AudioToolbox when the software encoder is missing.
		Container: "alac", Audio: format.Audio{Codec: "alac", BitDepth: 24}, Source: probe.Stream{Channels: 2},
		Avail: EncoderSet{"alac_at": true}, Want: ErrMissingEncoder,
	}, { // Test 9: AMV audio is mono 22.05 kHz in blocks of one video frame.
		Container: "amv", Audio: format.Audio{Codec: "adpcm_ima_amv"}, FPS: 25,
		Source:   probe.Stream{Channels: 2, SampleRate: 48000},
		WantArgs: []string{"-c:a", "adpcm_ima_amv", "-ac", "1", "-ar", "22050", "-block_size", "882"},
	}, { // Test 10: SWF with an unknown source rate takes the container's nearest rate.
		Container: "swf", Audio: format.Audio{Codec: "mp3"}, Source: probe.Stream{Channels: 2},
		WantArgs: []string{"-c:a", "libmp3lame", "-ar", "44100"},
	}, { // Test 11: Copy.
		Container: "mkv", Audio: format.Audio{Codec: format.Copy}, WantArgs: []string{"-c:a", "copy"},
	}, { // Test 12: Unknown codec.
		Container: "mkv", Audio: format.Audio{Codec: "nope"}, Want: format.ErrUnknownCodec,
	}, { // Test 13: Vorbis takes the generic bitrate.
		Container: "ogg", Audio: format.Audio{Codec: "vorbis", Bitrate: 192_000},
		Source:   probe.Stream{Channels: 2, SampleRate: 48000},
		WantArgs: []string{"-c:a", "libvorbis", "-b:a", "192000"},
	}, { // Test 14: AMR-NB is mono 8 kHz.
		Container: "amr", Audio: format.Audio{Codec: "amr_nb", Bitrate: 12_200},
		Source:   probe.Stream{Channels: 2, SampleRate: 48000},
		WantArgs: []string{"-c:a", "libopencore_amrnb", "-b:a", "12200", "-ac", "1", "-ar", "8000"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			box, err := format.ContainerByID(test.Container)
			if err != nil {
				t.Fatal(err)
			}
			avail := full
			if test.Avail != nil {
				avail = test.Avail
			}
			got, err := planAudio(box, test.Audio, test.Source, test.FPS, avail)
			if !errors.Is(err, test.Want) {
				t.Fatalf("error = %v, want %v", err, test.Want)
			}
			if diff := cmp.Diff(test.WantArgs, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRateArg(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In       float64
		WantRate string
	}{
		{In: 29.97, WantRate: "30000/1001"},   // Test 0: NTSC video.
		{In: 23.976, WantRate: "24000/1001"},  // Test 1: NTSC film.
		{In: 59.94, WantRate: "60000/1001"},   // Test 2: NTSC high rate.
		{In: 25, WantRate: "25"},              // Test 3: PAL.
		{In: 24, WantRate: "24"},              // Test 4: Film.
		{In: 12.5, WantRate: "12.5"},          // Test 5: Fractional.
		{In: 119.88, WantRate: "120000/1001"}, // Test 6: NTSC 120.
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := rateArg(test.In); got != test.WantRate {
				t.Errorf("rate = %q, want %q", got, test.WantRate)
			}
		})
	}
}

func TestMuxArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Container string
		WantArgs  []string
	}{{ // Test 0: MP4 moves the index to the front.
		Container: "mp4", WantArgs: []string{"-map_metadata", "0", "-movflags", "+faststart", "-f", "mp4", "o"},
	}, { // Test 1: M2TS writes Blu-ray packets.
		Container: "m2ts", WantArgs: []string{"-map_metadata", "0", "-mpegts_m2ts_mode", "1", "-f", "mpegts", "o"},
	}, { // Test 2: DV ends with its shortest stream.
		Container: "dv", WantArgs: []string{"-map_metadata", "0", "-shortest", "-f", "dv", "o"},
	}, { // Test 3: Ringtones name the muxer ffmpeg cannot guess from the extension.
		Container: "m4r", WantArgs: []string{"-map_metadata", "0", "-movflags", "+faststart", "-f", "ipod", "o"},
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			box, err := format.ContainerByID(test.Container)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.WantArgs, muxArgs(box, Streams{}, "o")); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

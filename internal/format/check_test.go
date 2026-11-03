package format

import (
	"errors"
	"fmt"
	"testing"
)

// h264 returns a valid H.264 quality video for test targets.
func h264() Video {
	return Video{Codec: "h264", RateControl: RateQuality, Quality: QualityBalanced}
}

func TestCheck(t *testing.T) { //nolint:funlen,maintidx // Test function.
	t.Parallel()
	tests := []struct {
		Want error
		In   Target
	}{{ // Test 0: Plain MP4 passes.
		In: Target{Container: "mp4", Video: h264(), Audio: Audio{Codec: "aac"}},
	}, { // Test 1: Unknown container.
		In: Target{Container: "betamax", Video: h264()}, Want: ErrUnknownContainer,
	}, { // Test 2: No streams.
		In: Target{Container: "mp4"}, Want: ErrSetting,
	}, { // Test 3: Negative target size.
		In: Target{Container: "mp4", Video: h264(), TargetBytes: -1}, Want: ErrSetting,
	}, { // Test 4: Unknown video codec.
		In: Target{Container: "mp4", Video: Video{Codec: "cinepak"}}, Want: ErrUnknownCodec,
	}, { // Test 5: VP9 in WebM passes.
		In: Target{Container: "webm", Video: Video{Codec: "vp9", RateControl: RateQuality, Quality: 1}},
	}, { // Test 6: H.264 in WebM fails.
		In: Target{Container: "webm", Video: h264()}, Want: ErrSetting,
	}, { // Test 7: Audio codec in the video slot fails.
		In: Target{Container: "mp4", Video: Video{Codec: "aac"}}, Want: ErrSetting,
	}, { // Test 8: Video in an audio container fails.
		In: Target{Container: "m4a", Video: h264(), Audio: Audio{Codec: "aac"}}, Want: ErrSetting,
	}, { // Test 9: Plain copy passes.
		In: Target{Container: "mkv", Video: Video{Codec: Copy}, Audio: Audio{Codec: Copy}},
	}, { // Test 10: Copy with a frame rate fails.
		In: Target{Container: "mkv", Video: Video{Codec: Copy, FPS: 25}}, Want: ErrSetting,
	}, { // Test 11: Target size on copied video fails.
		In: Target{Container: "mkv", Video: Video{Codec: Copy}, TargetBytes: 1}, Want: ErrSetting,
	}, { // Test 12: Target size on audio-only output fails.
		In: Target{Container: "m4a", Audio: Audio{Codec: "aac"}, TargetBytes: 1}, Want: ErrSetting,
	}, { // Test 13: Even exact size passes.
		In: Target{Container: "mp4", Video: withSize(h264(), 1280, 720)},
	}, { // Test 14: Width alone passes.
		In: Target{Container: "mp4", Video: withSize(h264(), 1280, 0)},
	}, { // Test 15: Odd width fails.
		In: Target{Container: "mp4", Video: withSize(h264(), 1279, 720)}, Want: ErrSetting,
	}, { // Test 16: Width below the domain fails.
		In: Target{Container: "mp4", Video: withSize(h264(), 30, 30)}, Want: ErrSetting,
	}, { // Test 17: Width above the domain fails.
		In: Target{Container: "mp4", Video: withSize(h264(), 8194, 720)}, Want: ErrSetting,
	}, { // Test 18: Negative box fails.
		In: Target{Container: "mp4", Video: withBox(h264(), -1, 0)}, Want: ErrSetting,
	}, { // Test 19: Size and box together fail.
		In:   Target{Container: "mp4", Video: withBox(withSize(h264(), 1280, 720), 1920, 1080)},
		Want: ErrSetting,
	}, { // Test 20: 240 fps passes.
		In: Target{Container: "mp4", Video: withFPS(h264(), 240)},
	}, { // Test 21: 300 fps fails.
		In: Target{Container: "mp4", Video: withFPS(h264(), 300)}, Want: ErrSetting,
	}, { // Test 22: Negative fps fails.
		In: Target{Container: "mp4", Video: withFPS(h264(), -1)}, Want: ErrSetting,
	}, { // Test 23: MPEG-1 at 25 fps passes.
		In: Target{Container: "mpg", Video: Video{Codec: "mpeg1video", RateControl: RateCBR,
			Bitrate: 1_000_000, FPS: 25}},
	}, { // Test 24: MPEG-1 at 7 fps fails.
		In: Target{Container: "mpg", Video: Video{Codec: "mpeg1video", RateControl: RateCBR,
			Bitrate: 1_000_000, FPS: 7}},
		Want: ErrSetting,
	}, { // Test 25: H.264 High profile at level 4.1 passes.
		In: Target{Container: "mp4", Video: withProfile(h264(), "high", "4.1")},
	}, { // Test 26: H.264 has no Main 10 profile.
		In: Target{Container: "mp4", Video: withProfile(h264(), "main10", "")}, Want: ErrSetting,
	}, { // Test 27: H.264 has no level 7.0.
		In: Target{Container: "mp4", Video: withProfile(h264(), "", "7.0")}, Want: ErrSetting,
	}, { // Test 28: MPEG-4 tagged XVID passes.
		In: Target{Container: "avi", Video: Video{Codec: "mpeg4", RateControl: RateVBR,
			Bitrate: 2_000_000, Tag: "XVID"}},
	}, { // Test 29: H.264 has no XVID tag.
		In: Target{Container: "avi", Video: Video{Codec: "h264", RateControl: RateQuality, Quality: 1,
			Tag: "XVID"}},
		Want: ErrSetting,
	}, { // Test 30: Quality level out of range.
		In:   Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateQuality, Quality: 4}},
		Want: ErrSetting,
	}, { // Test 31: Missing rate control fails.
		In: Target{Container: "mp4", Video: Video{Codec: "h264"}}, Want: ErrSetting,
	}, { // Test 32: AV1 has no constant bitrate.
		In: Target{Container: "mp4", Video: Video{Codec: "av1", RateControl: RateCBR,
			Bitrate: 2_000_000}},
		Want: ErrSetting,
	}, { // Test 33: Quality rate control needs a level.
		In:   Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateQuality}},
		Want: ErrSetting,
	}, { // Test 34: Quality rate control takes no bitrate.
		In: Target{Container: "mp4", Video: withBitrate(h264(), 2_000_000)}, Want: ErrSetting,
	}, { // Test 35: Constant bitrate passes with a bitrate.
		In: Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateCBR,
			Bitrate: 2_000_000}},
	}, { // Test 36: Constant bitrate needs a bitrate.
		In:   Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateCBR}},
		Want: ErrSetting,
	}, { // Test 37: Negative bitrate fails.
		In:   Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateVBR, Bitrate: -1}},
		Want: ErrSetting,
	}, { // Test 38: Two-pass at a bitrate passes.
		In: Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateTwoPass,
			Bitrate: 2_000_000}},
	}, { // Test 39: Two-pass to a target size passes.
		In: Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateTwoPass},
			TargetBytes: 8_000_000},
	}, { // Test 40: Two-pass with neither bitrate nor size fails.
		In:   Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateTwoPass}},
		Want: ErrSetting,
	}, { // Test 41: Two-pass with both bitrate and size fails.
		In: Target{Container: "mp4", Video: Video{Codec: "h264", RateControl: RateTwoPass,
			Bitrate: 2_000_000}, TargetBytes: 8_000_000},
		Want: ErrSetting,
	}, { // Test 42: Target size needs two-pass.
		In: Target{Container: "mp4", Video: h264(), TargetBytes: 8_000_000}, Want: ErrSetting,
	}, { // Test 43: AV1 cannot aim for a size.
		In: Target{Container: "mp4", Video: Video{Codec: "av1", RateControl: RateTwoPass},
			TargetBytes: 8_000_000},
		Want: ErrSetting,
	}, { // Test 44: ProRes HQ passes with no rate control.
		In: Target{Container: "mov", Video: Video{Codec: "prores", Profile: "hq"}},
	}, { // Test 45: ProRes takes no rate control.
		In:   Target{Container: "mov", Video: Video{Codec: "prores", RateControl: RateCBR}},
		Want: ErrSetting,
	}, { // Test 46: ProRes sets its own bitrate.
		In:   Target{Container: "mov", Video: Video{Codec: "prores", Bitrate: 100_000_000}},
		Want: ErrSetting,
	}, { // Test 47: ProRes cannot aim for a size.
		In:   Target{Container: "mov", Video: Video{Codec: "prores"}, TargetBytes: 8_000_000},
		Want: ErrSetting,
	}, { // Test 48: Legacy DNxHD at a listed bitrate passes.
		In: Target{Container: "mov", Video: Video{Codec: "dnxhd", Profile: "dnxhd", Width: 1920,
			Height: 1080, Bitrate: 145_000_000}},
	}, { // Test 49: Legacy DNxHD at an unlisted bitrate fails.
		In: Target{Container: "mov", Video: Video{Codec: "dnxhd", Profile: "dnxhd", Width: 1920,
			Height: 1080, Bitrate: 50_000_000}},
		Want: ErrMode,
	}, { // Test 50: DNxHD needs a profile, since its sizes depend on it.
		In:   Target{Container: "mov", Video: Video{Codec: "dnxhd", Width: 1920, Height: 1080}},
		Want: ErrSetting,
	}, { // Test 51: DNxHR at UHD passes.
		In: Target{Container: "mov", Video: Video{Codec: "dnxhd", Profile: "dnxhr_sq", Width: 3840,
			Height: 2160}},
	}, { // Test 52: DNxHR sets its own bitrate.
		In: Target{Container: "mov", Video: Video{Codec: "dnxhd", Profile: "dnxhr_sq",
			Bitrate: 145_000_000}},
		Want: ErrSetting,
	}, { // Test 53: DV PAL at 25 fps passes.
		In: Target{Container: "dv", Video: Video{Codec: "dvvideo", Width: 720, Height: 576, FPS: 25}},
	}, { // Test 54: DV PAL at 29.97 fps fails.
		In: Target{Container: "dv", Video: Video{Codec: "dvvideo", Width: 720, Height: 576,
			FPS: 29.97}},
		Want: ErrMode,
	}, { // Test 55: DV takes no fit box.
		In:   Target{Container: "dv", Video: Video{Codec: "dvvideo", MaxWidth: 720, MaxHeight: 576}},
		Want: ErrSetting,
	}, { // Test 56: DVD PAL in VOB passes.
		In: Target{Container: "vob", Video: dvd(720, 576, 25), Audio: Audio{Codec: "ac3"}},
	}, { // Test 57: VOB refuses HD.
		In: Target{Container: "vob", Video: dvd(1920, 1080, 0)}, Want: ErrMode,
	}, { // Test 58: DVD PAL forces 25 fps.
		In: Target{Container: "vob", Video: dvd(720, 576, 29.97)}, Want: ErrMode,
	}, { // Test 59: VOB takes no fit box.
		In: Target{Container: "vob", Video: withBox(dvd(0, 0, 0), 720, 576)}, Want: ErrSetting,
	}, { // Test 60: MPEG-2 in TS has no fixed sizes.
		In: Target{Container: "ts", Video: dvd(1920, 1080, 0)},
	}, { // Test 61: MP3 at 44.1 kHz passes.
		In: Target{Container: "mp3", Audio: Audio{Codec: "mp3", SampleRate: 44100}},
	}, { // Test 62: MP3 has no 96 kHz.
		In: Target{Container: "mp3", Audio: Audio{Codec: "mp3", SampleRate: 96000}}, Want: ErrSetting,
	}, { // Test 63: SWF takes 22.05 kHz MP3.
		In: Target{Container: "swf", Video: Video{Codec: "flv1", RateControl: RateVBR,
			Bitrate: 500_000}, Audio: Audio{Codec: "mp3", SampleRate: 22050}},
	}, { // Test 64: SWF refuses 48 kHz.
		In: Target{Container: "swf", Video: Video{Codec: "flv1", RateControl: RateVBR,
			Bitrate: 500_000}, Audio: Audio{Codec: "mp3", SampleRate: 48000}},
		Want: ErrSetting,
	}, { // Test 65: AAC at 96 kHz passes through the software encoder.
		In: Target{Container: "m4a", Audio: Audio{Codec: "aac", SampleRate: 96000}},
	}, { // Test 66: A rate outside the domain fails.
		In: Target{Container: "m4a", Audio: Audio{Codec: "aac", SampleRate: 12345}}, Want: ErrSetting,
	}, { // Test 67: Stereo MP3 passes.
		In: Target{Container: "mp3", Audio: Audio{Codec: "mp3", Channels: 2}},
	}, { // Test 68: MP3 has no 5.1.
		In: Target{Container: "mp3", Audio: Audio{Codec: "mp3", Channels: 6}}, Want: ErrSetting,
	}, { // Test 69: Three channels is not a layout.
		In: Target{Container: "m4a", Audio: Audio{Codec: "aac", Channels: 3}}, Want: ErrSetting,
	}, { // Test 70: MP3 at 320 kb/s passes.
		In: Target{Container: "mp3", Audio: Audio{Codec: "mp3", Bitrate: 320_000}},
	}, { // Test 71: MP3 at 384 kb/s fails.
		In: Target{Container: "mp3", Audio: Audio{Codec: "mp3", Bitrate: 384_000}}, Want: ErrSetting,
	}, { // Test 72: MPEG-2 audio rates cap MP3 at 160 kb/s.
		In:   Target{Container: "mp3", Audio: Audio{Codec: "mp3", SampleRate: 22050, Bitrate: 192_000}},
		Want: ErrSetting,
	}, { // Test 73: Opus at 600 kb/s fails in stereo.
		In:   Target{Container: "opus", Audio: Audio{Codec: "opus", Channels: 2, Bitrate: 600_000}},
		Want: ErrSetting,
	}, { // Test 74: Opus at 600 kb/s passes when the source layout decides.
		In: Target{Container: "opus", Audio: Audio{Codec: "opus", Bitrate: 600_000}},
	}, { // Test 75: AMR-NB at a listed rate passes.
		In: Target{Container: "amr", Audio: Audio{Codec: "amr_nb", Bitrate: 12_200}},
	}, { // Test 76: AMR-NB at an unlisted rate fails.
		In: Target{Container: "amr", Audio: Audio{Codec: "amr_nb", Bitrate: 12_000}}, Want: ErrSetting,
	}, { // Test 77: Lossless audio takes no bitrate.
		In: Target{Container: "flac", Audio: Audio{Codec: "flac", Bitrate: 320_000}}, Want: ErrSetting,
	}, { // Test 78: Negative audio bitrate fails.
		In: Target{Container: "m4a", Audio: Audio{Codec: "aac", Bitrate: -1}}, Want: ErrSetting,
	}, { // Test 79: 24-bit FLAC passes.
		In: Target{Container: "flac", Audio: Audio{Codec: "flac", BitDepth: 24}},
	}, { // Test 80: 32-bit FLAC fails.
		In: Target{Container: "flac", Audio: Audio{Codec: "flac", BitDepth: 32}}, Want: ErrSetting,
	}, { // Test 81: AAC has no bit depth.
		In: Target{Container: "m4a", Audio: Audio{Codec: "aac", BitDepth: 16}}, Want: ErrSetting,
	}, { // Test 82: Copied audio takes no other settings.
		In: Target{Container: "mkv", Audio: Audio{Codec: Copy, Bitrate: 128_000}}, Want: ErrSetting,
	}, { // Test 83: Unknown audio codec.
		In: Target{Container: "m4a", Audio: Audio{Codec: "atrac"}}, Want: ErrUnknownCodec,
	}, { // Test 84: Opus refuses 44.1 kHz.
		In:   Target{Container: "opus", Audio: Audio{Codec: "opus", SampleRate: 44100}},
		Want: ErrSetting,
	}, { // Test 85: DV carries stereo.
		In: Target{Container: "dv", Video: Video{Codec: "dvvideo"},
			Audio: Audio{Codec: "pcm_s16le", Channels: 2}},
	}, { // Test 86: DV refuses mono.
		In: Target{Container: "dv", Video: Video{Codec: "dvvideo"},
			Audio: Audio{Codec: "pcm_s16le", Channels: 1}},
		Want: ErrSetting,
	}, { // Test 87: DV refuses 44.1 kHz.
		In: Target{Container: "dv", Video: Video{Codec: "dvvideo"},
			Audio: Audio{Codec: "pcm_s16le", SampleRate: 44100}},
		Want: ErrSetting,
	}, { // Test 88: AMV at 25 fps passes.
		In: Target{Container: "amv", Video: Video{Codec: "amv", RateControl: RateVBR, Bitrate: 500_000,
			FPS: 25}},
	}, { // Test 89: AMV at 24 fps fails, since 22050 Hz audio does not split into 24 blocks.
		In: Target{Container: "amv", Video: Video{Codec: "amv", RateControl: RateVBR, Bitrate: 500_000,
			FPS: 24}},
		Want: ErrSetting,
	}, { // Test 90: DV audio only is refused.
		In:   Target{Container: "dv", Audio: Audio{Codec: "pcm_s16le", SampleRate: 48000, Channels: 2}},
		Want: ErrSetting,
	}, { // Test 91: AMV audio only is refused.
		In:   Target{Container: "amv", Audio: Audio{Codec: "adpcm_ima_amv"}},
		Want: ErrSetting,
	}, { // Test 92: MP4 audio only passes.
		In: Target{Container: "mp4", Audio: Audio{Codec: "aac"}},
	}, { // Test 93: MXF audio only is refused.
		In:   Target{Container: "mxf", Audio: Audio{Codec: "pcm_s16le"}},
		Want: ErrSetting,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if err := Check(test.In); !errors.Is(err, test.Want) {
				t.Errorf("error = %v, want %v", err, test.Want)
			}
		})
	}
}

func TestCheckSampleRateEncoders(t *testing.T) {
	t.Parallel()
	capped := Codec{ID: "x", Encoders: []Encoder{{Name: "x_at", MaxSampleRate: 48000}}}
	open := Codec{ID: "x", Encoders: []Encoder{{Name: "x_at", MaxSampleRate: 48000}, {Name: "x"}}}
	tests := []struct {
		Want  error
		Codec Codec
		Rate  int
	}{{ // Test 0: The capped encoder takes 48 kHz.
		Codec: capped, Rate: 48000,
	}, { // Test 1: No encoder takes 96 kHz.
		Codec: capped, Rate: 96000, Want: ErrSetting,
	}, { // Test 2: A second encoder without a cap takes 96 kHz.
		Codec: open, Rate: 96000,
	}, { // Test 3: Same as source is always allowed.
		Codec: capped,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			err := checkSampleRate(Container{ID: "c"}, test.Codec, test.Rate)
			if !errors.Is(err, test.Want) {
				t.Errorf("error = %v, want %v", err, test.Want)
			}
		})
	}
}

// withSize returns v with an exact frame size.
func withSize(v Video, w, h int) Video {
	v.Width, v.Height = w, h
	return v
}

// withBox returns v with a fit box.
func withBox(v Video, w, h int) Video {
	v.MaxWidth, v.MaxHeight = w, h
	return v
}

// withFPS returns v with a frame rate.
func withFPS(v Video, fps float64) Video {
	v.FPS = fps
	return v
}

// withProfile returns v with a profile and level.
func withProfile(v Video, profile, level string) Video {
	v.Profile, v.Level = profile, level
	return v
}

// withBitrate returns v with a bitrate.
func withBitrate(v Video, bps int) Video {
	v.Bitrate = bps
	return v
}

// dvd returns a constant-bitrate MPEG-2 video at a frame size and rate.
func dvd(w, h int, fps float64) Video {
	return Video{Codec: "mpeg2video", RateControl: RateCBR, Bitrate: 6_000_000, Width: w, Height: h,
		FPS: fps}
}

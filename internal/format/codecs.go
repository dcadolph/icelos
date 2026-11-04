package format

// Rate control sets shared by codecs whose encoders behave alike.
var ( //nolint:gochecknoglobals // Read-only catalog data.
	rateAll     = []RateControl{RateQuality, RateCBR, RateVBR, RateTwoPass}
	rateBitrate = []RateControl{RateCBR, RateVBR, RateTwoPass}
)

// Profile, level, and size lists shared by more than one codec or read by tests.
var ( //nolint:gochecknoglobals // Read-only catalog data.
	h264Levels = []string{"3.0", "3.1", "3.2", "4.0", "4.1", "4.2", "5.0", "5.1", "5.2", "6.0", "6.1",
		"6.2"}
	hevcLevels = []string{"3.1", "4.0", "4.1", "5.0", "5.1", "5.2", "6.0", "6.1", "6.2"}
	h263Modes  = []Mode{
		{Name: "SQCIF", Width: 128, Height: 96},
		{Name: "QCIF", Width: 176, Height: 144},
		{Name: "CIF", Width: 352, Height: 288},
		{Name: "4CIF", Width: 704, Height: 576},
		{Name: "16CIF", Width: 1408, Height: 1152},
	}
	amvModes = []Mode{
		{Name: "128x96", Width: 128, Height: 96},
		{Name: "128x128", Width: 128, Height: 128},
		{Name: "160x128", Width: 160, Height: 128},
		{Name: "176x144", Width: 176, Height: 144},
		{Name: "208x176", Width: 208, Height: 176},
		{Name: "220x176", Width: 220, Height: 176},
		{Name: "320x240", Width: 320, Height: 240},
	}
	// dnxhdModes are the frame sizes and bitrates of legacy 8-bit DNxHD, from the list ffmpeg's
	// dnxhd encoder prints when it refuses a size. DNxHR profiles have no fixed modes.
	dnxhdModes = []Mode{
		{Name: "1080p", Profile: "dnxhd", Width: 1920, Height: 1080, Bitrates: mbps(36, 45, 75, 90,
			115, 120, 145, 175, 185, 220, 240, 290, 365, 440)},
		{Name: "1080p anamorphic", Profile: "dnxhd", Width: 1440, Height: 1080,
			Bitrates: mbps(63, 84, 100, 110)},
		{Name: "720p", Profile: "dnxhd", Width: 1280, Height: 720,
			Bitrates: mbps(60, 75, 90, 110, 120, 145, 180, 220)},
		{Name: "720p anamorphic", Profile: "dnxhd", Width: 960, Height: 720,
			Bitrates: mbps(60, 75, 115)},
	}
	audioRates = []int{8000, 11025, 12000, 16000, 22050, 24000, 32000, 44100, 48000}
	aacRates   = []int{8000, 11025, 12000, 16000, 22050, 24000, 32000, 44100, 48000, 64000, 88200, 96000}
	dolbyRates = []int{32000, 44100, 48000}
)

// codecs is the catalog of output codecs: video, then audio, then subtitles.
var codecs = []Codec{ //nolint:gochecknoglobals // Read-only catalog data.
	{ID: "h264", Name: "H.264", Kind: KindVideo,
		Encoders: []Encoder{{Name: "libx264"}, {Name: "h264_videotoolbox", Hardware: true},
			{Name: "h264_nvenc", Hardware: true}, {Name: "h264_qsv", Hardware: true},
			{Name: "h264_amf", Hardware: true}},
		RateControls: rateAll, PixFmt: "yuv420p", Levels: h264Levels,
		Profiles: []Profile{{ID: "baseline", Name: "Baseline"}, {ID: "main", Name: "Main"},
			{ID: "high", Name: "High"}}},
	{ID: "hevc", Name: "HEVC (H.265)", Kind: KindVideo,
		Encoders: []Encoder{{Name: "libx265"}, {Name: "hevc_videotoolbox", Hardware: true},
			{Name: "hevc_nvenc", Hardware: true}, {Name: "hevc_qsv", Hardware: true},
			{Name: "hevc_amf", Hardware: true}},
		RateControls: rateAll, PixFmt: "yuv420p", Levels: hevcLevels,
		Profiles: []Profile{{ID: "main", Name: "Main"},
			{ID: "main10", Name: "Main 10", PixFmt: "yuv420p10le"}}},
	{ID: "av1", Name: "AV1", Kind: KindVideo,
		Encoders: []Encoder{{Name: "libsvtav1"}, {Name: "av1_nvenc", Hardware: true},
			{Name: "av1_qsv", Hardware: true}, {Name: "av1_amf", Hardware: true}},
		RateControls: []RateControl{RateQuality, RateVBR}, PixFmt: "yuv420p"},
	{ID: "vp9", Name: "VP9", Kind: KindVideo, Encoders: []Encoder{{Name: "libvpx-vp9"}},
		RateControls: rateAll, PixFmt: "yuv420p"},
	{ID: "vp8", Name: "VP8", Kind: KindVideo, Encoders: []Encoder{{Name: "libvpx"}},
		RateControls: rateAll, PixFmt: "yuv420p"},
	{ID: "mpeg4", Name: "MPEG-4 Part 2", Kind: KindVideo, Encoders: []Encoder{{Name: "mpeg4"}},
		RateControls: rateAll, PixFmt: "yuv420p", Tags: []string{"DIVX", "XVID"}},
	{ID: "msmpeg4v3", Name: "Microsoft MPEG-4 v3", Kind: KindVideo,
		Encoders: []Encoder{{Name: "msmpeg4"}}, RateControls: rateBitrate, PixFmt: "yuv420p"},
	{ID: "wmv1", Name: "Windows Media Video 7", Kind: KindVideo, Encoders: []Encoder{{Name: "wmv1"}},
		RateControls: rateBitrate, PixFmt: "yuv420p"},
	{ID: "wmv2", Name: "Windows Media Video 8", Kind: KindVideo, Encoders: []Encoder{{Name: "wmv2"}},
		RateControls: rateBitrate, PixFmt: "yuv420p"},
	{ID: "mpeg1video", Name: "MPEG-1", Kind: KindVideo, Encoders: []Encoder{{Name: "mpeg1video"}},
		RateControls: rateBitrate, PixFmt: "yuv420p",
		FrameRates: []float64{12, 15, 23.976, 24, 25, 29.97, 30, 50, 59.94, 60}},
	{ID: "mpeg2video", Name: "MPEG-2", Kind: KindVideo, Encoders: []Encoder{{Name: "mpeg2video"}},
		RateControls: rateBitrate, PixFmt: "yuv420p"},
	{ID: "dvvideo", Name: "DV", Kind: KindVideo, Encoders: []Encoder{{Name: "dvvideo"}},
		Modes: []Mode{
			{Name: "NTSC", Width: 720, Height: 480, FPS: 29.97, PixFmt: "yuv411p"},
			{Name: "PAL", Width: 720, Height: 576, FPS: 25, PixFmt: "yuv420p"},
		}},
	{ID: "h263", Name: "H.263", Kind: KindVideo, Encoders: []Encoder{{Name: "h263"}},
		RateControls: rateBitrate, PixFmt: "yuv420p", Modes: h263Modes},
	{ID: "flv1", Name: "Sorenson Spark (FLV1)", Kind: KindVideo, Encoders: []Encoder{{Name: "flv"}},
		RateControls: rateBitrate, PixFmt: "yuv420p"},
	{ID: "amv", Name: "AMV", Kind: KindVideo, Encoders: []Encoder{{Name: "amv"}},
		RateControls: []RateControl{RateVBR}, PixFmt: "yuvj420p", Modes: amvModes,
		FrameRates: []float64{15, 25, 30}},
	{ID: "mjpeg", Name: "Motion JPEG", Kind: KindVideo, Encoders: []Encoder{{Name: "mjpeg"}},
		RateControls: []RateControl{RateQuality, RateVBR}, PixFmt: "yuvj420p"},
	{ID: "theora", Name: "Theora", Kind: KindVideo, Encoders: []Encoder{{Name: "libtheora"}},
		RateControls: []RateControl{RateQuality, RateVBR}, PixFmt: "yuv420p"},
	{ID: "ffv1", Name: "FFV1", Kind: KindVideo, Encoders: []Encoder{{Name: "ffv1"}}, Lossless: true},
	{ID: "prores", Name: "Apple ProRes", Kind: KindVideo,
		Encoders: []Encoder{{Name: "prores_ks"}, {Name: "prores_videotoolbox", Hardware: true}},
		PixFmt:   "yuv422p10le",
		Profiles: []Profile{{ID: "proxy", Name: "422 Proxy"}, {ID: "lt", Name: "422 LT"},
			{ID: "standard", Name: "422"}, {ID: "hq", Name: "422 HQ"},
			{ID: "4444", Name: "4444", PixFmt: "yuv444p10le"},
			{ID: "4444xq", Name: "4444 XQ", PixFmt: "yuv444p10le"}}},
	{ID: "dnxhd", Name: "Avid DNxHD and DNxHR", Kind: KindVideo, Encoders: []Encoder{{Name: "dnxhd"}},
		PixFmt: "yuv422p", Modes: dnxhdModes,
		Profiles: []Profile{{ID: "dnxhd", Name: "DNxHD"}, {ID: "dnxhr_lb", Name: "DNxHR LB"},
			{ID: "dnxhr_sq", Name: "DNxHR SQ"}, {ID: "dnxhr_hq", Name: "DNxHR HQ"},
			{ID: "dnxhr_hqx", Name: "DNxHR HQX", PixFmt: "yuv422p10le"},
			{ID: "dnxhr_444", Name: "DNxHR 444", PixFmt: "yuv444p10le"}}},
	{ID: "cfhd", Name: "GoPro CineForm", Kind: KindVideo, Encoders: []Encoder{{Name: "cfhd"}},
		RateControls: []RateControl{RateQuality}, PixFmt: "yuv422p10le"},

	{ID: "aac", Name: "AAC", Kind: KindAudio,
		Encoders:    []Encoder{{Name: "aac_at", MaxSampleRate: 48000}, {Name: "aac"}},
		SampleRates: aacRates, MaxChannels: 8, BitrateCaps: []BitrateCap{{PerSample: 6}}},
	{ID: "mp3", Name: "MP3", Kind: KindAudio, Encoders: []Encoder{{Name: "libmp3lame"}},
		SampleRates: audioRates, MaxChannels: 2, BitrateCaps: mpegCaps(64_000, 160_000, 320_000)},
	{ID: "ac3", Name: "Dolby Digital (AC-3)", Kind: KindAudio, Encoders: []Encoder{{Name: "ac3"}},
		SampleRates: dolbyRates, MaxChannels: 6, BitrateCaps: []BitrateCap{{Total: 640_000}}},
	{ID: "eac3", Name: "Dolby Digital Plus (E-AC-3)", Kind: KindAudio,
		Encoders: []Encoder{{Name: "eac3"}}, SampleRates: dolbyRates, MaxChannels: 6,
		BitrateCaps: []BitrateCap{{Total: 6_144_000}}},
	{ID: "flac", Name: "FLAC", Kind: KindAudio, Encoders: []Encoder{{Name: "flac"}}, Lossless: true,
		MaxChannels: 8, BitDepths: []int{16, 24}},
	{ID: "alac", Name: "Apple Lossless", Kind: KindAudio,
		Encoders: []Encoder{{Name: "alac"}, {Name: "alac_at"}}, Lossless: true, MaxChannels: 8,
		BitDepths: []int{16, 24}},
	{ID: "vorbis", Name: "Vorbis", Kind: KindAudio, Encoders: []Encoder{{Name: "libvorbis"}},
		MaxChannels: 8, BitrateCaps: []BitrateCap{{Total: 500_000}}},
	{ID: "opus", Name: "Opus", Kind: KindAudio, Encoders: []Encoder{{Name: "libopus"}},
		SampleRates: []int{8000, 12000, 16000, 24000, 48000}, MaxChannels: 8,
		BitrateCaps: []BitrateCap{{PerChannel: 256_000}}},
	{ID: "wmav2", Name: "Windows Media Audio 2", Kind: KindAudio, Encoders: []Encoder{{Name: "wmav2"}},
		SampleRates: audioRates, MaxChannels: 2, BitrateCaps: []BitrateCap{{Total: 320_000}}},
	{ID: "pcm_s16le", Name: "PCM 16-bit", Kind: KindAudio, Encoders: []Encoder{{Name: "pcm_s16le"}},
		Lossless: true, MaxChannels: 8, BitDepths: []int{16}},
	{ID: "pcm_s24le", Name: "PCM 24-bit", Kind: KindAudio, Encoders: []Encoder{{Name: "pcm_s24le"}},
		Lossless: true, MaxChannels: 8, BitDepths: []int{24}},
	{ID: "pcm_s16be", Name: "PCM 16-bit big-endian", Kind: KindAudio,
		Encoders: []Encoder{{Name: "pcm_s16be"}}, Lossless: true, MaxChannels: 8, BitDepths: []int{16}},
	{ID: "pcm_s24be", Name: "PCM 24-bit big-endian", Kind: KindAudio,
		Encoders: []Encoder{{Name: "pcm_s24be"}}, Lossless: true, MaxChannels: 8, BitDepths: []int{24}},
	{ID: "amr_nb", Name: "AMR-NB", Kind: KindAudio, Encoders: []Encoder{{Name: "libopencore_amrnb"}},
		SampleRates: []int{8000}, MaxChannels: 1,
		Bitrates: []int{4750, 5150, 5900, 6700, 7400, 7950, 10_200, 12_200}},
	{ID: "mp2", Name: "MP2", Kind: KindAudio, Encoders: []Encoder{{Name: "mp2"}},
		SampleRates: []int{16000, 22050, 24000, 32000, 44100, 48000}, MaxChannels: 2,
		BitrateCaps: mpegCaps(160_000, 160_000, 384_000)},
	{ID: "adpcm_ima_amv", Name: "AMV ADPCM", Kind: KindAudio,
		Encoders: []Encoder{{Name: "adpcm_ima_amv"}}, SampleRates: []int{22050}, MaxChannels: 1},

	{ID: "mov_text", Name: "MP4 text", Kind: KindSubtitle, Encoders: []Encoder{{Name: "mov_text"}}},
	{ID: "subrip", Name: "SubRip (SRT)", Kind: KindSubtitle, Encoders: []Encoder{{Name: "srt"}}},
	{ID: "ass", Name: "ASS", Kind: KindSubtitle, Encoders: []Encoder{{Name: "ass"}}},
	{ID: "webvtt", Name: "WebVTT", Kind: KindSubtitle, Encoders: []Encoder{{Name: "webvtt"}}},
}

// mbps converts a list of megabit rates to bits per second.
func mbps(rates ...int) []int {
	out := make([]int, len(rates))
	for i, r := range rates {
		out[i] = r * 1_000_000
	}
	return out
}

// mpegCaps returns the bitrate caps of an MPEG audio layer: low below 16 kHz (MPEG-2.5), mid from
// 16 kHz (MPEG-2), and high from 32 kHz (MPEG-1).
func mpegCaps(low, mid, high int) []BitrateCap {
	return []BitrateCap{{Total: low}, {MinRate: 16000, Total: mid}, {MinRate: 32000, Total: high}}
}

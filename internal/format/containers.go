package format

// Codec ID lists shared by containers that carry the same codecs.
var ( //nolint:gochecknoglobals // Read-only catalog data.
	mp4Video = []string{"h264", "hevc", "av1", "vp9", "mpeg4"}
	mp4Audio = []string{"aac", "alac", "mp3", "ac3", "eac3", "flac", "opus"}
	movVideo = []string{"h264", "hevc", "prores", "mpeg4", "mjpeg", "dvvideo", "dnxhd", "cfhd"}
	movAudio = []string{"aac", "alac", "pcm_s16le", "pcm_s24le", "mp3", "ac3", "eac3"}
	movSubs  = []string{"mov_text"}
	movFlags = Flags{Faststart: true, HVC1Tag: true}
	mkvAudio = []string{"aac", "ac3", "eac3", "flac", "opus", "mp3", "alac", "vorbis", "mp2",
		"pcm_s16le", "pcm_s24le"}
	mkvSubs  = []string{"subrip", "ass", "webvtt"}
	tsVideo  = []string{"h264", "hevc", "mpeg2video", "mpeg1video"}
	tsAudio  = []string{"aac", "ac3", "eac3", "mp3", "mp2", "opus"}
	gppVideo = []string{"h264", "mpeg4", "h263"}
	gppAudio = []string{"aac", "amr_nb"}
	asfVideo = []string{"wmv2", "wmv1", "msmpeg4v3", "mpeg4"}
	asfAudio = []string{"wmav2", "mp3"}
	m4aAudio = []string{"aac", "alac"}
	oggAudio = []string{"vorbis", "opus", "flac"}
	pcmLE    = []string{"pcm_s16le", "pcm_s24le"}
	pcmBE    = []string{"pcm_s16be", "pcm_s24be"}
)

// dvdModes are the MPEG-2 frame sizes the DVD-Video spec allows, NTSC at 29.97 fps and PAL at 25.
var dvdModes = []Mode{ //nolint:gochecknoglobals // Read-only catalog data.
	{Name: "NTSC", Width: 720, Height: 480, FPS: 29.97},
	{Name: "NTSC 704", Width: 704, Height: 480, FPS: 29.97},
	{Name: "NTSC half D1", Width: 352, Height: 480, FPS: 29.97},
	{Name: "NTSC SIF", Width: 352, Height: 240, FPS: 29.97},
	{Name: "PAL", Width: 720, Height: 576, FPS: 25},
	{Name: "PAL 704", Width: 704, Height: 576, FPS: 25},
	{Name: "PAL half D1", Width: 352, Height: 576, FPS: 25},
	{Name: "PAL CIF", Width: 352, Height: 288, FPS: 25},
}

// containers is the catalog of output containers, video first, then audio-only.
var containers = []Container{ //nolint:gochecknoglobals // Read-only catalog data.
	{ID: "mp4", Name: "MP4", Ext: "mp4", Muxer: "mp4", Kind: KindVideo,
		Video: mp4Video, Audio: mp4Audio, Subtitles: movSubs, Flags: movFlags},
	{ID: "mov", Name: "MOV", Ext: "mov", Muxer: "mov", Kind: KindVideo,
		Video: movVideo, Audio: movAudio, Subtitles: movSubs, Flags: movFlags},
	{ID: "mkv", Name: "MKV", Ext: "mkv", Muxer: "matroska", Kind: KindVideo,
		Video: []string{"h264", "hevc", "av1", "vp9", "vp8", "mpeg4", "msmpeg4v3", "mpeg2video",
			"mpeg1video", "mjpeg", "theora", "ffv1", "prores", "dnxhd"},
		Audio: mkvAudio, Subtitles: mkvSubs},
	{ID: "m4v", Name: "M4V", Ext: "m4v", Muxer: "ipod", Kind: KindVideo,
		Video: []string{"h264", "mpeg4"}, Audio: []string{"aac", "alac", "ac3"},
		Subtitles: movSubs, Flags: movFlags},
	{ID: "avi", Name: "AVI", Ext: "avi", Muxer: "avi", Kind: KindVideo,
		Video: []string{"mpeg4", "msmpeg4v3", "h264", "mjpeg", "mpeg2video", "mpeg1video",
			"dvvideo", "ffv1"},
		Audio: []string{"mp3", "ac3", "pcm_s16le", "mp2", "aac"}},
	{ID: "wmv", Name: "WMV", Ext: "wmv", Muxer: "asf", Kind: KindVideo,
		Video: asfVideo, Audio: asfAudio},
	{ID: "webm", Name: "WebM", Ext: "webm", Muxer: "webm", Kind: KindVideo,
		Video: []string{"vp9", "vp8", "av1"}, Audio: []string{"opus", "vorbis"},
		Subtitles: []string{"webvtt"}},
	{ID: "flv", Name: "FLV", Ext: "flv", Muxer: "flv", Kind: KindVideo,
		Video: []string{"h264", "flv1"}, Audio: []string{"aac", "mp3"}},
	{ID: "f4v", Name: "F4V", Ext: "f4v", Muxer: "f4v", Kind: KindVideo,
		Video: []string{"h264"}, Audio: []string{"aac", "mp3"}},
	{ID: "mpg", Name: "MPG", Ext: "mpg", Muxer: "mpeg", Kind: KindVideo,
		Video: []string{"mpeg2video", "mpeg1video"}, Audio: []string{"mp2", "mp3", "ac3"}},
	{ID: "vob", Name: "VOB", Ext: "vob", Muxer: "vob", Kind: KindVideo,
		Video: []string{"mpeg2video"}, Audio: []string{"ac3", "mp2"}, Modes: dvdModes},
	{ID: "ts", Name: "TS", Ext: "ts", Muxer: "mpegts", Kind: KindVideo,
		Video: tsVideo, Audio: tsAudio},
	{ID: "trp", Name: "TRP", Ext: "trp", Muxer: "mpegts", Kind: KindVideo,
		Video: tsVideo, Audio: tsAudio},
	{ID: "m2ts", Name: "M2TS", Ext: "m2ts", Muxer: "mpegts", Kind: KindVideo,
		Video: []string{"h264", "hevc", "mpeg2video"}, Audio: []string{"ac3", "eac3", "aac", "mp2"},
		Flags: Flags{M2TS: true}},
	{ID: "3gp", Name: "3GP", Ext: "3gp", Muxer: "3gp", Kind: KindVideo,
		Video: gppVideo, Audio: gppAudio},
	{ID: "3g2", Name: "3G2", Ext: "3g2", Muxer: "3g2", Kind: KindVideo,
		Video: gppVideo, Audio: gppAudio},
	{ID: "mxf", Name: "MXF", Ext: "mxf", Muxer: "mxf", Kind: KindVideo,
		Video: []string{"mpeg2video", "dnxhd", "h264", "dvvideo", "prores"}, Audio: pcmLE},
	{ID: "asf", Name: "ASF", Ext: "asf", Muxer: "asf", Kind: KindVideo,
		Video: asfVideo, Audio: asfAudio},
	{ID: "dv", Name: "DV", Ext: "dv", Muxer: "dv", Kind: KindVideo,
		Video: []string{"dvvideo"}, Audio: []string{"pcm_s16le"}},
	{ID: "amv", Name: "AMV", Ext: "amv", Muxer: "amv", Kind: KindVideo,
		Video: []string{"amv"}, Audio: []string{"adpcm_ima_amv"}},
	{ID: "swf", Name: "SWF", Ext: "swf", Muxer: "swf", Kind: KindVideo,
		Video: []string{"flv1", "mjpeg"}, Audio: []string{"mp3"},
		SampleRates: []int{44100, 22050, 11025}},
	{ID: "ogv", Name: "OGV", Ext: "ogv", Muxer: "ogv", Kind: KindVideo,
		Video: []string{"theora", "vp8"}, Audio: oggAudio},

	{ID: "mp3", Name: "MP3", Ext: "mp3", Muxer: "mp3", Kind: KindAudio, Audio: []string{"mp3"}},
	{ID: "m4a", Name: "M4A", Ext: "m4a", Muxer: "ipod", Kind: KindAudio, Audio: m4aAudio,
		Flags: Flags{Faststart: true}},
	{ID: "m4r", Name: "M4R Ringtone", Ext: "m4r", Muxer: "ipod", Kind: KindAudio,
		Audio: []string{"aac"}, Flags: Flags{Faststart: true}},
	{ID: "m4b", Name: "M4B Audiobook", Ext: "m4b", Muxer: "ipod", Kind: KindAudio, Audio: m4aAudio,
		Flags: Flags{Faststart: true}},
	{ID: "aac", Name: "AAC", Ext: "aac", Muxer: "adts", Kind: KindAudio, Audio: []string{"aac"}},
	{ID: "wav", Name: "WAV", Ext: "wav", Muxer: "wav", Kind: KindAudio, Audio: pcmLE},
	{ID: "aiff", Name: "AIFF", Ext: "aiff", Muxer: "aiff", Kind: KindAudio, Audio: pcmBE},
	{ID: "flac", Name: "FLAC", Ext: "flac", Muxer: "flac", Kind: KindAudio, Audio: []string{"flac"}},
	{ID: "alac", Name: "Apple Lossless", Ext: "m4a", Muxer: "ipod", Kind: KindAudio,
		Audio: []string{"alac"}, Flags: Flags{Faststart: true}},
	{ID: "ac3", Name: "AC3", Ext: "ac3", Muxer: "ac3", Kind: KindAudio, Audio: []string{"ac3"}},
	{ID: "ogg", Name: "OGG", Ext: "ogg", Muxer: "ogg", Kind: KindAudio, Audio: oggAudio},
	{ID: "opus", Name: "Opus", Ext: "opus", Muxer: "opus", Kind: KindAudio, Audio: []string{"opus"}},
	{ID: "wma", Name: "WMA", Ext: "wma", Muxer: "asf", Kind: KindAudio, Audio: []string{"wmav2"}},
	{ID: "mka", Name: "MKA", Ext: "mka", Muxer: "matroska", Kind: KindAudio, Audio: mkvAudio},
	{ID: "caf", Name: "CAF", Ext: "caf", Muxer: "caf", Kind: KindAudio,
		Audio: []string{"alac", "pcm_s16le", "pcm_s24le", "pcm_s16be", "pcm_s24be"}},
	{ID: "au", Name: "AU", Ext: "au", Muxer: "au", Kind: KindAudio, Audio: pcmBE},
	{ID: "amr", Name: "AMR", Ext: "amr", Muxer: "amr", Kind: KindAudio, Audio: []string{"amr_nb"}},
	{ID: "mp2", Name: "MP2", Ext: "mp2", Muxer: "mp2", Kind: KindAudio, Audio: []string{"mp2"}},
}

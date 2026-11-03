package engine

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/probe"
)

// SubtitleMode says what a job does with subtitles.
type SubtitleMode string

// Subtitle modes.
const (
	// SubtitlesNone leaves subtitles out. The empty mode means the same.
	SubtitlesNone SubtitleMode = "none"
	// SubtitlesKeep carries every subtitle track the output container can hold.
	SubtitlesKeep SubtitleMode = "keep"
	// SubtitlesPick carries one subtitle track.
	SubtitlesPick SubtitleMode = "pick"
	// SubtitlesBurn draws one subtitle track into the picture.
	SubtitlesBurn SubtitleMode = "burn"
)

// Stream selection limits.
const (
	// AllTracks as the only entry of Streams.AudioTracks keeps every audio track.
	AllTracks = -1
	// MaxExternalAudio is the most external audio files one job may add.
	MaxExternalAudio = 6
)

// Subtitles chooses what happens to a job's subtitles.
type Subtitles struct {
	// Mode is what happens to subtitles. Empty means SubtitlesNone.
	Mode SubtitleMode `json:"mode,omitempty"`
	// Track is the position among the input's subtitle streams that pick and burn use, from 0.
	Track int `json:"track,omitempty"`
	// External is a text subtitle file (.srt, .ass, .ssa, or .vtt). Keep adds it beside the
	// input's tracks, and pick and burn use it instead of Track.
	External string `json:"external,omitempty"`
}

// Streams chooses the streams and extras a job's output carries. The zero value carries the
// first audio track, no subtitles, and the source's chapters and metadata, with ffmpeg's own
// thread count.
type Streams struct {
	// AudioTracks lists positions among the input's audio streams, from 0. Empty means the first
	// track, and AllTracks alone means every track. Ignored when the preset carries no audio.
	AudioTracks []int `json:"audioTracks,omitempty"`
	// ExternalAudio lists audio files added as extra tracks after the input's own.
	ExternalAudio []string `json:"externalAudio,omitempty"`
	// Subtitles chooses the subtitle handling.
	Subtitles Subtitles `json:"subtitles,omitzero"`
	// DropChapters leaves the source's chapter marks out of the output.
	DropChapters bool `json:"dropChapters,omitempty"`
	// DropMetadata leaves the source's tags, such as title and date, out of the output.
	DropMetadata bool `json:"dropMetadata,omitempty"`
	// Threads caps the encoder threads, zero for ffmpeg's default.
	Threads int `json:"threads,omitempty"`
}

// subtitleExts maps the text subtitle file extensions icelos takes to catalog codec IDs.
var subtitleExts = map[string]string{ //nolint:gochecknoglobals // Read-only table.
	".srt": "subrip", ".ass": "ass", ".ssa": "ass", ".vtt": "webvtt",
}

// subtitleAliases maps ffprobe subtitle codec names to the catalog codec ID that holds them.
var subtitleAliases = map[string]string{"ssa": "ass", "srt": "subrip"} //nolint:gochecknoglobals // Read-only table.

// textSubtitles lists the ffprobe codec names of text subtitles, which convert between formats
// and which the subtitles filter draws. Every other subtitle codec is a bitmap.
var textSubtitles = []string{ //nolint:gochecknoglobals // Read-only table.
	"subrip", "srt", "ass", "ssa", "mov_text", "webvtt", "text"}

// Validate reports whether the selection is well formed on its own, before the input is known.
func (s Streams) Validate() error {
	if len(s.ExternalAudio) > MaxExternalAudio {
		return fmt.Errorf("%w: %d external audio files, at most %d", ErrStreams,
			len(s.ExternalAudio), MaxExternalAudio)
	}
	if slices.Contains(s.ExternalAudio, "") {
		return fmt.Errorf("%w: external audio path empty", ErrStreams)
	}
	for i, t := range s.AudioTracks {
		switch {
		case t == AllTracks && len(s.AudioTracks) != 1:
			return fmt.Errorf("%w: all tracks cannot be combined with single tracks", ErrStreams)
		case t < AllTracks:
			return fmt.Errorf("%w: audio track %d", ErrStreams, t)
		case slices.Contains(s.AudioTracks[:i], t):
			return fmt.Errorf("%w: audio track %d listed twice", ErrStreams, t)
		}
	}
	sub := s.Subtitles
	switch sub.mode() {
	case SubtitlesNone:
		if sub.External != "" {
			return fmt.Errorf("%w: subtitle file %s needs keep, pick, or burn", ErrStreams, sub.External)
		}
	case SubtitlesKeep, SubtitlesPick, SubtitlesBurn:
	default:
		return fmt.Errorf("%w: subtitle mode %q", ErrStreams, sub.Mode)
	}
	if sub.Track < 0 {
		return fmt.Errorf("%w: subtitle track %d", ErrStreams, sub.Track)
	}
	if sub.External != "" {
		if _, ok := subtitleExts[strings.ToLower(filepath.Ext(sub.External))]; !ok {
			return fmt.Errorf("%w: subtitle file %s: use .srt, .ass, .ssa, or .vtt", ErrStreams,
				sub.External)
		}
	}
	if s.Threads < 0 {
		return fmt.Errorf("%w: threads %d", ErrStreams, s.Threads)
	}
	return nil
}

// mode returns the subtitle mode, reading the empty mode as SubtitlesNone.
func (s Subtitles) mode() SubtitleMode {
	if s.Mode == "" {
		return SubtitlesNone
	}
	return s.Mode
}

// audioOut is one audio stream the output carries.
type audioOut struct {
	// Map is the -map argument that selects the stream.
	Map string
	// Source is the probed stream, zero when the file was not probed.
	Source probe.Stream
}

// subtitleOut is one soft subtitle stream the output carries.
type subtitleOut struct {
	// Map is the -map argument that selects the stream.
	Map string
	// Codec is the -c:s value, "copy" or an encoder name.
	Codec string
}

// burn is how subtitles are drawn into the picture.
type burn struct {
	// Filter is the subtitles filter that draws text subtitles, empty when none.
	Filter string
	// Overlay is the stream specifier of a bitmap subtitle stream drawn with the overlay filter,
	// empty when none.
	Overlay string
}

// layout is the stream plan of one job apart from its video stream.
type layout struct {
	// Inputs lists the extra input files that follow the source, in -i order.
	Inputs []string
	// Audio lists the audio streams in output order.
	Audio []audioOut
	// Subtitles lists the soft subtitle streams in output order.
	Subtitles []subtitleOut
	// Burn is the subtitle burn-in, zero when none.
	Burn burn
}

// planStreams lays out the audio and subtitle streams of job's output in container box. Extra
// holds the probed external audio files in order; a missing entry is planned as unknown.
func planStreams(job Job, info probe.Info, extra []probe.Info, box format.Container,
	avail EncoderSet) (layout, error) {
	var l layout
	if err := l.addAudio(job, info, extra, box); err != nil {
		return layout{}, err
	}
	if err := l.addSubtitles(job, info, box, avail); err != nil {
		return layout{}, err
	}
	return l, nil
}

// addAudio adds the input's chosen audio tracks and the external audio files.
func (l *layout) addAudio(job Job, info probe.Info, extra []probe.Info, box format.Container) error {
	if !job.Preset.HasAudio() {
		return nil
	}
	s := job.Streams
	tracks := info.Kind("audio")
	add := func(src probe.Stream) {
		l.Audio = append(l.Audio, audioOut{Map: "0:" + strconv.Itoa(src.Index), Source: src})
	}
	switch {
	case len(s.AudioTracks) == 0:
		if len(tracks) != 0 {
			add(tracks[0])
		}
	case s.AudioTracks[0] == AllTracks:
		for _, src := range tracks {
			add(src)
		}
	default:
		for _, t := range s.AudioTracks {
			if t >= len(tracks) {
				return fmt.Errorf("%w: %s: audio track %d of %d", ErrNoAudio, job.Input, t, len(tracks))
			}
			add(tracks[t])
		}
	}
	for i, path := range s.ExternalAudio {
		var src probe.Stream
		if i < len(extra) {
			a, ok := extra[i].FirstAudio()
			if !ok {
				return fmt.Errorf("%w: %s", ErrNoAudio, path)
			}
			src = a
		}
		l.Inputs = append(l.Inputs, path)
		l.Audio = append(l.Audio, audioOut{Map: strconv.Itoa(len(l.Inputs)) + ":a:0", Source: src})
	}
	if box.AudioTracks > 0 && len(l.Audio) > box.AudioTracks {
		return fmt.Errorf("%w: %s holds %d audio track, the job has %d", ErrStreams, box.Name,
			box.AudioTracks, len(l.Audio))
	}
	return nil
}

// addSubtitles adds the soft subtitle streams or the burn-in the job asks for.
func (l *layout) addSubtitles(job Job, info probe.Info, box format.Container, avail EncoderSet) error {
	sub := job.Streams.Subtitles
	tracks := info.Kind("subtitle")
	pickTrack := func() (probe.Stream, error) {
		if sub.Track >= len(tracks) {
			return probe.Stream{}, fmt.Errorf("%w: %s: subtitle track %d of %d", ErrNoSubtitle, job.Input,
				sub.Track, len(tracks))
		}
		return tracks[sub.Track], nil
	}
	switch sub.mode() {
	case SubtitlesBurn:
		return l.addBurn(job, sub, pickTrack)
	case SubtitlesPick:
		if len(box.Subtitles) == 0 {
			return fmt.Errorf("%w: %s holds no soft subtitles: burn them in instead", ErrStreams, box.Name)
		}
		if sub.External != "" {
			return l.addExternalSubtitle(sub.External, box, avail)
		}
		src, err := pickTrack()
		if err != nil {
			return err
		}
		codec, ok, err := softCodec(box, src.Codec, avail)
		switch {
		case err != nil:
			return err
		case !ok:
			return fmt.Errorf("%w: %s cannot hold %s subtitles: burn them in instead", ErrStreams,
				box.Name, src.Codec)
		}
		l.Subtitles = append(l.Subtitles, subtitleOut{Map: "0:" + strconv.Itoa(src.Index), Codec: codec})
	case SubtitlesKeep:
		if len(box.Subtitles) == 0 {
			return nil
		}
		for _, src := range tracks {
			codec, ok, err := softCodec(box, src.Codec, avail)
			if err != nil {
				return err
			}
			if ok {
				l.Subtitles = append(l.Subtitles, subtitleOut{Map: "0:" + strconv.Itoa(src.Index), Codec: codec})
			}
		}
		if sub.External != "" {
			return l.addExternalSubtitle(sub.External, box, avail)
		}
	}
	return nil
}

// addBurn plans the burn-in of the external subtitle file or the picked track.
func (l *layout) addBurn(job Job, sub Subtitles, pickTrack func() (probe.Stream, error)) error {
	if !job.Preset.HasVideo() || job.Preset.Video.Codec == format.Copy {
		return fmt.Errorf("%w: burning subtitles needs encoded video", ErrStreams)
	}
	if sub.External != "" {
		l.Burn.Filter = subtitlesFilter(sub.External, -1)
		return nil
	}
	src, err := pickTrack()
	if err != nil {
		return err
	}
	if slices.Contains(textSubtitles, src.Codec) {
		l.Burn.Filter = subtitlesFilter(job.Input, sub.Track)
		return nil
	}
	l.Burn.Overlay = "0:" + strconv.Itoa(src.Index)
	return nil
}

// addExternalSubtitle adds a text subtitle file as an extra input and a soft subtitle stream.
func (l *layout) addExternalSubtitle(path string, box format.Container, avail EncoderSet) error {
	codec, _, err := softCodec(box, subtitleExts[strings.ToLower(filepath.Ext(path))], avail)
	if err != nil {
		return err
	}
	l.Inputs = append(l.Inputs, path)
	l.Subtitles = append(l.Subtitles, subtitleOut{Map: strconv.Itoa(len(l.Inputs)) + ":s:0", Codec: codec})
	return nil
}

// softCodec returns the -c:s value that puts a subtitle stream of codec src into box, and false
// when box cannot hold it: a bitmap codec the container does not carry. Text subtitles the
// container does not carry are converted to its first subtitle codec.
func softCodec(box format.Container, src string, avail EncoderSet) (string, bool, error) {
	id := src
	if alias, ok := subtitleAliases[src]; ok {
		id = alias
	}
	switch {
	case len(box.Subtitles) == 0:
		return "", false, nil
	case box.Allows(format.KindSubtitle, id):
		return "copy", true, nil
	case !slices.Contains(textSubtitles, src):
		return "", false, nil
	}
	codec, err := format.CodecByID(box.Subtitles[0])
	if err != nil {
		return "", false, err
	}
	for _, e := range codec.Encoders {
		if avail.Has(e.Name) && Drives(e.Name) {
			return e.Name, true, nil
		}
	}
	return "", false, fmt.Errorf("%w: %s", ErrMissingEncoder, preferred(codec))
}

// subtitlesFilter returns the subtitles filter that draws the file at path, using its subtitle
// stream at position track, or its only one when track is negative.
func subtitlesFilter(path string, track int) string {
	f := "subtitles=filename=" + filterValue(path)
	if track >= 0 {
		f += ":si=" + strconv.Itoa(track)
	}
	return f
}

// filterValue escapes s for use as one option value inside a filtergraph description: first for
// the option parser, which splits on colons, then for the graph parser.
func filterValue(s string) string {
	return escapeAll(escapeAll(s, `\':`), `\'[],;`)
}

// escapeAll puts a backslash before every rune of s found in special.
func escapeAll(s, special string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// audioOutputArgs returns the maps and encoder arguments of every audio stream in l. A single
// stream takes the plain -c:a form. Several take per-stream specifiers, since sources can differ
// in channels and sample rate.
func audioOutputArgs(l layout, box format.Container, a format.Audio, fps float64,
	avail EncoderSet) ([]string, []string, error) {
	var maps, args []string
	for i, out := range l.Audio {
		maps = append(maps, "-map", out.Map)
		one, err := planAudio(box, a, out.Source, fps, avail)
		if err != nil {
			return nil, nil, err
		}
		if len(l.Audio) > 1 {
			one = perStream(one, "a", i)
		}
		args = append(args, one...)
	}
	return maps, args, nil
}

// subtitleOutputArgs returns the maps and codec arguments of every soft subtitle stream in l.
func subtitleOutputArgs(l layout) []string {
	var maps, codecs []string
	for i, out := range l.Subtitles {
		maps = append(maps, "-map", out.Map)
		codecs = append(codecs, "-c:s:"+strconv.Itoa(i), out.Codec)
	}
	return append(maps, codecs...)
}

// perStream rewrites the option names in args, which alternate name and value, to apply only to
// output stream n of the given kind. Names that already carry the kind, such as -c:a, gain the
// index, and the rest gain both.
func perStream(args []string, kind string, n int) []string {
	out := slices.Clone(args)
	idx := strconv.Itoa(n)
	for i := 0; i < len(out); i += 2 {
		if strings.HasSuffix(out[i], ":"+kind) {
			out[i] += ":" + idx
		} else {
			out[i] += ":" + kind + ":" + idx
		}
	}
	return out
}

// inputArgs returns the -i arguments for the extra input files.
func inputArgs(paths []string) []string {
	var args []string
	for _, p := range paths {
		args = append(args, "-i", p)
	}
	return args
}

// threadArgs returns -threads for the selection, or nothing for ffmpeg's default.
func threadArgs(s Streams) []string {
	if s.Threads == 0 {
		return nil
	}
	return []string{"-threads", strconv.Itoa(s.Threads)}
}

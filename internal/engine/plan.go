package engine

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/probe"
)

// Plan is the ffmpeg work for one job.
type Plan struct {
	// Passes holds the ffmpeg arguments for each pass in run order.
	Passes [][]string
	// Encoder is the video encoder name, or empty when video is copied or absent.
	Encoder string
	// Hardware is true when Encoder is a VideoToolbox encoder.
	Hardware bool
	// Filters lists the ffmpeg filters the plan needs that some ffmpeg builds lack, such as the
	// subtitles filter, which needs libass.
	Filters []string
	// Copied records which kinds of stream the plan copies instead of encoding.
	Copied Copy
}

// Bitrate budget for target-size encodes.
const (
	// muxOverhead is the share of the target left after container overhead.
	muxOverhead = 0.97
	// minVideoBitrate is the lowest video bitrate worth producing, in bits per second.
	minVideoBitrate = 64_000
)

// TargetVideoBitrate returns the video bitrate in bits per second that lands a file of
// duration dur at target bytes, after audioBps of audio and container overhead.
func TargetVideoBitrate(target int64, dur time.Duration, audioBps int) (int, error) {
	if dur <= 0 {
		return 0, ErrNoDuration
	}
	total := float64(target) * 8 * muxOverhead / dur.Seconds()
	video := total - float64(audioBps)
	if video < minVideoBitrate {
		return 0, fmt.Errorf("%w: %d bytes over %s leaves %.0f bps for video", ErrTargetTooSmall,
			target, dur.Round(time.Second), max(video, 0))
	}
	return int(video), nil
}

// Build plans the ffmpeg passes that turn job.Input into dest. Two-pass encodes write their
// statistics under passlog. Extra holds the probed external audio files of job.Streams in order;
// files missing from it are planned without their sample rate and channels. The video encoder is
// the first one avail offers in the codec's catalog order that can honor the settings. When
// job.Hardware is set, a VideoToolbox encoder comes first, except for two-pass encodes, which
// VideoToolbox cannot run.
func Build(job Job, info probe.Info, avail EncoderSet, dest, passlog string, //nolint:funlen // Composes every pass shape.
	extra ...probe.Info) (Plan, error) {
	p := job.Preset
	if err := p.Validate(); err != nil {
		return Plan{}, err
	}
	if err := job.Streams.Validate(); err != nil {
		return Plan{}, err
	}
	if err := job.Range.Validate(); err != nil {
		return Plan{}, err
	}
	box, err := format.ContainerByID(p.Container)
	if err != nil {
		return Plan{}, err
	}
	video, hasVideo := info.FirstVideo()
	if p.HasVideo() && !hasVideo {
		return Plan{}, fmt.Errorf("%w: %s: pick an audio preset", ErrNoVideo, job.Input)
	}
	lay, err := planStreams(job, info, extra, box, avail)
	if err != nil {
		return Plan{}, err
	}
	if !p.HasVideo() && len(lay.Audio) == 0 {
		return Plan{}, fmt.Errorf("%w: %s", ErrNoAudio, job.Input)
	}
	verdict, err := verdicts(box, p.Target, info, lay)
	if err != nil {
		return Plan{}, err
	}
	if job.Passthrough {
		p.Target = passthrough(p.Target, verdict)
	}
	done := copied(p.Target, verdict)
	lay.Burn.Filter = shiftBurn(lay.Burn.Filter, job.Range.Start)

	ranged := job.Range.args()
	head := slices.Concat([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-nostats",
		"-progress", "pipe:1", "-y"}, ranged, []string{"-i", job.Input})
	final := slices.Concat(head, inputArgs(lay.Inputs, ranged))
	threads := threadArgs(job.Streams)
	tail := slices.Concat(subtitleOutputArgs(lay), threads, muxArgs(box, job.Streams, dest))

	var (
		vplan    videoPlan
		fps      float64
		mapVideo []string
	)
	if p.HasVideo() {
		mapVideo = []string{"-map", "0:" + strconv.Itoa(video.Index)}
	}
	if p.HasVideo() && p.Video.Codec != format.Copy {
		vplan, err = planVideo(box, p.Video, video, avail, job.Hardware && p.Video.RateControl != format.RateTwoPass,
			lay.Burn)
		if err != nil {
			return Plan{}, err
		}
		fps = vplan.Frame.FPS
		if vplan.Graph != "" {
			mapVideo = []string{"-filter_complex", vplan.Graph, "-map", "[v]"}
		}
	}
	mapAudio, audioArgs, err := audioOutputArgs(lay, box, p.Audio, fps, avail)
	if err != nil {
		return Plan{}, err
	}

	switch {
	case !p.HasVideo():
		return Plan{Passes: [][]string{slices.Concat(final, mapAudio, audioArgs, tail)}, Copied: done}, nil
	case p.Video.Codec == format.Copy:
		vargs := []string{"-c:v", "copy"}
		if box.Flags.HVC1Tag && video.Codec == "hevc" {
			vargs = append(vargs, "-tag:v", "hvc1")
		}
		pass := slices.Concat(final, mapVideo, vargs, mapAudio, audioArgs, tail)
		return Plan{Passes: [][]string{pass}, Copied: done}, nil
	}

	plan := Plan{Encoder: vplan.Name, Hardware: vplan.Hardware, Filters: vplan.Filters, Copied: done}
	spec := videoSpec{Video: p.Video, Bitrate: p.Video.Bitrate, PassLog: passlog}
	if spec.Bitrate == 0 {
		spec.Bitrate = vplan.Frame.Bitrate
	}
	if p.Video.RateControl != format.RateTwoPass {
		vargs := slices.Concat(vplan.Head, vplan.Encoder.Args(spec))
		plan.Passes = [][]string{slices.Concat(final, mapVideo, vargs, mapAudio, audioArgs, tail)}
		return plan, nil
	}

	if p.TargetBytes > 0 {
		audioBps := 0
		if len(lay.Audio) != 0 {
			audioBps = p.Audio.Bitrate * len(lay.Audio)
		}
		span := job.Range.Span(info.Duration)
		if spec.Bitrate, err = TargetVideoBitrate(p.TargetBytes, span, audioBps); err != nil {
			return Plan{}, err
		}
	}
	pass := func(n int) []string {
		spec.Pass = n
		return slices.Concat(vplan.Head, vplan.Encoder.Args(spec))
	}
	first := slices.Concat(head, mapVideo, pass(1), threads, []string{"-an", "-f", "null", os.DevNull})
	second := slices.Concat(final, mapVideo, pass(2), mapAudio, audioArgs, tail)
	plan.Passes = [][]string{first, second}
	return plan, nil
}

// shiftBurn returns the subtitles filter f adjusted for an input read from start. The filter
// opens its file on its own and reads it from the beginning, while the seeked frames are stamped
// from zero, so the frames are moved back onto the file's timeline for the filter and then
// returned to zero. An empty filter or a zero start is returned unchanged.
func shiftBurn(f string, start time.Duration) string {
	if f == "" || start <= 0 {
		return f
	}
	return "setpts=PTS+" + seconds(start) + "/TB," + f + ",setpts=PTS-STARTPTS"
}

// videoPlan is the chosen encoder and frame settings for an encoded video stream.
type videoPlan struct {
	// Name is the ffmpeg encoder name.
	Name string
	// Encoder is how icelos drives the encoder.
	Encoder videoEncoder
	// Hardware is true when the encoder is a VideoToolbox encoder.
	Hardware bool
	// Frame is the output frame shape.
	Frame frame
	// Head holds -c:v, the encoder setup, and the frame arguments, which every pass shares.
	Head []string
	// Graph is the -filter_complex graph whose output [v] replaces the source video, empty when
	// the filters fit -vf.
	Graph string
	// Filters lists the filters that some ffmpeg builds lack.
	Filters []string
}

// planVideo picks the encoder and frame settings that turn the source stream src into v inside
// container box, drawing in subtitles as b says. Hardware asks for a VideoToolbox encoder where
// one can honor v.
func planVideo(box format.Container, v format.Video, src probe.Stream, avail EncoderSet,
	hardware bool, b burn) (videoPlan, error) {
	codec, err := format.CodecByID(v.Codec)
	if err != nil {
		return videoPlan{}, err
	}
	f, err := frameFor(box, codec, v, src)
	if err != nil {
		return videoPlan{}, err
	}
	vp, err := pickVideoEncoder(box, codec, v, avail, hardware)
	if err != nil {
		return videoPlan{}, err
	}
	enc := vp.Encoder
	head := append([]string{"-c:v", vp.Name}, enc.Setup...)
	var chain []string
	if f.Scale != "" {
		chain = append(chain, f.Scale)
	}
	switch {
	case b.Overlay != "":
		vp.Graph = fmt.Sprintf("[0:%d][%s]overlay=eof_action=pass", src.Index, b.Overlay)
		if len(chain) != 0 {
			vp.Graph += "," + strings.Join(chain, ",")
		}
		vp.Graph += "[v]"
		vp.Filters = []string{"overlay"}
	case b.Filter != "":
		chain = append(chain, b.Filter)
		vp.Filters = []string{"subtitles"}
		fallthrough
	case len(chain) != 0:
		head = append(head, "-vf", strings.Join(chain, ","))
	}
	if f.FPS > 0 {
		head = append(head, "-r", rateArg(f.FPS))
	}
	if pix := f.PixFmt; pix != "" {
		if renamed, ok := enc.PixFmts[pix]; ok {
			pix = renamed
		}
		head = append(head, "-pix_fmt", pix)
	}
	switch {
	case box.Flags.HVC1Tag && codec.ID == "hevc":
		head = append(head, "-tag:v", "hvc1")
	case v.Tag != "":
		head = append(head, "-tag:v", v.Tag)
	}
	vp.Frame, vp.Head = f, head
	return vp, nil
}

// pickVideoEncoder returns a plan holding the first encoder for codec that avail offers, icelos
// can drive, box can carry, and that accepts v, trying VideoToolbox encoders first when hardware
// is set. The plan's frame and head are left for the caller.
func pickVideoEncoder(box format.Container, codec format.Codec, v format.Video, avail EncoderSet,
	hardware bool) (videoPlan, error) {
	pick := func(hw bool) (videoPlan, bool) {
		for _, e := range codec.Encoders {
			enc, ok := videoEncoders[e.Name]
			switch {
			case e.Hardware != hw, !ok, !avail.Has(e.Name):
			case enc.Accepts != nil && !enc.Accepts(v):
			case slices.Contains(enc.RefusedBy, box.Muxer):
			default:
				return videoPlan{Name: e.Name, Encoder: enc, Hardware: hw}, true
			}
		}
		return videoPlan{}, false
	}
	if hardware {
		if vp, ok := pick(true); ok {
			return vp, nil
		}
	}
	if vp, ok := pick(false); ok {
		return vp, nil
	}
	return videoPlan{}, fmt.Errorf("%w: %s", ErrMissingEncoder, preferred(codec))
}

// preferred returns the name of the codec's first software encoder, or its first encoder.
func preferred(codec format.Codec) string {
	for _, e := range codec.Encoders {
		if !e.Hardware {
			return e.Name
		}
	}
	if len(codec.Encoders) != 0 {
		return codec.Encoders[0].Name
	}
	return codec.ID
}

// frame is the output frame shape of an encoded video stream.
type frame struct {
	// Scale is the scale filter, empty to keep the source size.
	Scale string
	// FPS is the output frame rate, zero to keep the source rate.
	FPS float64
	// PixFmt is the catalog pixel format, empty to let the encoder choose.
	PixFmt string
	// Bitrate is the bitrate a fixed mode needs when the settings leave it open.
	Bitrate int
}

// frameFor returns the frame shape that turns the source stream src into v. Codecs and
// containers with fixed modes get the mode closest to the source when v leaves it open.
func frameFor(box format.Container, codec format.Codec, v format.Video,
	src probe.Stream) (frame, error) {
	w, h := src.DisplaySize()
	srcFPS := src.FrameRate()
	f := frame{FPS: v.FPS, PixFmt: codec.PixFmt}
	if prof, ok := codec.ProfileByID(v.Profile); ok && prof.PixFmt != "" {
		f.PixFmt = prof.PixFmt
	}
	modes := box.Modes
	if codec.Fixed(v.Profile) {
		modes = codec.Modes
	}
	switch {
	case len(modes) != 0:
		m, err := pickMode(modes, v, w, h, srcFPS)
		if err != nil {
			return frame{}, fmt.Errorf("%w: %s in %s", err, codec.ID, box.ID)
		}
		if w != m.Width || h != m.Height {
			f.Scale = fmt.Sprintf("scale=%d:%d", m.Width, m.Height)
		}
		if f.FPS == 0 {
			f.FPS = m.FPS
		}
		if m.PixFmt != "" {
			f.PixFmt = m.PixFmt
		}
		if v.Bitrate == 0 && len(m.Bitrates) != 0 {
			f.Bitrate = m.Bitrates[len(m.Bitrates)/2]
		}
	case v.Width != 0 && v.Height != 0:
		if w != v.Width || h != v.Height {
			f.Scale = fmt.Sprintf("scale=%d:%d", v.Width, v.Height)
		}
	case v.Width != 0:
		if w != v.Width {
			f.Scale = fmt.Sprintf("scale=%d:-2", v.Width)
		}
	case v.Height != 0:
		if h != v.Height {
			f.Scale = fmt.Sprintf("scale=-2:%d", v.Height)
		}
	default:
		if nw, nh, ok := fit(w, h, v.MaxWidth, v.MaxHeight); ok {
			f.Scale = fmt.Sprintf("scale=%d:%d", nw, nh)
		}
	}
	if f.FPS == 0 && len(codec.FrameRates) != 0 && (srcFPS == 0 || !codec.AllowsFrameRate(srcFPS)) {
		f.FPS = nearestRate(codec.FrameRates, srcFPS)
	}
	return f, nil
}

// pickMode returns the mode that v allows and that is closest to the source: the same frame rate
// family (25 and 50 fps sources are PAL, the rest NTSC), then the nearest frame area, then table
// order.
func pickMode(modes []format.Mode, v format.Video, w, h int, fps float64) (format.Mode, error) {
	pal := format.SameRate(fps, 25) || format.SameRate(fps, 50)
	best, bestScore := format.Mode{}, math.Inf(1)
	for _, m := range modes {
		if !modeAllows(m, v) {
			continue
		}
		score := 0.0
		if m.FPS != 0 && format.SameRate(m.FPS, 25) != pal {
			score += modeFamilyPenalty
		}
		if w > 0 && h > 0 {
			score += math.Abs(math.Log(float64(m.Width*m.Height) / float64(w*h)))
		}
		if score < bestScore {
			best, bestScore = m, score
		}
	}
	if math.IsInf(bestScore, 1) {
		return format.Mode{}, format.ErrMode
	}
	return best, nil
}

// modeFamilyPenalty outweighs any frame area difference, so a mode in the source's frame rate
// family always wins over one outside it.
const modeFamilyPenalty = 100

// modeAllows reports whether mode m fits the profile and every frame setting v pins.
func modeAllows(m format.Mode, v format.Video) bool {
	switch {
	case m.Profile != "" && m.Profile != v.Profile:
	case v.Width != 0 && v.Width != m.Width, v.Height != 0 && v.Height != m.Height:
	case v.FPS != 0 && m.FPS != 0 && !format.SameRate(v.FPS, m.FPS):
	case v.Bitrate != 0 && len(m.Bitrates) != 0 && !slices.Contains(m.Bitrates, v.Bitrate):
	default:
		return true
	}
	return false
}

// nearestRate returns the rate in rates closest to fps, treating an unknown rate as 30.
func nearestRate(rates []float64, fps float64) float64 {
	if fps <= 0 {
		fps = 30
	}
	best := rates[0]
	for _, r := range rates[1:] {
		if math.Abs(r-fps) < math.Abs(best-fps) {
			best = r
		}
	}
	return best
}

// rateArg formats a frame rate for -r, writing the NTSC rates as exact fractions.
func rateArg(fps float64) string {
	for _, base := range []int{24, 30, 48, 60, 120, 240} {
		if format.SameRate(fps, float64(base)*1000/1001) {
			return strconv.Itoa(base*1000) + "/1001"
		}
	}
	return strconv.FormatFloat(fps, 'f', -1, 64)
}

// planAudio returns the audio arguments that turn the source stream src into a inside container
// box. Fps is the output video frame rate, zero when there is no video or it follows the source.
func planAudio(box format.Container, a format.Audio, src probe.Stream, fps float64,
	avail EncoderSet) ([]string, error) {
	if a.Codec == format.Copy {
		return []string{"-c:a", "copy"}, nil
	}
	codec, err := format.CodecByID(a.Codec)
	if err != nil {
		return nil, err
	}
	rate := sampleRate(box, codec, a, src)
	effective := rate
	if effective == 0 {
		effective = src.SampleRate
	}
	name, enc, err := pickAudioEncoder(codec, a, effective, avail)
	if err != nil {
		return nil, err
	}
	args := []string{"-c:a", name}
	if a.Bitrate > 0 {
		args = append(args, "-b:a", strconv.Itoa(a.Bitrate))
	}
	if n := channels(box, codec, a, src); n > 0 {
		args = append(args, "-ac", strconv.Itoa(n))
	}
	if rate > 0 {
		args = append(args, "-ar", strconv.Itoa(rate))
	}
	if enc.Args != nil {
		args = append(args, enc.Args(a, fps)...)
	}
	return args, nil
}

// pickAudioEncoder returns the first encoder for codec that avail offers, icelos can drive, and
// that accepts a at the sample rate, which is zero when unknown.
func pickAudioEncoder(codec format.Codec, a format.Audio, rate int, avail EncoderSet) (string, audioEncoder, error) {
	for _, e := range codec.Encoders {
		enc, ok := audioEncoders[e.Name]
		switch {
		case !ok, !avail.Has(e.Name):
		case e.MaxSampleRate > 0 && rate > e.MaxSampleRate:
		case enc.Accepts != nil && !enc.Accepts(a):
		default:
			return e.Name, enc, nil
		}
	}
	return "", audioEncoder{}, fmt.Errorf("%w: %s", ErrMissingEncoder, preferred(codec))
}

// sampleRate returns the -ar value for a, or zero to keep the source rate. A source rate that
// the codec or container refuses is moved to the nearest one they take. When the source rate is
// unknown, a rate is forced only where the container restricts it or the codec has one rate.
func sampleRate(box format.Container, codec format.Codec, a format.Audio, src probe.Stream) int {
	if a.SampleRate > 0 {
		return a.SampleRate
	}
	allowed := codec.SampleRates
	if len(box.SampleRates) != 0 {
		allowed = slices.DeleteFunc(slices.Clone(box.SampleRates), func(r int) bool {
			return len(codec.SampleRates) != 0 && !slices.Contains(codec.SampleRates, r)
		})
	}
	switch {
	case len(allowed) == 0:
		return 0
	case src.SampleRate > 0 && slices.Contains(allowed, src.SampleRate):
		return 0
	case src.SampleRate == 0 && len(allowed) > 1 && len(box.SampleRates) == 0:
		return 0
	}
	want := src.SampleRate
	if want == 0 {
		want = defaultSampleRate
	}
	best := allowed[0]
	for _, r := range allowed[1:] {
		if abs(r-want) < abs(best-want) {
			best = r
		}
	}
	return best
}

// defaultSampleRate is the rate a forced choice aims for when the source rate is unknown.
const defaultSampleRate = 48000

// abs returns the absolute value of n.
func abs(n int) int {
	return max(n, -n)
}

// channels returns the -ac value for a, or zero to keep the source layout. The container's fixed
// count wins, then the settings' cap, then the codec's.
func channels(box format.Container, codec format.Codec, a format.Audio, src probe.Stream) int {
	if box.Channels != 0 {
		if src.Channels != box.Channels {
			return box.Channels
		}
		return 0
	}
	limit := a.Channels
	if limit == 0 {
		limit = codec.MaxChannels
	}
	if limit > 0 && src.Channels > limit {
		return limit
	}
	return 0
}

// muxArgs returns the output arguments that end every final pass: chapters, metadata, the
// container's muxer flags, the muxer itself, and the destination. Naming the muxer keeps
// extensions ffmpeg does not know, such as .m4r and .trp, working.
func muxArgs(box format.Container, s Streams, dest string) []string {
	var args []string
	if s.DropChapters {
		args = append(args, "-map_chapters", "-1")
	}
	if s.DropMetadata {
		args = append(args, "-map_metadata", "-1")
	} else {
		args = append(args, "-map_metadata", "0")
	}
	if box.Flags.Faststart {
		args = append(args, "-movflags", "+faststart")
	}
	if box.Flags.M2TS {
		args = append(args, "-mpegts_m2ts_mode", "1")
	}
	if box.Flags.Shortest {
		args = append(args, "-shortest")
	}
	return append(args, "-f", box.Muxer, dest)
}

// qualityIndex maps q to a quality table index, treating unset as balanced.
func qualityIndex(q format.Quality) int {
	if q < format.QualityHigh || q > format.QualitySmall {
		q = format.QualityBalanced
	}
	return int(q - format.QualityHigh)
}

// fit returns the even frame size that fits w by h inside the box, and whether it differs
// from the source. maxLong bounds the long side and maxShort the short side, so portrait
// video is treated the same as landscape. A zero bound is unbounded.
func fit(w, h, maxLong, maxShort int) (int, int, bool) {
	if w <= 0 || h <= 0 {
		return w, h, false
	}
	bound := func(v int) int {
		if v <= 0 {
			return math.MaxInt32
		}
		return v
	}
	boxW, boxH := bound(maxLong), bound(maxShort)
	if h > w {
		boxW, boxH = boxH, boxW
	}
	scale := min(1, float64(boxW)/float64(w), float64(boxH)/float64(h))
	nw := even(int(math.Round(float64(w) * scale)))
	nh := even(int(math.Round(float64(h) * scale)))
	return nw, nh, nw != w || nh != h
}

// even rounds n down to an even number of at least two.
func even(n int) int {
	return max(2, n-n%2)
}

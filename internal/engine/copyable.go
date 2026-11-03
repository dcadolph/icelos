package engine

import (
	"fmt"

	"github.com/dcadolph/icelos/internal/format"
	"github.com/dcadolph/icelos/internal/probe"
)

// Reasons a stream cannot pass through untouched.
const (
	// ReasonUnknown means the source stream was not probed, so its codec is unknown.
	ReasonUnknown = "unknown source"
	// ReasonCodec means the target codec differs from the source codec.
	ReasonCodec = "codec"
	// ReasonContainer means the output container cannot carry the source codec.
	ReasonContainer = "container"
	// ReasonTargetSize means a target size needs the stream encoded to a computed bitrate.
	ReasonTargetSize = "target size"
	// ReasonResize means the settings change the frame size.
	ReasonResize = "resize"
	// ReasonFrameRate means the settings change the frame rate.
	ReasonFrameRate = "frame rate"
	// ReasonProfile means the settings pin a profile the source may not have.
	ReasonProfile = "profile"
	// ReasonLevel means the settings pin a level the source may not have.
	ReasonLevel = "level"
	// ReasonTag means the settings pin a fourcc tag the source may not have.
	ReasonTag = "tag"
	// ReasonBurn means subtitles are drawn into the picture.
	ReasonBurn = "burn subtitles"
	// ReasonSampleRate means the settings or the container change the sample rate.
	ReasonSampleRate = "sample rate"
	// ReasonChannels means the settings or the container change the channel count.
	ReasonChannels = "channels"
	// ReasonBitDepth means the settings pin a bit depth the source may not have.
	ReasonBitDepth = "bit depth"
)

// Verdict says whether one kind of stream can pass through untouched.
type Verdict struct {
	// Copy is true when the stream can be copied instead of encoded.
	Copy bool `json:"copy"`
	// Reason is the first Reason constant that rules copying out, empty when Copy is true.
	Reason string `json:"reason,omitempty"`
}

// Copy holds the passthrough verdict for each kind of stream a job's output carries.
type Copy struct {
	// Video is the video verdict, nil when the output has no video.
	Video *Verdict `json:"video,omitempty"`
	// Audio is the verdict for every audio track together, nil when the output has no audio.
	Audio *Verdict `json:"audio,omitempty"`
}

// Lossless reports whether the output carries at least one stream and every one can be copied.
func (c Copy) Lossless() bool {
	switch {
	case c.Video == nil && c.Audio == nil:
		return false
	case c.Video != nil && !c.Video.Copy, c.Audio != nil && !c.Audio.Copy:
		return false
	}
	return true
}

// Copyable reports, for each kind of stream in job's output, whether the source stream can pass
// through untouched instead of being encoded. A stream qualifies when the source already is the
// target codec, the container can carry it, and no setting reshapes it. Quality and bitrate
// settings do not rule copying out, since copying keeps the source's own quality, but a target
// size does. Extra holds the probed external audio files of job.Streams in order. The error is
// the one Build would return for a job it cannot plan.
func Copyable(job Job, info probe.Info, avail EncoderSet, extra ...probe.Info) (Copy, error) {
	p := job.Preset
	if err := p.Validate(); err != nil {
		return Copy{}, err
	}
	if err := job.Streams.Validate(); err != nil {
		return Copy{}, err
	}
	box, err := format.ContainerByID(p.Container)
	if err != nil {
		return Copy{}, err
	}
	if _, ok := info.FirstVideo(); p.HasVideo() && !ok {
		return Copy{}, fmt.Errorf("%w: %s: pick an audio preset", ErrNoVideo, job.Input)
	}
	lay, err := planStreams(job, info, extra, box, avail)
	if err != nil {
		return Copy{}, err
	}
	if !p.HasVideo() && len(lay.Audio) == 0 {
		return Copy{}, fmt.Errorf("%w: %s", ErrNoAudio, job.Input)
	}
	var c Copy
	if p.HasVideo() {
		v, err := videoVerdict(box, p.Target, info, lay.Burn)
		if err != nil {
			return Copy{}, err
		}
		c.Video = &v
	}
	if p.HasAudio() && len(lay.Audio) != 0 {
		a := Verdict{Copy: true}
		for _, out := range lay.Audio {
			if a = audioVerdict(box, p.Target, out.Source); !a.Copy {
				break
			}
		}
		c.Audio = &a
	}
	return c, nil
}

// videoVerdict says whether the first motion video stream of info, which the caller has checked
// exists, can be copied into box under target t, with subtitles drawn in as b says.
func videoVerdict(box format.Container, t format.Target, info probe.Info, b burn) (Verdict, error) {
	src, _ := info.FirstVideo()
	v := t.Video
	switch {
	case v.Codec == format.Copy && !box.Allows(format.KindVideo, src.Codec):
		return no(ReasonContainer), nil
	case v.Codec == format.Copy:
		return Verdict{Copy: true}, nil
	case v.Codec != src.Codec:
		return no(ReasonCodec), nil
	case t.TargetBytes > 0:
		return no(ReasonTargetSize), nil
	case v.Profile != "":
		return no(ReasonProfile), nil
	case v.Level != "":
		return no(ReasonLevel), nil
	case v.Tag != "":
		return no(ReasonTag), nil
	case b.Filter != "" || b.Overlay != "":
		return no(ReasonBurn), nil
	}
	codec, err := format.CodecByID(v.Codec)
	if err != nil {
		return Verdict{}, err
	}
	f, err := frameFor(box, codec, v, src)
	if err != nil {
		return Verdict{}, err
	}
	switch {
	case f.Scale != "":
		return no(ReasonResize), nil
	case f.FPS > 0 && !format.SameRate(f.FPS, src.FrameRate()):
		return no(ReasonFrameRate), nil
	}
	return Verdict{Copy: true}, nil
}

// audioVerdict says whether the audio stream src can be copied into box under target t.
func audioVerdict(box format.Container, t format.Target, src probe.Stream) Verdict {
	a := t.Audio
	switch {
	case src.Codec == "":
		return no(ReasonUnknown)
	case a.Codec != format.Copy && a.Codec != src.Codec:
		return no(ReasonCodec)
	case !box.Allows(format.KindAudio, src.Codec):
		return no(ReasonContainer)
	case t.TargetBytes > 0:
		return no(ReasonTargetSize)
	case a.BitDepth != 0:
		return no(ReasonBitDepth)
	}
	codec, err := format.CodecByID(src.Codec)
	if err != nil {
		return no(ReasonContainer)
	}
	if a.Codec == format.Copy {
		a = format.Audio{Codec: src.Codec}
	}
	if rate := sampleRate(box, codec, a, src); rate != 0 && rate != src.SampleRate {
		return no(ReasonSampleRate)
	}
	if channels(box, codec, a, src) != 0 {
		return no(ReasonChannels)
	}
	return Verdict{Copy: true}
}

// no returns a verdict that rules copying out for reason.
func no(reason string) Verdict {
	return Verdict{Reason: reason}
}

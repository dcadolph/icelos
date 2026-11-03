package format

import (
	"fmt"
	"slices"
)

// maxFPS is the highest output frame rate a target may ask for.
const maxFPS = 240

// Check reports whether t is a target the catalog can produce: the container exists, it carries
// the chosen codecs, and every setting is one the codec and container accept. Settings that
// depend on the source, such as whether a copied stream fits the container, are not checked.
func Check(t Target) error {
	c, err := ContainerByID(t.Container)
	if err != nil {
		return err
	}
	if !t.HasVideo() && !t.HasAudio() {
		return fmt.Errorf("%w: %s: no video and no audio", ErrSetting, c.ID)
	}
	if c.Flags.NeedsVideo && !t.HasVideo() {
		return fmt.Errorf("%w: %s: needs a video stream", ErrSetting, c.ID)
	}
	if t.TargetBytes < 0 {
		return fmt.Errorf("%w: negative target size", ErrSetting)
	}
	if err := checkVideo(c, t); err != nil {
		return err
	}
	return checkAudio(c, t.Audio)
}

// checkVideo checks the video half of t against container c.
func checkVideo(c Container, t Target) error {
	v := t.Video
	switch v.Codec {
	case "":
		if t.TargetBytes > 0 {
			return fmt.Errorf("%w: target size needs an encoded video stream", ErrSetting)
		}
		return nil
	case Copy:
		if v != (Video{Codec: Copy}) {
			return fmt.Errorf("%w: copied video takes no other video settings", ErrSetting)
		}
		if t.TargetBytes > 0 {
			return fmt.Errorf("%w: target size needs an encoded video stream", ErrSetting)
		}
		return nil
	}
	codec, err := CodecByID(v.Codec)
	if err != nil {
		return err
	}
	if codec.Kind != KindVideo || !c.Allows(KindVideo, codec.ID) {
		return fmt.Errorf("%w: %s does not carry %s video", ErrSetting, c.ID, codec.ID)
	}
	for _, check := range []func(Codec, Video) error{checkSize, checkFrameRate, checkProfile} {
		if err := check(codec, v); err != nil {
			return err
		}
	}
	if err := checkRate(codec, v, t.TargetBytes); err != nil {
		return err
	}
	return checkModes(c, codec, v)
}

// checkSize checks the frame size and the fit box.
func checkSize(codec Codec, v Video) error {
	for _, n := range []int{v.Width, v.Height, v.MaxWidth, v.MaxHeight} {
		if n < 0 {
			return fmt.Errorf("%w: %s: negative frame size", ErrSetting, codec.ID)
		}
	}
	if (v.Width != 0 || v.Height != 0) && (v.MaxWidth != 0 || v.MaxHeight != 0) {
		return fmt.Errorf("%w: %s: set a frame size or a fit box, not both", ErrSetting, codec.ID)
	}
	for _, n := range []int{v.Width, v.Height} {
		switch {
		case n == 0:
		case n%2 != 0:
			return fmt.Errorf("%w: %s: frame size %d is odd", ErrSetting, codec.ID, n)
		case n < domains.MinDimension || n > domains.MaxDimension:
			return fmt.Errorf("%w: %s: frame size %d is outside %d to %d", ErrSetting, codec.ID, n,
				domains.MinDimension, domains.MaxDimension)
		}
	}
	return nil
}

// checkFrameRate checks the output frame rate.
func checkFrameRate(codec Codec, v Video) error {
	if v.FPS < 0 || v.FPS > maxFPS {
		return fmt.Errorf("%w: %s: frame rate %g is outside 0 to %d", ErrSetting, codec.ID, v.FPS,
			maxFPS)
	}
	if !codec.AllowsFrameRate(v.FPS) {
		return fmt.Errorf("%w: %s does not encode at %g fps", ErrSetting, codec.ID, v.FPS)
	}
	return nil
}

// checkProfile checks the profile, level, tag, and quality level.
func checkProfile(codec Codec, v Video) error {
	if v.Profile != "" {
		if _, ok := codec.ProfileByID(v.Profile); !ok {
			return fmt.Errorf("%w: %s has no profile %q", ErrSetting, codec.ID, v.Profile)
		}
	} else if slices.ContainsFunc(codec.Modes, func(m Mode) bool { return m.Profile != "" }) {
		return fmt.Errorf("%w: %s needs a profile", ErrSetting, codec.ID)
	}
	if v.Level != "" && !slices.Contains(codec.Levels, v.Level) {
		return fmt.Errorf("%w: %s has no level %q", ErrSetting, codec.ID, v.Level)
	}
	if v.Tag != "" && !slices.Contains(codec.Tags, v.Tag) {
		return fmt.Errorf("%w: %s has no tag %q", ErrSetting, codec.ID, v.Tag)
	}
	if v.Quality < 0 || v.Quality > QualitySmall {
		return fmt.Errorf("%w: %s: quality %d is outside 1 to %d", ErrSetting, codec.ID, v.Quality,
			QualitySmall)
	}
	return nil
}

// checkRate checks the rate control, bitrate, and target size.
func checkRate(codec Codec, v Video, targetBytes int64) error {
	if v.Bitrate < 0 {
		return fmt.Errorf("%w: %s: negative bitrate", ErrSetting, codec.ID)
	}
	if len(codec.RateControls) == 0 {
		if v.RateControl != "" {
			return fmt.Errorf("%w: %s has no rate control choice", ErrSetting, codec.ID)
		}
		if targetBytes > 0 {
			return fmt.Errorf("%w: %s cannot aim for a target size", ErrSetting, codec.ID)
		}
		listed := slices.ContainsFunc(codec.Modes, func(m Mode) bool {
			return (m.Profile == "" || m.Profile == v.Profile) && len(m.Bitrates) != 0
		})
		if v.Bitrate != 0 && !listed {
			return fmt.Errorf("%w: %s sets its own bitrate", ErrSetting, codec.ID)
		}
		return nil
	}
	if !slices.Contains(codec.RateControls, v.RateControl) {
		return fmt.Errorf("%w: %s does not support rate control %q", ErrSetting, codec.ID,
			v.RateControl)
	}
	switch v.RateControl {
	case RateQuality:
		if v.Quality == 0 {
			return fmt.Errorf("%w: %s: quality rate control needs a quality level", ErrSetting, codec.ID)
		}
		if v.Bitrate != 0 {
			return fmt.Errorf("%w: %s: quality rate control takes no bitrate", ErrSetting, codec.ID)
		}
	case RateCBR, RateVBR:
		if v.Bitrate == 0 {
			return fmt.Errorf("%w: %s: %s needs a bitrate", ErrSetting, codec.ID, v.RateControl)
		}
	case RateTwoPass:
		if (v.Bitrate == 0) == (targetBytes == 0) {
			return fmt.Errorf("%w: %s: two-pass needs exactly one of a bitrate and a target size",
				ErrSetting, codec.ID)
		}
	}
	if targetBytes > 0 && v.RateControl != RateTwoPass {
		return fmt.Errorf("%w: %s: target size needs two-pass rate control", ErrSetting, codec.ID)
	}
	return nil
}

// checkModes checks fixed frame shapes from the codec and the container. Both take a frame size,
// never a fit box.
func checkModes(c Container, codec Codec, v Video) error {
	fixed := codec.Fixed(v.Profile) || len(c.Modes) != 0
	if fixed && (v.MaxWidth != 0 || v.MaxHeight != 0) {
		return fmt.Errorf("%w: %s in %s takes a fixed frame size, not a fit box", ErrSetting,
			codec.ID, c.ID)
	}
	if _, err := codec.Match(v.Profile, v.Width, v.Height, v.FPS, v.Bitrate); err != nil {
		return err
	}
	_, err := c.Match(v.Width, v.Height, v.FPS)
	return err
}

// checkAudio checks the audio settings a against container c.
func checkAudio(c Container, a Audio) error {
	switch a.Codec {
	case "":
		return nil
	case Copy:
		if a != (Audio{Codec: Copy}) {
			return fmt.Errorf("%w: copied audio takes no other audio settings", ErrSetting)
		}
		return nil
	}
	codec, err := CodecByID(a.Codec)
	if err != nil {
		return err
	}
	if codec.Kind != KindAudio || !c.Allows(KindAudio, codec.ID) {
		return fmt.Errorf("%w: %s does not carry %s audio", ErrSetting, c.ID, codec.ID)
	}
	if err := checkSampleRate(c, codec, a.SampleRate); err != nil {
		return err
	}
	if a.Channels != 0 {
		known := slices.ContainsFunc(domains.Channels, func(l ChannelLayout) bool {
			return l.Count == a.Channels
		})
		if !known || (codec.MaxChannels != 0 && a.Channels > codec.MaxChannels) ||
			(c.Channels != 0 && a.Channels != c.Channels) {
			return fmt.Errorf("%w: %s does not carry %d channels", ErrSetting, codec.ID, a.Channels)
		}
	}
	if err := checkAudioBitrate(codec, a); err != nil {
		return err
	}
	if a.BitDepth != 0 && !slices.Contains(codec.BitDepths, a.BitDepth) {
		return fmt.Errorf("%w: %s does not write %d-bit samples", ErrSetting, codec.ID, a.BitDepth)
	}
	return nil
}

// checkSampleRate checks an audio sample rate against the domain, the codec, the container, and
// the encoders. Zero, meaning same as source, is always allowed.
func checkSampleRate(c Container, codec Codec, rate int) error {
	if rate == 0 {
		return nil
	}
	ok := slices.Contains(domains.SampleRates, rate) &&
		(len(codec.SampleRates) == 0 || slices.Contains(codec.SampleRates, rate)) &&
		(len(c.SampleRates) == 0 || slices.Contains(c.SampleRates, rate)) &&
		slices.ContainsFunc(codec.Encoders, func(e Encoder) bool {
			return e.MaxSampleRate == 0 || rate <= e.MaxSampleRate
		})
	if !ok {
		return fmt.Errorf("%w: %s in %s does not take %d Hz", ErrSetting, codec.ID, c.ID, rate)
	}
	return nil
}

// checkAudioBitrate checks an audio bitrate against the codec's fixed list and caps. When the
// sample rate or channel count follows the source, the cap is taken at the most the codec allows,
// so only bitrates no source could reach are refused.
func checkAudioBitrate(codec Codec, a Audio) error {
	switch {
	case a.Bitrate < 0:
		return fmt.Errorf("%w: %s: negative bitrate", ErrSetting, codec.ID)
	case a.Bitrate == 0:
		return nil
	case codec.Lossless:
		return fmt.Errorf("%w: %s is lossless and takes no bitrate", ErrSetting, codec.ID)
	case len(codec.Bitrates) != 0:
		if !slices.Contains(codec.Bitrates, a.Bitrate) {
			return fmt.Errorf("%w: %s does not encode at %d b/s", ErrSetting, codec.ID, a.Bitrate)
		}
		return nil
	}
	rate := a.SampleRate
	if rate == 0 {
		rate = slices.Max(domains.SampleRates)
		if len(codec.SampleRates) != 0 {
			rate = slices.Max(codec.SampleRates)
		}
	}
	channels := a.Channels
	if channels == 0 {
		channels = codec.MaxChannels
	}
	if limit := codec.MaxBitrate(rate, channels); limit != 0 && a.Bitrate > limit {
		return fmt.Errorf("%w: %s tops out at %d b/s, asked for %d", ErrSetting, codec.ID, limit,
			a.Bitrate)
	}
	return nil
}

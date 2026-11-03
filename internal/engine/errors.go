package engine

import "errors"

var (
	// ErrEncode is returned when ffmpeg exits with an error.
	ErrEncode = errors.New("encode failed")
	// ErrEncoders is returned when the encoder list cannot be read.
	ErrEncoders = errors.New("encoder list failed")
	// ErrMissingEncoder is returned when ffmpeg lacks an encoder the preset needs.
	ErrMissingEncoder = errors.New("encoder not available")
	// ErrNoVideo is returned when a video preset is given a file without motion video.
	ErrNoVideo = errors.New("input has no video")
	// ErrNoAudio is returned when an audio preset is given a file without audio.
	ErrNoAudio = errors.New("input has no audio")
	// ErrNoDuration is returned when a target size is requested for a file of unknown length.
	ErrNoDuration = errors.New("input duration unknown")
	// ErrTargetTooSmall is returned when the target size leaves no room for watchable video.
	ErrTargetTooSmall = errors.New("target size too small")
	// ErrExists is returned when the output exists and overwrite is off.
	ErrExists = errors.New("output exists")
	// ErrSameFile is returned when the output path is the input path.
	ErrSameFile = errors.New("output is the input")
	// ErrOutput is returned when the output cannot be prepared or moved into place.
	ErrOutput = errors.New("output failed")
	// ErrStreams is returned when a job's stream selection is invalid or the output cannot hold it.
	ErrStreams = errors.New("stream selection invalid")
	// ErrNoSubtitle is returned when a job asks for a subtitle track the input does not have.
	ErrNoSubtitle = errors.New("input has no such subtitle")
	// ErrMissingFilter is returned when ffmpeg lacks a filter the job needs.
	ErrMissingFilter = errors.New("filter not available")
	// ErrRange is returned when a job's time range is not a forward span.
	ErrRange = errors.New("time range invalid")
)

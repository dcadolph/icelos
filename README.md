<p align="center"><img src="assets/banner.png" alt="Icelos"></p>

<h1 align="center">Icelos</h1>

A free, open source video and audio converter for Mac.

Icelos takes video and audio files in and hands them back in a new format or a smaller size. It runs on your machine, needs no account, and shows no upsells or trial limits.

## Download

[Download Icelos for Mac](https://github.com/dcadolph/icelos/releases/latest/download/Icelos_macos_universal.dmg). The app is signed and notarized by Apple and includes its own FFmpeg, so nothing else needs to be installed. It runs on macOS 14 or later, on Apple silicon and Intel.

With Homebrew:

```sh
brew install --cask dcadolph/tap/icelos
```

## Status

Early. The Mac app is on the [releases page](https://github.com/dcadolph/icelos/releases) as a DMG and a zip, with a signed checksum file and build provenance. The command line tool has release archives for macOS, Linux, and Windows there too, each with a signed checksum file and build provenance. It needs FFmpeg on the PATH. The goals below describe where the project is headed, not what works today.

What works today:

- A format catalog of video and audio containers and codecs, from MP4, MOV, and MKV to MXF, DV, and AMV, with the rules each one follows built into the engine, so the app, the command line, and the backend refuse the same settings.
- Presets for common targets, target-size compression in two passes, and custom presets saved as files that the app and the command line share.
- An inspector in the app for every setting a format takes: codec, resolution, frame rate, rate control, bitrate, profile, level, sample rate, channels, and bit depth. Any file can carry its own settings.
- Stream selection: keep any of a file's audio tracks, add up to six audio files as extra tracks, keep, pick, or burn in subtitles, add a subtitle file, drop chapters or metadata, and split a file into one output per chapter.
- Passthrough: a stream that already matches the target is copied instead of encoded, and the app marks a file LOSSLESS when nothing is re-encoded.
- Size estimates on each file before it converts, and the Apple media engine for H.264, HEVC, and ProRes.
- Twelve themes with light and dark appearance, and a Settings window for the default destination, the media engine, and a CPU limit.

The app's bundled FFmpeg writes every format in the catalog, including Theora (OGV), Vorbis (OGG audio), and AMR-NB, and burns in text subtitles through libass. Homebrew's FFmpeg lacks those encoders and libass, so a source build or the command line on Homebrew's FFmpeg shows them as unavailable. `mac/build-ffmpeg.sh` builds the same FFmpeg the app ships, from the pinned sources in `mac/ffmpeg-sources.txt`, and `Icelos.app/Contents/Resources/THIRD_PARTY.md` lists each bundled library and its license.

## Build from source

Icelos needs Go, Xcode or the Swift toolchain, and FFmpeg on the PATH (for example `brew install ffmpeg`).

```sh
go test ./...
go run . presets
go run . convert --preset compatible input.mov
mac/bundle.sh
```

`mac/bundle.sh` writes an unsigned `Icelos.app` to `mac/.build/`.

`go run . encoders` shows which catalog codecs the local FFmpeg can write. `convert` takes the preset as a starting point and flags on top of it:

```sh
go run . convert --container mkv --resolution 720p --audio-tracks all --subtitles keep input.mov
go run . convert --preset web --passthrough --split-chapters lecture.mp4
go run . convert --video-codec hevc --video-bitrate 6M --audio-bitrate 160k --threads 4 input.mov
```

Run `go run . convert --help` for every flag.

## Goals

- Convert between common video and audio formats.
- Compress a video to a target size.
- Extract audio and trim clips.
- Work through a queue of files with a progress bar for each.
- Run natively on Apple silicon and use the hardware encoders on the chip.
- Stay local. No account, no network calls, no telemetry.

## Not goals

- Removing copy protection. Movies bought from Apple and other protected files cannot be converted.
- Cloud features or paid tiers.

## Credits

Icelos is built on [FFmpeg](https://ffmpeg.org), which does the encoding and decoding. Icelos is named for the Greek god of dreams who takes the shape of beasts and birds.

## License

GPL-3.0-or-later. See the LICENSE file.

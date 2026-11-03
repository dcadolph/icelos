<p align="center"><img src="assets/banner.png" alt="Icelos"></p>

<h1 align="center">Icelos</h1>

A free, open source video and audio converter for Mac.

Icelos takes video and audio files in and hands them back in a new format or a smaller size. It runs on your machine, needs no account, and shows no upsells or trial limits.

## Status

Early and unreleased. The converter engine, the command line tool, and the Mac app work from source, with presets for common targets, target-size compression, and a job queue. There are no signed builds yet. The goals below describe where the project is headed, not what works today.

## Build from source

Icelos needs Go, Xcode or the Swift toolchain, and FFmpeg on the PATH (for example `brew install ffmpeg`).

```sh
go test ./...
go run . presets
go run . convert --preset compatible input.mov
mac/bundle.sh
```

`mac/bundle.sh` writes an unsigned `Icelos.app` to `mac/.build/`.

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

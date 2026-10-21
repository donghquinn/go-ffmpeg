# go-ffmpeg

[![Go Reference](https://pkg.go.dev/badge/github.com/donghquinn/go-ffmpeg.svg)](https://pkg.go.dev/github.com/donghquinn/go-ffmpeg)

A small, dependency-free Go wrapper around the `ffmpeg` and `ffprobe` CLI tools.
Failures come back as structured Go errors you can match with `errors.Is`
and `errors.As`, so you don't have to dig through raw stderr.

```sh
go get github.com/donghquinn/go-ffmpeg
```

`ffmpeg` (and optionally `ffprobe`) must be installed on the host.

## Usage

```go
ff, err := ffmpeg.New() // looks up ffmpeg/ffprobe in $PATH
if err != nil {
	return err // errors.Is(err, ffmpeg.ErrBinaryNotFound)
}

err = ff.Command().
	Input("in.mov").
	Output("out.mp4", "-c:v", "libx264", "-crf", "23").
	Overwrite(). // -y; without it an existing output returns ErrOutputExists
	Run(ctx)
```

Raw arguments are also supported:

```go
err := ff.Run(ctx, "-i", "in.mp4", "-vn", "out.mp3")
```

Probing:

```go
info, err := ff.Probe(ctx, "in.mp4")
fmt.Println(info.Duration(), info.VideoStreams()[0].Width)
```

## Error handling

Every failed process returns an `*ffmpeg.Error`:

```go
type Error struct {
	Binary   string   // "ffmpeg" or "ffprobe"
	Args     []string // arguments passed
	ExitCode int      // process exit code (-1 if it didn't exit normally)
	Message  string   // most relevant line from stderr, e.g. "Unknown encoder 'libx265'"
	File     string   // file the error refers to, if reported
	Stderr   string   // tail of stderr (last 64 KiB)
	Kind     error    // classified sentinel error, or nil
	Err      error    // underlying cause (*exec.ExitError or ctx.Err())
}
```

```
ffmpeg: in.mp4: Error opening input: No such file or directory (exit status 254)
```

Common failures are classified into sentinel errors:

| Sentinel               | Typical cause                                         |
| ---------------------- | ----------------------------------------------------- |
| `ErrBinaryNotFound`    | ffmpeg / ffprobe not installed or not in `$PATH`      |
| `ErrFileNotFound`      | input file or output directory does not exist         |
| `ErrPermissionDenied`  | file not readable / writable                          |
| `ErrOutputExists`      | output exists and `Overwrite()` was not used          |
| `ErrInvalidData`       | corrupt or non-media input                            |
| `ErrEncoderNotFound`   | unknown encoder or not compiled in (`-c:v libx265`)   |
| `ErrDecoderNotFound`   | no decoder for an input stream                        |
| `ErrFilterNotFound`    | unknown filter in `-vf` / `-af` / `-filter_complex`   |
| `ErrUnknownFormat`     | unknown container format / unguessable extension      |
| `ErrStreamNotFound`    | `-map` selects a stream that doesn't exist            |
| `ErrInvalidArgument`   | unknown option or invalid option value                |

```go
var fe *ffmpeg.Error
switch {
case errors.Is(err, ffmpeg.ErrFileNotFound):
case errors.Is(err, context.DeadlineExceeded): // ctx expired, ffmpeg was stopped
case errors.As(err, &fe):
	log.Printf("exit %d: %s\n%s", fe.ExitCode, fe.Message, fe.Stderr)
}
```

## Behavior notes

- `-hide_banner`, `-nostdin` and `-loglevel error` are added automatically so
  ffmpeg never waits on stdin and stderr only holds error messages. Change the
  log level with `WithLogLevel`, and mirror stderr with `WithStderr(os.Stderr)`.
- When the context is canceled, ffmpeg receives an interrupt so it can finalize
  the output, and is killed if it hasn't exited after `WithWaitDelay` (default 5s).

package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// maxStderr is the number of trailing stderr bytes kept for error reporting.
const maxStderr = 64 << 10

// FFmpeg runs ffmpeg and ffprobe commands. It is safe for concurrent use.
type FFmpeg struct {
	ffmpegPath  string
	ffprobePath string
	logLevel    string
	stderr      io.Writer
	waitDelay   time.Duration
}

// Option configures an [FFmpeg].
type Option func(*FFmpeg)

// WithFFmpegPath sets the ffmpeg executable to use. A bare name is looked up
// in $PATH. The default is "ffmpeg".
func WithFFmpegPath(path string) Option {
	return func(f *FFmpeg) { f.ffmpegPath = path }
}

// WithFFprobePath sets the ffprobe executable to use. A bare name is looked up
// in $PATH. The default is "ffprobe".
func WithFFprobePath(path string) Option {
	return func(f *FFmpeg) { f.ffprobePath = path }
}

// WithLogLevel sets ffmpeg's -loglevel. The default is "error", which keeps
// stderr limited to error messages so they can be reported accurately.
// Pass an empty string to leave ffmpeg's own default.
func WithLogLevel(level string) Option {
	return func(f *FFmpeg) { f.logLevel = level }
}

// WithStderr additionally copies the processes' stderr to w, e.g. os.Stderr
// for debugging. Stderr is always captured for error reporting regardless.
func WithStderr(w io.Writer) Option {
	return func(f *FFmpeg) { f.stderr = w }
}

// WithWaitDelay sets how long to wait, after the context passed to Run is
// done and ffmpeg was asked to stop, before the process is killed. ffmpeg
// uses this time to finalize the output file. The default is 5 seconds.
func WithWaitDelay(d time.Duration) Option {
	return func(f *FFmpeg) { f.waitDelay = d }
}

// New returns an FFmpeg configured with opts. It returns an error wrapping
// [ErrBinaryNotFound] if the ffmpeg executable cannot be found. A missing
// ffprobe is only reported when [FFmpeg.Probe] is called.
func New(opts ...Option) (*FFmpeg, error) {
	f := &FFmpeg{
		ffmpegPath:  "ffmpeg",
		ffprobePath: "ffprobe",
		logLevel:    "error",
		waitDelay:   5 * time.Second,
	}
	for _, opt := range opts {
		opt(f)
	}

	p, err := lookPath(f.ffmpegPath)
	if err != nil {
		return nil, err
	}
	f.ffmpegPath = p

	// ffprobe is optional; resolve it now if possible and report later.
	if p, err := exec.LookPath(f.ffprobePath); err == nil {
		f.ffprobePath = p
	}
	return f, nil
}

func lookPath(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrBinaryNotFound, name, err)
	}
	return p, nil
}

// Command returns a new [Command] builder that runs with f.
func (f *FFmpeg) Command() *Command {
	return &Command{ff: f}
}

// Run runs ffmpeg with the given raw arguments. "-hide_banner", "-nostdin"
// and the configured -loglevel are prepended automatically.
//
// Run returns nil on success, and an [*Error] otherwise.
func (f *FFmpeg) Run(ctx context.Context, args ...string) error {
	full := append([]string{"-hide_banner", "-nostdin"}, f.logLevelArgs()...)
	full = append(full, args...)
	_, err := f.run(ctx, f.ffmpegPath, "ffmpeg", full, nil)
	return err
}

func (f *FFmpeg) logLevelArgs() []string {
	if f.logLevel == "" {
		return nil
	}
	return []string{"-loglevel", f.logLevel}
}

// run executes bin with args, capturing stderr. If stdout is nil, stdout is
// discarded. name is the program name used in errors.
func (f *FFmpeg) run(ctx context.Context, bin, name string, args []string, stdout io.Writer) (stderr string, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	// Ask ffmpeg to stop gracefully on cancellation so it can finalize the
	// output; it is killed if it has not exited after waitDelay.
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill() // e.g. Windows, where Interrupt is unsupported
		}
		return nil
	}
	cmd.WaitDelay = f.waitDelay
	cmd.Stdout = stdout

	tail := &tailBuffer{max: maxStderr}
	cmd.Stderr = tail
	if f.stderr != nil {
		cmd.Stderr = io.MultiWriter(tail, f.stderr)
	}

	runErr := cmd.Run()
	stderr = tail.String()

	if runErr == nil {
		// ffmpeg exits 0 when it refuses to overwrite an existing output.
		if kind, msg, file := classify(stderr); kind == ErrOutputExists {
			return stderr, &Error{Binary: name, Args: args, Message: msg, File: file, Stderr: stderr, Kind: kind}
		}
		return stderr, nil
	}

	e := &Error{Binary: name, Args: args, ExitCode: -1, Stderr: stderr, Err: runErr}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		e.ExitCode = exitErr.ExitCode()
	}
	switch {
	case ctx.Err() != nil:
		// Stopped by the context: report that rather than the side effects
		// of the interruption printed on stderr.
		e.Err = ctx.Err()
	case exitErr == nil:
		// The process could not be started.
		if errors.Is(runErr, exec.ErrNotFound) || errors.Is(runErr, os.ErrNotExist) {
			e.Kind = ErrBinaryNotFound
		}
	default:
		e.Kind, e.Message, e.File = classify(stderr)
	}
	return stderr, e
}

// tailBuffer is an io.Writer that keeps only the last max bytes written.
type tailBuffer struct {
	buf []byte
	max int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= t.max {
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		return n, nil
	}
	if over := len(t.buf) + len(p) - t.max; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	t.buf = append(t.buf, p...)
	return n, nil
}

func (t *tailBuffer) String() string { return string(t.buf) }

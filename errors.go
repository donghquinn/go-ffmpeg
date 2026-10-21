package ffmpeg

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Sentinel errors describing why an ffmpeg or ffprobe invocation failed.
// They are matched with [errors.Is] against errors returned by this package.
var (
	// ErrBinaryNotFound means the ffmpeg or ffprobe executable could not be found.
	ErrBinaryNotFound = errors.New("ffmpeg: executable not found")

	// ErrFileNotFound means an input file, or the directory of an output
	// file, does not exist.
	ErrFileNotFound = errors.New("ffmpeg: file not found")

	// ErrPermissionDenied means a file could not be read or written.
	ErrPermissionDenied = errors.New("ffmpeg: permission denied")

	// ErrOutputExists means the output file already exists and overwriting
	// was not enabled (see [Command.Overwrite]).
	ErrOutputExists = errors.New("ffmpeg: output file already exists")

	// ErrInvalidData means the input could not be parsed as media
	// (corrupt, truncated, or not a media file).
	ErrInvalidData = errors.New("ffmpeg: invalid data found when processing input")

	// ErrEncoderNotFound means the requested encoder is unknown or was not
	// compiled into the ffmpeg binary.
	ErrEncoderNotFound = errors.New("ffmpeg: encoder not found")

	// ErrDecoderNotFound means no decoder is available for an input stream.
	ErrDecoderNotFound = errors.New("ffmpeg: decoder not found")

	// ErrFilterNotFound means a filter used in -vf/-af/-filter_complex is unknown.
	ErrFilterNotFound = errors.New("ffmpeg: filter not found")

	// ErrUnknownFormat means an input or output container format is unknown
	// or could not be guessed from the file name.
	ErrUnknownFormat = errors.New("ffmpeg: unknown format")

	// ErrStreamNotFound means a stream selected with -map (or required for
	// the output) does not exist in the input.
	ErrStreamNotFound = errors.New("ffmpeg: stream not found")

	// ErrInvalidArgument means ffmpeg rejected an option or its value.
	ErrInvalidArgument = errors.New("ffmpeg: invalid argument")
)

// Error is returned when an ffmpeg or ffprobe process fails.
//
// Use [errors.As] to inspect it, and [errors.Is] to match it against the
// package's sentinel errors or against [context.Canceled] /
// [context.DeadlineExceeded] when the process was stopped by its context.
type Error struct {
	// Binary is the program that failed, e.g. "ffmpeg" or "ffprobe".
	Binary string

	// Args are the arguments the program was started with (without the
	// program name).
	Args []string

	// ExitCode is the process exit code, or -1 if the process did not exit
	// normally (killed by a signal, failed to start, ...).
	ExitCode int

	// Message is the most relevant error line printed by the program,
	// with ffmpeg's "[component @ 0x...]" prefix removed. It may be empty.
	Message string

	// File is the input or output file the error refers to, when ffmpeg
	// reported one. It may be empty.
	File string

	// Stderr is the (tail of the) program's standard error output.
	Stderr string

	// Kind is the sentinel error this failure was classified as
	// (e.g. [ErrFileNotFound]), or nil if it could not be classified.
	Kind error

	// Err is the underlying cause: usually an [*exec.ExitError], or the
	// context's error when the process was stopped by its context.
	Err error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Binary)
	if e.File != "" {
		b.WriteString(": ")
		b.WriteString(e.File)
	}
	b.WriteString(": ")
	switch {
	case e.Message != "":
		b.WriteString(e.Message)
	case e.Kind != nil:
		b.WriteString(strings.TrimPrefix(e.Kind.Error(), "ffmpeg: "))
	case e.Err != nil:
		b.WriteString(e.Err.Error())
	default:
		b.WriteString("failed")
	}
	if e.ExitCode > 0 {
		fmt.Fprintf(&b, " (exit status %d)", e.ExitCode)
	}
	return b.String()
}

// Unwrap returns the classified sentinel error and the underlying cause, so
// both can be matched with [errors.Is] and [errors.As].
func (e *Error) Unwrap() []error {
	errs := make([]error, 0, 2)
	if e.Kind != nil {
		errs = append(errs, e.Kind)
	}
	if e.Err != nil {
		errs = append(errs, e.Err)
	}
	return errs
}

// rule maps substrings found in ffmpeg's stderr (compared case-insensitively)
// to a sentinel error. Rules are evaluated in order, so more specific rules
// come first and the generic "invalid argument" comes last.
type rule struct {
	kind     error
	patterns []string
}

var rules = []rule{
	{ErrOutputExists, []string{"already exists. exiting", "not overwriting - exiting"}},
	{ErrFileNotFound, []string{"no such file or directory"}},
	{ErrPermissionDenied, []string{"permission denied"}},
	{ErrEncoderNotFound, []string{"unknown encoder", "encoder not found"}},
	{ErrDecoderNotFound, []string{"unknown decoder", "decoder not found", "decoding is not supported"}},
	{ErrFilterNotFound, []string{"no such filter", "filter not found"}},
	{ErrUnknownFormat, []string{"unknown input format", "unable to choose an output format", "not a suitable output format", "unable to find a suitable output format"}},
	{ErrStreamNotFound, []string{"matches no streams", "does not contain any stream", "stream specifier"}},
	{ErrInvalidData, []string{"invalid data found when processing input", "moov atom not found"}},
	{ErrInvalidArgument, []string{"unrecognized option", "option not found", "error splitting the argument list", "invalid argument"}},
}

// "Error opening input file nope.mp4." / "Error opening output file out.mp4."
var fileLine = regexp.MustCompile(`^Error (?:opening|parsing options for) (?:input|output) file (.+?)\.?$`)

// classify inspects ffmpeg's stderr and returns the matching sentinel error,
// the most relevant line, and the file the error refers to (if any).
func classify(stderr string) (kind error, message, file string) {
	lines := splitLines(stderr)
	// Lowercase once: rules × patterns × lines comparisons follow.
	lower := make([]string, len(lines))
	for i, l := range lines {
		lower[i] = strings.ToLower(l)
	}

	for _, l := range lines {
		if strings.HasPrefix(l, "Error ") {
			if m := fileLine.FindStringSubmatch(l); m != nil {
				file = m[1]
				break
			}
		}
	}

	for _, r := range rules {
		for _, p := range r.patterns {
			for i, l := range lower {
				if strings.Contains(l, p) {
					return r.kind, lines[i], file
				}
			}
		}
	}

	// Unclassified: with -loglevel error the first line is usually the root
	// cause; with chattier log levels the last line usually is.
	if len(lines) > 0 {
		message = lines[len(lines)-1]
		for i, l := range lower {
			if strings.Contains(l, "error") {
				message = lines[i]
				break
			}
		}
	}
	return nil, message, file
}

// splitLines returns the non-empty lines of s with ffmpeg's component prefix
// stripped. Progress updates separated by '\r' are treated as lines too.
func splitLines(s string) []string {
	out := make([]string, 0, strings.Count(s, "\n")+1)
	for len(s) > 0 {
		var l string
		if i := strings.IndexAny(s, "\r\n"); i >= 0 {
			l, s = s[:i], s[i+1:]
		} else {
			l, s = s, ""
		}
		if l = strings.TrimSpace(stripComponent(strings.TrimSpace(l))); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// stripComponent removes ffmpeg's log prefix such as "[in#0 @ 0x841028000] "
// or "[AVFormatContext @ 0x70d02c500] " from l.
func stripComponent(l string) string {
	if !strings.HasPrefix(l, "[") {
		return l
	}
	end := strings.IndexByte(l, ']')
	if end < 0 || !strings.Contains(l[:end], " @ 0x") {
		return l
	}
	return l[end+1:]
}

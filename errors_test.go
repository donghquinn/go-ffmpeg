package ffmpeg

import (
	"context"
	"errors"
	"testing"
)

// Fixtures are real stderr outputs of ffmpeg 9 run with
// -hide_banner -nostdin -loglevel error.
func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		stderr  string
		kind    error
		message string
		file    string
	}{
		{
			name: "missing input",
			stderr: "[in#0 @ 0x841028000] Error opening input: No such file or directory\n" +
				"Error opening input file nope.mp4.\n" +
				"Error opening input files: No such file or directory\n",
			kind:    ErrFileNotFound,
			message: "Error opening input: No such file or directory",
			file:    "nope.mp4",
		},
		{
			name: "missing output directory",
			stderr: "[out#0/mp4 @ 0xa40c3c180] Error opening output /nonexist/dir/x.mp4: No such file or directory\n" +
				"Error opening output file /nonexist/dir/x.mp4.\n" +
				"Error opening output files: No such file or directory\n",
			kind:    ErrFileNotFound,
			message: "Error opening output /nonexist/dir/x.mp4: No such file or directory",
			file:    "/nonexist/dir/x.mp4",
		},
		{
			name:    "output exists",
			stderr:  "File 'ok.mp4' already exists. Exiting.\nError opening output file ok.mp4.\n",
			kind:    ErrOutputExists,
			message: "File 'ok.mp4' already exists. Exiting.",
			file:    "ok.mp4",
		},
		{
			name: "permission denied",
			stderr: "[in#0 @ 0x933028000] Error opening input: Permission denied\n" +
				"Error opening input file noperm.mp4.\n" +
				"Error opening input files: Permission denied\n",
			kind:    ErrPermissionDenied,
			message: "Error opening input: Permission denied",
			file:    "noperm.mp4",
		},
		{
			name: "unknown encoder",
			stderr: "[vost#0:0 @ 0xb8ac64300] Unknown encoder 'nopecodec'\n" +
				"[vost#0:0 @ 0xb8ac64300] Error selecting an encoder\n" +
				"Error opening output file x.mp4.\n" +
				"Error opening output files: Encoder not found\n",
			kind:    ErrEncoderNotFound,
			message: "Unknown encoder 'nopecodec'",
			file:    "x.mp4",
		},
		{
			name:    "unknown option",
			stderr:  "Unrecognized option 'bogus'.\nError splitting the argument list: Option not found\n",
			kind:    ErrInvalidArgument,
			message: "Unrecognized option 'bogus'.",
		},
		{
			name: "invalid data",
			stderr: "[in#0 @ 0xcad028000] moov atom not found\n" +
				"[in#0 @ 0xcacc0c000] Error opening input: Invalid data found when processing input\n" +
				"Error opening input file bad.mp4.\n" +
				"Error opening input files: Invalid data found when processing input\n",
			kind:    ErrInvalidData,
			message: "Error opening input: Invalid data found when processing input",
			file:    "bad.mp4",
		},
		{
			name: "unknown output format",
			stderr: "[AVFormatContext @ 0x70d02c500] Unable to choose an output format for 'x.nopeext'; use a standard extension for the filename or specify the format manually.\n" +
				"[out#0 @ 0x70cc40180] Error initializing the muxer for x.nopeext: Invalid argument\n" +
				"Error opening output file x.nopeext.\n" +
				"Error opening output files: Invalid argument\n",
			kind:    ErrUnknownFormat,
			message: "Unable to choose an output format for 'x.nopeext'; use a standard extension for the filename or specify the format manually.",
			file:    "x.nopeext",
		},
		{
			name: "unknown input format",
			stderr: "[in#0 @ 0xa5ec10000] Unknown input format: 'nopefmt'\n" +
				"Error opening input file ok.mp4.\n" +
				"Error opening input files: Invalid argument\n",
			kind:    ErrUnknownFormat,
			message: "Unknown input format: 'nopefmt'",
			file:    "ok.mp4",
		},
		{
			name: "map matches no streams",
			stderr: "Stream map '' matches no streams.\n" +
				"To ignore this, add a trailing '?' to the map.\n" +
				"Failed to set value '0:a' for option 'map': Invalid argument\n" +
				"Error parsing options for output file x2.mp4.\n" +
				"Error opening output files: Invalid argument\n",
			kind:    ErrStreamNotFound,
			message: "Stream map '' matches no streams.",
			file:    "x2.mp4",
		},
		{
			name: "unknown filter",
			stderr: "[AVFilterGraph @ 0xc8b010400] No such filter: 'nopefilter'\n" +
				"Error opening output file x4.mp4.\n" +
				"Error opening output files: Filter not found\n",
			kind:    ErrFilterNotFound,
			message: "No such filter: 'nopefilter'",
			file:    "x4.mp4",
		},
		{
			name:    "ffprobe missing input",
			stderr:  "nope.mp4: No such file or directory\n",
			kind:    ErrFileNotFound,
			message: "nope.mp4: No such file or directory",
		},
		{
			name:    "unclassified",
			stderr:  "something odd\nError while doing something\nmore noise\n",
			kind:    nil,
			message: "Error while doing something",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, msg, file := classify(tt.stderr)
			if kind != tt.kind {
				t.Errorf("kind = %v, want %v", kind, tt.kind)
			}
			if msg != tt.message {
				t.Errorf("message = %q, want %q", msg, tt.message)
			}
			if file != tt.file {
				t.Errorf("file = %q, want %q", file, tt.file)
			}
		})
	}
}

func TestErrorIsAs(t *testing.T) {
	var err error = &Error{
		Binary:   "ffmpeg",
		ExitCode: 254,
		Message:  "Error opening input: No such file or directory",
		File:     "nope.mp4",
		Kind:     ErrFileNotFound,
		Err:      context.Canceled,
	}

	if !errors.Is(err, ErrFileNotFound) {
		t.Error("errors.Is(err, ErrFileNotFound) = false")
	}
	if !errors.Is(err, context.Canceled) {
		t.Error("errors.Is(err, context.Canceled) = false")
	}
	if errors.Is(err, ErrPermissionDenied) {
		t.Error("errors.Is(err, ErrPermissionDenied) = true")
	}
	var fe *Error
	if !errors.As(err, &fe) || fe.ExitCode != 254 {
		t.Errorf("errors.As failed: %#v", fe)
	}
	want := "ffmpeg: nope.mp4: Error opening input: No such file or directory (exit status 254)"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestTailBuffer(t *testing.T) {
	b := &tailBuffer{max: 5}
	b.Write([]byte("abc"))
	b.Write([]byte("def"))
	if got := b.String(); got != "bcdef" {
		t.Errorf("got %q, want %q", got, "bcdef")
	}
	b.Write([]byte("0123456789"))
	if got := b.String(); got != "56789" {
		t.Errorf("got %q, want %q", got, "56789")
	}
}

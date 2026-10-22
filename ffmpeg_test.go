package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTest returns an FFmpeg using the binaries in $PATH, skipping the test
// when ffmpeg is not installed.
func newTest(t *testing.T) *FFmpeg {
	t.Helper()
	ff, err := New()
	if errors.Is(err, ErrBinaryNotFound) {
		t.Skip("ffmpeg not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	return ff
}

// testVideo generates a short test clip and returns its path.
func testVideo(t *testing.T, ff *FFmpeg) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "in.mp4")
	err := ff.Command().
		Options("-f", "lavfi").
		Input("testsrc=duration=1:size=64x64:rate=10").
		Options("-pix_fmt", "yuv420p").
		Output(out).
		Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNewBinaryNotFound(t *testing.T) {
	_, err := New(WithFFmpegPath("definitely-not-ffmpeg"))
	if !errors.Is(err, ErrBinaryNotFound) {
		t.Fatalf("err = %v, want ErrBinaryNotFound", err)
	}
}

func TestCommandArgs(t *testing.T) {
	c := (&FFmpeg{}).Command().
		Options("-threads", "2", "-ss", "5").
		Input("in.mp4").
		Options("-c:v", "libx264", "-metadata", "title=My Movie").
		Output("out.mp4").
		Overwrite()
	want := []string{"-y", "-threads", "2", "-ss", "5", "-i", "in.mp4", "-c:v", "libx264", "-metadata", "title=My Movie", "out.mp4"}
	got := c.Args()
	if len(got) != len(want) {
		t.Fatalf("Args() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Args() = %q, want %q", got, want)
		}
	}
}

func TestFlagsMixWithStrings(t *testing.T) {
	withConsts := (&FFmpeg{}).Command().
		Input("in.mp4").
		Options(VideoCodec, "libx264", AudioCodec, Copy, "-x264-params", "keyint=60", NoSubtitle).
		Output("out.mp4").
		Args()
	withStrings := (&FFmpeg{}).Command().
		Input("in.mp4").
		Options("-c:v", "libx264", "-c:a", "copy", "-x264-params", "keyint=60", "-sn").
		Output("out.mp4").
		Args()
	if strings.Join(withConsts, " ") != strings.Join(withStrings, " ") {
		t.Fatalf("constants %q != strings %q", withConsts, withStrings)
	}
}

func TestRunErrors(t *testing.T) {
	ff := newTest(t)
	in := testVideo(t, ff)
	dir := t.TempDir()
	ctx := context.Background()

	tests := []struct {
		name string
		cmd  *Command
		kind error
	}{
		{"missing input", ff.Command().Input(filepath.Join(dir, "nope.mp4")).Output(filepath.Join(dir, "a.mp4")), ErrFileNotFound},
		{"output exists", ff.Command().Input(in).Output(in), ErrOutputExists},
		{"unknown encoder", ff.Command().Input(in).Options("-c:v", "nopecodec").Output(filepath.Join(dir, "b.mp4")), ErrEncoderNotFound},
		{"unknown option", ff.Command().Input(in).Options("-bogus", "1").Output(filepath.Join(dir, "c.mp4")), ErrInvalidArgument},
		{"unknown format", ff.Command().Input(in).Output(filepath.Join(dir, "d.nopeext")), ErrUnknownFormat},
		{"no such stream", ff.Command().Input(in).Options("-map", "0:a").Output(filepath.Join(dir, "e.mp4")), ErrStreamNotFound},
		{"unknown filter", ff.Command().Input(in).Options("-vf", "nopefilter").Output(filepath.Join(dir, "f.mp4")), ErrFilterNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Run(ctx)
			if !errors.Is(err, tt.kind) {
				t.Fatalf("err = %v, want %v", err, tt.kind)
			}
			var fe *Error
			if !errors.As(err, &fe) {
				t.Fatalf("err is %T, want *Error", err)
			}
			if fe.Message == "" || fe.Stderr == "" {
				t.Errorf("missing details: %#v", fe)
			}
			t.Logf("%v", err)
		})
	}

	t.Run("invalid data", func(t *testing.T) {
		bad := filepath.Join(dir, "bad.mp4")
		if err := os.WriteFile(bad, []byte("not a video"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := ff.Command().Input(bad).Output(filepath.Join(dir, "g.mp4")).Run(ctx)
		if !errors.Is(err, ErrInvalidData) {
			t.Fatalf("err = %v, want ErrInvalidData", err)
		}
	})
}

func TestRunContextCanceled(t *testing.T) {
	ff := newTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// An endless source never finishes on its own.
	err := ff.Command().
		Options("-re", "-f", "lavfi").
		Input("testsrc=size=64x64").
		Output(filepath.Join(t.TempDir(), "out.mp4")).
		Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestProbe(t *testing.T) {
	ff := newTest(t)
	in := testVideo(t, ff)

	res, err := ff.Probe(context.Background(), in)
	if errors.Is(err, ErrBinaryNotFound) {
		t.Skip("ffprobe not installed")
	}
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Duration(); d < 900*time.Millisecond || d > 1100*time.Millisecond {
		t.Errorf("Duration() = %v, want ~1s", d)
	}
	v := res.VideoStreams()
	if len(v) != 1 || v[0].Width != 64 || v[0].Height != 64 {
		t.Errorf("VideoStreams() = %+v", v)
	}
	if len(res.Raw) == 0 {
		t.Error("Raw is empty")
	}

	_, err = ff.Probe(context.Background(), filepath.Join(t.TempDir(), "nope.mp4"))
	var fe *Error
	if !errors.Is(err, ErrFileNotFound) || !errors.As(err, &fe) || fe.Binary != "ffprobe" {
		t.Fatalf("err = %v, want ffprobe ErrFileNotFound", err)
	}
}

func TestCommandString(t *testing.T) {
	f := &FFmpeg{ffmpegPath: "/usr/bin/ffmpeg", logLevel: "error"}
	c := f.Command().
		Input("my clip.mp4").
		Options("-vf", "scale=-2:720", "-metadata", "title=It's here").
		Output("out.mp4")
	want := `/usr/bin/ffmpeg -hide_banner -nostdin -loglevel error -n -i 'my clip.mp4' -vf scale=-2:720 -metadata 'title=It'\''s here' out.mp4`
	if got := c.String(); got != want {
		t.Fatalf("String() =\n%s\nwant\n%s", got, want)
	}
	if got := (&FFmpeg{}).Command().Output("").String(); got != "ffmpeg -hide_banner -nostdin -n ''" {
		t.Fatalf("String() = %s", got)
	}
}

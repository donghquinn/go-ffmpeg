package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func cmd() *Command { return (&FFmpeg{}).Command() }

func TestValidateValid(t *testing.T) {
	tests := []struct {
		name string
		cmd  *Command
	}{
		{"transcode", cmd().Input("in.mov").
			Options(VideoCodec, "libx264", CRF, "23", Preset, "fast", AudioCodec, Copy, Metadata, "title=My Movie").
			Output("out.mp4")},
		{"input and output options", cmd().
			Options(Seek, "30", ReadRealtime).Input("in.mp4").
			Options(Duration, "10", Codec, Copy).Output("clip.mp4")},
		{"negative values", cmd().
			Options(StreamLoop, "-1", SeekFromEnd, "-5").Input("in.mp4").
			Options(Map, "0", Map, "-0:a").Output("out.mp4")},
		{"time formats", cmd().
			Options(Seek, "01:02:03.5").Input("in.mp4").
			Options(Duration, "500ms", To, "90").Output("out.mp4")},
		{"value formats", cmd().Input("in.mp4").
			Options(Size, "hd720", FrameRate, "30000/1001", VideoBitrate, "2M", AudioBitrate, "128k",
				AspectRatio, "16:9", SampleRate, "48000", AudioChannels, "2", Threads, "auto").
			Output("out.mp4")},
		{"stream specifiers", cmd().Input("in.mp4").
			Options(Codec+":v:0", "libx264", "-b:v:0", "1M", Metadata+":s:a:0", "language=eng").
			Output("out.mkv")},
		{"unknown flags are accepted", cmd().Input("in.mp4").
			Options("-x264-params", "keyint=60", "-fflags", "+genpts", "-sws_flags", "lanczos").
			Output("out.mp4")},
		{"no-value flags", cmd().Input("in.mp4").Options(NoAudio, NoSubtitle, Shortest).Output("out.mp4")},
		{"multiple outputs", cmd().Input("in.mp4").
			Options(Size, "1280x720").Output("720.mp4").
			Options(Size, "640x360").Output("360.mp4")},
		{"pipe output", cmd().Input("pipe:0").Options(ForceFormat, "mp4").Output("pipe:1")},
		{"global filter_complex", cmd().Input("a.mp4").Input("b.mp4").
			Options(FilterComplex, "[0:v][1:v]hstack", "-y_unrelated_private_opt", "1").
			Output("out.mp4")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cmd.Validate(); err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestValidateInvalid(t *testing.T) {
	tests := []struct {
		name string
		cmd  *Command
		arg  string
		msg  string
	}{
		{"no input", cmd().Output("out.mp4"), "", "no input"},
		{"no output", cmd().Input("in.mp4"), "", "no output"},
		{"empty input path", cmd().Input("").Output("out.mp4"), "", "empty input path"},
		{"flag and value in one string", cmd().Input("in.mp4").Options("-c:v libx264").Output("out.mp4"),
			"-c:v libx264", `Options("-c:v", "libx264")`},
		{"missing value at end", cmd().Input("in.mp4").Options(VideoCodec).Output("out.mp4"), "-c:v", "missing value"},
		{"missing value before flag", cmd().Input("in.mp4").Options(VideoCodec, CRF, "23").Output("out.mp4"), "-c:v", "missing value"},
		{"value without option", cmd().Input("in.mp4").Options("libx264").Output("out.mp4"), "libx264", "value without an option"},
		{"unexpected value", cmd().Input("in.mp4").Options(NoAudio, "1").Output("out.mp4"), "1", "takes no value"},
		{"empty argument", cmd().Input("in.mp4").Options("").Output("out.mp4"), "", "empty argument"},
		{"bad time", cmd().Input("in.mp4").Options(Seek, "abc").Output("out.mp4"), "-ss", "invalid value"},
		{"bad number", cmd().Input("in.mp4").Options(CRF, "high").Output("out.mp4"), "-crf", "must be a number"},
		{"bad int", cmd().Input("in.mp4").Options(AudioChannels, "stereo").Output("out.mp4"), "-ac", "must be an integer"},
		{"bad size", cmd().Input("in.mp4").Options(Size, "1280*720").Output("out.mp4"), "-s", "must be a size"},
		{"bad bitrate", cmd().Input("in.mp4").Options(VideoBitrate, "fast").Output("out.mp4"), "-b:v", "must be a bitrate"},
		{"bad metadata", cmd().Input("in.mp4").Options(Metadata, "title").Output("out.mp4"), "-metadata", `"key=value"`},
		{"input-only option on output", cmd().Input("in.mp4").Options(ReadRealtime).Output("out.mp4"), "-re", "input-only"},
		{"output-only option on input", cmd().Options(Map, "0").Input("in.mp4").Output("out.mp4"), "-map", "output-only"},
		{"-i in options", cmd().Options("-i", "in.mp4").Output("out.mp4"), "-i", "use Input()"},
		{"-y in options", cmd().Input("in.mp4").Options("-y").Output("out.mp4"), "-y", "use Overwrite()"},
		{"trailing options", cmd().Input("in.mp4").Output("out.mp4").Options(VideoCodec, "libx264"), "-c:v", "after the last output"},
		{"flag as output path", cmd().Input("in.mp4").Output("-c:v"), "-c:v", "looks like an option"},
		{"output same as input", cmd().Input("in.mp4").Output("in.mp4"), "in.mp4", "same file as an input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.Validate()
			if !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("Validate() = %v, want ErrInvalidCommand", err)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err is %T, want *ValidationError", err)
			}
			for _, is := range ve.Issues {
				if is.Arg == tt.arg && strings.Contains(is.Message, tt.msg) {
					return
				}
			}
			t.Errorf("no issue with arg %q and message containing %q in: %v", tt.arg, tt.msg, err)
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	err := cmd().Options(ReadRealtime).Input("in.mp4").Options(VideoCodec, "-crf", "23").Output("out.mp4").Validate()
	want := `ffmpeg: invalid command: "-c:v" (for "out.mp4"): missing value`
	if err == nil || err.Error() != want {
		t.Errorf("Error() = %v, want %s", err, want)
	}
}

func TestFFmpegValidate(t *testing.T) {
	ff := newTest(t)
	ctx := context.Background()
	caps, err := ff.capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}

	valid := []*Command{
		ff.Command().Options(ForceFormat, "lavfi").Input("testsrc").Options(VideoCodec, "h264", "-nostats").Output("out.mp4"),
		ff.Command().Input("in.mp4").Options(MovFlags, "+faststart", "-fflags", "+genpts", "-bsf:v", "null").Output("out.mp4"),
		ff.Command().Options("-readrate", "1.5").Input("in.mp4").Options(AudioCodec, "aac").Output("out.m4a"),
	}
	if caps.encoders["libx264"] {
		valid = append(valid, ff.Command().Input("in.mp4").
			Options(VideoCodec, "libx264", "-x264-params", "keyint=60", Preset, "veryfast").Output("out.mp4"))
	}
	for _, c := range valid {
		if err := ff.Validate(ctx, c); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", c.Args(), err)
		}
	}

	invalid := []struct {
		cmd *Command
		msg string
	}{
		{ff.Command().Input("in.mp4").Options("-bogus", "1").Output("out.mp4"), "unknown option"},
		{ff.Command().Input("in.mp4").Options(VideoCodec, "nopecodec").Output("out.mp4"), "unknown encoder"},
		{ff.Command().Options(VideoCodec, "nopecodec").Input("in.mp4").Output("out.mp4"), "unknown decoder"},
		{ff.Command().Options(ForceFormat, "nopefmt").Input("in.mp4").Output("out.mp4"), "unknown input format"},
		{ff.Command().Input("in.mp4").Options(ForceFormat, "nopefmt").Output("out"), "unknown output format"},
		// -readrate is input-only according to ffmpeg itself (not in the built-in table).
		{ff.Command().Input("in.mp4").Options("-readrate", "1").Output("out.mp4"), "input-only"},
		// Static checks still apply.
		{ff.Command().Input("in.mp4").Options("-c:v libx264").Output("out.mp4"), "contains a space"},
	}
	for _, tt := range invalid {
		err := ff.Validate(ctx, tt.cmd)
		if !errors.Is(err, ErrInvalidCommand) || !strings.Contains(err.Error(), tt.msg) {
			t.Errorf("Validate(%q) = %v, want issue %q", tt.cmd.Args(), err, tt.msg)
		}
	}
}

// TestValidateAgreesWithFFmpeg runs commands that ffmpeg accepts and checks
// that the validator accepts them too (no false positives).
func TestValidateAgreesWithFFmpeg(t *testing.T) {
	ff := newTest(t)
	ctx := context.Background()
	in := testVideo(t, ff)
	dir := t.TempDir()
	out := func(name string) string { return filepath.Join(dir, name) }

	cmds := []*Command{
		ff.Command().Input(in).Options(VideoCodec, "mpeg4", Metadata, "title=My Movie", NoAudio).Output(out("a.mp4")),
		ff.Command().Options(Seek, "0.2").Input(in).Options(Duration, "0.5", Codec, Copy).Output(out("b.mp4")),
		ff.Command().Options(StreamLoop, "1", ReadRealtime).Input(in).Options(VideoFrames, "3").Output(out("c.mp4")),
		ff.Command().Input(in).Options(VideoFilter, "scale=32:32", PixelFormat, "yuv420p", FrameRate, "5", "-g", "10").Output(out("d.mp4")),
		ff.Command().Input(in).Options(Size, "32x32", AspectRatio, "1:1", VideoBitrate, "100k", MaxRate, "200k", BufSize, "400k").Output(out("e.mp4")),
		ff.Command().Options(ForceFormat, "lavfi").Input("sine=duration=0.2").
			Options(AudioCodec, "aac", SampleRate, "44100", AudioChannels, "1", AudioBitrate, "64k").Output(out("f.m4a")),
		ff.Command().Input(in).Options(Map, "0:v", MapMetadata, "-1", MovFlags, "+faststart", "-nostats").Output(out("g.mp4")),
		ff.Command().Input(in).Options(Size, "16x16").Output(out("h1.mp4")).Options(NoAudio, ForceFormat, "null").Output("-"),
		ff.Command().Input(in).Options(FilterComplex, "[0:v]split[a][b]", Map, "[a]").Output(out("i1.mp4")).Options(Map, "[b]").Output(out("i2.mp4")),
	}
	for i, c := range cmds {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if err := c.Run(ctx); err != nil {
				t.Fatalf("ffmpeg rejected the test command %q: %v", c.Args(), err)
			}
			if err := ff.Validate(ctx, c); err != nil {
				t.Errorf("ffmpeg accepts %q but Validate() = %v", c.Args(), err)
			}
		})
	}
}

func TestFFmpegValidateConcurrent(t *testing.T) {
	ff := newTest(t)
	c := ff.Command().Input("in.mp4").Options(VideoCodec, "mpeg4").Output("out.mp4")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := ff.Validate(context.Background(), c); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestFFmpegValidateCanceled(t *testing.T) {
	ff := newTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := ff.Command().Input("in.mp4").Output("out.mp4")
	if err := ff.Validate(ctx, c); !errors.Is(err, context.Canceled) {
		t.Fatalf("Validate() = %v, want context.Canceled", err)
	}
	// The failure must not be cached.
	if err := ff.Validate(context.Background(), c); err != nil {
		t.Fatalf("Validate() after cancel = %v, want nil", err)
	}
}

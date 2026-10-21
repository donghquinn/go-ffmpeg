package ffmpeg

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The wrapper's work on top of ffmpeg itself is building arguments,
// capturing stderr and, on failure only, classifying it. These benchmarks
// compare a wrapped run against a plain os/exec run of the same command, and
// measure the wrapper-only parts in isolation.

const benchStderr = "[in#0 @ 0x841028000] Error opening input: No such file or directory\n" +
	"Error opening input file nope.mp4.\n" +
	"Error opening input files: No such file or directory\n"

func benchFFmpeg(b *testing.B) *FFmpeg {
	b.Helper()
	ff, err := New()
	if err != nil {
		b.Skip("ffmpeg not installed")
	}
	return ff
}

// benchArgs encodes a tiny generated clip, so the timing is dominated by
// process startup and the wrapper's share is as visible as it can be.
func benchArgs(b *testing.B) []string {
	out := filepath.Join(b.TempDir(), "out.mp4")
	return []string{"-y", "-f", "lavfi", "-i", "testsrc=duration=0.1:size=32x32:rate=10", out}
}

func BenchmarkRun(b *testing.B) {
	ff := benchFFmpeg(b)
	args := benchArgs(b)
	ctx := context.Background()

	b.Run("exec", func(b *testing.B) {
		full := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, args...)
		for i := 0; i < b.N; i++ {
			var stderr bytes.Buffer
			cmd := exec.Command(ff.ffmpegPath, full...)
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				b.Fatal(err, stderr.String())
			}
		}
	})
	b.Run("wrapper", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := ff.Run(ctx, args...); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkRunError(b *testing.B) {
	ff := benchFFmpeg(b)
	missing := filepath.Join(b.TempDir(), "nope.mp4")
	args := []string{"-i", missing, filepath.Join(b.TempDir(), "out.mp4")}
	ctx := context.Background()

	b.Run("exec", func(b *testing.B) {
		full := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, args...)
		for i := 0; i < b.N; i++ {
			var stderr bytes.Buffer
			cmd := exec.Command(ff.ffmpegPath, full...)
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				b.Fatal("expected error")
			}
		}
	})
	b.Run("wrapper", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := ff.Run(ctx, args...); err == nil {
				b.Fatal("expected error")
			}
		}
	})
}

func BenchmarkCommandArgs(b *testing.B) {
	ff := &FFmpeg{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ff.Command().
			Options("-ss", "5").
			Input("in.mp4").
			Options("-c:v", "libx264", "-crf", "23").
			Output("out.mp4").
			Overwrite().
			Args()
	}
}

func BenchmarkClassify(b *testing.B) {
	b.Run("typical", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			classify(benchStderr)
		}
	})
	// Worst case: a full 64 KiB stderr tail (e.g. -loglevel info) with no
	// recognizable error, so every rule scans every line.
	b.Run("64KiB-unclassified", func(b *testing.B) {
		line := "frame=  100 fps= 25 q=28.0 size=    256KiB time=00:00:04.00 bitrate= 524.3kbits/s speed=1.0x\n"
		big := strings.Repeat(line, maxStderr/len(line))
		b.SetBytes(int64(len(big)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			classify(big)
		}
	})
}

func BenchmarkTailBuffer(b *testing.B) {
	chunk := bytes.Repeat([]byte("x"), 4096)
	t := &tailBuffer{max: maxStderr}
	b.SetBytes(int64(len(chunk)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		t.Write(chunk)
	}
}

func benchCommand(ff *FFmpeg) *Command {
	return ff.Command().
		Options(Seek, "30").
		Input("in.mp4").
		Options(VideoCodec, "libx264", CRF, "23", AudioCodec, Copy, Metadata, "title=My Movie", MovFlags, "+faststart").
		Output("out.mp4")
}

func BenchmarkValidate(b *testing.B) {
	b.Run("static", func(b *testing.B) {
		c := benchCommand(&FFmpeg{})
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if err := c.Validate(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ffmpeg-cached", func(b *testing.B) {
		ff := benchFFmpeg(b)
		c := benchCommand(ff)
		ctx := context.Background()
		if err := ff.Validate(ctx, c); err != nil { // load capabilities once
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := ff.Validate(ctx, c); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("ffmpeg-first-call", func(b *testing.B) {
		ff := benchFFmpeg(b)
		c := benchCommand(ff)
		ctx := context.Background()
		for i := 0; i < b.N; i++ {
			ff.caps = nil
			if err := ff.Validate(ctx, c); err != nil {
				b.Fatal(err)
			}
		}
	})
}

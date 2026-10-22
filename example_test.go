package ffmpeg_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/donghquinn/go-ffmpeg"
)

func Example() {
	ff, err := ffmpeg.New()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	err = ff.Command().
		Input("in.mov").
		Options("-c:v", "libx264", "-crf", "23", "-c:a", "aac").
		Output("out.mp4").
		Overwrite().
		Run(ctx)

	var fe *ffmpeg.Error
	switch {
	case err == nil:
		fmt.Println("done")
	case errors.Is(err, ffmpeg.ErrFileNotFound):
		fmt.Println("input not found:", err)
	case errors.Is(err, ffmpeg.ErrEncoderNotFound):
		fmt.Println("ffmpeg was built without libx264")
	case errors.Is(err, context.DeadlineExceeded):
		fmt.Println("transcoding took too long")
	case errors.As(err, &fe):
		fmt.Printf("ffmpeg failed with exit code %d: %s\n", fe.ExitCode, fe.Message)
	default:
		log.Fatal(err)
	}
}

// Flags can be package constants or plain strings, mixed freely.
func ExampleCommand_Options() {
	ff, err := ffmpeg.New()
	if err != nil {
		log.Fatal(err)
	}

	// ffmpeg -y -ss 30 -i in.mp4 -t 10 -c:v libx264 -crf 23 -c:a copy -movflags +faststart clip.mp4
	err = ff.Command().
		Options(ffmpeg.Seek, "30").
		Input("in.mp4").
		Options(
			ffmpeg.Duration, "10",
			ffmpeg.VideoCodec, "libx264",
			"-crf", "23", // a plain string works too
			ffmpeg.AudioCodec, ffmpeg.Copy,
			ffmpeg.MovFlags, "+faststart",
		).
		Output("clip.mp4").
		Overwrite().
		Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}

// Build options per case from a shared preset.
func ExampleOptionSet() {
	base := ffmpeg.NewOptionSet().
		VideoCodec("libx264").Preset("veryfast").PixelFormat("yuv420p").
		AudioCodec("aac").MovFlags("+faststart")

	type rendition struct {
		height  int
		crf     float64
		noAudio bool
	}
	cmd := (&ffmpeg.FFmpeg{}).Command().
		With(ffmpeg.NewOptionSet().Seek(90 * time.Second)).
		Input("in.mov")
	for _, r := range []rendition{{1080, 20, false}, {480, 26, true}} {
		opts := base.Clone().
			VideoFilter(fmt.Sprintf("scale=-2:%d", r.height)).
			CRF(r.crf).
			When(r.noAudio, (*ffmpeg.OptionSet).NoAudio)
		cmd.With(opts).Output(fmt.Sprintf("out_%dp.mp4", r.height))
	}
	fmt.Println(strings.Join(cmd.Args(), " "))
	// Output:
	// -n -ss 90 -i in.mov -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -movflags +faststart -vf scale=-2:1080 -crf 20 out_1080p.mp4 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac -movflags +faststart -vf scale=-2:480 -crf 26 -an out_480p.mp4
}

func ExampleCommand_String() {
	cmd := (&ffmpeg.FFmpeg{}).Command().
		Input("my video.mov").
		Options("-vf", "scale=-2:720").
		Output("out.mp4").
		Overwrite()
	fmt.Println(cmd)
	// Output:
	// ffmpeg -hide_banner -nostdin -y -i 'my video.mov' -vf scale=-2:720 out.mp4
}

func ExampleCommand_Validate() {
	cmd := (&ffmpeg.FFmpeg{}).Command().
		Input("in.mp4").
		Options("-c:v libx264", ffmpeg.CRF, "high", ffmpeg.ReadRealtime).
		Output("out.mp4")

	var ve *ffmpeg.ValidationError
	if errors.As(cmd.Validate(), &ve) {
		for _, issue := range ve.Issues {
			fmt.Println(issue)
		}
	}
	// Output:
	// "-c:v libx264" (for "out.mp4"): contains a space; pass the option and its value as separate arguments, e.g. Options("-c:v", "libx264")
	// "-crf" (for "out.mp4"): invalid value "high": must be a number
	// "-re" (for "out.mp4"): input-only option placed before an output; move it before the Input() it applies to
}

func ExampleFFmpeg_Probe() {
	ff, err := ffmpeg.New()
	if err != nil {
		log.Fatal(err)
	}

	info, err := ff.Probe(context.Background(), "in.mp4")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("duration:", info.Duration())
	for _, s := range info.VideoStreams() {
		fmt.Printf("video #%d: %s %dx%d\n", s.Index, s.CodecName, s.Width, s.Height)
	}
}

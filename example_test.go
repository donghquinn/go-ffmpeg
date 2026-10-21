package ffmpeg_test

import (
	"context"
	"errors"
	"fmt"
	"log"
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

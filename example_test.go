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
		Output("out.mp4", "-c:v", "libx264", "-crf", "23", "-c:a", "aac").
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

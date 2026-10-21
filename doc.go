// Package ffmpeg is a thin, dependency-free wrapper around the ffmpeg and
// ffprobe command-line tools.
//
// The package runs the binaries installed on the host (found in $PATH or set
// explicitly with [WithFFmpegPath] / [WithFFprobePath]) and turns their exit
// status and stderr output into structured Go errors.
//
// # Running commands
//
//	ff, err := ffmpeg.New()
//	if err != nil {
//		return err // errors.Is(err, ffmpeg.ErrBinaryNotFound)
//	}
//
//	err = ff.Command().
//		Input("in.mov").
//		Output("out.mp4", "-c:v", "libx264", "-crf", "23").
//		Overwrite().
//		Run(ctx)
//
// # Error handling
//
// Every failure of an ffmpeg or ffprobe process is reported as an [*Error],
// which carries the exit code, the arguments, the captured stderr and the most
// relevant error line reported by ffmpeg. Common failures are also classified
// into sentinel errors that can be matched with [errors.Is]:
//
//	var fe *ffmpeg.Error
//	switch {
//	case errors.Is(err, ffmpeg.ErrFileNotFound):
//		// input (or output directory) does not exist
//	case errors.Is(err, ffmpeg.ErrEncoderNotFound):
//		// e.g. ffmpeg was built without libx264
//	case errors.Is(err, context.DeadlineExceeded):
//		// the context passed to Run expired and ffmpeg was stopped
//	case errors.As(err, &fe):
//		log.Printf("ffmpeg exited with %d: %s\n%s", fe.ExitCode, fe.Message, fe.Stderr)
//	}
package ffmpeg

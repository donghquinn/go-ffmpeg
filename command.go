package ffmpeg

import "context"

// Command builds an ffmpeg invocation. Arguments are emitted in the order the
// methods are called, exactly like on the ffmpeg command line, where an
// option applies to the next input or output that follows it:
//
//	ff.Command().
//		Options("-ss", "5").                    // input option for in.mp4
//		Input("in.mp4").
//		Options("-c:v", "libx264", "-crf", "23"). // output options for out.mp4
//		Output("out.mp4").
//		Run(ctx)
//
// Create one with [FFmpeg.Command]. Methods return the receiver so calls can
// be chained. A Command is not safe for concurrent modification.
type Command struct {
	ff        *FFmpeg
	args      []string
	overwrite bool
}

// Input adds an input file or URL ("-i path"). Options added before it are
// input options for this input.
func (c *Command) Input(path string) *Command {
	c.args = append(c.args, "-i", path)
	return c
}

// Options adds options at the current position. Placed before [Command.Input]
// they apply to that input; placed before [Command.Output] they apply to that
// output.
//
// Pass each flag and value as a separate argument ("-c:v", "libx264"), as with
// [os/exec]. Arguments are passed to ffmpeg as-is, never split or interpreted
// by a shell, so values containing spaces (paths, filter graphs, metadata)
// need no quoting.
func (c *Command) Options(args ...string) *Command {
	c.args = append(c.args, args...)
	return c
}

// Output adds an output file or URL. Options added since the previous input
// or output are output options for this output.
func (c *Command) Output(path string) *Command {
	c.args = append(c.args, path)
	return c
}

// Overwrite makes ffmpeg overwrite existing output files ("-y"). Without it,
// an existing output file makes Run fail with [ErrOutputExists] ("-n").
func (c *Command) Overwrite() *Command {
	c.overwrite = true
	return c
}

// Args returns the arguments the command will pass to ffmpeg, excluding the
// ones [FFmpeg.Run] adds itself. Useful for logging and tests.
func (c *Command) Args() []string {
	flag := "-n"
	if c.overwrite {
		flag = "-y"
	}
	return append([]string{flag}, c.args...)
}

// Run runs the command. It returns nil on success and an [*Error] otherwise.
// When ctx is done, ffmpeg is asked to stop and the returned error matches
// ctx.Err() with [errors.Is].
func (c *Command) Run(ctx context.Context) error {
	return c.ff.Run(ctx, c.Args()...)
}

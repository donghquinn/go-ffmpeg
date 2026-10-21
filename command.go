package ffmpeg

import "context"

// Command builds an ffmpeg invocation of the form
//
//	ffmpeg [global options] {[input options] -i input}... {[output options] output}...
//
// Create one with [FFmpeg.Command]. Methods return the receiver so calls can
// be chained. A Command is not safe for concurrent modification.
type Command struct {
	ff        *FFmpeg
	global    []string
	inputs    [][]string
	outputs   [][]string
	overwrite bool
}

// Global adds global options, placed before all inputs (e.g. "-threads", "4").
func (c *Command) Global(args ...string) *Command {
	c.global = append(c.global, args...)
	return c
}

// Input adds an input file. opts are input options applied to this input and
// placed before its "-i" (e.g. "-ss", "10", "-f", "lavfi").
func (c *Command) Input(path string, opts ...string) *Command {
	in := append(append([]string{}, opts...), "-i", path)
	c.inputs = append(c.inputs, in)
	return c
}

// Output adds an output file. opts are output options applied to this output
// and placed before its path (e.g. "-c:v", "libx264", "-crf", "23").
func (c *Command) Output(path string, opts ...string) *Command {
	out := append(append([]string{}, opts...), path)
	c.outputs = append(c.outputs, out)
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
	var args []string
	if c.overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	args = append(args, c.global...)
	for _, in := range c.inputs {
		args = append(args, in...)
	}
	for _, out := range c.outputs {
		args = append(args, out...)
	}
	return args
}

// Run runs the command. It returns nil on success and an [*Error] otherwise.
// When ctx is done, ffmpeg is asked to stop and the returned error matches
// ctx.Err() with [errors.Is].
func (c *Command) Run(ctx context.Context) error {
	return c.ff.Run(ctx, c.Args()...)
}

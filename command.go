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
	parts     []part
	n         int // total number of arguments in parts
	overwrite bool
}

type partKind uint8

const (
	partOptions partKind = iota
	partInput
	partOutput
)

// part is one builder call: a group of options, or an input or output path.
type part struct {
	kind partKind
	args []string // options, or the single path
}

func (c *Command) add(kind partKind, args []string) *Command {
	c.parts = append(c.parts, part{kind: kind, args: args})
	c.n += len(args)
	return c
}

// Input adds an input file or URL ("-i path"). Options added before it are
// input options for this input.
func (c *Command) Input(path string) *Command {
	c.n++ // "-i"
	return c.add(partInput, []string{path})
}

// Options adds options at the current position. Placed before [Command.Input]
// they apply to that input; placed before [Command.Output] they apply to that
// output.
//
// Pass each flag and value as a separate argument ("-c:v", "libx264"), as with
// [os/exec]. Arguments are passed to ffmpeg as-is, never split or interpreted
// by a shell, so values containing spaces (paths, filter graphs, metadata)
// need no quoting. Use [Command.Validate] to check them before running.
func (c *Command) Options(args ...string) *Command {
	if len(args) == 0 {
		return c
	}
	return c.add(partOptions, append([]string(nil), args...))
}

// With adds the options of each set at the current position, like
// [Command.Options]. Nil sets are skipped, so optional sets can be passed
// directly. The sets are copied; changing them later does not affect c.
func (c *Command) With(sets ...*OptionSet) *Command {
	for _, s := range sets {
		if s != nil && s.Len() > 0 {
			c.add(partOptions, s.Args())
		}
	}
	return c
}

// Output adds an output file or URL. Options added since the previous input
// or output are output options for this output.
func (c *Command) Output(path string) *Command {
	return c.add(partOutput, []string{path})
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
	args := make([]string, 0, c.n+1)
	if c.overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	for _, p := range c.parts {
		if p.kind == partInput {
			args = append(args, "-i")
		}
		args = append(args, p.args...)
	}
	return args
}

// Run runs the command. It returns nil on success and an [*Error] otherwise.
// When ctx is done, ffmpeg is asked to stop and the returned error matches
// ctx.Err() with [errors.Is].
//
// Run does not validate the command; call [Command.Validate] or
// [FFmpeg.Validate] first to catch mistakes without starting ffmpeg.
func (c *Command) Run(ctx context.Context) error {
	return c.ff.Run(ctx, c.Args()...)
}

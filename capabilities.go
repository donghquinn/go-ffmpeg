package ffmpeg

import (
	"bytes"
	"context"
	"regexp"
	"strings"
)

// capabilities is what the installed ffmpeg supports, parsed from its help
// and listing output.
type capabilities struct {
	options  map[string]optSpec
	encoders map[string]bool // encoder and codec names
	decoders map[string]bool
	muxers   map[string]bool
	demuxers map[string]bool
}

// Validate checks cmd like [Command.Validate] and also checks it against the
// installed ffmpeg: every option must exist (including codec and format
// private options such as -x264-params), must be applicable to an input or
// output where it is placed, and -c/-f values must name an available
// encoder, decoder, muxer or demuxer.
//
// The first call runs ffmpeg a few times to read its capabilities (typically
// well under a second); the result is cached for the lifetime of f.
//
// Validate returns nil, a [*ValidationError] matching [ErrInvalidCommand], or
// an [*Error] if ffmpeg's capabilities could not be read.
func (f *FFmpeg) Validate(ctx context.Context, cmd *Command) error {
	caps, err := f.capabilities(ctx)
	if err != nil {
		return err
	}
	return cmd.validate(caps)
}

func (f *FFmpeg) capabilities(ctx context.Context) (*capabilities, error) {
	f.capsMu.Lock()
	defer f.capsMu.Unlock()
	if f.caps != nil {
		return f.caps, nil
	}

	out := map[string]string{}
	for _, topic := range []string{"-encoders", "-decoders", "-muxers", "-demuxers"} {
		s, err := f.output(ctx, "-hide_banner", topic)
		if err != nil {
			return nil, err
		}
		out[topic] = s
	}
	help, err := f.output(ctx, "-hide_banner", "-h", "full")
	if err != nil {
		return nil, err
	}

	// Only cache on success, so a canceled context is not remembered.
	f.caps = &capabilities{
		options:  parseHelp(help),
		encoders: parseCodecs(out["-encoders"]),
		decoders: parseCodecs(out["-decoders"]),
		muxers:   parseFormats(out["-muxers"]),
		demuxers: parseFormats(out["-demuxers"]),
	}
	return f.caps, nil
}

// output runs ffmpeg with args and returns its stdout.
func (f *FFmpeg) output(ctx context.Context, args ...string) (string, error) {
	var stdout bytes.Buffer
	if _, err := f.run(ctx, f.ffmpegPath, "ffmpeg", args, &stdout); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// "-c[:<stream_spec>] <codec>  select encoder/decoder" / "-y   overwrite output files"
var helpOptionRe = regexp.MustCompile(`^-([^\s\[]+)(?:\[:<[^>]*>\])?( <([^>]*)>)?`)

// parseHelp parses the output of "ffmpeg -h full" into option specs.
//
// It has two kinds of option lines:
//
//	-ss <time_off>      start transcoding at specified time
//	  -crf               <float>      E..V....... Select the quality ...
//
// The first are ffmpeg's own options, grouped under section headers such as
// "Per-file options (input-only):"; the second are codec/format AVOptions
// whose flags column says whether they apply to encoding (E, outputs) or
// decoding (D, inputs). Arity is only trusted when the help shows a named
// argument, since boolean options are listed inconsistently ("-re <>").
func parseHelp(help string) map[string]optSpec {
	opts := make(map[string]optSpec, 4096)
	merge := func(name string, s optSpec) {
		if old, ok := opts[name]; ok {
			s.scope |= old.scope
			if s.arity != old.arity {
				s.arity = arityUnknown
			}
		}
		opts[name] = s
	}

	sc := scopeAny
	for _, line := range strings.Split(help, "\n") {
		switch {
		case line == "":
		case line[0] == '-':
			m := helpOptionRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			s := optSpec{arity: arityNone, scope: sc}
			if m[2] != "" {
				s.arity = arityOne
				if m[3] == "" || m[3] == m[1] {
					s.arity = arityUnknown
				}
			}
			merge(m[1], s)
		case strings.HasPrefix(line, "  -"):
			f := strings.Fields(line)
			if len(f) < 3 || !strings.HasPrefix(f[1], "<") || len(f[2]) < 2 {
				continue
			}
			s := optSpec{arity: arityOne, scope: scopeAny}
			switch enc, dec := f[2][0] == 'E', f[2][1] == 'D'; {
			case enc && !dec:
				s.scope = scopeOutput
			case dec && !enc:
				s.scope = scopeInput
			}
			merge(f[0][1:], s)
		case line[0] != ' ':
			// Section header.
			switch l := strings.ToLower(line); {
			case strings.Contains(l, "input-only"):
				sc = scopeInput
			case strings.Contains(l, "output-only"):
				sc = scopeOutput
			case strings.Contains(l, "global"), strings.Contains(l, "help"), strings.Contains(l, "capabilities"):
				sc = scopeGlobal | scopeAny
			default:
				sc = scopeAny
			}
		}
	}
	return opts
}

var codecRe = regexp.MustCompile(`\(codec (\S+)\)`)

// parseCodecs parses "ffmpeg -encoders" / "-decoders" output:
//
//	V....D libx264              libx264 H.264 / AVC ... (codec h264)
//
// Both the encoder name and the codec name are accepted by -c.
func parseCodecs(list string) map[string]bool {
	names := map[string]bool{}
	_, list, _ = strings.Cut(list, "------\n")
	for _, line := range strings.Split(list, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		names[f[1]] = true
		if m := codecRe.FindStringSubmatch(line); m != nil {
			names[m[1]] = true
		}
	}
	return names
}

// parseFormats parses "ffmpeg -muxers" / "-demuxers" output:
//
//	D   mov,mp4,m4a,3gp,3g2,mj2 QuickTime / MOV
//	D d lavfi           Libavfilter virtual input device
func parseFormats(list string) map[string]bool {
	names := map[string]bool{}
	_, list, _ = strings.Cut(list, "---\n")
	for _, line := range strings.Split(list, "\n") {
		if len(line) < 5 {
			continue
		}
		f := strings.Fields(line[4:])
		if len(f) == 0 {
			continue
		}
		for _, n := range strings.Split(f[0], ",") {
			names[n] = true
		}
	}
	return names
}

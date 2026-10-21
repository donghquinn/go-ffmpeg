package ffmpeg

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrInvalidCommand is matched by errors returned from [Command.Validate] and
// [FFmpeg.Validate] when the command has problems. Use [errors.As] with a
// [*ValidationError] to get the individual issues.
var ErrInvalidCommand = errors.New("ffmpeg: invalid command")

// ValidationError lists the problems found in a command.
type ValidationError struct {
	Issues []Issue
}

// Issue is a single problem found in a command.
type Issue struct {
	// Arg is the offending argument, e.g. "-c:v". It may be empty for
	// problems with the command as a whole, such as a missing output.
	Arg string

	// Target is the input or output path the argument applies to, if any.
	Target string

	// Message describes the problem.
	Message string
}

func (i Issue) String() string {
	var b strings.Builder
	if i.Arg != "" || i.Target != "" {
		b.WriteString(strconv.Quote(i.Arg))
		if i.Target != "" {
			fmt.Fprintf(&b, " (for %q)", i.Target)
		}
		b.WriteString(": ")
	}
	b.WriteString(i.Message)
	return b.String()
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		msgs[i] = is.String()
	}
	return ErrInvalidCommand.Error() + ": " + strings.Join(msgs, "; ")
}

// Unwrap returns [ErrInvalidCommand].
func (e *ValidationError) Unwrap() error { return ErrInvalidCommand }

// Validate checks the command for mistakes without running ffmpeg:
//
//   - at least one input and one output, with non-empty paths
//   - each Options argument is a flag ("-name") or the value of the flag
//     before it, and flags are not given as "-flag value" in one string
//   - known flags that need a value have one, and values have the right
//     shape (times for -ss/-t/-to, integers, sizes, bitrates, key=value
//     metadata, ...)
//   - input-only flags (e.g. -re, -stream_loop) are not placed before an
//     output, and output-only flags (e.g. -map, -crf) not before an input
//   - no options after the last output, which ffmpeg would ignore
//   - no "-i", "-y" or "-n" in Options: use [Command.Input] and
//     [Command.Overwrite]
//
// Flags this package does not know are accepted as-is. To also check every
// option, codec and format name against the installed ffmpeg, use
// [FFmpeg.Validate].
//
// Validate returns nil or a [*ValidationError] matching [ErrInvalidCommand].
func (c *Command) Validate() error {
	return c.validate(nil)
}

func (c *Command) validate(caps *capabilities) error {
	v := validator{caps: caps}
	var pending []string // options waiting for the input/output they apply to
	inputs := map[string]bool{}
	nIn, nOut := 0, 0

	for _, p := range c.parts {
		switch p.kind {
		case partOptions:
			pending = append(pending, p.args...)
		case partInput:
			path := p.args[0]
			nIn++
			inputs[path] = true
			v.group(pending, partInput, path)
			pending = pending[:0]
			if path == "" {
				v.add("", "", "empty input path")
			}
		case partOutput:
			path := p.args[0]
			nOut++
			v.group(pending, partOutput, path)
			pending = pending[:0]
			switch {
			case path == "":
				v.add("", "", "empty output path")
			case isFlag(path):
				v.add(path, "", "output path looks like an option; pass options with Options() before Output()")
			case inputs[path] && !strings.Contains(path, ":"):
				v.add(path, "", "output is the same file as an input")
			}
		}
	}
	if len(pending) > 0 {
		v.group(pending, partOptions, "")
		v.add(pending[0], "", "options after the last output are ignored by ffmpeg; move them before an Output()")
	}
	if nIn == 0 {
		v.add("", "", "no input: add one with Input()")
	}
	if nOut == 0 {
		v.add("", "", "no output: add one with Output()")
	}

	if len(v.issues) > 0 {
		return &ValidationError{Issues: v.issues}
	}
	return nil
}

type validator struct {
	caps   *capabilities // nil for static validation
	issues []Issue
}

func (v *validator) add(arg, target, format string, a ...any) {
	v.issues = append(v.issues, Issue{Arg: arg, Target: target, Message: fmt.Sprintf(format, a...)})
}

// group validates the options that apply to target, which is an input or
// output path (kind partInput/partOutput), or none (partOptions) for trailing
// options.
func (v *validator) group(args []string, kind partKind, target string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			v.add(arg, target, "empty argument")
			continue
		}
		if !isFlag(arg) {
			v.add(arg, target, "value without an option before it")
			continue
		}
		if strings.ContainsAny(arg, " \t") {
			v.add(arg, target, "contains a space; pass the option and its value as separate arguments, e.g. Options(%q, %q)",
				strings.Fields(arg)[0], strings.Join(strings.Fields(arg)[1:], " "))
			continue
		}

		name := optionName(arg)
		var value string
		hasValue := i+1 < len(args) && !isFlag(args[i+1])
		if hasValue {
			value = args[i+1]
		}

		switch name {
		case "i":
			v.add(arg, target, "use Input() to add inputs")
			if hasValue {
				i++
			}
			continue
		case "y", "n":
			v.add(arg, target, "conflicts with the -n/-y added by Command; use Overwrite()")
			continue
		}

		spec, known := v.lookup(name)
		if !known {
			if v.caps != nil {
				v.add(arg, target, "unknown option (see ffmpeg -h full)")
			}
			if hasValue {
				i++ // best effort: treat the next non-flag as its value
			}
			continue
		}

		switch spec.arity {
		case arityOne:
			if !hasValue {
				v.add(arg, target, "missing value")
				continue
			}
			i++
			if spec.check != nil {
				if err := spec.check(value); err != nil {
					v.add(arg, target, "invalid value %q: %v", value, err)
				}
			}
			v.checkName(arg, name, value, kind, target)
		case arityNone:
			if hasValue && spec.strict {
				v.add(args[i+1], target, "unexpected value: %s takes no value", arg)
				i++
			}
		case arityUnknown:
			if hasValue {
				i++
			}
		}

		switch {
		case kind == partOutput && spec.scope == scopeInput:
			v.add(arg, target, "input-only option placed before an output; move it before the Input() it applies to")
		case kind == partInput && spec.scope == scopeOutput:
			v.add(arg, target, "output-only option placed before an input; move it before the Output() it applies to")
		}
	}
}

// checkName checks codec and format names against the installed ffmpeg.
func (v *validator) checkName(arg, name, value string, kind partKind, target string) {
	if v.caps == nil || kind == partOptions {
		return
	}
	switch name {
	case "c", "codec", "vcodec", "acodec", "scodec", "dcodec":
		if value == Copy {
			return
		}
		if kind == partInput {
			if !v.caps.decoders[value] {
				v.add(arg, target, "unknown decoder %q (see ffmpeg -decoders)", value)
			}
		} else if !v.caps.encoders[value] {
			v.add(arg, target, "unknown encoder %q (see ffmpeg -encoders)", value)
		}
	case "f":
		if kind == partInput {
			if !v.caps.demuxers[value] {
				v.add(arg, target, "unknown input format %q (see ffmpeg -demuxers)", value)
			}
		} else if !v.caps.muxers[value] {
			v.add(arg, target, "unknown output format %q (see ffmpeg -muxers)", value)
		}
	}
}

// lookup returns the spec of an option: from the built-in table first, which
// has exact arity and value checks, then from the installed ffmpeg.
func (v *validator) lookup(name string) (optSpec, bool) {
	if s, ok := knownOptions[name]; ok {
		return s, true
	}
	if v.caps == nil {
		return optSpec{}, false
	}
	if s, ok := v.caps.options[name]; ok {
		return s, true
	}
	// Boolean options can be negated with a "no" prefix, e.g. -nostdin.
	if base, ok := strings.CutPrefix(name, "no"); ok {
		if s, ok := v.caps.options[base]; ok && s.arity != arityOne {
			return optSpec{arity: arityNone, scope: s.scope}, true
		}
	}
	return optSpec{}, false
}

// isFlag reports whether arg is an option name rather than a value. Negative
// numbers ("-1") and negative map specifiers ("-0:a") are values, as is "-"
// (stdin/stdout).
func isFlag(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	c := arg[1]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

// optionName returns the option name of a flag without the leading dash and
// any stream specifier: "-c:v:0" -> "c".
func optionName(flag string) string {
	name := flag[1:]
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name = name[:i]
	}
	return name
}

type arity uint8

const (
	arityUnknown arity = iota // takes a value or not; accept both
	arityNone                 // takes no value
	arityOne                  // takes exactly one value
)

type scope uint8

const (
	scopeInput  scope = 1 << iota // may be applied to inputs
	scopeOutput                   // may be applied to outputs
	scopeGlobal                   // global option, allowed anywhere

	scopeAny = scopeInput | scopeOutput
)

type optSpec struct {
	arity arity
	scope scope
	// strict reports a value following an arityNone option as an error.
	strict bool
	check  func(string) error
}

func one(sc scope, check func(string) error) optSpec {
	return optSpec{arity: arityOne, scope: sc, check: check}
}

func none(sc scope) optSpec { return optSpec{arity: arityNone, scope: sc, strict: true} }

// knownOptions describes common ffmpeg options precisely. Options missing
// here are accepted by [Command.Validate] and checked against the installed
// ffmpeg by [FFmpeg.Validate].
var knownOptions = map[string]optSpec{
	"f": one(scopeAny, checkName),

	"c":      one(scopeAny, checkName),
	"codec":  one(scopeAny, checkName),
	"vcodec": one(scopeAny, checkName),
	"acodec": one(scopeAny, checkName),
	"scodec": one(scopeAny, checkName),
	"dcodec": one(scopeAny, checkName),

	"ss":    one(scopeAny, checkTime),
	"sseof": one(scopeInput, checkTime),
	"t":     one(scopeAny, checkTime),
	"to":    one(scopeAny, checkTime),

	"map":          one(scopeOutput, checkNonEmpty),
	"map_metadata": one(scopeOutput, checkNonEmpty),
	"metadata":     one(scopeOutput, checkKeyValue),

	"vn": none(scopeAny),
	"an": none(scopeAny),
	"sn": none(scopeAny),
	"dn": none(scopeAny),

	"vf":             one(scopeOutput, checkNonEmpty),
	"af":             one(scopeOutput, checkNonEmpty),
	"filter":         one(scopeOutput, checkNonEmpty),
	"filter_complex": one(scopeGlobal, checkNonEmpty),

	"b":       one(scopeOutput, checkBitrate),
	"maxrate": one(scopeOutput, checkBitrate),
	"minrate": one(scopeOutput, checkBitrate),
	"bufsize": one(scopeOutput, checkBitrate),
	"crf":     one(scopeOutput, checkNumber),
	"preset":  one(scopeOutput, checkName),
	"tune":    one(scopeOutput, checkName),
	"profile": one(scopeOutput, checkName),
	"q":       one(scopeOutput, checkNumber),
	"qscale":  one(scopeOutput, checkNumber),

	"r":       one(scopeAny, checkRate),
	"s":       one(scopeAny, checkSize),
	"aspect":  one(scopeOutput, checkAspect),
	"pix_fmt": one(scopeAny, checkName),
	"frames":  one(scopeOutput, checkInt),
	"vframes": one(scopeOutput, checkInt),
	"aframes": one(scopeOutput, checkInt),

	"ar": one(scopeAny, checkInt),
	"ac": one(scopeAny, checkInt),

	"threads":     one(scopeAny, checkThreads),
	"hwaccel":     one(scopeInput, checkName),
	"re":          none(scopeInput),
	"stream_loop": one(scopeInput, checkInt),
	"shortest":    none(scopeOutput),
	"movflags":    one(scopeOutput, checkNonEmpty),

	"loglevel":    one(scopeGlobal, checkName),
	"v":           one(scopeGlobal, checkName),
	"hide_banner": none(scopeGlobal),
	"nostdin":     none(scopeGlobal),
	"stats":       none(scopeGlobal),
	"nostats":     none(scopeGlobal),
}

var (
	// ffmpeg time duration syntax: [-][HH:]MM:SS[.m...] or [-]S+[.m...][s|ms|us]
	timeRe    = regexp.MustCompile(`^-?(?:(?:\d+:)?\d+:\d+(?:\.\d+)?|\d+(?:\.\d+)?(?:s|ms|us)?)$`)
	bitrateRe = regexp.MustCompile(`^\d+(?:\.\d+)?(?:[kKMGTP]i?)?B?$`)
	rateRe    = regexp.MustCompile(`^(?:\d+(?:\.\d+)?(?:/\d+)?|[a-z][a-z0-9_-]*)$`)
	sizeRe    = regexp.MustCompile(`^(?:\d+x\d+|[a-z][a-z0-9]*)$`)
	aspectRe  = regexp.MustCompile(`^\d+(?:\.\d+)?(?:[:/]\d+(?:\.\d+)?)?$`)
)

func checkNonEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("must not be empty")
	}
	return nil
}

func checkName(s string) error {
	if s == "" || strings.ContainsAny(s, " \t") {
		return errors.New("must be a single name")
	}
	return nil
}

func checkTime(s string) error {
	if !timeRe.MatchString(s) {
		return errors.New(`must be a time like "90", "1.5", "500ms" or "01:30:00"`)
	}
	return nil
}

func checkInt(s string) error {
	if _, err := strconv.Atoi(s); err != nil {
		return errors.New("must be an integer")
	}
	return nil
}

func checkNumber(s string) error {
	if _, err := strconv.ParseFloat(s, 64); err != nil {
		return errors.New("must be a number")
	}
	return nil
}

func checkThreads(s string) error {
	if s == "auto" {
		return nil
	}
	return checkInt(s)
}

func checkBitrate(s string) error {
	if !bitrateRe.MatchString(s) {
		return errors.New(`must be a bitrate like "128000", "128k" or "2M"`)
	}
	return nil
}

func checkRate(s string) error {
	if !rateRe.MatchString(s) {
		return errors.New(`must be a rate like "30", "29.97", "30000/1001" or "ntsc"`)
	}
	return nil
}

func checkSize(s string) error {
	if !sizeRe.MatchString(s) {
		return errors.New(`must be a size like "1280x720" or "hd720"`)
	}
	return nil
}

func checkAspect(s string) error {
	if !aspectRe.MatchString(s) {
		return errors.New(`must be an aspect ratio like "16:9" or "1.7777"`)
	}
	return nil
}

func checkKeyValue(s string) error {
	if k, _, ok := strings.Cut(s, "="); !ok || k == "" {
		return errors.New(`must be "key=value"`)
	}
	return nil
}

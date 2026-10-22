package ffmpeg

import (
	"strconv"
	"strings"
	"time"
)

// OptionSet is a reusable, composable list of ffmpeg options, for building
// options per case (per rendition, codec, device, ...) and attaching them to a
// command with [Command.With]:
//
//	base := ffmpeg.NewOptionSet().
//		VideoCodec("libx264").Preset("veryfast").PixelFormat("yuv420p").
//		AudioCodec("aac").MovFlags("+faststart")
//
//	hd := base.Clone().Size(1920, 1080).CRF(20)
//	sd := base.Clone().Size(854, 480).CRF(26).When(mute, (*ffmpeg.OptionSet).NoAudio)
//
//	ff.Command().Input("in.mov").With(hd).Output("hd.mp4").With(sd).Output("sd.mp4")
//
// Methods for single-valued options (VideoCodec, CRF, Size, ...) and [OptionSet.Set]
// replace an earlier value of the same flag, so a clone of a preset can
// override it. Repeatable options ([OptionSet.Map], [OptionSet.Metadata])
// and [OptionSet.Add] append.
//
// The zero value is an empty set ready to use. An OptionSet must not be
// modified concurrently; sharing one read-only (e.g. a package-level preset
// passed to With or Clone) is safe.
type OptionSet struct {
	entries []entry
}

// entry is one option: a flag with an optional value.
type entry struct {
	flag     string
	value    string
	hasValue bool
}

// NewOptionSet returns a set containing args, given as with [Command.Options]
// (flags each followed by their value, if any).
func NewOptionSet(args ...string) *OptionSet {
	return new(OptionSet).Add(args...)
}

// Clone returns an independent copy of s, to derive variants of a preset.
func (s *OptionSet) Clone() *OptionSet {
	return &OptionSet{entries: append([]entry(nil), s.entries...)}
}

// Args returns the options as command-line arguments.
func (s *OptionSet) Args() []string {
	if s == nil {
		return nil
	}
	n := 0
	for _, e := range s.entries {
		n++
		if e.hasValue {
			n++
		}
	}
	args := make([]string, 0, n)
	for _, e := range s.entries {
		args = append(args, e.flag)
		if e.hasValue {
			args = append(args, e.value)
		}
	}
	return args
}

// String returns the options space-separated, for logging. It is not quoted
// for a shell.
func (s *OptionSet) String() string { return strings.Join(s.Args(), " ") }

// Len returns the number of options (flags) in s.
func (s *OptionSet) Len() int { return len(s.entries) }

// Has reports whether s contains flag, e.g. Has(ffmpeg.NoAudio).
func (s *OptionSet) Has(flag string) bool { return s.index(flag) >= 0 }

// Get returns the value of flag and whether s contains it.
func (s *OptionSet) Get(flag string) (string, bool) {
	if i := s.index(flag); i >= 0 {
		return s.entries[i].value, true
	}
	return "", false
}

func (s *OptionSet) index(flag string) int {
	for i := len(s.entries) - 1; i >= 0; i-- {
		if s.entries[i].flag == flag {
			return i
		}
	}
	return -1
}

// Add appends raw arguments, given as with [Command.Options]. A flag
// followed by a non-flag argument takes it as its value. Add never replaces
// existing options.
func (s *OptionSet) Add(args ...string) *OptionSet {
	for i := 0; i < len(args); i++ {
		e := entry{flag: args[i]}
		if isFlag(args[i]) && i+1 < len(args) && !isFlag(args[i+1]) {
			e.value, e.hasValue = args[i+1], true
			i++
		}
		s.entries = append(s.entries, e)
	}
	return s
}

// Set sets flag to value, replacing an earlier value of the same flag. Use it
// for options without a dedicated method, e.g. Set("-x264-params", "keyint=60")
// or Set(ffmpeg.VideoBitrate+":0", "1M").
func (s *OptionSet) Set(flag, value string) *OptionSet {
	if i := s.index(flag); i >= 0 {
		s.entries[i].value, s.entries[i].hasValue = value, true
		return s
	}
	s.entries = append(s.entries, entry{flag: flag, value: value, hasValue: true})
	return s
}

// Flag adds a flag that takes no value, e.g. Flag("-nostats"). It is not
// added twice.
func (s *OptionSet) Flag(flag string) *OptionSet {
	if s.index(flag) < 0 {
		s.entries = append(s.entries, entry{flag: flag})
	}
	return s
}

// Remove removes every occurrence of flag (with its value).
func (s *OptionSet) Remove(flag string) *OptionSet {
	kept := s.entries[:0]
	for _, e := range s.entries {
		if e.flag != flag {
			kept = append(kept, e)
		}
	}
	clear(s.entries[len(kept):])
	s.entries = kept
	return s
}

// Merge applies the options of others to s in order: their single-valued
// options replace those in s, and their repeatable options are appended.
// Nil sets are ignored.
func (s *OptionSet) Merge(others ...*OptionSet) *OptionSet {
	for _, o := range others {
		if o == nil {
			continue
		}
		for _, e := range o.entries {
			switch {
			case repeatable(e.flag) || !isFlag(e.flag):
				s.entries = append(s.entries, e)
			case e.hasValue:
				s.Set(e.flag, e.value)
			default:
				s.Flag(e.flag)
			}
		}
	}
	return s
}

// When calls fn with s if cond is true, to keep conditional options in a
// chain:
//
//	opts.When(mute, (*ffmpeg.OptionSet).NoAudio).
//		When(hls, func(o *ffmpeg.OptionSet) *ffmpeg.OptionSet {
//			return o.ForceFormat("hls").Set("-hls_time", "6")
//		})
func (s *OptionSet) When(cond bool, fn func(*OptionSet) *OptionSet) *OptionSet {
	if cond {
		fn(s)
	}
	return s
}

// repeatable reports whether flag may appear several times with different
// meanings, so merging must not collapse it.
func repeatable(flag string) bool {
	switch optionName(flag) {
	case "map", "metadata", "map_metadata", "i", "disposition", "attach", "bsf":
		return true
	}
	return false
}

func formatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// formatTime formats d as seconds in ffmpeg's time syntax, e.g. "90", "1.5".
func formatTime(d time.Duration) string { return formatFloat(d.Seconds()) }

// Codecs.

// Codec sets the codec for all streams ("-c"), e.g. Codec(ffmpeg.Copy).
func (s *OptionSet) Codec(name string) *OptionSet { return s.Set(Codec, name) }

// VideoCodec sets the video codec ("-c:v"), e.g. "libx264".
func (s *OptionSet) VideoCodec(name string) *OptionSet { return s.Set(VideoCodec, name) }

// AudioCodec sets the audio codec ("-c:a"), e.g. "aac".
func (s *OptionSet) AudioCodec(name string) *OptionSet { return s.Set(AudioCodec, name) }

// SubtitleCodec sets the subtitle codec ("-c:s"), e.g. "mov_text".
func (s *OptionSet) SubtitleCodec(name string) *OptionSet { return s.Set(SubtitleCodec, name) }

// Timing.

// Seek sets the start position ("-ss"). As an input option it seeks the
// input; as an output option it discards output until the position.
func (s *OptionSet) Seek(d time.Duration) *OptionSet { return s.Set(Seek, formatTime(d)) }

// SeekFromEnd seeks relative to the end of the input ("-sseof"); d is
// usually negative. Input option.
func (s *OptionSet) SeekFromEnd(d time.Duration) *OptionSet { return s.Set(SeekFromEnd, formatTime(d)) }

// Duration limits the duration read or written ("-t").
func (s *OptionSet) Duration(d time.Duration) *OptionSet { return s.Set(Duration, formatTime(d)) }

// To stops reading or writing at a position ("-to").
func (s *OptionSet) To(d time.Duration) *OptionSet { return s.Set(To, formatTime(d)) }

// Streams and metadata.

// Map selects input streams for the output ("-map"), e.g. "0:v:0". It can be
// called several times; each call adds a mapping.
func (s *OptionSet) Map(spec string) *OptionSet {
	s.entries = append(s.entries, entry{flag: Map, value: spec, hasValue: true})
	return s
}

// MapMetadata copies global metadata from an input ("-map_metadata"), e.g.
// "0", or "-1" to drop it.
func (s *OptionSet) MapMetadata(spec string) *OptionSet { return s.Set(MapMetadata, spec) }

// Metadata sets a global metadata key ("-metadata key=value"). It can be
// called several times; setting the same key again replaces its value.
func (s *OptionSet) Metadata(key, value string) *OptionSet {
	kv := key + "=" + value
	for i, e := range s.entries {
		if e.flag == Metadata && strings.HasPrefix(e.value, key+"=") {
			s.entries[i].value = kv
			return s
		}
	}
	s.entries = append(s.entries, entry{flag: Metadata, value: kv, hasValue: true})
	return s
}

// NoVideo disables video ("-vn").
func (s *OptionSet) NoVideo() *OptionSet { return s.Flag(NoVideo) }

// NoAudio disables audio ("-an").
func (s *OptionSet) NoAudio() *OptionSet { return s.Flag(NoAudio) }

// NoSubtitle disables subtitles ("-sn").
func (s *OptionSet) NoSubtitle() *OptionSet { return s.Flag(NoSubtitle) }

// NoData disables data streams ("-dn").
func (s *OptionSet) NoData() *OptionSet { return s.Flag(NoData) }

// Filters.

// VideoFilter sets the video filter chain ("-vf"); filters are joined with
// ",", e.g. VideoFilter("scale=1280:-2", "fps=30").
func (s *OptionSet) VideoFilter(filters ...string) *OptionSet {
	return s.Set(VideoFilter, strings.Join(filters, ","))
}

// AudioFilter sets the audio filter chain ("-af"); filters are joined with ",".
func (s *OptionSet) AudioFilter(filters ...string) *OptionSet {
	return s.Set(AudioFilter, strings.Join(filters, ","))
}

// FilterComplex sets a complex filter graph ("-filter_complex"); chains are
// joined with ";".
func (s *OptionSet) FilterComplex(chains ...string) *OptionSet {
	return s.Set(FilterComplex, strings.Join(chains, ";"))
}

// Quality and rate control.

// VideoBitrate sets the video bitrate ("-b:v"), e.g. "2M" or "2500k".
func (s *OptionSet) VideoBitrate(rate string) *OptionSet { return s.Set(VideoBitrate, rate) }

// AudioBitrate sets the audio bitrate ("-b:a"), e.g. "128k".
func (s *OptionSet) AudioBitrate(rate string) *OptionSet { return s.Set(AudioBitrate, rate) }

// MaxRate sets the maximum bitrate ("-maxrate"), e.g. "3M".
func (s *OptionSet) MaxRate(rate string) *OptionSet { return s.Set(MaxRate, rate) }

// BufSize sets the rate-control buffer size ("-bufsize"), e.g. "6M".
func (s *OptionSet) BufSize(size string) *OptionSet { return s.Set(BufSize, size) }

// CRF sets the constant rate factor ("-crf"), e.g. 23.
func (s *OptionSet) CRF(crf float64) *OptionSet { return s.Set(CRF, formatFloat(crf)) }

// Preset sets the encoder preset ("-preset"), e.g. "veryfast".
func (s *OptionSet) Preset(name string) *OptionSet { return s.Set(Preset, name) }

// Tune sets the encoder tuning ("-tune"), e.g. "film".
func (s *OptionSet) Tune(name string) *OptionSet { return s.Set(Tune, name) }

// VideoProfile sets the video encoder profile ("-profile:v"), e.g. "high".
func (s *OptionSet) VideoProfile(name string) *OptionSet { return s.Set(VideoProfile, name) }

// VideoQuality sets the video quality scale ("-q:v").
func (s *OptionSet) VideoQuality(q float64) *OptionSet { return s.Set(VideoQuality, formatFloat(q)) }

// AudioQuality sets the audio quality scale ("-q:a").
func (s *OptionSet) AudioQuality(q float64) *OptionSet { return s.Set(AudioQuality, formatFloat(q)) }

// Video.

// Size sets the frame size ("-s WxH"). To keep the aspect ratio, use a scale
// filter instead, e.g. VideoFilter("scale=1280:-2").
func (s *OptionSet) Size(width, height int) *OptionSet {
	return s.Set(Size, strconv.Itoa(width)+"x"+strconv.Itoa(height))
}

// FrameRate sets the frame rate ("-r"), e.g. 30 or 29.97. For an exact
// fraction use Set(ffmpeg.FrameRate, "30000/1001").
func (s *OptionSet) FrameRate(fps float64) *OptionSet { return s.Set(FrameRate, formatFloat(fps)) }

// PixelFormat sets the pixel format ("-pix_fmt"), e.g. "yuv420p".
func (s *OptionSet) PixelFormat(name string) *OptionSet { return s.Set(PixelFormat, name) }

// AspectRatio sets the display aspect ratio ("-aspect"), e.g. "16:9".
func (s *OptionSet) AspectRatio(ratio string) *OptionSet { return s.Set(AspectRatio, ratio) }

// VideoFrames limits the number of video frames written ("-frames:v").
func (s *OptionSet) VideoFrames(n int) *OptionSet { return s.Set(VideoFrames, strconv.Itoa(n)) }

// Audio.

// SampleRate sets the audio sample rate in Hz ("-ar"), e.g. 48000.
func (s *OptionSet) SampleRate(hz int) *OptionSet { return s.Set(SampleRate, strconv.Itoa(hz)) }

// AudioChannels sets the number of audio channels ("-ac").
func (s *OptionSet) AudioChannels(n int) *OptionSet { return s.Set(AudioChannels, strconv.Itoa(n)) }

// Format and misc.

// ForceFormat forces the container format ("-f"), e.g. "mp4", "hls", "lavfi".
func (s *OptionSet) ForceFormat(name string) *OptionSet { return s.Set(ForceFormat, name) }

// Threads sets the number of threads ("-threads"); 0 lets ffmpeg decide.
func (s *OptionSet) Threads(n int) *OptionSet { return s.Set(Threads, strconv.Itoa(n)) }

// HWAccel enables hardware-accelerated decoding ("-hwaccel"), e.g. "auto"
// or "videotoolbox". Input option.
func (s *OptionSet) HWAccel(name string) *OptionSet { return s.Set(HWAccel, name) }

// ReadRealtime reads the input at its native frame rate ("-re"). Input option.
func (s *OptionSet) ReadRealtime() *OptionSet { return s.Flag(ReadRealtime) }

// StreamLoop loops the input n times; -1 loops forever ("-stream_loop").
// Input option.
func (s *OptionSet) StreamLoop(n int) *OptionSet { return s.Set(StreamLoop, strconv.Itoa(n)) }

// Shortest ends the output when the shortest stream ends ("-shortest").
func (s *OptionSet) Shortest() *OptionSet { return s.Flag(Shortest) }

// MovFlags sets MP4/MOV muxer flags ("-movflags"), e.g. "+faststart".
func (s *OptionSet) MovFlags(flags string) *OptionSet { return s.Set(MovFlags, flags) }

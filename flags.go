package ffmpeg

// Common ffmpeg command-line flags, for use with [Command.Options].
//
// They are plain untyped string constants, so they can be freely mixed with
// string literals; any flag not listed here can simply be passed as a string:
//
//	ff.Command().
//		Input("in.mov").
//		Options(ffmpeg.VideoCodec, "libx264", ffmpeg.CRF, "23", "-x264-params", "keyint=60").
//		Output("out.mp4")
//
// See https://ffmpeg.org/ffmpeg.html for what each flag does. Stream
// specifiers can be appended to the generic flags, e.g. Codec+":v:1".
const (
	// ForceFormat forces the input or output container format ("-f").
	ForceFormat = "-f"

	// Codec selects the codec for all streams ("-c"). Use [Copy] to remux
	// without re-encoding.
	Codec = "-c"
	// VideoCodec selects the video codec ("-c:v").
	VideoCodec = "-c:v"
	// AudioCodec selects the audio codec ("-c:a").
	AudioCodec = "-c:a"
	// SubtitleCodec selects the subtitle codec ("-c:s").
	SubtitleCodec = "-c:s"

	// Seek seeks to a position, as an input option, or discards output until
	// that position, as an output option ("-ss").
	Seek = "-ss"
	// SeekFromEnd seeks relative to the end of the input ("-sseof").
	SeekFromEnd = "-sseof"
	// Duration limits the duration read or written ("-t").
	Duration = "-t"
	// To stops reading or writing at a position ("-to").
	To = "-to"

	// Map selects input streams for an output ("-map").
	Map = "-map"
	// MapMetadata copies metadata from an input ("-map_metadata").
	MapMetadata = "-map_metadata"
	// Metadata sets a metadata key, e.g. Metadata, "title=My Movie" ("-metadata").
	Metadata = "-metadata"

	// NoVideo disables video ("-vn").
	NoVideo = "-vn"
	// NoAudio disables audio ("-an").
	NoAudio = "-an"
	// NoSubtitle disables subtitles ("-sn").
	NoSubtitle = "-sn"
	// NoData disables data streams ("-dn").
	NoData = "-dn"

	// VideoFilter sets a video filter graph ("-vf").
	VideoFilter = "-vf"
	// AudioFilter sets an audio filter graph ("-af").
	AudioFilter = "-af"
	// FilterComplex sets a complex filter graph ("-filter_complex").
	FilterComplex = "-filter_complex"

	// VideoBitrate sets the video bitrate, e.g. "2M" ("-b:v").
	VideoBitrate = "-b:v"
	// AudioBitrate sets the audio bitrate, e.g. "128k" ("-b:a").
	AudioBitrate = "-b:a"
	// MaxRate sets the maximum bitrate ("-maxrate").
	MaxRate = "-maxrate"
	// BufSize sets the rate-control buffer size ("-bufsize").
	BufSize = "-bufsize"
	// CRF sets the constant rate factor of x264/x265/VP9/AV1 encoders ("-crf").
	CRF = "-crf"
	// Preset sets the encoder preset, e.g. "fast" ("-preset").
	Preset = "-preset"
	// Tune sets the encoder tuning, e.g. "film" ("-tune").
	Tune = "-tune"
	// VideoProfile sets the video encoder profile, e.g. "high" ("-profile:v").
	VideoProfile = "-profile:v"
	// VideoQuality sets the video quality scale ("-q:v").
	VideoQuality = "-q:v"
	// AudioQuality sets the audio quality scale ("-q:a").
	AudioQuality = "-q:a"

	// FrameRate sets the frame rate ("-r").
	FrameRate = "-r"
	// Size sets the frame size, e.g. "1280x720" ("-s").
	Size = "-s"
	// AspectRatio sets the display aspect ratio, e.g. "16:9" ("-aspect").
	AspectRatio = "-aspect"
	// PixelFormat sets the pixel format, e.g. "yuv420p" ("-pix_fmt").
	PixelFormat = "-pix_fmt"
	// VideoFrames limits the number of video frames written ("-frames:v").
	VideoFrames = "-frames:v"

	// SampleRate sets the audio sample rate ("-ar").
	SampleRate = "-ar"
	// AudioChannels sets the number of audio channels ("-ac").
	AudioChannels = "-ac"

	// Threads sets the number of threads ("-threads").
	Threads = "-threads"
	// HWAccel enables hardware-accelerated decoding, e.g. "auto" ("-hwaccel").
	HWAccel = "-hwaccel"
	// ReadRealtime reads the input at its native frame rate ("-re").
	ReadRealtime = "-re"
	// StreamLoop loops the input; -1 loops forever ("-stream_loop").
	StreamLoop = "-stream_loop"
	// Shortest ends the output when the shortest stream ends ("-shortest").
	Shortest = "-shortest"
	// MovFlags sets MP4/MOV muxer flags, e.g. "+faststart" ("-movflags").
	MovFlags = "-movflags"
)

// Copy is the codec value that copies streams without re-encoding,
// e.g. Options(ffmpeg.Codec, ffmpeg.Copy).
const Copy = "copy"

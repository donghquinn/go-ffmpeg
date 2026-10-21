package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// ProbeResult is the media information reported by ffprobe.
//
// Numeric values that ffprobe reports as strings (durations, bit rates, ...)
// are kept as strings; helper methods parse the common ones.
type ProbeResult struct {
	Format  Format   `json:"format"`
	Streams []Stream `json:"streams"`

	// Raw is ffprobe's complete JSON output, for fields not modeled here.
	Raw json.RawMessage `json:"-"`
}

// Format describes the container.
type Format struct {
	Filename       string            `json:"filename"`
	NbStreams      int               `json:"nb_streams"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	StartTime      string            `json:"start_time"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	Tags           map[string]string `json:"tags"`
}

// Stream describes a single audio, video, subtitle or data stream.
type Stream struct {
	Index         int               `json:"index"`
	CodecName     string            `json:"codec_name"`
	CodecLongName string            `json:"codec_long_name"`
	CodecType     string            `json:"codec_type"` // "video", "audio", "subtitle", "data", ...
	Profile       string            `json:"profile"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	PixFmt        string            `json:"pix_fmt"`
	RFrameRate    string            `json:"r_frame_rate"`
	AvgFrameRate  string            `json:"avg_frame_rate"`
	SampleRate    string            `json:"sample_rate"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	Duration      string            `json:"duration"`
	BitRate       string            `json:"bit_rate"`
	NbFrames      string            `json:"nb_frames"`
	Disposition   map[string]int    `json:"disposition"`
	Tags          map[string]string `json:"tags"`
}

// Duration returns the container duration, or 0 if ffprobe did not report one.
func (r *ProbeResult) Duration() time.Duration {
	return parseSeconds(r.Format.Duration)
}

// VideoStreams returns the video streams.
func (r *ProbeResult) VideoStreams() []Stream { return r.streams("video") }

// AudioStreams returns the audio streams.
func (r *ProbeResult) AudioStreams() []Stream { return r.streams("audio") }

func (r *ProbeResult) streams(typ string) []Stream {
	var out []Stream
	for _, s := range r.Streams {
		if s.CodecType == typ {
			out = append(out, s)
		}
	}
	return out
}

func parseSeconds(s string) time.Duration {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

// Probe runs ffprobe on input (a file path or URL) and returns its format and
// stream information. On failure it returns an [*Error] (with Binary
// "ffprobe"), classified like ffmpeg errors, e.g. [ErrFileNotFound].
func (f *FFmpeg) Probe(ctx context.Context, input string) (*ProbeResult, error) {
	bin, err := lookPath(f.ffprobePath)
	if err != nil {
		return nil, err
	}
	args := []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", input}

	var stdout bytes.Buffer
	if _, err := f.run(ctx, bin, "ffprobe", args, &stdout); err != nil {
		return nil, err
	}

	res := &ProbeResult{Raw: json.RawMessage(stdout.Bytes())}
	if err := json.Unmarshal(stdout.Bytes(), res); err != nil {
		return nil, fmt.Errorf("ffprobe: decoding output: %w", err)
	}
	return res, nil
}

package ffmpeg

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestOptionSetTypedMethods(t *testing.T) {
	got := NewOptionSet().
		VideoCodec("libx264").CRF(23.5).Preset("veryfast").VideoProfile("high").
		Size(1280, 720).FrameRate(29.97).PixelFormat("yuv420p").
		VideoBitrate("2M").MaxRate("3M").BufSize("6M").
		AudioCodec("aac").AudioBitrate("128k").SampleRate(48000).AudioChannels(2).
		Seek(90*time.Second).Duration(1500*time.Millisecond).SeekFromEnd(-5*time.Second).
		VideoFilter("scale=1280:-2", "fps=30").
		Metadata("title", "My Movie").Map("0:v").Map("0:a").
		NoSubtitle().MovFlags("+faststart").
		Args()
	want := []string{
		"-c:v", "libx264", "-crf", "23.5", "-preset", "veryfast", "-profile:v", "high",
		"-s", "1280x720", "-r", "29.97", "-pix_fmt", "yuv420p",
		"-b:v", "2M", "-maxrate", "3M", "-bufsize", "6M",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-ss", "90", "-t", "1.5", "-sseof", "-5",
		"-vf", "scale=1280:-2,fps=30",
		"-metadata", "title=My Movie", "-map", "0:v", "-map", "0:a",
		"-sn", "-movflags", "+faststart",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Args() =\n%q\nwant\n%q", got, want)
	}
}

func TestOptionSetOverride(t *testing.T) {
	base := NewOptionSet().VideoCodec("libx264").CRF(23).Metadata("title", "a").Map("0:v")
	hd := base.Clone().CRF(20).Metadata("title", "b").Metadata("artist", "c").Map("0:a").NoAudio().NoAudio()

	if got, want := base.Args(), []string{"-c:v", "libx264", "-crf", "23", "-metadata", "title=a", "-map", "0:v"}; !reflect.DeepEqual(got, want) {
		t.Errorf("base modified by clone: %q", got)
	}
	want := []string{"-c:v", "libx264", "-crf", "20", "-metadata", "title=b", "-map", "0:v",
		"-metadata", "artist=c", "-map", "0:a", "-an"}
	if got := hd.Args(); !reflect.DeepEqual(got, want) {
		t.Errorf("hd =\n%q\nwant\n%q", got, want)
	}
	if v, ok := hd.Get(CRF); !ok || v != "20" {
		t.Errorf("Get(CRF) = %q, %v", v, ok)
	}
	if !hd.Has(NoAudio) || hd.Has(NoVideo) {
		t.Error("Has() wrong")
	}
}

func TestOptionSetAddRemoveMerge(t *testing.T) {
	s := NewOptionSet("-c:v", "libx264", "-an", "-stream_loop", "-1", "-x264-params", "keyint=60")
	if got := s.Len(); got != 4 {
		t.Fatalf("Len() = %d, want 4", got)
	}
	s.Remove(NoAudio).Remove("-x264-params")
	if got, want := s.Args(), []string{"-c:v", "libx264", "-stream_loop", "-1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after Remove: %q", got)
	}

	video := NewOptionSet().VideoCodec("libx264").CRF(23).Map("0:v")
	override := NewOptionSet().CRF(18).Map("0:a").NoAudio()
	got := video.Merge(override, nil).Args()
	want := []string{"-c:v", "libx264", "-crf", "18", "-map", "0:v", "-map", "0:a", "-an"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Merge =\n%q\nwant\n%q", got, want)
	}
}

func TestOptionSetWhen(t *testing.T) {
	build := func(mute, hls bool) []string {
		return NewOptionSet().VideoCodec("libx264").
			When(mute, (*OptionSet).NoAudio).
			When(hls, func(o *OptionSet) *OptionSet { return o.ForceFormat("hls").Set("-hls_time", "6") }).
			Args()
	}
	if got, want := build(false, false), []string{"-c:v", "libx264"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	if got, want := build(true, true), []string{"-c:v", "libx264", "-an", "-f", "hls", "-hls_time", "6"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestOptionSetZeroAndNil(t *testing.T) {
	var zero OptionSet
	zero.NoVideo()
	if got := zero.String(); got != "-vn" {
		t.Errorf("zero value: %q", got)
	}
	var nilSet *OptionSet
	if nilSet.Args() != nil {
		t.Error("nil Args() != nil")
	}
	args := cmd().Input("in.mp4").With(nil, &OptionSet{}, nilSet).Output("out.mp4").Args()
	if want := []string{"-n", "-i", "in.mp4", "out.mp4"}; !reflect.DeepEqual(args, want) {
		t.Errorf("With(nil...) = %q", args)
	}
}

func TestCommandWith(t *testing.T) {
	in := NewOptionSet().Seek(5 * time.Second)
	out := NewOptionSet().VideoCodec("libx264")
	c := cmd().With(in).Input("in.mp4").With(out).Options("-crf", "23").Output("out.mp4")
	out.CRF(1) // must not affect c
	want := []string{"-n", "-ss", "5", "-i", "in.mp4", "-c:v", "libx264", "-crf", "23", "out.mp4"}
	if got := c.Args(); !reflect.DeepEqual(got, want) {
		t.Errorf("Args() = %q, want %q", got, want)
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
	// Option sets placed wrongly are caught by validation.
	if err := cmd().Input("in.mp4").With(NewOptionSet().ReadRealtime()).Output("out.mp4").Validate(); err == nil {
		t.Error("Validate() = nil for input-only option before output")
	}
}

// TestOptionSetRendition runs a typical multi-rendition build with ffmpeg.
func TestOptionSetRendition(t *testing.T) {
	ff := newTest(t)
	in := testVideo(t, ff)
	dir := t.TempDir()

	base := NewOptionSet().VideoCodec("mpeg4").PixelFormat("yuv420p").MovFlags("+faststart").Metadata("title", "test")
	renditions := []struct {
		name string
		opts *OptionSet
	}{
		{"64.mp4", base.Clone().Size(64, 64).VideoBitrate("200k")},
		{"32.mp4", base.Clone().Size(32, 32).VideoBitrate("100k").FrameRate(5)},
	}
	c := ff.Command().With(NewOptionSet().Seek(100 * time.Millisecond)).Input(in)
	for _, r := range renditions {
		c.With(r.opts).Output(filepath.Join(dir, r.name))
	}
	ctx := context.Background()
	if err := ff.Validate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := ff.Probe(ctx, filepath.Join(dir, "32.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if v := res.VideoStreams(); len(v) != 1 || v[0].Width != 32 || v[0].AvgFrameRate != "5/1" {
		t.Errorf("32.mp4 = %+v", v)
	}
}

package video

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
)

type RootStreamsDisposition struct {
	Default         int `json:"default"`
	Dub             int `json:"dub"`
	Original        int `json:"original"`
	Comment         int `json:"comment"`
	Lyrics          int `json:"lyrics"`
	Karaoke         int `json:"karaoke"`
	Forced          int `json:"forced"`
	HearingImpaired int `json:"hearing_impaired"`
	VisualImpaired  int `json:"visual_impaired"`
	CleanEffects    int `json:"clean_effects"`
	AttachedPic     int `json:"attached_pic"`
	TimedThumbnails int `json:"timed_thumbnails"`
	NonDiegetic     int `json:"non_diegetic"`
	Captions        int `json:"captions"`
	Descriptions    int `json:"descriptions"`
	Metadata        int `json:"metadata"`
	Dependent       int `json:"dependent"`
	StillImage      int `json:"still_image"`
	Multilayer      int `json:"multilayer"`
}

type RootStreamsTags struct {
	Language    string `json:"language"`
	HandlerName string `json:"handler_name"`
	VendorId    string `json:"vendor_id"`
	Encoder     string `json:"encoder"`
	Timecode    string `json:"timecode"`
}

type RootStreams struct {
	Index              int                    `json:"index"`
	CodecName          string                 `json:"codec_name"`
	CodecLongName      string                 `json:"codec_long_name"`
	Profile            string                 `json:"profile"`
	CodecType          string                 `json:"codec_type"`
	CodecTagString     string                 `json:"codec_tag_string"`
	CodecTag           string                 `json:"codec_tag"`
	Width              int                    `json:"width"`
	Height             int                    `json:"height"`
	CodedWidth         int                    `json:"coded_width"`
	CodedHeight        int                    `json:"coded_height"`
	HasBFrames         int                    `json:"has_b_frames"`
	SampleAspectRatio  string                 `json:"sample_aspect_ratio"`
	DisplayAspectRatio string                 `json:"display_aspect_ratio"`
	PixFmt             string                 `json:"pix_fmt"`
	Level              int                    `json:"level"`
	ColorRange         string                 `json:"color_range"`
	ColorSpace         string                 `json:"color_space"`
	ColorTransfer      string                 `json:"color_transfer"`
	ColorPrimaries     string                 `json:"color_primaries"`
	ChromaLocation     string                 `json:"chroma_location"`
	FieldOrder         string                 `json:"field_order"`
	Refs               int                    `json:"refs"`
	IsAvc              string                 `json:"is_avc"`
	NalLengthSize      string                 `json:"nal_length_size"`
	Id                 string                 `json:"id"`
	RFrameRate         string                 `json:"r_frame_rate"`
	AvgFrameRate       string                 `json:"avg_frame_rate"`
	TimeBase           string                 `json:"time_base"`
	StartPts           int                    `json:"start_pts"`
	StartTime          string                 `json:"start_time"`
	DurationTs         int                    `json:"duration_ts"`
	Duration           string                 `json:"duration"`
	BitRate            string                 `json:"bit_rate"`
	BitsPerRawSample   string                 `json:"bits_per_raw_sample"`
	NbFrames           string                 `json:"nb_frames"`
	ExtradataSize      int                    `json:"extradata_size"`
	Disposition        RootStreamsDisposition `json:"disposition"`
	Tags               RootStreamsTags        `json:"tags"`
}

type Root struct {
	Streams []RootStreams `json:"streams"`
}

func GetVideoAspectRatio(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.New("file path cannot be empty")
	}
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	outBuffer := bytes.NewBuffer([]byte{})
	cmd.Stdout = outBuffer
	if err := cmd.Run(); err != nil {
		return "", err
	}

	videoData := Root{}
	if err := json.Unmarshal(outBuffer.Bytes(), &videoData); err != nil {
		return "", err
	}

	ratio := videoData.Streams[0].DisplayAspectRatio
	return ratio, nil
}

func ProcessVideoForFastStart(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.New("file path cannot be empty")
	}
	outPath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outPath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outPath, nil
}

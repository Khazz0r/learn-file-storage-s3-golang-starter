package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
)

type VideoDetails struct {
	Streams []struct {
		CodecName          string `json:"codec_name"`
		CodecType          string `json:"codec_type"`
		Width              int    `json:"width"`
		Height             int    `json:"height"`
		DisplayAspectRatio string `json:"display_aspect_ratio"`
	} `json:"streams"`
}

func getVideoAspectRatio(filepath string) (string, error) {
	command := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filepath)

	buffer := bytes.Buffer{}
	command.Stdout = &buffer
	err := command.Run()
	if err != nil {
		return "", err
	}

	videoDetails := VideoDetails{}
	json.Unmarshal(buffer.Bytes(), &videoDetails)

	rawAspectRatio := float64(videoDetails.Streams[0].Width) / float64(videoDetails.Streams[0].Height)
	var aspectRatio string
	if rawAspectRatio >= 1.7 && rawAspectRatio <= 1.8 {
		aspectRatio = "16:9"
	} else if rawAspectRatio >= 0.5 && rawAspectRatio <= 0.6 {
		aspectRatio = "9:16"
	} else {
		aspectRatio = "other"
	}

	return aspectRatio, nil
}

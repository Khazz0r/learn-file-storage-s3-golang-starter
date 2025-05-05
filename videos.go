package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
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

func processVideoForFastStart(filepath string) (string, error) {
	processingEndFilepath := filepath + ".processing"
	command := exec.Command("ffmpeg", "-i", filepath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", processingEndFilepath)

	buffer := bytes.Buffer{}
	command.Stdout = &buffer
	err := command.Run()
	if err != nil {
		return "", err
	}

	return processingEndFilepath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	s3PresignClient := s3.NewPresignClient(s3Client)

	params := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}

	s3PresignedHTTPRequest, err := s3PresignClient.PresignGetObject(
		context.Background(),
		params,
		s3.WithPresignExpires(expireTime),
	)
	if err != nil {
		return "", err
	}

	return s3PresignedHTTPRequest.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		return video, fmt.Errorf("video URL is nil")
	}

	if *video.VideoURL == "" {
		return video, fmt.Errorf("video URL is empty")
	}

	splitVideoURL := strings.Split(*video.VideoURL, ",")
	if len(splitVideoURL) != 2 {
		return video, fmt.Errorf("invalid video URL format: %s", *video.VideoURL)
	}
	bucket := splitVideoURL[0]
	key := splitVideoURL[1]

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, time.Minute*5)
	if err != nil {
		return video, err
	}

	updatedVideo := video
	updatedVideo.VideoURL = &presignedURL

	return updatedVideo, nil
}

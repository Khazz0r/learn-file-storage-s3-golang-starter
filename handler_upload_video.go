package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

	if err := r.ParseMultipartForm(uploadLimit); err != nil {
		respondWithError(w, http.StatusBadRequest, "File is too large to upload, keep files below 1 GB", err)
		return
	}

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to modify this video", nil)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to return file from thumbnail", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only mp4 files are allowed for video file types", err)
		return
	}

	tempVideoFile, err := os.CreateTemp("", "tubley-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temp video file", err)
		return
	}
	defer os.Remove(tempVideoFile.Name())
	defer tempVideoFile.Close()

	_, err = io.Copy(tempVideoFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write contents of video file to temp file", err)
		return
	}

	_, err = tempVideoFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not reset file pointer", err)
		return
	}

	// randomize bytes to allow backend video file names to be completely unique
	randBytes := make([]byte, 32)
	_, err = rand.Read(randBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to make random bytes for thumbnail URL", err)
		return
	}
	randKey := base64.RawURLEncoding.EncodeToString(randBytes) + ".mp4"

	// set video aspect ratio type based on the width/height of the video
	aspectRatio, err := getVideoAspectRatio(tempVideoFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not obtain aspect ratio of video file", err)
		return
	}
	if aspectRatio == "16:9" {
		randKey = "landscape/" + randKey
	} else if aspectRatio == "9:16" {
		randKey = "portrait/" + randKey
	} else {
		randKey = "other/" + randKey
	}

	// process the temp video so that it moves the moov atom to the beginning
	processedTempVideoFilepath, err := processVideoForFastStart(tempVideoFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not process video file for fast start", err)
		return
	}
	processedTempVideoFile, err := os.Open(processedTempVideoFilepath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not open processed temp video file", err)
		return
	}
	defer os.Remove(processedTempVideoFile.Name())
	defer processedTempVideoFile.Close()

	params := &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(randKey),
		Body:        processedTempVideoFile,
		ContentType: aws.String(mediaType),
	}
	_, err = cfg.s3Client.PutObject(r.Context(), params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file to S3", err)
		return
	}

	// create Cloudfront URL for content access
	privateBucketURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, randKey)
	videoMetadata.VideoURL = &privateBucketURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "There was an error updating the video's data", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetadata)
}

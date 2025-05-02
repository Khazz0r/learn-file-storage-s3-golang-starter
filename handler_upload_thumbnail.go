package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to return file from thumbnail", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Only jpeg and png are allowed for thumbnail file types", err)
		return
	}
	fileType := strings.Split(mediaType, "/")[1]

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to modify this video", nil)
		return
	}

	// randomize bytes for the thumbnail URL to avoid any caching issues for user end
	randBytes := make([]byte, 32)
	_, err = rand.Read(randBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to make random bytes for thumbnail URL", err)
		return
	}

	randURL := base64.RawURLEncoding.EncodeToString(randBytes)
	imageDataURI := fmt.Sprintf("%v.%s", filepath.Join(cfg.assetsRoot, randURL), fileType)
	thumbnailURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, imageDataURI)
	videoMetadata.ThumbnailURL = &thumbnailURL
	newImageFile, err := os.Create(imageDataURI)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create new image file", err)
		return
	}
	_, err = io.Copy(newImageFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to write contents of thumbnail to new file", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "There was an error updating the video's data", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMetadata)
}

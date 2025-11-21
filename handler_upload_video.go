package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	video2 "github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/video"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// upload limit of 1GB
	const uploadLimit = 1 << 30
	http.MaxBytesReader(w, r.Body, uploadLimit)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// authentication
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	// check if user owns video
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You can't upload this video", err)
		return
	}

	// get video file
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video file", err)
		return
	}
	defer func(file multipart.File) {
		_ = file.Close()
	}(file)

	// get a media type and check if it's a "video/mp4"
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video media type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid video media type", nil)
		return
	}

	// get file extension
	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video file extension", err)
		return
	}

	// temporarily save the video file to disk
	tempFilename := "video-" + videoIDString + extensions[0]
	temp, err := os.CreateTemp("", tempFilename)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temporary file", err)
		return
	}
	defer func(name string) {
		_ = os.Remove(name)
	}(temp.Name())
	defer func(file multipart.File) {
		_ = file.Close()
	}(file)
	if _, err := io.Copy(temp, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write temporary file", err)
		return
	}

	// check for video's aspect ratio
	var prefix string
	ratio, err := video2.GetVideoAspectRatio(temp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}
	switch ratio {
	default:
		prefix = "other"
		break
	case "16:9":
		prefix = "landscape"
		break
	case "9:16":
		prefix = "portrait"
		break
	}

	_, _ = temp.Seek(0, io.SeekStart)

	// put object into s3
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate video name", err)
		return
	}
	filename := prefix + "/" + base64.RawURLEncoding.EncodeToString(randomBytes) + extensions[0]
	if _, err := cfg.s3Client.PutObject(
		r.Context(),
		&s3.PutObjectInput{
			Bucket:      &cfg.s3Bucket,
			Key:         &filename,
			Body:        temp,
			ContentType: &mediaType,
		},
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video to S3", err)
		return
	}

	// update video metadata
	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, filename)
	video.VideoURL = &videoURL
	video.UpdatedAt = time.Now()
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
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

	// process video for fast-start
	processed, err := video2.ProcessVideoForFastStart(temp.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video for fast start", err)
		return
	}
	processedFile, err := os.Open(processed)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video file", err)
		return
	}
	defer func(file multipart.File) {
		_ = file.Close()
	}(processedFile)

	// check for video's aspect ratio
	var prefix string
	ratio, err := video2.GetVideoAspectRatio(processed)
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

	// put object into s3
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate video name", err)
		return
	}
	videoKey := prefix + "/" + base64.RawURLEncoding.EncodeToString(randomBytes) + extensions[0]
	if _, err := cfg.s3Client.PutObject(
		r.Context(),
		&s3.PutObjectInput{
			Bucket:      &cfg.s3Bucket,
			Key:         &videoKey,
			Body:        processedFile,
			ContentType: &mediaType,
		},
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video to S3", err)
		return
	}

	// update video metadata
	videoURL := fmt.Sprintf("%s,%s", cfg.s3Bucket, videoKey)
	video.VideoURL = &videoURL
	video.UpdatedAt = time.Now()
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	// presign video URL
	video, err = cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate presigned video URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	s3Presigned := s3.NewPresignClient(s3Client)
	req, err := s3Presigned.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	keys := strings.Split(*video.VideoURL, ",")
	signedURL, err := generatePresignedURL(cfg.s3Client, keys[0], keys[1], 1*time.Hour)
	if err != nil {
		return video, err
	}
	video.VideoURL = &signedURL
	return video, nil
}

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could't create temporary file", err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error resetting file position", err)
		return
	}

	videoaAspectRatio, err := cfg.getVideoAspectRatio(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video aspect ratio", err)
		return
	}

	videoPrefix := ""

	if videoaAspectRatio == "16:9" {
		videoPrefix = "landscape"
	} else if videoaAspectRatio == "9:16" {
		videoPrefix = "portrait"
	} else {
		videoPrefix = "other"
	}

	processedFilePath, err := cfg.processVideoForFastStart(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not process video for fast start", err)
		return
	}

	processedFile, err := os.Open(processedFilePath)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not open processed video", err)
		return
	}

	defer os.Remove(processedFile.Name())
	defer processedFile.Close()

	videoFileKeySlice := make([]byte, 16)
	rand.Read(videoFileKeySlice)
	videoKeyString := hex.EncodeToString(videoFileKeySlice)
	videoFilename := fmt.Sprintf("%s/%s.mp4", videoPrefix, videoKeyString)

	log.Printf("Attempting to upload video to s3 Bucket:%s", cfg.s3Bucket)
	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(videoFilename),
		Body:        processedFile,
		ContentType: aws.String("video/mp4"),
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file", err)
		return
	}

	url := fmt.Sprintf("%s,%s", cfg.s3Bucket, videoFilename)
	log.Printf("Video url %s", url)
	video.VideoURL = &url

	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	video, err = cfg.dbVideoToSignedVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't not get signed video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func generatePresignedURL(s3Client *s3.Client, bucket string, key string, expireTime time.Duration) (string, error) {
	s3PSC := s3.NewPresignClient(s3Client)

	request, err := s3PSC.PresignGetObject(context.Background(), &s3.GetObjectInput{Bucket: &bucket, Key: &key}, s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	return request.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil {
		// let the client handle an empty url
		return video, nil
	}

	parts := strings.Split(*video.VideoURL, ",")

	if len(parts) < 2 {
		return video, nil
	}

	bucket := parts[0]
	key := parts[1]

	signedURL, error := generatePresignedURL(cfg.s3Client, bucket, key, time.Minute*5)

	if error != nil {
		return video, error
	}

	video.VideoURL = &signedURL
	return video, nil
}

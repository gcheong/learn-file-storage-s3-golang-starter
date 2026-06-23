package main

import (
	"net/http"
	"mime"
	"os"
	"io"
	"log"
	"crypto/rand"
	"fmt"
	"encoding/hex"
	"context"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid" 

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	r.Body = http.MaxBytesReader(w, r.Body, 1 << 30 )

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

	tempFile, err := os.CreateTemp("",  "tubely-upload.mp4")
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

	_ , err = tempFile.Seek(0, io.SeekStart)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error resetting file position", err)
		return
	}

	processedFilePath, err := cfg.processVideoForFastStart(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not process video for fast start", err)
		return
	}

	processedFile, err := os.Open(processedFilePath)

	if err != nil {
		{
		respondWithError(w, http.StatusInternalServerError, "Could open processed video", err)
		return
	}
	}

	defer os.Remove(processedFile.Name())
	defer processedFile.Close()



	videoFileKeySlice := make([]byte, 16)
	rand.Read(videoFileKeySlice)
	videoKeyString := hex.EncodeToString(videoFileKeySlice)
	videoFilename := fmt.Sprintf("%s.mp4",videoKeyString)

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


	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",cfg.s3Bucket,cfg.s3Region,videoFilename)
	log.Printf("Video url %s", url)
	video.VideoURL = &url
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

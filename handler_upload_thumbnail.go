package main

import (
	"fmt"
	"net/http"
	"io"
	"path/filepath"
	"os"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	 // validate the request

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	// "thumbnail" should match the HTML form input name
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}

	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}

	// imageData, err := io.ReadAll(file)
	// if err != nil {
	// 	respondWithError(w, http.StatusBadRequest, "Unable to read thumbnail image data", err)
	// 	return
	// }
	defer file.Close()

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
		respondWithError(w, http.StatusNotFound, "Couldn't get video for thumbnail", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You can't upload a thumbnail for this video", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	mediaTypeParts := strings.Split(mediaType, "/")
	fileExtension := mediaTypeParts[1]
	fileName := fmt.Sprintf("%s.%s",videoIDString,fileExtension)
	path := filepath.Join("assets",fileName )
	thumbnailFile, err := os.Create(path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create thubnail file", err)
		return
	}

	_, err = io.Copy(thumbnailFile, file)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not copy thumbnail file to disk", err)
		return
	}

	fileUrl := fmt.Sprintf("http://localhost:%s/%s",cfg.port, path)
	video.ThumbnailURL = &fileUrl

	err = cfg.db.UpdateVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update video metadata", err)
		return
	}

	// TODO: implement the upload here
	respondWithJSON(w, http.StatusOK, video)
}


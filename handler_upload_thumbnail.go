package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	maxMemory := 10 << 20

	err = r.ParseMultipartForm(int64(maxMemory))

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not parse Multipart Form", err)
		return
	}

	img, header, err := r.FormFile("thumbnail")

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	if mediaType != "image/png" && mediaType != "image/jpeg" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Format unsupported", nil)
		return
	}

	imgData, err := io.ReadAll(img)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	metadata, err := cfg.db.GetVideo(videoID)

	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error(), err)
		return
	}

	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Invalid Credentials", nil)
		return
	}

	videoThumbnails[videoID] = thumbnail{
		data:      imgData,
		mediaType: mediaType,
	}

	extension := strings.ReplaceAll(mediaType, "image/", "")

	byteSlice := make([]byte, 32)

	_, err = rand.Read(byteSlice)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to encode video ID", err)
		return
	}

	filePathEncoded := base64.RawURLEncoding.EncodeToString(byteSlice)

	fileName := fmt.Sprintf("%s.%s", filePathEncoded, extension)

	filePath := filepath.Join(cfg.assetsRoot, fileName)

	file, err := os.Create(filePath)
	defer file.Close()

	reader := bytes.NewReader(imgData)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	_, err = io.Copy(file, reader)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	dataURL := fmt.Sprintf("http://localhost:8091/assets/%s", fileName)

	fmt.Println(dataURL)

	metadata.UpdatedAt = time.Now().UTC()
	metadata.ThumbnailURL = &dataURL

	err = cfg.db.UpdateVideo(metadata)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}

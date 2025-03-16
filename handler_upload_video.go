package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Bucket: %s\nRegion: %s\n", cfg.s3Bucket, cfg.s3Region)

	videoLimit := 1 << 30

	r.Body = http.MaxBytesReader(w, r.Body, int64(videoLimit))

	videoID := r.PathValue("videoID")

	if videoID == "" {
		fmt.Println("Video id not found")
		respondWithError(w, http.StatusNotFound, "Video id not found", nil)
		return
	}

	jwtToken, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}

	userId, err := auth.ValidateJWT(jwtToken, cfg.jwtSecret)

	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error(), err)
		return
	}

	videoUUID, err := uuid.Parse(videoID)

	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error(), err)
		return
	}

	video, err := cfg.db.GetVideo(videoUUID)

	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}

	if video.UserID != userId {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	multipartVideoFile, fileHeader, err := r.FormFile("video")

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	defer multipartVideoFile.Close()
	contentType := fileHeader.Header.Get("Content-Type")
	fileType, _, err := mime.ParseMediaType(contentType)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	if fileType != "video/mp4" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Unsupported file format. Only video/mp4 allowed.", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, multipartVideoFile)

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	prefix := "other"

	if aspectRatio == "16:9" {
		prefix = "landscape"
	}

	if aspectRatio == "9:16" {
		prefix = "portrait"
	}

	processedPath, err := processVideoForFastStart(tempFile.Name())

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	processedFile, err := os.Open(processedPath)

	defer processedFile.Close()
	defer os.Remove(processedPath)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	bucketFileNameSlice := make([]byte, 32)

	_, err = rand.Read(bucketFileNameSlice)

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	bucketFileName := fmt.Sprintf("%s/%x.mp4", prefix, bucketFileNameSlice)

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &bucketFileName,
		Body:        processedFile,
		ContentType: &fileType,
	})

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	newVideoURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, bucketFileName)

	video.UpdatedAt = time.Now().UTC()
	video.VideoURL = &newVideoURL
	err = cfg.db.UpdateVideo(video)

	if err != nil {
		fmt.Println(err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	/* video, err = cfg.dbVideoToSignedVideo(video)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	} */

	fmt.Printf("Key: %s\nLink: %s\n", bucketFileName, newVideoURL)

	w.WriteHeader(http.StatusOK)
}

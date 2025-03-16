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

func getVideoAspectRatio(filePath string) (string, error) {

	res := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var buffer bytes.Buffer
	res.Stdout = &buffer

	err := res.Run()

	if err != nil {
		return "", err
	}

	type expectedBody struct {
		Streams []struct {
			DisplayAspectRatio string `json:"display_aspect_ratio"`
		} `json:"streams"`
	}

	var actualBody expectedBody

	err = json.Unmarshal(buffer.Bytes(), &actualBody)

	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	if len(actualBody.Streams) == 0 {
		return "", fmt.Errorf("Empty json")
	}

	aspectRatio := actualBody.Streams[0].DisplayAspectRatio
	fmt.Printf("Video Aspect Ratio: %s", aspectRatio)
	if aspectRatio != "9:16" && aspectRatio != "16:9" {
		aspectRatio = "other"
	}

	return aspectRatio, nil

}

func processVideoForFastStart(filePath string) (string, error) {
	newPath := fmt.Sprintf("%s.processing", filePath)

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", newPath)
	err := cmd.Run()

	if err != nil {
		return "", err
	}

	return newPath, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	client := s3.NewPresignClient(s3Client)

	req, err := client.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))

	if err != nil {
		return "", err
	}

	return req.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	expireTime := time.Hour * 4

	if video.VideoURL == nil {
		return video, nil
	}

	if !strings.Contains(*video.VideoURL, ",") {
		return video, fmt.Errorf("Malformed video url")
	}

	bucketAndKey := strings.Split(*video.VideoURL, ",")

	if len(bucketAndKey) != 2 {
		return video, fmt.Errorf("Invalid video url")
	}

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucketAndKey[0], bucketAndKey[1], expireTime)

	if err != nil {
		return video, err
	}

	video.VideoURL = &presignedURL
	return video, nil

}

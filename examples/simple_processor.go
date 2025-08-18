package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"distributed-job-processor/internal/config"
	"distributed-job-processor/internal/server"
	"distributed-job-processor/pkg/job"
)

type EmailProcessor struct{}

func (e *EmailProcessor) Process(ctx context.Context, j *job.Job) error {
	recipient, ok := j.Payload["recipient"].(string)
	if !ok {
		return fmt.Errorf("recipient not found in job payload")
	}

	subject, ok := j.Payload["subject"].(string)
	if !ok {
		return fmt.Errorf("subject not found in job payload")
	}

	body, ok := j.Payload["body"].(string)
	if !ok {
		return fmt.Errorf("body not found in job payload")
	}

	time.Sleep(2 * time.Second)

	fmt.Printf("Sending email to %s: Subject=%s, Body=%s\n", recipient, subject, body)
	return nil
}

func (e *EmailProcessor) Type() string {
	return "email"
}

type ImageProcessor struct{}

func (i *ImageProcessor) Process(ctx context.Context, j *job.Job) error {
	imageURL, ok := j.Payload["image_url"].(string)
	if !ok {
		return fmt.Errorf("image_url not found in job payload")
	}

	operation, ok := j.Payload["operation"].(string)
	if !ok {
		operation = "resize"
	}

	time.Sleep(5 * time.Second)

	fmt.Printf("Processing image %s with operation: %s\n", imageURL, operation)
	return nil
}

func (i *ImageProcessor) Type() string {
	return "image_processing"
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatal("Failed to create server:", err)
	}

	srv.RegisterProcessor(&EmailProcessor{})
	srv.RegisterProcessor(&ImageProcessor{})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
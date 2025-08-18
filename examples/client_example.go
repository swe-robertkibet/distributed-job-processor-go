package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"distributed-job-processor/pkg/job"
)

const BaseURL = "http://localhost:8080/api/v1"

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

func (c *Client) CreateJob(j *job.Job) (*job.Job, error) {
	data, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(c.baseURL+"/jobs", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create job: %s", string(body))
	}

	var createdJob job.Job
	if err := json.NewDecoder(resp.Body).Decode(&createdJob); err != nil {
		return nil, err
	}

	return &createdJob, nil
}

func (c *Client) GetJob(id string) (*job.Job, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/jobs/" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("job not found")
	}

	var j job.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}

	return &j, nil
}

func (c *Client) GetStats() (map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/stats")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}

	return stats, nil
}

func main() {
	client := NewClient(BaseURL)

	fmt.Println("Creating jobs...")

	emailJob := &job.Job{
		Type:     "email",
		Priority: job.PriorityNormal,
		Payload: map[string]any{
			"recipient": "client@example.com",
			"subject":   "Test from Go Client",
			"body":      "This email was sent using the Go client!",
		},
		MaxRetries: 3,
	}

	createdJob, err := client.CreateJob(emailJob)
	if err != nil {
		fmt.Printf("Failed to create email job: %v\n", err)
		return
	}

	fmt.Printf("Created email job: %s\n", createdJob.ID.Hex())

	imageJob := &job.Job{
		Type:     "image_processing",
		Priority: job.PriorityHigh,
		Payload: map[string]any{
			"image_url": "https://example.com/test-image.png",
			"operation": "thumbnail",
			"size":      "200x200",
		},
		MaxRetries: 2,
	}

	createdImageJob, err := client.CreateJob(imageJob)
	if err != nil {
		fmt.Printf("Failed to create image job: %v\n", err)
		return
	}

	fmt.Printf("Created image processing job: %s\n", createdImageJob.ID.Hex())

	time.Sleep(2 * time.Second)

	fmt.Println("\nChecking job status...")
	updatedJob, err := client.GetJob(createdJob.ID.Hex())
	if err != nil {
		fmt.Printf("Failed to get job: %v\n", err)
	} else {
		fmt.Printf("Email job status: %s\n", updatedJob.Status)
	}

	fmt.Println("\nGetting system stats...")
	stats, err := client.GetStats()
	if err != nil {
		fmt.Printf("Failed to get stats: %v\n", err)
	} else {
		fmt.Printf("System stats: %+v\n", stats)
	}
}
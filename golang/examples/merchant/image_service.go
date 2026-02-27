package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"google.golang.org/genai"

	"github.com/google-agentic-commerce/a2a-x402/core/business"
)

type ImageService struct {
	client *genai.Client
}

func NewImageService() *ImageService {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, nil)
	if err != nil {
		return &ImageService{client: nil}
	}

	return &ImageService{
		client: client,
	}
}

func (s *ImageService) Execute(ctx context.Context, prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	if s.client == nil {
		return "", fmt.Errorf("genai client is not initialized. Please set GEMINI_API_KEY environment variable")
	}

	result, err := s.client.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash-image",
		genai.Text(prompt),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate image: %w", err)
	}

	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in result")
	}

	var imageBase64 string
	var resultText string

	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			resultText = part.Text
		} else if part.InlineData != nil {
			imageBytes := part.InlineData.Data
			outputFilename := "./gemini_generated_image.png"
			if err := os.WriteFile(outputFilename, imageBytes, 0644); err != nil {
				log.Printf("failed to write image to file: %v", err)
			}

			imageBase64 = base64.StdEncoding.EncodeToString(imageBytes)
		}
	}

	if imageBase64 == "" && resultText == "" {
		return "", fmt.Errorf("no image or text data found in result")
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": "Image generated successfully",
		"prompt":  prompt,
	}

	// if imageBase64 != "" {
	// 	imageDataURL := fmt.Sprintf("data:image/png;base64,%s", imageBase64)
	// 	response["url"] = imageDataURL
	// } else {
	// 	response["content"] = resultText
	// }

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(jsonResponse), nil
}

func (s *ImageService) ServiceRequirements(prompt string) business.ServiceRequirements {
	basePrice := 1.0
	if len(prompt) > 100 {
		basePrice = 1.5
	}
	if len(prompt) > 500 {
		basePrice = 2.0
	}

	priceStr := fmt.Sprintf("%.1f", basePrice)

	description := "Generate an AI image"
	if len(prompt) > 50 {
		description = fmt.Sprintf("Generate an AI image: %s...", prompt[:50])
	}

	return business.ServiceRequirements{
		Price:             priceStr,
		Resource:          "/generate-image",
		Description:       description,
		MimeType:          "application/json",
		Scheme:            "exact",
		MaxTimeoutSeconds: 600,
	}
}

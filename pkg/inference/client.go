// Package inference provides an OpenAI-compatible client for VLM inference.
package inference

import (
	"context"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"golang.org/x/sync/errgroup"

	"github.com/ninehills/pdf2md/pkg/base64util"
	"github.com/ninehills/pdf2md/pkg/models"
)

// Client wraps an OpenAI-compatible client for vLLM inference.
type Client struct {
	client  openai.Client
	model   models.Config
	timeout time.Duration
}

// NewClient creates a new inference client for the given model config.
func NewClient(baseURL string, modelCfg models.Config, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:8000/v1"
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("not-needed"),
	)

	return &Client{
		client:  client,
		model:   modelCfg,
		timeout: timeout,
	}
}

// ParseImage sends an image to the inference server and returns the model's response.
func (c *Client) ParseImage(ctx context.Context, imagePath string) (string, error) {
	return c.ParseImageWithPrompt(ctx, imagePath, c.model.DefaultPrompt)
}

// ParseImageWithPrompt sends an image with an explicit prompt.
func (c *Client) ParseImageWithPrompt(ctx context.Context, imagePath string, prompt string) (string, error) {
	dataURI, err := base64util.ImageToBase64DataURI(imagePath)
	if err != nil {
		return "", fmt.Errorf("encoding image: %w", err)
	}

	if c.model.ImagePromptTag != "" {
		prompt = c.model.ImagePromptTag + prompt
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
			openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL: dataURI,
			}),
			openai.TextContentPart(prompt),
		}),
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.client.Chat.Completions.New(reqCtx, openai.ChatCompletionNewParams{
		Messages:            messages,
		Model:               c.model.ServedModelName,
		Temperature:         openai.Float(c.model.Temperature),
		TopP:                openai.Float(c.model.TopP),
		MaxCompletionTokens: openai.Int(c.model.MaxCompletionTokens),
	})
	if err != nil {
		return "", fmt.Errorf("vLLM inference request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}

	return resp.Choices[0].Message.Content, nil
}

// ParseImages parses multiple images concurrently with the given concurrency limit.
func (c *Client) ParseImages(ctx context.Context, imagePaths []string, concurrency int) ([]string, error) {
	if concurrency <= 0 {
		concurrency = 16
	}

	results := make([]string, len(imagePaths))

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, path := range imagePaths {
		g.Go(func() error {
			resp, err := c.ParseImage(gCtx, path)
			if err != nil {
				return fmt.Errorf("page %d: %w", i+1, err)
			}
			results[i] = resp
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return results, err
	}
	return results, nil
}

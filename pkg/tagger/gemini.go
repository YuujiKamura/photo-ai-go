package tagger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/YuujiKamura/resident-agent/pkg/api"
)

func init() {
	viper.SetDefault("model", "auto")
	viper.SetConfigName(".photo-ai")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	_ = viper.ReadInConfig()
	viper.SetEnvPrefix("PHOTO_AI")
	viper.AutomaticEnv()
}

func GetDefaultModel() string {
	return viper.GetString("model")
}

type ClassifyConfig struct {
	Model      string
	Vocabulary []string
	PayPerUse  bool
}

func (c *ClassifyConfig) model() string {
	m := c.Model
	if m == "" {
		m = GetDefaultModel()
	}
	if m == "auto" {
		return ""
	}
	return m
}

type BatchResult struct {
	Items     []GroupItem
	LatencyMs int64
	Mode      string
}

func ClassifyGroupBatch(ctx context.Context, images []string, cfg *ClassifyConfig) (BatchResult, error) {
	start := time.Now()
	opts := api.DefaultOptions().WithModel(cfg.model()).WithJSON()
	prompt := GroupPrompt(images, cfg.Vocabulary)
	rawResponse, err := api.Analyze(prompt, images, opts)
	if err != nil {
		return BatchResult{}, fmt.Errorf("api.Analyze failed: %w", err)
	}
	items, err := parseItems(rawResponse)
	if err != nil {
		return BatchResult{}, err
	}
	return BatchResult{
		Items:     items,
		LatencyMs: time.Since(start).Milliseconds(),
		Mode:      "DIRECT_LIB_CALL",
	}, nil
}

func parseItems(rawText string) ([]GroupItem, error) {
	jsonStr, err := extractJSONArray(rawText)
	if err != nil {
		return nil, fmt.Errorf("no JSON array found")
	}
	var items []GroupItem
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, err
	}
	return items, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func extractJSONArray(s string) (string, error) {
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		rest := s[start:]
		if end := strings.Index(rest, "```"); end >= 0 {
			return strings.TrimSpace(rest[:end]), nil
		}
	}
	lastBracket := strings.LastIndex(s, "]")
	if lastBracket < 0 {
		return "", fmt.Errorf("no JSON array found")
	}
	for i := lastBracket; i >= 0; i-- {
		if s[i] == '[' {
			candidate := s[i : lastBracket+1]
			var v json.RawMessage
			if json.Unmarshal([]byte(candidate), &v) == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("no valid JSON array found")
}

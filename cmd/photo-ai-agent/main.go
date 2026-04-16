package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/YuujiKamura/resident-agent/pkg/api"
)

// This agent acts as a simple bridge to the resident-agent's analysis logic
// via a Named Pipe. It expects a single JSON payload containing the analysis request.

type analysisRequest struct {
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
	Model     string   `json:"model"`
	Format    string   `json:"format"`
	UsageMode string   `json:"usage_mode"`
}

type analysisResponse struct {
	OK    bool   `json:"ok"`
	Data  string `json:"data"`
	Error string `json:"error"`
}

func main() {
	pipePath := os.Getenv("PHOTO_AI_AGENT_PIPE")
	if pipePath == "" {
		pipePath = `\\.\pipe\photo-ai-agent`
	}

	log.Printf("Starting photo-ai-agent (REST bridge) on %s", pipePath)

	listener, err := winio.ListenPipe(pipePath, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}
		go handleConn(ctx, conn)
	}
}

func handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(120 * time.Second))

	var req analysisRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		sendError(conn, fmt.Errorf("decode request: %w", err))
		return
	}

	opts := api.DefaultOptions().WithModel(req.Model).WithJSON()
	if req.UsageMode == "pay-per-use" {
		opts = opts.WithUsageMode(api.PayPerUse)
	} else {
		opts = opts.WithUsageMode(api.TimeBasedQuota)
	}

	rawText, err := api.Analyze(req.Prompt, req.Images, opts)
	if err != nil {
		sendError(conn, fmt.Errorf("API error: %w", err))
		return
	}

	resp := analysisResponse{
		OK:   true,
		Data: rawText,
	}
	json.NewEncoder(conn).Encode(resp)
}

func sendError(conn net.Conn, err error) {
	resp := analysisResponse{
		OK:    false,
		Error: err.Error(),
	}
	json.NewEncoder(conn).Encode(resp)
}

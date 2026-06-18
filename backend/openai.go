package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
//  OpenAI-compatible Chat Completions endpoint
//  GET  /v1/chat/completions?stream=true&message=...
//  POST /v1/chat/completions   { model, messages, stream }
// ---------------------------------------------------------------------------

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Stream   bool            `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

type chatChoice struct {
	Index        int          `json:"index"`
	Message      *chatMsg     `json:"message,omitempty"`
	Delta        *chatMsg     `json:"delta,omitempty"`
	FinishReason *string      `json:"finish_reason"`
	LogProbs     interface{}  `json:"logprobs"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatChunk struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []chatChoice `json:"choices"`
}

// handleChatCompletions handles both GET and POST requests.
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var (
		stream   bool
		messages []chatMessage
	)

	switch r.Method {
	case http.MethodGet:
		stream = r.URL.Query().Get("stream") == "true"
		msg := r.URL.Query().Get("message")
		if msg == "" {
			msg = r.URL.Query().Get("messages")
		}
		if msg == "" {
			msg = "Show all services"
		}
		messages = []chatMessage{{Role: "user", Content: msg}}

	case http.MethodPost:
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		stream = req.Stream
		messages = req.Messages

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Build content
	services := s.monitor.Snapshot()
	content := buildStatusContent(messages, services)

	if stream {
		writeSSEChatStream(w, content, services)
	} else {
		writeChatResponse(w, content)
	}
}

func buildStatusContent(messages []chatMessage, services []ServiceState) string {
	var lastMsg string
	if len(messages) > 0 {
		lastMsg = messages[len(messages)-1].Content
	}

	total := len(services)
	online := 0
	for _, svc := range services {
		if svc.Online {
			online++
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Service Monitor Status — %d/%d online\n\n", online, total))
	b.WriteString("```\n")
	for _, svc := range services {
		status := "HEALTHY"
		if !svc.Online {
			status = "UNHEALTHY"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", padRight(status, 12), svc.Name))
		b.WriteString(fmt.Sprintf("      Response: %dms  |  Conns: %d  |  Error rate: %.0f%%\n",
			svc.ResponseTimeMs, svc.ActiveConns, svc.ErrorRate*100))
		if svc.SSECapable {
			b.WriteString("      [SSE streaming supported]\n")
		}
	}
	b.WriteString("```\n\n")
	b.WriteString(fmt.Sprintf("**Query:** %s\n\n", lastMsg))
	b.WriteString(fmt.Sprintf("_Last updated: %s_", time.Now().Format(time.RFC3339)))

	return b.String()
}

func writeChatResponse(w http.ResponseWriter, content string) {
	now := time.Now()
	resp := chatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d-%s", now.UnixMilli(), randString(6)),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   "service-monitor-v1",
		Choices: []chatChoice{{
			Index: 0,
			Message: &chatMsg{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: strPtr("stop"),
			LogProbs:     nil,
		}},
		Usage: chatUsage{TotalTokens: 0},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func writeSSEChatStream(w http.ResponseWriter, content string, services []ServiceState) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	now := time.Now()

	// Send one chunk per service
	for i, svc := range services {
		status := "OK"
		if !svc.Online {
			status = "DOWN"
		}
		line := fmt.Sprintf("[%s] %s — %dms, %d conns\n", status, svc.Name, svc.ResponseTimeMs, svc.ActiveConns)

		chunk := chatChunk{
			ID:      fmt.Sprintf("chatcmpl-%d", now.UnixMilli()),
			Object:  "chat.completion.chunk",
			Created: now.Unix(),
			Model:   "service-monitor-v1",
			Choices: []chatChoice{{
				Index: i,
				Delta: &chatMsg{Content: line},
			}},
		}

		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Final chunk with finish_reason
	finalChunk := chatChunk{
		ID:      fmt.Sprintf("chatcmpl-%d", now.UnixMilli()),
		Object:  "chat.completion.chunk",
		Created: now.Unix(),
		Model:   "service-monitor-v1",
		Choices: []chatChoice{{
			Index:        len(services),
			Delta:        &chatMsg{},
			FinishReason: strPtr("stop"),
		}},
	}
	data, _ := json.Marshal(finalChunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string { return &s }

func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

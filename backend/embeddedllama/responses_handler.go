//go:build cgo

package embeddedllama

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	llama "github.com/tcpipuk/llama-go"
)

// handleResponses serves one OpenAI Responses API request (/v1/responses),
// streaming it when the request asks for it. It is the tool-calling endpoint:
// see responses.go for the text protocol it layers over llama-go's ordinary
// role/content chat completions.
func (g *embeddedRunningModel) handleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req responsesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	turns, err := req.chatTurns()
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid input: %v", err), http.StatusBadRequest)
		return
	}
	if len(turns) == 0 {
		http.Error(w, "no input provided", http.StatusBadRequest)
		return
	}

	messages := make([]llama.ChatMessage, len(turns))
	for i, turn := range turns {
		messages[i] = llama.ChatMessage{Role: turn.Role, Content: turn.Content}
	}
	opts := llama.ChatOptions{
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	if req.Stream {
		g.streamResponses(w, r, req, messages, opts)
		return
	}

	started := time.Now()
	reply, err := g.chat(r.Context(), messages, opts)
	if err != nil {
		g.httpError(w, r, http.StatusInternalServerError, "responses failed: %v", err)
		return
	}

	content, calls := parseToolCalls(reply.Content, knownToolNames(req.Tools))
	g.writeJSON(w, buildResponsesResponse(newResponseID("resp"), started, g.modelName, "completed", content, calls, req))
	g.logger.Printf("responses completed in %s (%d tool call(s))", time.Since(started).Round(time.Millisecond), len(calls))
}

// streamResponses streams one Responses API request as server-sent events, in
// OpenAI's event shape (the subset the backend emits).
func (g *embeddedRunningModel) streamResponses(w http.ResponseWriter, r *http.Request, req responsesRequest, messages []llama.ChatMessage, opts llama.ChatOptions) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		g.httpError(w, r, http.StatusInternalServerError, "streaming is not supported by this connection")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := newResponseID("resp")
	created := time.Now()
	events := &responsesStreamWriter{w: w, flusher: flusher}

	// response.created: a snapshot with no output yet.
	inProgress := buildResponsesResponse(id, created, g.modelName, "in_progress", "", nil, req)
	inProgress.Output = []any{}
	events.event("response.created", map[string]any{"response": inProgress})

	msgID := newResponseID("msg")
	messageOpen := false
	openMessage := func() {
		if messageOpen {
			return
		}
		messageOpen = true
		events.event("response.output_item.added", map[string]any{
			"output_index": 0,
			"item": responsesMessageItem{
				Type: "message", ID: msgID, Status: "in_progress", Role: "assistant",
				Content: []responsesOutputContent{},
			},
		})
		events.event("response.content_part.added", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0,
			"part": responsesOutputContent{Type: "output_text", Text: "", Annotations: []any{}},
		})
	}

	var (
		content   strings.Builder
		toolCalls []parsedToolCall
		streamErr error
		scanner   = &toolCallScanner{known: knownToolNames(req.Tools)}
	)
	emitText := func(text string) {
		if text == "" {
			return
		}
		openMessage()
		content.WriteString(text)
		events.event("response.output_text.delta", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0, "delta": text,
		})
	}

	g.runMu.Lock()
	deltaCh, errCh := g.ctx.ChatStream(r.Context(), messages, opts)
	for deltaCh != nil || errCh != nil {
		select {
		case delta, open := <-deltaCh:
			if !open {
				deltaCh = nil
				continue
			}
			text, calls := scanner.Feed(delta.Content)
			emitText(text)
			toolCalls = append(toolCalls, calls...)
		case err, open := <-errCh:
			if !open {
				errCh = nil
				continue
			}
			if err != nil {
				streamErr = err
			}
		}
	}
	g.runMu.Unlock()

	tail, calls := scanner.Flush()
	emitText(tail)
	toolCalls = append(toolCalls, calls...)

	if streamErr != nil {
		g.logger.Printf("error: %s %s: responses stream failed: %v", r.Method, r.URL.Path, streamErr)
		failed := buildResponsesResponse(id, created, g.modelName, "failed", content.String(), toolCalls, req)
		events.event("response.failed", map[string]any{"response": failed})
		return
	}

	// Close the assistant text item, or emit an empty one when the model went
	// straight to a tool call (or produced nothing at all).
	if messageOpen {
		text := content.String()
		events.event("response.output_text.done", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0, "text": text,
		})
		events.event("response.content_part.done", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0,
			"part": responsesOutputContent{Type: "output_text", Text: text, Annotations: []any{}},
		})
		events.event("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": responsesMessageItem{
				Type: "message", ID: msgID, Status: "completed", Role: "assistant",
				Content: []responsesOutputContent{{Type: "output_text", Text: text, Annotations: []any{}}},
			},
		})
	} else if len(toolCalls) == 0 {
		openMessage()
		events.event("response.output_text.done", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0, "text": "",
		})
		events.event("response.content_part.done", map[string]any{
			"item_id": msgID, "output_index": 0, "content_index": 0,
			"part": responsesOutputContent{Type: "output_text", Text: "", Annotations: []any{}},
		})
		events.event("response.output_item.done", map[string]any{
			"output_index": 0,
			"item": responsesMessageItem{
				Type: "message", ID: msgID, Status: "completed", Role: "assistant",
				Content: []responsesOutputContent{{Type: "output_text", Text: "", Annotations: []any{}}},
			},
		})
	}

	// Tool calls are only known once the stream has been parsed, so they are
	// emitted after the text item.
	nextIndex := 0
	if messageOpen {
		nextIndex = 1
	}
	for _, call := range toolCalls {
		callID := call.CallID
		if callID == "" {
			callID = newResponseID("call")
		}
		itemID := newResponseID("fc")
		events.event("response.output_item.added", map[string]any{
			"output_index": nextIndex,
			"item": responsesFunctionCallItem{
				Type: "function_call", ID: itemID, Status: "in_progress",
				CallID: callID, Name: call.Name,
			},
		})
		events.event("response.function_call_arguments.delta", map[string]any{
			"item_id": itemID, "output_index": nextIndex, "delta": call.Arguments,
		})
		events.event("response.function_call_arguments.done", map[string]any{
			"item_id": itemID, "output_index": nextIndex, "arguments": call.Arguments,
		})
		events.event("response.output_item.done", map[string]any{
			"output_index": nextIndex,
			"item": responsesFunctionCallItem{
				Type: "function_call", ID: itemID, Status: "completed",
				CallID: callID, Name: call.Name, Arguments: call.Arguments,
			},
		})
		nextIndex++
	}

	completed := buildResponsesResponse(id, created, g.modelName, "completed", content.String(), toolCalls, req)
	events.event("response.completed", map[string]any{"response": completed})
	g.logger.Printf("responses streamed in %s (%d tool call(s))", time.Since(created).Round(time.Millisecond), len(toolCalls))
}

// responsesStreamWriter writes typed Responses API server-sent events, adding
// the event's "type" and a per-stream "sequence_number".
type responsesStreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	seq     int
}

// event writes one named SSE event.
func (s *responsesStreamWriter) event(name string, payload map[string]any) {
	s.seq++
	payload["type"] = name
	payload["sequence_number"] = s.seq
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
	s.flusher.Flush()
}

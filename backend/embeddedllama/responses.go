package embeddedllama

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// The OpenAI Responses API (/v1/responses) is tool-centric: unlike chat
// completions it is built around function calls. The embedded runtime is an
// ordinary llama.cpp chat completion running under llama-go, which has no
// native tool support (see docs/BACKENDS.md), so this backend carries a small,
// self-contained tool protocol over the usual role/content messages:
//
//   - Tool definitions are advertised in a system message inside <tools>.
//   - The model asks to call a tool by emitting a JSON object wrapped in
//     <tool_call></tool_call>.
//   - Tool results are fed back wrapped in
//     <tool_response call_id="...">...</tool_response>.
//
// This follows the convention popularised by Qwen/Hermes models, so a model
// that has seen tool-calling data usually complies. It is an instruction-level
// protocol rather than grammar-constrained decoding, so a model may ignore it,
// in which case its plain text is returned as-is.
const (
	toolCallOpen  = "<tool_call>"
	toolCallClose = "</tool_call>"
)

// responsesRequest is the body of a POST /v1/responses request, in OpenAI's
// shape (the subset the backend honours).
type responsesRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Instructions    string          `json:"instructions"`
	Tools           []responsesTool `json:"tools"`
	ToolChoice      json.RawMessage `json:"tool_choice"`
	MaxOutputTokens *int            `json:"max_output_tokens"`
	Temperature     *float32        `json:"temperature"`
	TopP            *float32        `json:"top_p"`
	Stream          bool            `json:"stream"`
}

// responsesTool is one entry of the request's tools array. The Responses API
// puts the function fields at the top level, but many clients reuse the
// chat-completions shape with a nested "function" object, so both are accepted.
type responsesTool struct {
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  json.RawMessage        `json:"parameters"`
	Function    *responsesToolFunction `json:"function,omitempty"`
}

// responsesToolFunction is the nested chat-completions form of a tool.
type responsesToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// functionName returns the tool's function name from whichever shape was used.
func (t responsesTool) functionName() string {
	if t.Function != nil && t.Function.Name != "" {
		return t.Function.Name
	}
	return t.Name
}

// functionDescription returns the tool's description from whichever shape was
// used.
func (t responsesTool) functionDescription() string {
	if t.Function != nil && t.Function.Description != "" {
		return t.Function.Description
	}
	return t.Description
}

// functionParameters returns the tool's JSON schema from whichever shape was
// used, defaulting to an empty object schema.
func (t responsesTool) functionParameters() json.RawMessage {
	params := t.Parameters
	if t.Function != nil && len(t.Function.Parameters) > 0 {
		params = t.Function.Parameters
	}
	if len(params) == 0 || !json.Valid(params) {
		return json.RawMessage("{}")
	}
	return params
}

// responsesToolEcho is the shape tools are echoed back in the response.
type responsesToolEcho struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// responsesInputItem is one entry of the request's input array. Depending on
// Type it is a message, a prior function call, or a function result.
type responsesInputItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
}

// chatTurn is one role/content message sent to the model. It is deliberately
// independent of llama-go, so the Responses protocol can be exercised in tests
// without cgo or a loaded model.
type chatTurn struct {
	Role    string
	Content string
}

// chatTurns converts the request into the role/content messages the model is
// prompted with, prefixed by a system message carrying any instructions and the
// tool protocol when tools are in play.
func (req responsesRequest) chatTurns() ([]chatTurn, error) {
	turns, err := req.inputTurns()
	if err != nil {
		return nil, err
	}
	if system := req.systemPrompt(); system != "" {
		turns = append([]chatTurn{{Role: "system", Content: system}}, turns...)
	}
	return turns, nil
}

// inputTurns converts the request's input (a bare string or an array of items)
// into role/content messages.
func (req responsesRequest) inputTurns() ([]chatTurn, error) {
	if len(req.Input) == 0 {
		return nil, nil
	}

	// A bare string is a single user message.
	var text string
	if err := json.Unmarshal(req.Input, &text); err == nil {
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return []chatTurn{{Role: "user", Content: text}}, nil
	}

	var items []responsesInputItem
	if err := json.Unmarshal(req.Input, &items); err != nil {
		// A single item object is also accepted, alongside a string or array.
		var single responsesInputItem
		if err := json.Unmarshal(req.Input, &single); err != nil {
			return nil, fmt.Errorf("input must be a string, an item, or an array of items: %w", err)
		}
		items = []responsesInputItem{single}
	}

	turns := make([]chatTurn, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case "", "message", "input_message":
			role := item.Role
			if role == "" {
				role = "user"
			}
			if role == "developer" {
				role = "system"
			}
			turns = append(turns, chatTurn{Role: role, Content: flattenContent(item.Content)})
		case "function_call":
			turns = append(turns, chatTurn{
				Role:    "assistant",
				Content: renderToolCall(item.Name, rawString(item.Arguments)),
			})
		case "function_call_output":
			turns = append(turns, chatTurn{
				Role:    "user",
				Content: renderToolResponse(item.CallID, rawString(item.Output)),
			})
		default:
			// Reasoning items and anything else are not fed back to the model.
		}
	}
	return turns, nil
}

// flattenContent reads a message's content, which is either a string or an
// array of typed parts (input_text/output_text), as a single string.
func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

// rawString reads a JSON value that should be text, but tolerates clients that
// send a raw object/array by returning its JSON encoding.
func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(raw))
}

// renderToolCall renders a prior function call back into the text protocol, so
// the model can see what it asked for on the previous turn.
func renderToolCall(name, arguments string) string {
	encodedName, _ := json.Marshal(name)
	return fmt.Sprintf("%s\n{\"name\":%s,\"arguments\":%s}\n%s",
		toolCallOpen, encodedName, jsonOrString(arguments), toolCallClose)
}

// renderToolResponse renders a tool result back into the text protocol.
func renderToolResponse(callID, output string) string {
	encodedID, _ := json.Marshal(callID)
	return fmt.Sprintf("<tool_response call_id=%s>%s</tool_response>", encodedID, output)
}

// jsonOrString returns value if it is already valid JSON, otherwise it quotes it
// as a JSON string.
func jsonOrString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" && json.Valid([]byte(trimmed)) {
		return trimmed
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// systemPrompt builds the system message: the caller's instructions followed by
// the tool protocol when tools were supplied.
func (req responsesRequest) systemPrompt() string {
	var sections []string
	if trimmed := strings.TrimSpace(req.Instructions); trimmed != "" {
		sections = append(sections, trimmed)
	}
	if prompt := req.toolPrompt(); prompt != "" {
		sections = append(sections, prompt)
	}
	return strings.Join(sections, "\n\n")
}

// toolPrompt describes the available tools and the calling convention. It is
// empty when there are no tools or when tool_choice is "none".
func (req responsesRequest) toolPrompt() string {
	if len(req.Tools) == 0 || parseToolChoice(req.ToolChoice).mode == "none" {
		return ""
	}

	var schemas strings.Builder
	for _, tool := range req.Tools {
		name := tool.functionName()
		if name == "" {
			continue
		}
		schema := map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": tool.functionDescription(),
				"parameters":  tool.functionParameters(),
			},
		}
		encoded, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		schemas.Write(encoded)
		schemas.WriteByte('\n')
	}
	if schemas.Len() == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("# Tools\n\n")
	b.WriteString("You can call the functions listed below. Call a function when it helps answer the user.\n\n")
	b.WriteString("<tools>\n")
	b.WriteString(schemas.String())
	b.WriteString("</tools>\n\n")
	b.WriteString("To call one or more functions, reply with each call as a JSON object wrapped in ")
	b.WriteString(toolCallOpen)
	b.WriteString(toolCallClose)
	b.WriteString(" tags, exactly like this:\n")
	b.WriteString(toolCallOpen + "\n")
	b.WriteString("{\"name\": \"function_name\", \"arguments\": {\"argument\": \"value\"}}\n")
	b.WriteString(toolCallClose + "\n\n")
	b.WriteString("Tool results come back as a user message wrapped in <tool_response> tags. ")
	b.WriteString("Only call the functions listed above, and never invent a tool result.\n")

	switch choice := parseToolChoice(req.ToolChoice); choice.mode {
	case "required":
		b.WriteString("\nYou must call at least one function in your reply.")
	case "function":
		if choice.name != "" {
			b.WriteString("\nYou must call the function named \"" + choice.name + "\" in your reply.")
		}
	}
	return b.String()
}

// toolChoice is the parsed form of the request's tool_choice.
type toolChoice struct {
	mode string // "auto", "none", "required" or "function"
	name string // set when mode is "function"
}

// parseToolChoice decodes tool_choice, which is "auto", "none", "required", or
// an object naming a specific function.
func parseToolChoice(raw json.RawMessage) toolChoice {
	if len(raw) == 0 {
		return toolChoice{mode: "auto"}
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		switch single {
		case "auto", "none", "required":
			return toolChoice{mode: single}
		default:
			return toolChoice{mode: "auto"}
		}
	}

	var obj struct {
		Name     string `json:"name"`
		Function *struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		name := obj.Name
		if name == "" && obj.Function != nil {
			name = obj.Function.Name
		}
		if name != "" {
			return toolChoice{mode: "function", name: name}
		}
	}
	return toolChoice{mode: "auto"}
}

// knownToolNames collects the function names a request advertised, used to keep
// the looser parsing heuristics from mistaking ordinary JSON for a tool call.
func knownToolNames(tools []responsesTool) map[string]bool {
	if len(tools) == 0 {
		return nil
	}
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if name := tool.functionName(); name != "" {
			names[name] = true
		}
	}
	return names
}

// parsedToolCall is a tool call extracted from model output.
type parsedToolCall struct {
	Name      string
	Arguments string // JSON-encoded arguments object, as a string
	CallID    string // empty when the model did not provide one
}

// parseToolCalls splits model output into the text to show and any tool calls
// the model made. Tagged calls are extracted first; if there are none and the
// whole reply is a bare JSON call, that is accepted too.
func parseToolCalls(text string, known map[string]bool) (string, []parsedToolCall) {
	scanner := &toolCallScanner{known: known}
	emitted, calls := scanner.Feed(text)
	tail, more := scanner.Flush()
	calls = append(calls, more...)

	content := strings.TrimSpace(emitted + tail)
	if len(calls) == 0 {
		if bare := parseToolCallBlock(content, known, false); len(bare) > 0 {
			return "", bare
		}
	}
	return content, calls
}

// toolCallScanner extracts <tool_call></tool_call> blocks from a stream of
// text, holding back a possible partial opening tag so tags split across
// chunks are still recognised.
type toolCallScanner struct {
	known   map[string]bool
	pending string
	inCall  bool
	callBuf string
}

// Feed adds a chunk of generated text and returns the text that is safe to
// show (never part of a tool call) plus any calls completed by this chunk.
func (s *toolCallScanner) Feed(chunk string) (string, []parsedToolCall) {
	s.pending += chunk
	var out strings.Builder
	var calls []parsedToolCall

	for {
		if s.inCall {
			idx := strings.Index(s.pending, toolCallClose)
			if idx < 0 {
				s.callBuf += s.pending
				s.pending = ""
				break
			}
			s.callBuf += s.pending[:idx]
			s.pending = s.pending[idx+len(toolCallClose):]
			calls = append(calls, parseToolCallBlock(s.callBuf, s.known, true)...)
			s.callBuf = ""
			s.inCall = false
			continue
		}

		idx := strings.Index(s.pending, toolCallOpen)
		if idx >= 0 {
			out.WriteString(s.pending[:idx])
			s.pending = s.pending[idx+len(toolCallOpen):]
			s.inCall = true
			continue
		}

		// No complete opening tag: emit everything except a tail that could
		// still turn into one.
		hold := len(toolCallOpen) - 1
		if len(s.pending) <= hold {
			break
		}
		out.WriteString(s.pending[:len(s.pending)-hold])
		s.pending = s.pending[len(s.pending)-hold:]
		break
	}
	return out.String(), calls
}

// Flush returns any text still held back at the end of a stream (together with
// a call left unterminated).
func (s *toolCallScanner) Flush() (string, []parsedToolCall) {
	text := s.pending
	s.pending = ""
	var calls []parsedToolCall
	if s.inCall {
		calls = append(calls, parseToolCallBlock(s.callBuf, s.known, true)...)
		s.callBuf = ""
		s.inCall = false
	}
	return text, calls
}

// parseToolCallBlock parses the contents of one <tool_call> block, which is
// either a single call object or an array of them. When allowSingleKey is set,
// a lone object whose key is a known tool name is treated as a call, as some
// models emit {"tool_name": {...}}.
func parseToolCallBlock(raw string, known map[string]bool, allowSingleKey bool) []parsedToolCall {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var entries []json.RawMessage
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil
		}
	} else {
		entries = []json.RawMessage{json.RawMessage(raw)}
	}

	var calls []parsedToolCall
	for _, entry := range entries {
		if call, ok := parseToolCallObject(entry, known, allowSingleKey); ok {
			calls = append(calls, call)
		}
	}
	return calls
}

// parseToolCallObject parses one call object.
func parseToolCallObject(raw json.RawMessage, known map[string]bool, allowSingleKey bool) (parsedToolCall, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return parsedToolCall{}, false
	}

	// Chat-completions style wraps the call in a "function" object; flatten it.
	if fn, ok := obj["function"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(fn, &inner) == nil {
			for key, value := range obj {
				if key != "function" {
					if _, exists := inner[key]; !exists {
						inner[key] = value
					}
				}
			}
			obj = inner
		}
	}

	name := decodeJSONString(obj["name"])
	var arguments json.RawMessage
	for _, key := range []string{"arguments", "parameters", "args", "input"} {
		if value, ok := obj[key]; ok {
			arguments = value
			break
		}
	}

	// Some models emit a single-key object mapping the tool name to its
	// arguments, eg {"get_weather": {"city": "Paris"}}.
	if name == "" && arguments == nil && allowSingleKey && len(obj) == 1 {
		for key, value := range obj {
			if known == nil || known[key] {
				name = key
				arguments = value
			}
		}
	}
	if name == "" {
		return parsedToolCall{}, false
	}

	callID := decodeJSONString(obj["call_id"])
	if callID == "" {
		callID = decodeJSONString(obj["id"])
	}
	return parsedToolCall{
		Name:      name,
		Arguments: normalizeArguments(arguments),
		CallID:    callID,
	}, true
}

// decodeJSONString reads a JSON string value, returning "" for anything else.
func decodeJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return ""
}

// normalizeArguments turns a call's arguments into the JSON-encoded string the
// Responses API uses: an object stays an object, a string is kept, and anything
// missing becomes "{}".
func normalizeArguments(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	trimmed := strings.TrimSpace(string(raw))
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		trimmed = strings.TrimSpace(text)
	}
	if trimmed == "" {
		return "{}"
	}
	if !json.Valid([]byte(trimmed)) {
		encoded, err := json.Marshal(trimmed)
		if err != nil {
			return "{}"
		}
		return string(encoded)
	}
	return trimmed
}

// responsesResponse is a complete Responses API response object.
type responsesResponse struct {
	ID                string              `json:"id"`
	Object            string              `json:"object"`
	CreatedAt         int64               `json:"created_at"`
	Status            string              `json:"status"`
	Model             string              `json:"model"`
	Output            []any               `json:"output"`
	OutputText        string              `json:"output_text,omitempty"`
	ParallelToolCalls bool                `json:"parallel_tool_calls"`
	ToolChoice        json.RawMessage     `json:"tool_choice,omitempty"`
	Tools             []responsesToolEcho `json:"tools,omitempty"`
	Temperature       *float32            `json:"temperature,omitempty"`
	TopP              *float32            `json:"top_p,omitempty"`
	MaxOutputTokens   *int                `json:"max_output_tokens,omitempty"`
	Instructions      string              `json:"instructions,omitempty"`
}

// responsesMessageItem is an assistant text item in the response output.
type responsesMessageItem struct {
	Type    string                   `json:"type"`
	ID      string                   `json:"id"`
	Status  string                   `json:"status"`
	Role    string                   `json:"role"`
	Content []responsesOutputContent `json:"content"`
}

// responsesFunctionCallItem is a tool call item in the response output.
type responsesFunctionCallItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// responsesOutputContent is one content part of a message item.
type responsesOutputContent struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

// buildResponsesResponse assembles a response object from the model's text and
// any tool calls, echoing the request's tool configuration.
func buildResponsesResponse(id string, created time.Time, model, status, content string, calls []parsedToolCall, req responsesRequest) responsesResponse {
	output := make([]any, 0, len(calls)+1)
	if content != "" || len(calls) == 0 {
		output = append(output, responsesMessageItem{
			Type:   "message",
			ID:     newResponseID("msg"),
			Status: status,
			Role:   "assistant",
			Content: []responsesOutputContent{{
				Type:        "output_text",
				Text:        content,
				Annotations: []any{},
			}},
		})
	}
	for _, call := range calls {
		callID := call.CallID
		if callID == "" {
			callID = newResponseID("call")
		}
		output = append(output, responsesFunctionCallItem{
			Type:      "function_call",
			ID:        newResponseID("fc"),
			Status:    status,
			CallID:    callID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}

	response := responsesResponse{
		ID:                id,
		Object:            "response",
		CreatedAt:         created.Unix(),
		Status:            status,
		Model:             model,
		Output:            output,
		OutputText:        content,
		ParallelToolCalls: true,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxOutputTokens:   req.MaxOutputTokens,
		Instructions:      req.Instructions,
	}
	if len(req.ToolChoice) > 0 && json.Valid(req.ToolChoice) {
		response.ToolChoice = req.ToolChoice
	} else {
		response.ToolChoice = json.RawMessage(`"auto"`)
	}
	if len(req.Tools) > 0 {
		response.Tools = echoTools(req.Tools)
	}
	return response
}

// echoTools converts the request's tools into the shape echoed in a response.
func echoTools(tools []responsesTool) []responsesToolEcho {
	echoed := make([]responsesToolEcho, 0, len(tools))
	for _, tool := range tools {
		name := tool.functionName()
		if name == "" {
			continue
		}
		echoed = append(echoed, responsesToolEcho{
			Type:        "function",
			Name:        name,
			Description: tool.functionDescription(),
			Parameters:  tool.functionParameters(),
		})
	}
	return echoed
}

// responseIDCounter keeps ids unique even when several are generated within the
// same nanosecond.
var responseIDCounter uint64

// newResponseID builds an OpenAI-style resource id, eg "resp_1a2b3c4d0001".
func newResponseID(prefix string) string {
	n := atomic.AddUint64(&responseIDCounter, 1)
	return fmt.Sprintf("%s_%x%04x", prefix, time.Now().UnixNano(), n)
}

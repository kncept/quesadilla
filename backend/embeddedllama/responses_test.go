package embeddedllama

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseToolCallsTagged(t *testing.T) {
	text := "Let me check.\n" +
		`<tool_call>` + "\n" +
		`{"name": "get_weather", "arguments": {"city": "Paris"}}` + "\n" +
		`</tool_call>`

	content, calls := parseToolCalls(text, map[string]bool{"get_weather": true})
	if content != "Let me check." {
		t.Errorf("content = %q, want %q", content, "Let me check.")
	}
	want := []parsedToolCall{{Name: "get_weather", Arguments: `{"city": "Paris"}`}}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
}

func TestParseToolCallsMultiple(t *testing.T) {
	text := `A<tool_call>{"name":"f","arguments":{"x":1}}</tool_call>` +
		`B<tool_call>{"name":"g","arguments":{}}</tool_call>C`

	content, calls := parseToolCalls(text, nil)
	if content != "ABC" {
		t.Errorf("content = %q, want %q", content, "ABC")
	}
	want := []parsedToolCall{
		{Name: "f", Arguments: `{"x":1}`},
		{Name: "g", Arguments: `{}`},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
}

func TestParseToolCallsArgumentForms(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "arguments as JSON string",
			text: `<tool_call>{"name":"f","arguments":"{\"a\":1}"}</tool_call>`,
			want: `{"a":1}`,
		},
		{
			name: "nested function object",
			text: `<tool_call>{"function":{"name":"f","arguments":{"a":1}},"id":"call_9"}</tool_call>`,
			want: `{"a":1}`,
		},
		{
			name: "missing arguments",
			text: `<tool_call>{"name":"f"}</tool_call>`,
			want: `{}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, calls := parseToolCalls(tc.text, nil)
			if len(calls) != 1 {
				t.Fatalf("calls = %+v, want exactly one", calls)
			}
			if calls[0].Name != "f" || calls[0].Arguments != tc.want {
				t.Errorf("call = %+v, want name f arguments %s", calls[0], tc.want)
			}
		})
	}
}

func TestParseToolCallsNestedFunctionKeepsCallID(t *testing.T) {
	_, calls := parseToolCalls(`<tool_call>{"function":{"name":"f","arguments":{}},"id":"call_9"}</tool_call>`, nil)
	if len(calls) != 1 || calls[0].CallID != "call_9" {
		t.Fatalf("calls = %+v, want one call with CallID call_9", calls)
	}
}

func TestParseToolCallsSingleKey(t *testing.T) {
	text := `<tool_call>{"get_weather":{"city":"Paris"}}</tool_call>`

	_, calls := parseToolCalls(text, map[string]bool{"get_weather": true})
	if len(calls) != 1 || calls[0].Name != "get_weather" || calls[0].Arguments != `{"city":"Paris"}` {
		t.Errorf("calls = %+v, want get_weather with arguments", calls)
	}

	// A single-key object whose key is not an advertised tool is not a call.
	_, calls = parseToolCalls(text, map[string]bool{"other": true})
	if len(calls) != 0 {
		t.Errorf("calls = %+v, want none for an unknown tool name", calls)
	}
}

func TestParseToolCallsBareJSON(t *testing.T) {
	content, calls := parseToolCalls(`{"name":"f","arguments":{"a":1}}`, map[string]bool{"f": true})
	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}
	if len(calls) != 1 || calls[0].Name != "f" || calls[0].Arguments != `{"a":1}` {
		t.Errorf("calls = %+v, want one f call", calls)
	}
}

func TestParseToolCallsPlainText(t *testing.T) {
	cases := []string{
		"The capital of France is Paris.",
		`{"answer":42}`, // ordinary JSON is not mistaken for a call
	}
	for _, text := range cases {
		content, calls := parseToolCalls(text, nil)
		if len(calls) != 0 {
			t.Errorf("parseToolCalls(%q) calls = %+v, want none", text, calls)
		}
		if content != text {
			t.Errorf("parseToolCalls(%q) content = %q, want unchanged", text, content)
		}
	}
}

func TestToolCallScannerChunking(t *testing.T) {
	scanner := &toolCallScanner{known: map[string]bool{"f": true}}
	chunks := []string{
		"Hello ",
		"<tool",
		"_call>",
		`{"name":`,
		`"f",`,
		`"arguments":{}}`,
		"</tool_call>",
		" done",
	}

	var text strings.Builder
	var calls []parsedToolCall
	for _, chunk := range chunks {
		emitted, done := scanner.Feed(chunk)
		text.WriteString(emitted)
		calls = append(calls, done...)
	}
	tail, done := scanner.Flush()
	text.WriteString(tail)
	calls = append(calls, done...)

	if got := text.String(); got != "Hello  done" {
		t.Errorf("text = %q, want %q", got, "Hello  done")
	}
	if len(calls) != 1 || calls[0].Name != "f" {
		t.Errorf("calls = %+v, want one f call", calls)
	}
}

func TestToolCallScannerReleasesHeldText(t *testing.T) {
	scanner := &toolCallScanner{}
	if emitted, _ := scanner.Feed("hello <too"); emitted != "" {
		t.Errorf("emitted = %q before flush, want empty (partial tag held back)", emitted)
	}
	text, calls := scanner.Flush()
	if text != "hello <too" || len(calls) != 0 {
		t.Errorf("Flush() = (%q, %+v), want (\"hello <too\", none)", text, calls)
	}
}

func TestChatTurnsFromString(t *testing.T) {
	req := responsesRequest{Input: json.RawMessage(`"hello there"`)}
	turns, err := req.chatTurns()
	if err != nil {
		t.Fatal(err)
	}
	want := []chatTurn{{Role: "user", Content: "hello there"}}
	if !reflect.DeepEqual(turns, want) {
		t.Errorf("turns = %+v, want %+v", turns, want)
	}
}

func TestChatTurnsFromItems(t *testing.T) {
	input := `[
		{"type":"message","role":"developer","content":"be nice"},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_text","text":"world"}]},
		{"type":"function_call","name":"f","arguments":"{\"a\":1}","call_id":"call_1"},
		{"type":"function_call_output","call_id":"call_1","output":"42"},
		{"type":"reasoning","summary":[]}
	]`
	req := responsesRequest{Input: json.RawMessage(input)}
	turns, err := req.chatTurns()
	if err != nil {
		t.Fatal(err)
	}
	want := []chatTurn{
		{Role: "system", Content: "be nice"},
		{Role: "user", Content: "hello\nworld"},
		{Role: "assistant", Content: "<tool_call>\n{\"name\":\"f\",\"arguments\":{\"a\":1}}\n</tool_call>"},
		{Role: "user", Content: `<tool_response call_id="call_1">42</tool_response>`},
	}
	if !reflect.DeepEqual(turns, want) {
		t.Errorf("turns = %+v, want %+v", turns, want)
	}
}

func TestChatTurnsPrependsInstructions(t *testing.T) {
	req := responsesRequest{
		Input:        json.RawMessage(`"hi"`),
		Instructions: "Be terse.",
		Tools:        []responsesTool{{Type: "function", Name: "get_weather"}},
	}
	turns, err := req.chatTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Role != "system" {
		t.Fatalf("turns = %+v, want a leading system turn", turns)
	}
	if !strings.Contains(turns[0].Content, "Be terse.") || !strings.Contains(turns[0].Content, "get_weather") {
		t.Errorf("system content = %q, want instructions and tools", turns[0].Content)
	}
}

func TestSystemPromptTools(t *testing.T) {
	tool := responsesTool{Type: "function", Name: "get_weather", Description: "Weather", Parameters: json.RawMessage(`{"type":"object"}`)}

	cases := []struct {
		name       string
		choice     string
		wantIn     []string
		wantAbsent []string
	}{
		{
			name:   "auto",
			choice: `"auto"`,
			wantIn: []string{"get_weather", "<tools>", toolCallOpen},
		},
		{
			name:       "none hides tools",
			choice:     `"none"`,
			wantAbsent: []string{"get_weather", "<tools>"},
		},
		{
			name:   "required",
			choice: `"required"`,
			wantIn: []string{"must call at least one function"},
		},
		{
			name:   "specific function",
			choice: `{"type":"function","name":"get_weather"}`,
			wantIn: []string{`must call the function named "get_weather"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := responsesRequest{
				Tools:      []responsesTool{tool},
				ToolChoice: json.RawMessage(tc.choice),
			}
			prompt := req.systemPrompt()
			for _, want := range tc.wantIn {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt missing %q:\n%s", want, prompt)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(prompt, absent) {
					t.Errorf("prompt unexpectedly contains %q:\n%s", absent, prompt)
				}
			}
		})
	}
}

func TestParseToolChoice(t *testing.T) {
	cases := []struct {
		raw  string
		want toolChoice
	}{
		{``, toolChoice{mode: "auto"}},
		{`"auto"`, toolChoice{mode: "auto"}},
		{`"none"`, toolChoice{mode: "none"}},
		{`"required"`, toolChoice{mode: "required"}},
		{`{"type":"function","name":"f"}`, toolChoice{mode: "function", name: "f"}},
		{`{"type":"function","function":{"name":"g"}}`, toolChoice{mode: "function", name: "g"}},
		{`{}`, toolChoice{mode: "auto"}},
	}
	for _, tc := range cases {
		if got := parseToolChoice(json.RawMessage(tc.raw)); got != tc.want {
			t.Errorf("parseToolChoice(%s) = %+v, want %+v", tc.raw, got, tc.want)
		}
	}
}

func TestBuildResponsesResponse(t *testing.T) {
	req := responsesRequest{
		Tools:      []responsesTool{{Type: "function", Name: "f"}},
		ToolChoice: json.RawMessage(`"auto"`),
	}
	resp := buildResponsesResponse("resp_1", time.Now(), "my-model", "completed", "hi", []parsedToolCall{{Name: "f", Arguments: "{}"}}, req)

	if resp.ID != "resp_1" || resp.Object != "response" || resp.Status != "completed" || resp.Model != "my-model" {
		t.Errorf("response envelope = %+v", resp)
	}
	if resp.OutputText != "hi" {
		t.Errorf("output_text = %q, want hi", resp.OutputText)
	}
	if len(resp.Output) != 2 {
		t.Fatalf("output = %+v, want a message and a function call", resp.Output)
	}
	message, ok := resp.Output[0].(responsesMessageItem)
	if !ok {
		t.Fatalf("output[0] = %T, want responsesMessageItem", resp.Output[0])
	}
	if message.Role != "assistant" || len(message.Content) != 1 || message.Content[0].Text != "hi" {
		t.Errorf("message item = %+v", message)
	}
	if message.ID == "" {
		t.Error("message item has no id")
	}
	call, ok := resp.Output[1].(responsesFunctionCallItem)
	if !ok {
		t.Fatalf("output[1] = %T, want responsesFunctionCallItem", resp.Output[1])
	}
	if call.Name != "f" || call.CallID == "" || call.ID == "" {
		t.Errorf("function call item = %+v", call)
	}
	if string(resp.ToolChoice) != `"auto"` {
		t.Errorf("tool_choice = %s, want \"auto\"", resp.ToolChoice)
	}
	if len(resp.Tools) != 1 || resp.Tools[0].Name != "f" {
		t.Errorf("tools echo = %+v", resp.Tools)
	}
}

func TestBuildResponsesResponseToolOnly(t *testing.T) {
	// No text: the message item is omitted and only the function call remains.
	resp := buildResponsesResponse("resp_2", time.Now(), "m", "completed", "", []parsedToolCall{{Name: "f", Arguments: "{}"}}, responsesRequest{})
	if len(resp.Output) != 1 {
		t.Fatalf("output = %+v, want just the function call", resp.Output)
	}
	if _, ok := resp.Output[0].(responsesFunctionCallItem); !ok {
		t.Errorf("output[0] = %T, want responsesFunctionCallItem", resp.Output[0])
	}
}

func TestEchoToolsNestedForm(t *testing.T) {
	tools := []responsesTool{{
		Type: "function",
		Function: &responsesToolFunction{
			Name:       "nested",
			Parameters: json.RawMessage(`{"type":"object"}`),
		},
	}}
	echoed := echoTools(tools)
	if len(echoed) != 1 || echoed[0].Name != "nested" {
		t.Fatalf("echoed = %+v, want one nested tool", echoed)
	}
	if string(echoed[0].Parameters) != `{"type":"object"}` {
		t.Errorf("parameters = %s, want the schema", echoed[0].Parameters)
	}
}

func TestFlattenContent(t *testing.T) {
	if got := flattenContent(json.RawMessage(`"plain"`)); got != "plain" {
		t.Errorf("flattenContent(string) = %q, want plain", got)
	}
	parts := json.RawMessage(`[{"type":"input_text","text":"a"},{"type":"input_text","text":"b"},{"type":"input_text"}]`)
	if got := flattenContent(parts); got != "a\nb" {
		t.Errorf("flattenContent(parts) = %q, want a\\nb", got)
	}
}

func TestChatTurnsFromSingleItem(t *testing.T) {
	req := responsesRequest{Input: json.RawMessage(`{"role":"user","content":"hi"}`)}
	turns, err := req.chatTurns()
	if err != nil {
		t.Fatal(err)
	}
	want := []chatTurn{{Role: "user", Content: "hi"}}
	if !reflect.DeepEqual(turns, want) {
		t.Errorf("turns = %+v, want %+v", turns, want)
	}
}

func TestDecodeResponsesRequest(t *testing.T) {
	body := `{
		"model": "m",
		"instructions": "Be terse.",
		"input": [{"role": "user", "content": "weather?"}],
		"tools": [{"type": "function", "name": "get_weather", "description": "d",
			"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}],
		"tool_choice": "auto",
		"max_output_tokens": 64,
		"temperature": 0.2,
		"stream": false
	}`
	var req responsesRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "m" {
		t.Errorf("model = %q, want m", req.Model)
	}
	if req.MaxOutputTokens == nil || *req.MaxOutputTokens != 64 {
		t.Errorf("max_output_tokens = %v, want 64", req.MaxOutputTokens)
	}

	turns, err := req.chatTurns()
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Role != "system" || turns[1].Role != "user" {
		t.Fatalf("turns = %+v, want a system turn then a user turn", turns)
	}
	if !strings.Contains(turns[0].Content, `"get_weather"`) {
		t.Errorf("system prompt missing tool schema:\n%s", turns[0].Content)
	}
	if turns[1].Content != "weather?" {
		t.Errorf("user turn = %q, want weather?", turns[1].Content)
	}
}

func TestNewResponseID(t *testing.T) {
	a := newResponseID("resp")
	b := newResponseID("resp")
	if !strings.HasPrefix(a, "resp_") {
		t.Errorf("id = %q, want resp_ prefix", a)
	}
	if a == b {
		t.Errorf("ids are not unique: %q == %q", a, b)
	}
}

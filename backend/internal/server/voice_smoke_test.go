//go:build smoke

// Live Gemini voice turn through the mddb embedded gateway and workspace MCP tools.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/gomode/voicegateway"
	voicev1 "github.com/maruel/gomode/voicegateway/api/v1"
	"github.com/maruel/gomode/voicegateway/voicertc"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/storage"
	"github.com/pion/webrtc/v4"
)

// TestSmokeVoiceGatewayGemini exercises the real embedded gateway against Gemini
// Live: it authenticates a user through mddb, opens a WebRTC session, declares
// the workspace MCP tools, completes a tool round trip, receives assistant
// audio, and confirms the gateway releases session capacity on hang-up.
//
// It is opt-in because it needs GEMINI_API_KEY and reachable WebRTC UDP. Run:
//
//	GEMINI_API_KEY=... go test -tags=smoke -run TestSmokeVoiceGatewayGemini -v ./backend/internal/server/
func TestSmokeVoiceGatewayGemini(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY is required for the live voice gateway smoke test")
	}
	env := setupTestEnv(t)

	var auth dto.AuthResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/register", dto.RegisterRequest{
		Email: "voice-smoke@example.com", Password: "Pass1234", Name: "Voice Smoke",
	}, &auth, ""); status != http.StatusOK {
		t.Fatalf("register status = %d", status)
	}
	var org dto.OrganizationResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations", dto.CreateOrganizationRequest{
		Name: "Voice Smoke Org",
	}, &org, auth.Token); status != http.StatusOK {
		t.Fatalf("create organization status = %d", status)
	}
	var ws dto.WorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{
		Name: "Voice Smoke Workspace",
	}, &ws, auth.Token); status != http.StatusOK {
		t.Fatalf("create workspace status = %d", status)
	}
	var page dto.CreatePageResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/workspaces/"+ws.ID.String()+"/nodes/0/page/create", dto.CreatePageRequest{
		Title: "Voice Smoke Document",
	}, &page, auth.Token); status != http.StatusOK {
		t.Fatalf("create page status = %d", status)
	}
	var switched dto.SwitchWorkspaceResponse
	if status := env.doJSON(t, http.MethodPost, "/api/v1/auth/switch-workspace", dto.SwitchWorkspaceRequest{
		WsID: ws.ID,
	}, &switched, auth.Token); status != http.StatusOK {
		t.Fatalf("switch workspace status = %d", status)
	}

	voiceCfg := voicegateway.DefaultConfig()
	if err := voiceCfg.ValidateEmbedded(); err != nil {
		t.Fatalf("voice config: %v", err)
	}
	bridgeCtx, bridgeCancel := context.WithCancel(context.WithoutCancel(t.Context()))
	t.Cleanup(bridgeCancel)
	started := time.Now()
	bridge, err := voicertc.NewBridge(bridgeCtx, &voiceCfg, apiKey, voiceCfg.Server.WebRTCUDPPort, t.TempDir())
	if err != nil {
		t.Fatalf("start voice bridge: %v", err)
	}
	t.Cleanup(func() { bridge.CloseAll(context.WithoutCancel(t.Context())) })
	t.Logf("voice bridge setup time, excluded from turn latency: %s", time.Since(started))

	serverCfg := &storage.ServerConfig{JWTSecret: testJWTSecret, Quotas: storage.DefaultServerQuotas(), RateLimits: storage.DefaultRateLimits()}
	server := httptest.NewServer(NewRouter(env.services, &Config{ServerConfig: serverCfg, Version: "smoke", VoiceBridge: bridge}))
	t.Cleanup(server.Close)

	tools := smokeToolDeclarations(t, env, switched.Token)

	client, sessionID := newSmokeVoiceClient(t, server.URL, switched.Token)
	client.send(t, voicev1.SessionSetup{
		Kind:  voicev1.MessageKindSessionSetup,
		Voice: voicev1.VoiceConfig{Name: "Orus", Language: "en"},
		Tools: tools,
		Context: voicev1.Context{
			SystemInstruction: "You are the mddb workspace voice assistant. When the user asks about documents or pages, call the nodes_list tool before answering and answer only from its result. Keep answers to one sentence.",
		},
	})
	client.wait(t, voicev1.MessageKindSessionReady)

	ask := time.Now()
	client.send(t, voicev1.UserMessage{Kind: voicev1.MessageKindUserMessage, Text: "List the documents in my workspace."})
	sawToolCall, answered, err := client.runTurn(t, env, switched.Token, ask)
	if err != nil {
		t.Fatal(err)
	}
	if !sawToolCall {
		t.Fatal("assistant never called the workspace MCP nodes_list tool")
	}
	if !answered {
		t.Fatal("assistant produced no response after the tool result")
	}

	// A hang-up must release the session so the next offer is not a replacement.
	client.close(t)
	waitVoiceSessionGone(t, server.URL, switched.Token, sessionID, 20*time.Second)
	t.Log("voice session released after hang-up")
}

// smokeToolDeclarations fetches the workspace MCP tool catalog and converts it
// to provider-neutral voice tool declarations, mirroring the browser client.
func smokeToolDeclarations(t *testing.T, env *testEnv, token string) []voicev1.ToolDeclaration {
	t.Helper()
	raw := smokeMCPCall(t, env, token, "tools/list", "", map[string]any{})
	var listed struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	if len(listed.Tools) == 0 {
		t.Fatal("workspace MCP advertised no tools")
	}
	out := make([]voicev1.ToolDeclaration, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		out = append(out, voicev1.ToolDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}
	return out
}

type smokeVoiceMessage struct {
	kind voicev1.MessageKind
	raw  []byte
}

type smokeVoiceClient struct {
	t        *testing.T
	pc       *webrtc.PeerConnection
	dc       *webrtc.DataChannel
	messages chan smokeVoiceMessage
	opened   chan struct{}
	audio    atomic.Int64
}

func newSmokeVoiceClient(t *testing.T, serverURL, token string) (client *smokeVoiceClient, sessionID string) {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("new peer connection: %v", err)
	}
	c := &smokeVoiceClient{t: t, pc: pc, messages: make(chan smokeVoiceMessage, 64), opened: make(chan struct{})}
	t.Cleanup(func() { _ = pc.Close() })
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			return
		}
		for {
			pkt, _, readErr := track.ReadRTP()
			if readErr != nil {
				return
			}
			if len(pkt.Payload) > 0 {
				c.audio.Add(1)
			}
		}
	})
	mic, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "smoke-mic")
	if err != nil {
		t.Fatalf("new mic track: %v", err)
	}
	if _, err := pc.AddTrack(mic); err != nil {
		t.Fatalf("add mic track: %v", err)
	}
	dc, err := pc.CreateDataChannel("voice-gateway", nil)
	if err != nil {
		t.Fatalf("create data channel: %v", err)
	}
	c.dc = dc
	dc.OnOpen(func() { close(c.opened) })
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var env voicev1.MessageEnvelope
		if err := json.Unmarshal(msg.Data, &env); err != nil {
			return
		}
		c.messages <- smokeVoiceMessage{kind: env.Kind, raw: append([]byte(nil), msg.Data...)}
	})

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description: %v", err)
	}
	select {
	case <-gathered:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out gathering ICE candidates")
	}
	local := pc.LocalDescription()
	if local == nil {
		t.Fatal("missing local description")
	}
	answer := postVoiceOffer(t, serverURL, token, local.SDP)
	if answer.SessionID == "" {
		t.Fatal("gateway returned an empty session ID")
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatalf("set remote description: %v", err)
	}
	select {
	case <-c.opened:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out opening the voice data channel")
	}
	return c, answer.SessionID
}

func (c *smokeVoiceClient) send(t *testing.T, msg any) {
	t.Helper()
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal voice message: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- c.dc.SendText(string(data)) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("send voice message: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out sending voice message")
	}
}

func (c *smokeVoiceClient) wait(t *testing.T, want voicev1.MessageKind) []byte {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case msg := <-c.messages:
			if msg.kind == voicev1.MessageKindError {
				t.Fatalf("gateway error: %s", msg.raw)
			}
			if msg.kind == want {
				return msg.raw
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

// runTurn drives one assistant turn, answering tool calls from the workspace MCP
// endpoint. It reports whether a tool call happened and whether the assistant
// produced a spoken/text reply afterwards.
func (c *smokeVoiceClient) runTurn(t *testing.T, env *testEnv, token string, sent time.Time) (sawToolCall, answered bool, err error) {
	t.Helper()
	deadline := time.After(90 * time.Second)
	var toolResultAt, firstTextAt time.Time
	var assistant strings.Builder
	var grace <-chan time.Time
	finish := func() (bool, bool, error) {
		answered = strings.TrimSpace(assistant.String()) != ""
		if answered {
			t.Logf("user.message -> first assistant text: %s", firstTextAt.Sub(sent))
		}
		t.Logf("assistant reply: %q", strings.TrimSpace(assistant.String()))
		t.Logf("assistant RTP packets received: %d", c.audio.Load())
		return sawToolCall, answered, nil
	}
	for {
		select {
		case msg := <-c.messages:
			switch msg.kind {
			case voicev1.MessageKindError:
				return sawToolCall, answered, fmt.Errorf("gateway error: %s", msg.raw)
			case voicev1.MessageKindToolCall:
				var call voicev1.ToolCall
				if err := json.Unmarshal(msg.raw, &call); err != nil {
					return sawToolCall, answered, fmt.Errorf("decode tool.call: %w", err)
				}
				sawToolCall = true
				started := time.Now()
				result := smokeCallMCPTool(t, env, token, call.Name, call.Args)
				toolResultAt = time.Now()
				t.Logf("tool.call %s executed in %s", call.Name, toolResultAt.Sub(started))
				c.send(t, voicev1.ToolResult{
					Kind:   voicev1.MessageKindToolResult,
					ID:     call.ID,
					Name:   call.Name,
					Result: result,
				})
			case voicev1.MessageKindAssistantTextDelta:
				var delta voicev1.AssistantTextDelta
				if err := json.Unmarshal(msg.raw, &delta); err != nil {
					return sawToolCall, answered, fmt.Errorf("decode assistant text: %w", err)
				}
				if firstTextAt.IsZero() && strings.TrimSpace(delta.Text) != "" {
					firstTextAt = time.Now()
					if !toolResultAt.IsZero() {
						t.Logf("tool.result -> first assistant text: %s", firstTextAt.Sub(toolResultAt))
					}
				}
				assistant.WriteString(delta.Text)
				if grace == nil && sawToolCall && strings.TrimSpace(assistant.String()) != "" {
					grace = time.After(5 * time.Second)
				}
			case voicev1.MessageKindSpeechEnded:
				if sawToolCall {
					return finish()
				}
			default:
			}
		case <-grace:
			return finish()
		case <-deadline:
			return sawToolCall, answered, errors.New("timed out waiting for the assistant turn")
		}
	}
}

func (c *smokeVoiceClient) close(t *testing.T) {
	t.Helper()
	if err := c.pc.Close(); err != nil {
		t.Fatalf("close peer connection: %v", err)
	}
}

func postVoiceOffer(t *testing.T, serverURL, token, sdp string) voicev1.VoiceRTCAnswerResp {
	t.Helper()
	body, err := json.Marshal(voicev1.VoiceRTCOfferReq{SDP: sdp})
	if err != nil {
		t.Fatalf("marshal offer: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL+"/api/voicegateway/v1/voice/rtc/offer", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new offer request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send offer: %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read offer response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("offer status = %d: %s", resp.StatusCode, data)
	}
	var answer voicev1.VoiceRTCAnswerResp
	if err := json.Unmarshal(data, &answer); err != nil {
		t.Fatalf("decode offer response: %v", err)
	}
	return answer
}

// smokeCallMCPTool runs one workspace MCP tool call and returns the structured
// content the assistant should reason over.
func smokeCallMCPTool(t *testing.T, env *testEnv, token, name string, args json.RawMessage) json.RawMessage {
	t.Helper()
	arguments := map[string]any{}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			t.Fatalf("decode tool args: %v", err)
		}
	}
	raw := smokeMCPCall(t, env, token, "tools/call", name, map[string]any{"name": name, "arguments": arguments})
	var result struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode tools/call: %v", err)
	}
	if result.IsError {
		t.Fatalf("tools/call %s failed: %s", name, raw)
	}
	if len(result.StructuredContent) == 0 {
		return json.RawMessage(`{}`)
	}
	return result.StructuredContent
}

// smokeMCPCall issues one workspace MCP request, setting the Mcp-Name header
// that tools/call requires. name is empty for methods that do not need it.
func smokeMCPCall(t *testing.T, env *testEnv, token, method, name string, params map[string]any) json.RawMessage {
	t.Helper()
	full := make(map[string]any, len(params)+1)
	for k, v := range params {
		full[k] = v
	}
	full["_meta"] = map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcp.ProtocolVersion,
		"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "mddb-voice-smoke", "version": "1"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  method,
		"params":  full,
	})
	if err != nil {
		t.Fatalf("marshal MCP request: %v", err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, env.server.URL+goModeMCPEndpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new MCP request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Protocol-Version", mcp.ProtocolVersion)
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send MCP request: %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read MCP response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MCP status = %d: %s", resp.StatusCode, data)
	}
	var result struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode MCP response: %v", err)
	}
	if len(result.Error) != 0 {
		t.Fatalf("MCP error: %s", result.Error)
	}
	return result.Result
}

// waitVoiceSessionGone polls diagnostics until the gateway has forgotten the
// session, proving hang-up released capacity.
func waitVoiceSessionGone(t *testing.T, serverURL, token, sessionID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	path := "/api/voicegateway/v1/voice/rtc/" + sessionID + "/diagnostics"
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL+path, strings.NewReader(`{"client":{}}`))
		if err != nil {
			t.Fatalf("new diagnostics request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("diagnostics request: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("diagnostics status = %d", resp.StatusCode)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("voice session %s was not released within %s", sessionID, timeout)
}

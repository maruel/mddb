// Measures committed mutation fan-out through workspace registries and native/released MCP handlers.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/maruel/gomode/mcp"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/server/sse"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

// BenchmarkWorkspaceInvalidations measures actual committed page/record writes,
// broker routing, registry reads and full MCP response framing. Each operation
// changes an unrelated page and record before changing the subscribed page.
// Clients reread the subscribed page upon invalidation, representing context
// availability rather than only wire receipt. Target latency starts immediately
// before the watched mutation and ends at the last client read completion;
// workload latency also includes preceding unrelated mutations. Startup is
// excluded. Scanned bytes count resource-read payloads, not mutation file I/O.
func BenchmarkWorkspaceInvalidations(b *testing.B) {
	for _, size := range []struct{ subscribers, nodes int }{{1, 16}, {16, 256}} {
		for _, protocol := range []string{mcp.ProtocolVersion, "2025-06-18", "2025-11-25"} {
			b.Run(fmt.Sprintf("%s/subs%d/nodes%d", protocol, size.subscribers, size.nodes), func(b *testing.B) {
				env := setupTestEnv(b)
				env.services.Broker = sse.NewBroker()
				_, wsID := setupGoModeWorkspace(b, env, "fanout@example.com")
				user, err := env.userService.GetByEmail("fanout@example.com")
				if err != nil {
					b.Fatal(err)
				}
				store, err := env.fileStore.GetWorkspaceStore(b.Context(), wsID)
				if err != nil {
					b.Fatal(err)
				}
				author := handlers.GitAuthor(user)
				payload := strings.Repeat("body payload\n", 2048)
				watched, err := store.CreatePageUnderParent(b.Context(), 0, "Watched", payload, author)
				if err != nil {
					b.Fatal(err)
				}
				watched, err = store.ReadNode(b.Context(), watched.ID)
				if err != nil {
					b.Fatal(err)
				}
				other, err := store.CreatePageUnderParent(b.Context(), 0, "Unrelated", payload, author)
				if err != nil {
					b.Fatal(err)
				}
				other, err = store.ReadNode(b.Context(), other.ID)
				if err != nil {
					b.Fatal(err)
				}
				table, err := store.CreateTableUnderParent(b.Context(), 0, "Records", nil, author)
				if err != nil {
					b.Fatal(err)
				}
				for i := 0; i < size.nodes-3; i++ {
					if _, err := store.CreatePageUnderParent(b.Context(), 0, fmt.Sprintf("Catalog %d", i), "small", author); err != nil {
						b.Fatal(err)
					}
				}
				nodeH := &handlers.NodeHandler{Svc: env.services}
				reg := &invalidationMeasuredRegistry{workspaceRegistry: &workspaceRegistry{svc: env.services, nodeH: nodeH, user: user, wsID: wsID, role: identity.WSRoleAdmin, focus: newVoiceFocusState()}}
				uri := reg.nodeURI(watched.ID)
				h := &mcp.Handler{Registry: reg, Sessions: mcp.NewSessionStore(func(context.Context) (string, error) { return "benchmark-owner", nil })}
				ctx, cancel := context.WithCancel(b.Context())
				var wg sync.WaitGroup
				b.Cleanup(func() { cancel(); wg.Wait() })
				subscribers := size.subscribers
				writers := make([]*invalidationMeasuredWriter, 0, subscribers)
				for i := 0; i < subscribers; i++ {
					w := &invalidationMeasuredWriter{header: make(http.Header), uri: uri, registry: reg, ctx: ctx, ready: make(chan struct{}, 1), applied: make(chan invalidationApplied, 64)}
					var request *http.Request
					if protocol == mcp.ProtocolVersion {
						request = invalidationRequest(ctx, protocol, "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourcesListChanged": true, "resourceSubscriptions": []string{uri, voiceItemsURI}}})
					} else {
						post := func(method, session string, params map[string]any) *httptest.ResponseRecorder {
							r := invalidationRequest(ctx, protocol, method, params)
							r.Header.Set("Mcp-Session-Id", session)
							out := httptest.NewRecorder()
							h.HandleMCP(out, r)
							if out.Code != 200 {
								b.Fatalf("%s: %d %s", method, out.Code, out.Body.String())
							}
							return out
						}
						initialized := post("initialize", "", map[string]any{"protocolVersion": protocol, "clientInfo": map[string]string{"name": "fanout", "version": "1"}})
						id := initialized.Header().Get("Mcp-Session-Id")
						post("resources/subscribe", id, map[string]any{"uri": uri})
						post("resources/subscribe", id, map[string]any{"uri": voiceItemsURI})
						post("resources/list", id, map[string]any{})
						request = httptest.NewRequestWithContext(ctx, http.MethodGet, "/mcp", nil)
						request.Header.Set("Mcp-Protocol-Version", protocol)
						request.Header.Set("Mcp-Session-Id", id)
						request.Header.Set("Accept", "text/event-stream")
					}
					wg.Go(func() { h.HandleMCP(w, request) })
					select {
					case <-w.ready:
					case <-time.After(5 * time.Second):
						b.Fatal("initial snapshots did not complete")
					}
					// The initial target invalidation is a baseline, not a measured mutation.
					select {
					case applied := <-w.applied:
						if applied.err != nil {
							b.Fatal(applied.err)
						}
					case <-time.After(time.Second):
						b.Fatal("missing initial target")
					}
					writers = append(writers, w)
				}
				screens := make([]*invalidationScreenWriter, 0, subscribers)
				screenH := &handlers.SSEHandler{Svc: env.services, Cfg: &handlers.Config{Revision: "benchmark"}}
				for range subscribers {
					// Screen admission is per user; model distinct logged-in readers.
					screenUser := *user
					screenUser.ID = ksid.NewID()
					w := &invalidationScreenWriter{header: make(http.Header), ready: make(chan struct{}, 1), events: make(chan invalidationScreenReceipt, 64)}
					request := httptest.NewRequestWithContext(reqctx.WithUser(ctx, &screenUser), http.MethodGet, "/events", nil)
					request.SetPathValue("wsID", wsID.String())
					wg.Go(func() { screenH.ServeHTTP(w, request) })
					select {
					case <-w.ready:
					case <-time.After(5 * time.Second):
						b.Fatal("screen not connected")
					}
					screens = append(screens, w)
				}
				reg.reads.Store(0)
				reg.scanned.Store(0)
				reg.catalog.Store(0)
				for _, w := range writers {
					w.bytes.Store(0)
					w.catalogEvents.Store(0)
					w.itemEvents.Store(0)
					w.targetEvents.Store(0)
				}
				var before, after syscall.Rusage
				if err := syscall.Getrusage(syscall.RUSAGE_SELF, &before); err != nil {
					b.Fatal(err)
				}
				var latency, targetLatency, screenLatency time.Duration
				iteration := 0
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					started := time.Now()
					created, createErr := nodeH.CreatePage(ctx, wsID, user, &dto.CreatePageRequest{Title: fmt.Sprintf("Temporary %d", iteration), Content: "small"})
					if createErr != nil {
						b.Fatal(createErr)
					}
					if _, deleteErr := nodeH.DeleteNode(ctx, wsID, user, &dto.DeleteNodeRequest{ID: created.ID}); deleteErr != nil {
						b.Fatal(deleteErr)
					}
					other, err = nodeH.ReplacePage(ctx, wsID, user, &dto.UpdatePageRequest{ID: other.ID, Title: "Unrelated", Content: payload + strconv.Itoa(iteration), ExpectedRevision: other.Revision})
					if err != nil {
						b.Fatal(err)
					}
					if _, err := nodeH.CreateRecord(ctx, wsID, user, &dto.CreateRecordRequest{ID: table.ID, Data: map[string]any{"value": iteration}}); err != nil {
						b.Fatal(err)
					}
					targetStarted := time.Now()
					watched, err = nodeH.ReplacePage(ctx, wsID, user, &dto.UpdatePageRequest{ID: watched.ID, Title: "Watched", Content: payload + strconv.Itoa(iteration), ExpectedRevision: watched.Revision})
					if err != nil {
						b.Fatal(err)
					}
					var lastApplied time.Time
					for _, w := range writers {
						select {
						case applied := <-w.applied:
							if applied.err != nil {
								b.Fatal(applied.err)
							}
							if applied.at.Before(targetStarted) {
								b.Fatal("stale context receipt preceded watched mutation")
							}
							if applied.at.After(lastApplied) {
								lastApplied = applied.at
							}
						case <-time.After(5 * time.Second):
							b.Fatal("committed target did not reach context")
						}
					}
					for _, w := range screens {
						for range 5 {
							select {
							case receipt := <-w.events:
								if receipt.at.Before(started) {
									b.Fatal("stale screen receipt")
								}
								if receipt.node == watched.ID {
									screenLatency += receipt.at.Sub(targetStarted) / time.Duration(subscribers)
								}
							case <-time.After(5 * time.Second):
								b.Fatal("screen missed committed mutation")
							}
						}
					}
					latency += time.Since(started)
					targetLatency += lastApplied.Sub(targetStarted)
					iteration++
				}
				b.StopTimer()
				if err := syscall.Getrusage(syscall.RUSAGE_SELF, &after); err != nil {
					b.Fatal(err)
				}
				cpu := func(r *syscall.Rusage) int64 { return r.Utime.Nano() + r.Stime.Nano() }
				var wire, catalogs, items, targets int64
				for _, w := range writers {
					wire += w.bytes.Load()
					catalogs += w.catalogEvents.Load()
					items += w.itemEvents.Load()
					targets += w.targetEvents.Load()
				}
				var screenBytes, screenEvents int64
				for _, w := range screens {
					screenBytes += w.bytes.Load()
					screenEvents += w.count.Load()
				}
				b.ReportMetric(float64(screenBytes)/float64(b.N), "screen-delivery-B/op")
				b.ReportMetric(float64(screenEvents)/float64(b.N), "screen-events/op")
				b.ReportMetric(float64(screenLatency.Nanoseconds())/float64(b.N), "screen-target-ns/op")
				b.ReportMetric(float64(catalogs)/float64(b.N), "catalog-events/op")
				b.ReportMetric(float64(items)/float64(b.N), "item-events/op")
				b.ReportMetric(float64(targets)/float64(b.N), "target-events/op")
				b.ReportMetric(float64(reg.reads.Load())/float64(b.N), "reads/op")
				b.ReportMetric(float64(reg.scanned.Load())/float64(b.N), "scanned-B/op")
				b.ReportMetric(float64(reg.catalog.Load())/float64(b.N), "descriptors/op")
				b.ReportMetric(float64(wire)/float64(b.N), "delivery-B/op")
				b.ReportMetric(float64(cpu(&after)-cpu(&before))/float64(b.N), "cpu-ns/op")
				b.ReportMetric(float64(latency.Nanoseconds())/float64(b.N), "workload-ns/op")
				b.ReportMetric(float64(targetLatency.Nanoseconds())/float64(b.N), "target-context-ns/op")
			})
		}
	}
}

func invalidationRequest(ctx context.Context, protocol, method string, params map[string]any) *http.Request {
	if protocol == mcp.ProtocolVersion {
		params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": protocol, "io.modelcontextprotocol/clientInfo": map[string]string{"name": "fanout", "version": "1"}, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		panic(err)
	}
	r := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Mcp-Protocol-Version", protocol)
	if protocol == mcp.ProtocolVersion {
		r.Header.Set("Mcp-Method", method)
	}
	return r
}

type invalidationMeasuredRegistry struct {
	*workspaceRegistry

	reads   atomic.Int64
	scanned atomic.Int64
	catalog atomic.Int64
}

func (r *invalidationMeasuredRegistry) ReadResource(ctx context.Context, p *mcp.ResourcesReadParams) (mcp.ResourcesReadResult, error) {
	out, err := r.workspaceRegistry.ReadResource(ctx, p)
	r.reads.Add(1)
	for _, c := range out.Contents {
		r.scanned.Add(int64(len(c.Text) + len(c.Blob)))
	}
	return out, err
}
func (r *invalidationMeasuredRegistry) Resources(ctx context.Context) iter.Seq2[mcp.ResourceDescriptor, error] {
	return func(yield func(mcp.ResourceDescriptor, error) bool) {
		for d, err := range r.workspaceRegistry.Resources(ctx) {
			r.catalog.Add(1)
			if !yield(d, err) {
				return
			}
		}
	}
}

type invalidationMeasuredWriter struct {
	header        http.Header
	uri           string
	registry      *invalidationMeasuredRegistry
	ctx           context.Context
	ready         chan struct{}
	applied       chan invalidationApplied
	bytes         atomic.Int64
	catalogEvents atomic.Int64
	itemEvents    atomic.Int64
	targetEvents  atomic.Int64
	itemsReady    bool
	catalogReady  bool
}

func (w *invalidationMeasuredWriter) Header() http.Header { return w.header }
func (*invalidationMeasuredWriter) WriteHeader(int)       {}
func (*invalidationMeasuredWriter) Flush()                {}
func (w *invalidationMeasuredWriter) Write(p []byte) (int, error) {
	w.bytes.Add(int64(len(p)))
	if data, ok := strings.CutPrefix(string(p), "data: "); ok {
		var n struct {
			Method string `json:"method"`
			Params struct {
				URI string `json:"uri"`
			} `json:"params"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &n); err != nil {
			return 0, err
		}
		if n.Method == string(mcp.NotificationMethodResourcesUpdated) {
			if n.Params.URI == voiceItemsURI {
				w.itemEvents.Add(1)
				w.itemsReady = true
			}
			if n.Params.URI == w.uri {
				w.targetEvents.Add(1)
				_, err := w.registry.ReadResource(w.ctx, &mcp.ResourcesReadParams{URI: w.uri})
				select {
				case w.applied <- invalidationApplied{err: err, at: time.Now()}:
				case <-w.ctx.Done():
					return 0, w.ctx.Err()
				}
			}
		}
		if n.Method == string(mcp.NotificationMethodResourcesListChanged) {
			w.catalogEvents.Add(1)
			w.catalogReady = true
		}
		if w.itemsReady && w.catalogReady {
			select {
			case w.ready <- struct{}{}:
			default:
			}
		}
	}
	return len(p), nil
}

type invalidationApplied struct {
	err error
	at  time.Time
}

// invalidationScreenWriter parses real SSE handler output to time wire receipt.
// It measures transport availability, not browser rendering or visible latency.
type invalidationScreenWriter struct {
	header http.Header
	ready  chan struct{}
	events chan invalidationScreenReceipt
	bytes  atomic.Int64
	count  atomic.Int64
}

func (w *invalidationScreenWriter) Header() http.Header { return w.header }
func (*invalidationScreenWriter) WriteHeader(int)       {}
func (*invalidationScreenWriter) Flush()                {}
func (w *invalidationScreenWriter) Write(p []byte) (int, error) {
	frame := string(p)
	if strings.HasPrefix(frame, "event: server\n") {
		w.ready <- struct{}{}
	}
	if strings.Contains(frame, "\nevent: workspace\n") {
		_, data, ok := strings.Cut(frame, "\ndata: ")
		if !ok {
			return 0, errors.New("workspace frame has no payload")
		}
		var event dto.WorkspaceEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &event); err != nil {
			return 0, err
		}
		w.bytes.Add(int64(len(p)))
		w.count.Add(1)
		w.events <- invalidationScreenReceipt{node: event.NodeID, at: time.Now()}
	}
	return len(p), nil
}

type invalidationScreenReceipt struct {
	node ksid.ID
	at   time.Time
}

// BenchmarkConversationWatchBursts measures an authenticated HTTP context read
// after committed offscreen page and record writes.
// Setup registers 32 bounded watches through actual tools;
// the measured boundary is context-read completion, not model application.
func BenchmarkConversationWatchBursts(b *testing.B) {
	env := setupTestEnv(b)
	token, ws := setupGoModeWorkspace(b, env, "watch-bursts@example.com")
	endpoint := goModeMCPEndpoint + "?workspaceId=" + ws.String()
	store, err := env.services.FileStore.GetWorkspaceStore(b.Context(), ws)
	if err != nil {
		b.Fatal(err)
	}
	user, err := env.services.User.GetByEmail("watch-bursts@example.com")
	if err != nil {
		b.Fatal(err)
	}
	author := handlers.GitAuthor(user)
	page, err := store.CreatePageUnderParent(b.Context(), 0, "Offscreen", "body", author)
	if err != nil {
		b.Fatal(err)
	}
	table, err := store.CreateTableUnderParent(b.Context(), 0, "Records", nil, author)
	if err != nil {
		b.Fatal(err)
	}
	meta := map[string]any{"openai/session": "burst-conversation"}
	call := func(name string, args map[string]any) {
		callMCPNamedAt(b, env, token, endpoint, "tools/call", map[string]any{"name": name, "arguments": args, "_meta": meta}, name)
	}
	call("node_read", map[string]any{"nodeId": page.ID.String()})
	call("records_query", map[string]any{"nodeId": table.ID.String(), "limit": 1})
	for range 30 {
		node, err := store.CreatePageUnderParent(b.Context(), 0, "Other dependency", "body", author)
		if err != nil {
			b.Fatal(err)
		}
		call("node_read", map[string]any{"nodeId": node.ID.String()})
	}
	nodeH := handlers.NodeHandler{Svc: env.services}
	var before, after syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &before); err != nil {
		b.Fatal(err)
	}
	var elapsed time.Duration
	var bytes int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		started := time.Now()
		for j := 0; j < 8; j++ {
			if _, err := nodeH.UpdatePageFrontmatter(b.Context(), ws, user, &dto.UpdatePageFrontmatterRequest{ID: page.ID, Icon: fmt.Sprintf("icon-%d-%d", i, j)}); err != nil {
				b.Fatal(err)
			}
			if _, err := nodeH.CreateRecord(b.Context(), ws, user, &dto.CreateRecordRequest{ID: table.ID, Data: map[string]any{"value": j}}); err != nil {
				b.Fatal(err)
			}
		}
		raw := callMCPNamedAt(b, env, token, endpoint, "resources/read", map[string]any{"uri": voiceContextURI, "_meta": meta}, voiceContextURI)
		var response mcp.ResourcesReadResult
		if err := json.Unmarshal(raw, &response); err != nil {
			b.Fatal(err)
		}
		var projection struct {
			Resources []struct {
				State string `json:"state"`
			} `json:"resources"`
			OmittedCount int `json:"omittedCount"`
		}
		if err := json.Unmarshal([]byte(response.Contents[0].Text), &projection); err != nil {
			b.Fatal(err)
		}
		if len(projection.Resources)+projection.OmittedCount != 32 {
			b.Fatalf("watch bound: %d", len(projection.Resources))
		}
		changed := 0
		for _, r := range projection.Resources {
			if r.State == "changed" {
				changed++
			}
		}
		if changed != 2 {
			b.Fatalf("changed resources=%d", changed)
		}
		bytes += len(raw)
		elapsed += time.Since(started)
	}
	b.StopTimer()
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &after); err != nil {
		b.Fatal(err)
	}
	cpu := time.Duration(after.Utime.Nano() - before.Utime.Nano() + after.Stime.Nano() - before.Stime.Nano())
	b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N), "burst-to-context-ns/op")
	b.ReportMetric(float64(cpu.Nanoseconds())/float64(b.N), "process-CPU-ns/op")
	b.ReportMetric(float64(bytes)/float64(b.N), "HTTP-result-B/op")
	b.ReportMetric(1, "context-reads/op")
}

// Measures committed mutation fan-out through workspace registries and native/released MCP handlers.

package server

import (
	"context"
	"encoding/json"
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
	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
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
				var latency, targetLatency time.Duration
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

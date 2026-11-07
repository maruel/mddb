// Defines MCP page bounds, encodes continuation cursors, and parses node resource addresses.

package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
)

const mcpNodePageSize = 50
const mcpContentPageSize = 32 << 10
const maxMCPCursorBytes = 512

func encodeMCPCursor(c *mcpschema.Cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func parseNodeResource(uri string) (workspace, node ksid.ID, cursor string, resultErr error) {
	u, err := url.Parse(uri)
	if err != nil {
		return 0, 0, "", err
	}
	var ws, id ksid.ID
	if u.Scheme != "mddb" || u.Host != "workspaces" || u.Fragment != "" || u.User != nil {
		return 0, 0, "", errors.New("invalid node resource")
	}
	// Parse IDs independently; slashes, extra path segments and query keys fail closed.
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 3 || parts[1] != "nodes" {
		return 0, 0, "", errors.New("invalid node resource")
	}
	ws, err = ksid.Parse(parts[0])
	if err != nil || ws == 0 {
		return 0, 0, "", errors.New("invalid workspace resource")
	}
	id, err = ksid.Parse(parts[2])
	if err != nil || id == 0 {
		return 0, 0, "", errors.New("invalid node resource")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return 0, 0, "", err
	}
	if len(q) > 1 || (len(q) == 1 && len(q["cursor"]) != 1) {
		return 0, 0, "", errors.New("invalid resource query")
	}
	return ws, id, q.Get("cursor"), nil
}

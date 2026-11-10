// Projects canonical bounded MCP node snapshots, continuation cursors, and resource addresses.

package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/mcpschema"
	"github.com/maruel/mddb/backend/internal/storage/content"
)

const mcpNodePageSize = 50
const mcpContentPageSize = 32 << 10
const maxMCPCursorBytes = 512

func nodeDetailFrom(n *content.Node) mcpschema.NodeDetail {
	out := mcpschema.NodeDetail{
		ID: n.ID.String(), ParentID: n.ParentID.String(), Title: n.Title,
		Content: n.Content, Type: string(n.Type), Created: float64(n.Created) / 1000,
		Modified: float64(n.Modified) / 1000, Tags: n.Tags,
		FaviconURL: n.FaviconURL, Icon: n.Icon, Cover: n.Cover,
	}
	for i := range n.Properties {
		p := &n.Properties[i]
		prop := mcpschema.Property{Name: p.Name, Type: string(p.Type), Required: p.Required}
		for _, o := range p.Options {
			prop.Options = append(prop.Options, mcpschema.SelectOption(o))
		}
		if p.RelationConfig != nil {
			v := mcpschema.RelationConfig(*p.RelationConfig)
			prop.RelationConfig = &v
		}
		if p.RollupConfig != nil {
			v := p.RollupConfig
			prop.RollupConfig = &mcpschema.RollupConfig{RelationProperty: v.RelationProperty, TargetProperty: v.TargetProperty, Aggregation: string(v.Aggregation)}
		}
		if p.FormulaConfig != nil {
			v := mcpschema.FormulaConfig(*p.FormulaConfig)
			prop.FormulaConfig = &v
		}
		out.Properties = append(out.Properties, prop)
	}
	for i := range n.Views {
		v := &n.Views[i]
		view := mcpschema.View{ID: v.ID, Name: v.Name, Type: string(v.Type), Default: v.Default, Filters: snapshotFilters(v.Filters)}
		for _, c := range v.Columns {
			view.Columns = append(view.Columns, mcpschema.ViewColumn(c))
		}
		for _, s := range v.Sorts {
			view.Sorts = append(view.Sorts, mcpschema.Sort{Property: s.Property, Direction: string(s.Direction)})
		}
		for _, g := range v.Groups {
			view.Groups = append(view.Groups, mcpschema.Group{Property: g.Property})
		}
		out.Views = append(out.Views, view)
	}
	return out
}

func snapshotFilters(filters []content.Filter) []mcpschema.Filter {
	out := make([]mcpschema.Filter, 0, len(filters))
	for i := range filters {
		f := &filters[i]
		out = append(out, mcpschema.Filter{Property: f.Property, Operator: string(f.Operator), Value: f.Value, And: snapshotFilters(f.And), Or: snapshotFilters(f.Or)})
	}
	return out
}

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

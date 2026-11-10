// Defines bounded child-list resource identities independently of canonical page bodies.

package server

import (
	"errors"
	"net/url"
	"strings"

	"github.com/maruel/ksid"
)

type childrenResource struct {
	workspace, parent ksid.ID
	cursor            string
}

func parseChildrenResource(uri string) (childrenResource, error) {
	var out childrenResource
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mddb" || u.Host != "workspaces" || u.User != nil || u.Fragment != "" {
		return out, errors.New("invalid child-list resource")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 4 || parts[1] != "nodes" || parts[3] != "children" {
		return out, errors.New("invalid child-list resource path")
	}
	out.workspace, err = ksid.Parse(parts[0])
	if err != nil || out.workspace == 0 {
		return out, errors.New("invalid workspace")
	}
	out.parent, err = ksid.Parse(parts[2])
	if err != nil {
		return out, errors.New("invalid parent")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return out, err
	}
	for key, values := range q {
		if key != "cursor" || len(values) != 1 {
			return out, errors.New("invalid child-list parameters")
		}
	}
	out.cursor = q.Get("cursor")
	if len(out.cursor) > maxMCPCursorBytes {
		return out, errors.New("child-list cursor exceeds limit")
	}
	return out, nil
}

func (q *childrenResource) uri() string {
	base := "mddb://workspaces/" + q.workspace.String() + "/nodes/" + q.parent.String() + "/children"
	if q.cursor != "" {
		base += "?cursor=" + url.QueryEscape(q.cursor)
	}
	return base
}

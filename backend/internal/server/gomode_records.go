// Serves authorized bounded record/query resources and registers their actual table-data dependencies.

package server

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/maruel/ksid"
)

const maxMCPRecords = 50
const maxMCPRecordBytes = 64 << 10

type recordResource struct {
	workspace ksid.ID
	node      ksid.ID
	record    ksid.ID
	view      ksid.ID
	offset    int
	limit     int
}

func parseRecordResource(uri string) (recordResource, error) {
	r := recordResource{}
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mddb" || u.Host != "workspaces" || u.User != nil || u.Fragment != "" {
		return r, errors.New("invalid record resource")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if (len(parts) != 4 && len(parts) != 5) || parts[1] != "nodes" || parts[3] != "records" {
		return r, errors.New("invalid record resource path")
	}
	r.workspace, err = ksid.Parse(parts[0])
	if err != nil || r.workspace == 0 {
		return r, errors.New("invalid workspace ID")
	}
	r.node, err = ksid.Parse(parts[2])
	if err != nil || r.node == 0 {
		return r, errors.New("invalid table ID")
	}
	if len(parts) == 5 {
		r.record, err = ksid.Parse(parts[4])
		if err != nil || r.record == 0 || u.RawQuery != "" {
			return r, errors.New("invalid record ID or record query")
		}
		return r, nil
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return r, err
	}
	for key, values := range q {
		if len(values) != 1 || (key != "viewId" && key != "offset" && key != "limit") {
			return r, errors.New("invalid query resource parameters")
		}
	}
	r.limit = 20
	if raw := q.Get("viewId"); raw != "" {
		r.view, err = ksid.Parse(raw)
		if err != nil || r.view == 0 {
			return r, errors.New("invalid saved view ID")
		}
	}
	if raw := q.Get("limit"); raw != "" {
		r.limit, err = strconv.Atoi(raw)
		if err != nil || r.limit < 1 || r.limit > maxMCPRecords {
			return r, errors.New("record limit must be between 1 and 50")
		}
	}
	if raw := q.Get("offset"); raw != "" {
		r.offset, err = strconv.Atoi(raw)
		if err != nil || r.offset < 0 || r.offset > math.MaxInt-maxMCPRecords-1 {
			return r, errors.New("invalid record offset")
		}
	}
	return r, nil
}

func (r *recordResource) uri() string {
	base := fmt.Sprintf("mddb://workspaces/%s/nodes/%s/records", r.workspace, r.node)
	if r.record != 0 {
		return base + "/" + r.record.String()
	}
	q := url.Values{"limit": {strconv.Itoa(r.limit)}, "offset": {strconv.Itoa(r.offset)}}
	if r.view != 0 {
		q.Set("viewId", r.view.String())
	}
	return base + "?" + q.Encode()
}

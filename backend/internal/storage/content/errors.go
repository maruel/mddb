package content

import "errors"

var (
	errOrgIDRequired    = errors.New("organization ID is required")
	errIDRequired       = errors.New("id is required")
	errPageNotFound     = errors.New("page not found")
	errNodeNotFound     = errors.New("node not found")
	errDatabaseNotFound = errors.New("database not found")
	errRecordNotFound   = errors.New("record not found")
	errAssetNotFound    = errors.New("asset not found")
)
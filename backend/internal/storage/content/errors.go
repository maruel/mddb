package content

import "errors"

var (
	errWSIDRequired   = errors.New("workspace ID is required")
	errIDRequired     = errors.New("id is required")
	errPageNotFound   = errors.New("page not found")
	errNodeNotFound   = errors.New("node not found")
	errTableNotFound  = errors.New("table not found")
	errRecordNotFound = errors.New("record not found")
	errAssetNotFound  = errors.New("asset not found")
	errQuotaExceeded  = errors.New("quota exceeded")
)

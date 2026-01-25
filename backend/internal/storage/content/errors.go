package content

import "errors"

var (
	errWSIDRequired  = errors.New("workspace ID is required")
	errOrgIDRequired = errors.New("organization ID is required")
	errPageNotFound  = errors.New("page not found")
	errTableNotFound = errors.New("table not found")
	errAssetNotFound = errors.New("asset not found")
	errIDRequired    = errors.New("ID is required")
	errQuotaExceeded = errors.New("quota exceeded")
)

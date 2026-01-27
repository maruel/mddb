// Provides filtering and sorting logic for records.

package content

import (
	"cmp"
	"slices"
	"strings"
)

// QueryRecords applies filters and sorts from a view to records.
func QueryRecords(records []*DataRecord, view *View) []*DataRecord {
	result := make([]*DataRecord, 0, len(records))

	// Apply filters
	for _, r := range records {
		if view == nil || len(view.Filters) == 0 || matchesFilters(r, view.Filters) {
			result = append(result, r)
		}
	}

	// Apply sorts
	if view != nil && len(view.Sorts) > 0 {
		sortRecords(result, view.Sorts)
	}

	return result
}

// FilterRecords applies filters to records.
func FilterRecords(records []*DataRecord, filters []Filter) []*DataRecord {
	if len(filters) == 0 {
		return records
	}

	result := make([]*DataRecord, 0, len(records))
	for _, r := range records {
		if matchesFilters(r, filters) {
			result = append(result, r)
		}
	}
	return result
}

// SortRecords sorts records by the given sort criteria.
func SortRecords(records []*DataRecord, sorts []Sort) {
	if len(sorts) == 0 {
		return
	}
	sortRecords(records, sorts)
}

// matchesFilters checks if a record matches all filter conditions.
func matchesFilters(r *DataRecord, filters []Filter) bool {
	for i := range filters {
		if !matchesFilter(r, &filters[i]) {
			return false
		}
	}
	return true
}

// matchesFilter checks if a record matches a single filter condition.
func matchesFilter(r *DataRecord, f *Filter) bool {
	// Handle compound filters
	if len(f.And) > 0 {
		for i := range f.And {
			if !matchesFilter(r, &f.And[i]) {
				return false
			}
		}
		return true
	}

	if len(f.Or) > 0 {
		for i := range f.Or {
			if matchesFilter(r, &f.Or[i]) {
				return true
			}
		}
		return false
	}

	// Handle simple filter
	if f.Property == "" {
		return true // No property means no filtering
	}

	value, ok := r.Data[f.Property]
	if !ok {
		// Property not set - only match is_empty
		return f.Operator == FilterOpIsEmpty
	}

	return matchesOperator(value, f.Operator, f.Value)
}

// matchesOperator applies the filter operator to compare values.
func matchesOperator(value any, op FilterOp, filterValue any) bool {
	switch op {
	case FilterOpIsEmpty:
		return isEmpty(value)
	case FilterOpIsNotEmpty:
		return !isEmpty(value)
	case FilterOpEquals:
		return compareValues(value, filterValue) == 0
	case FilterOpNotEquals:
		return compareValues(value, filterValue) != 0
	case FilterOpGreaterThan:
		return compareValues(value, filterValue) > 0
	case FilterOpLessThan:
		return compareValues(value, filterValue) < 0
	case FilterOpGreaterEqual:
		return compareValues(value, filterValue) >= 0
	case FilterOpLessEqual:
		return compareValues(value, filterValue) <= 0
	case FilterOpContains:
		return containsString(value, filterValue)
	case FilterOpNotContains:
		return !containsString(value, filterValue)
	case FilterOpStartsWith:
		return startsWithString(value, filterValue)
	case FilterOpEndsWith:
		return endsWithString(value, filterValue)
	default:
		return false
	}
}

// isEmpty checks if a value is empty/null.
func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case []string:
		return len(v) == 0
	default:
		return false
	}
}

// compareValues compares two values, returning -1, 0, or 1.
func compareValues(a, b any) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try type-specific comparisons
	switch va := a.(type) {
	case string:
		if vb, ok := b.(string); ok {
			return cmp.Compare(va, vb)
		}
	case float64:
		if vb, ok := b.(float64); ok {
			return cmp.Compare(va, vb)
		}
		if vb, ok := b.(int); ok {
			return cmp.Compare(va, float64(vb))
		}
	case int:
		if vb, ok := b.(int); ok {
			return cmp.Compare(va, vb)
		}
		if vb, ok := b.(float64); ok {
			return cmp.Compare(float64(va), vb)
		}
	case bool:
		if vb, ok := b.(bool); ok {
			if va == vb {
				return 0
			}
			if !va && vb {
				return -1
			}
			return 1
		}
	}

	// Fallback: compare string representations
	return cmp.Compare(toString(a), toString(b))
}

// containsString checks if value contains the filter string (case-insensitive).
func containsString(value, filterValue any) bool {
	vs := strings.ToLower(toString(value))
	fs := strings.ToLower(toString(filterValue))
	return strings.Contains(vs, fs)
}

// startsWithString checks if value starts with the filter string (case-insensitive).
func startsWithString(value, filterValue any) bool {
	vs := strings.ToLower(toString(value))
	fs := strings.ToLower(toString(filterValue))
	return strings.HasPrefix(vs, fs)
}

// endsWithString checks if value ends with the filter string (case-insensitive).
func endsWithString(value, filterValue any) bool {
	vs := strings.ToLower(toString(value))
	fs := strings.ToLower(toString(filterValue))
	return strings.HasSuffix(vs, fs)
}

// toString converts a value to its string representation.
func toString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		// Use default formatting for other types
		return ""
	}
}

// sortRecords sorts records in place by the given sort criteria.
func sortRecords(records []*DataRecord, sorts []Sort) {
	slices.SortStableFunc(records, func(a, b *DataRecord) int {
		for i := range sorts {
			sort := &sorts[i]
			va := a.Data[sort.Property]
			vb := b.Data[sort.Property]
			c := compareValues(va, vb)
			if c != 0 {
				if sort.Direction == SortDesc {
					return -c
				}
				return c
			}
		}
		return 0
	})
}

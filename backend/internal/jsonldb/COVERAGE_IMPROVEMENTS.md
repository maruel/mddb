# jsonldb Package Test Coverage Improvements

## Summary

Test coverage was improved from **59.0%** to **88.9%** through comprehensive table-driven tests organized by exported types.

## Test Structure

Tests are organized following the requested pattern:
- One root `Test` function per exported type (`TestID`, `TestTable`, `TestRow`)
- Subtests for each method using `t.Run("method_name", ...)`
- `t.Run("valid")` for success cases
- `t.Run("errors")` for error cases
- Table-driven tests where applicable

### Test Files

- `id_test.go` - Tests for the `ID` type
- `table_test.go` - Tests for the `Table` type and `Row` interface
- `database_test.go` - Tests for schema-related types and functions

## Coverage by Function

| Function | Before | After | Notes |
|----------|--------|-------|-------|
| `ID.String` | 90.9% | 90.9% | Final `return "-"` unreachable |
| `ID.MarshalJSON` | 0% | 100% | ✓ |
| `ID.UnmarshalJSON` | 0% | 100% | ✓ |
| `ID.IsZero` | 100% | 100% | ✓ |
| `ID.RandomBits` | 0% | 100% | ✓ |
| `DecodeID` | 93.8% | 100% | ✓ |
| `Table.Last` | 0% | 100% | ✓ |
| `Table.Get` | 0% | 100% | ✓ |
| `Table.Delete` | 0% | 91.7% | Save error path requires FS mocking |
| `Table.Update` | 0% | 92.3% | Save error path requires FS mocking |
| `Table.load` | 80% | 100% | ✓ |
| `Table.Append` | 72% | 84% | FS error paths require mocking |
| `schemaHeader.Validate` | 37.5% | 100% | ✓ |
| `jsonFieldName` | 0% | 100% | ✓ |
| `goTypeToColumnType` | 0% | 92.9% | Final return unreachable |

## Issues Found (Recommended for Follow-up)

### 1. `jsonFieldName` Returns Empty String for `json:",omitempty"`

**Location:** `database.go:127`

**Issue:** When a struct field has the tag `json:",omitempty"` (name omitted), `jsonFieldName` returns an empty string instead of the Go field name.

**Current Behavior:**
```go
type Example struct {
    Field string `json:",omitempty"`
}
// jsonFieldName returns "" instead of "Field"
```

**Expected Behavior:** Should return the Go field name when no JSON name is specified before the comma.

**Suggested Fix:**
```go
func jsonFieldName(field *reflect.StructField) string {
    tag := field.Tag.Get("json")
    if tag == "" || tag == "-" {
        return field.Name
    }
    // Handle "name,omitempty" format
    for i, c := range tag {
        if c == ',' {
            if i == 0 {
                return field.Name // Empty name, use Go field name
            }
            return tag[:i]
        }
    }
    return tag
}
```

### 2. Unreachable Code in `goTypeToColumnType`

**Location:** `database.go:177`

**Issue:** The final `return columnTypeText` is unreachable because the switch statement exhaustively handles all `reflect.Kind` values.

**Recommendation:** Consider adding a comment explaining this is a safety fallback, or use a `//nolint:govet` directive to suppress static analysis warnings.

### 3. Unreachable Code in `ID.String`

**Location:** `id.go:118`

**Issue:** The final `return "-"` after the loop is unreachable because any non-zero ID will have at least one non-'-' character in its encoding.

**Recommendation:** Same as above - add a comment or suppress warnings.

### 4. Error Paths in Save Functions Untested

**Location:** `table.go:310-377`

**Issue:** The `save()` and `saveSchemaHeaderLocked()` functions have error paths for:
- `os.Create` failure
- `json.Marshal` failure
- `bufio.Writer.Write` failure
- `bufio.Writer.WriteByte` failure
- `bufio.Writer.Flush` failure

**Recommendation:** These would require either:
- Filesystem mocking (consider interface injection for the file operations)
- Using a test filesystem abstraction like `afero`

### 5. `schemaFromType` Depends on jsonschema Library Behavior

**Location:** `database.go:68`

**Issue:** The `schemaFromType` function relies on `github.com/invopop/jsonschema` to extract schema information. This library:
- Returns empty properties for test-defined struct types
- Panics on `complex128` fields
- May not handle all Go type scenarios predictably

**Recommendation:** Consider:
- Adding integration tests with real application types
- Documenting supported and unsupported field types
- Adding defensive checks for unsupported types

### 6. ID Encoding/Decoding Not Intuitive

**Location:** `id.go`

**Observation:** The sortable alphabet encoding means:
- ID value `1` encodes to string `"0"` (not `"1"`)
- String `"1"` decodes to ID value `2` (not `1`)

This is correct by design (using a custom base-64 alphabet), but could be confusing. The documentation is accurate, but consider adding examples in the godoc.

## Uncovered Code Summary

The remaining ~11% uncovered code consists of:

1. **Unreachable safety returns** (2 instances)
   - `goTypeToColumnType` final return
   - `ID.String` final return

2. **Filesystem error paths** (~8%)
   - `save()` - 33.3% uncovered
   - `saveSchemaHeaderLocked()` - 31.2% uncovered
   - `Append` write errors - 16% uncovered
   - `Delete`/`Update` save errors - ~8% uncovered

3. **Type constraint error paths** (~1%)
   - `NewTable` schemaFromType error (would fail at compile time)

These are acceptable gaps for production code. To achieve 100% coverage would require:
- Removing unreachable code (changes functionality)
- Injecting filesystem interfaces (significant refactoring)

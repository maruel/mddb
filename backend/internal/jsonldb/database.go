package jsonldb

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// CurrentVersion is the current version of the JSONL database format.
const CurrentVersion = "1.0"

// Column represents a database column in storage.
type Column struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// DataRecord represents a database record in storage.
type DataRecord struct {
	ID       string         `json:"id"`
	Data     map[string]any `json:"data"`
	Created  time.Time      `json:"created"`
	Modified time.Time      `json:"modified"`
}

// SchemaHeader represents the first row of a JSONL data file containing schema and metadata.
type SchemaHeader struct {
	Version  string    `json:"version"`
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Columns  []Column  `json:"columns"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
}

// Database handles JSONL-based database storage with schema header in the first row.
type Database struct {
	path string
	Mu   sync.RWMutex

	Header  *SchemaHeader
	Records []DataRecord
}

// NewDatabase creates or loads a database from a JSONL file.
// If the file doesn't exist, it creates a new database with the given schema.
func NewDatabase(path, id, title string, columns []Column) (*Database, error) {
	db := &Database{
		path:    path,
		Records: []DataRecord{},
	}

	// Try to load existing database
	if err := db.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load database: %w", err)
	}

	// If new database or schema not yet initialized
	if db.Header == nil {
		now := time.Now()
		db.Header = &SchemaHeader{
			Version:  CurrentVersion,
			ID:       id,
			Title:    title,
			Columns:  columns,
			Created:  now,
			Modified: now,
		}
		if err := db.persist(); err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	return db, nil
}

// load reads the JSONL file, extracting the header and records.
func (db *Database) load() error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	f, err := os.Open(db.path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	var records []DataRecord
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// First line is the schema header
		if lineNum == 1 {
			var header SchemaHeader
			if err := json.Unmarshal(line, &header); err != nil {
				return fmt.Errorf("failed to parse schema header: %w", err)
			}
			db.Header = &header
			continue
		}

		// Subsequent lines are records
		var record DataRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return fmt.Errorf("failed to parse record at line %d: %w", lineNum, err)
		}
		records = append(records, record)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read database file: %w", err)
	}

	db.Records = records
	return nil
}

// persist writes the header and all records to the JSONL file.
func (db *Database) persist() error {
	f, err := os.Create(db.path)
	if err != nil {
		return fmt.Errorf("failed to create database file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	writer := bufio.NewWriter(f)

	// Write schema header
	if db.Header != nil {
		headerData, err := json.Marshal(db.Header)
		if err != nil {
			return fmt.Errorf("failed to marshal header: %w", err)
		}
		if _, err := writer.Write(headerData); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	// Write records
	for _, record := range db.Records {
		data, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("failed to marshal record: %w", err)
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
		if err := writer.WriteByte('\n'); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	return nil
}

// GetRecords returns a copy of all records.
func (db *Database) GetRecords() []DataRecord {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	records := make([]DataRecord, len(db.Records))
	copy(records, db.Records)
	return records
}

// GetRecord retrieves a specific record by ID.
func (db *Database) GetRecord(id string) *DataRecord {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	for i := range db.Records {
		if db.Records[i].ID == id {
			return &db.Records[i]
		}
	}
	return nil
}

// AppendRecord adds a new record and persists.
func (db *Database) AppendRecord(record DataRecord) error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	db.Records = append(db.Records, record)
	return db.persist()
}

// UpdateRecord updates an existing record and persists.
func (db *Database) UpdateRecord(record DataRecord) error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	for i := range db.Records {
		if db.Records[i].ID == record.ID {
			db.Records[i] = record
			return db.persist()
		}
	}
	return fmt.Errorf("record not found")
}

// DeleteRecord removes a record and persists.
func (db *Database) DeleteRecord(id string) error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	for i := range db.Records {
		if db.Records[i].ID == id {
			db.Records = append(db.Records[:i], db.Records[i+1:]...)
			return db.persist()
		}
	}
	return fmt.Errorf("record not found")
}

// GetRecordsPage returns a paginated slice of records.
func (db *Database) GetRecordsPage(offset, limit int) []DataRecord {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	if offset < 0 {
		offset = 0
	}
	if offset >= len(db.Records) {
		return []DataRecord{}
	}

	end := offset + limit
	if end > len(db.Records) {
		end = len(db.Records)
	}

	records := make([]DataRecord, end-offset)
	copy(records, db.Records[offset:end])
	return records
}

// UpdateSchema updates the database title and columns.
func (db *Database) UpdateSchema(title string, columns []Column) error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	if db.Header == nil {
		return fmt.Errorf("database header not initialized")
	}

	db.Header.Title = title
	db.Header.Columns = columns
	db.Header.Modified = time.Now()

	return db.persist()
}

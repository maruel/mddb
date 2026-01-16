// Package models defines the core data structures used throughout the application.
package models

import "time"

// Database

// Database represents a database with schema
type Database struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Columns  []Column  `json:"columns"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
	Path     string    `json:"path"`
}

// Column represents a database column
type Column struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     string   `json:"type"` // text, number, select, multi_select, checkbox, date
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

// Record represents a database record
type Record struct {
	ID       string                 `json:"id"`
	Data     map[string]interface{} `json:"data"`
	Created  time.Time              `json:"created"`
	Modified time.Time              `json:"modified"`
}

// Assets

// Asset represents an uploaded file/image
type Asset struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	MimeType string    `json:"mime_type"`
	Size     int64     `json:"size"`
	Created  time.Time `json:"created"`
	Path     string    `json:"path"` // Relative path in assets directory
}

// Pages

// Page represents a markdown document
type Page struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Created    time.Time `json:"created"`
	Modified   time.Time `json:"modified"`
	Tags       []string  `json:"tags,omitempty"`
	Path       string    `json:"path"`                  // Relative path from pages directory
	FaviconURL string    `json:"favicon_url,omitempty"` // URL to favicon in page directory
}

// PageMetadata contains the YAML front matter of a page
type PageMetadata struct {
	ID       string    `yaml:"id"`
	Title    string    `yaml:"title"`
	Created  time.Time `yaml:"created"`
	Modified time.Time `yaml:"modified"`
	Tags     []string  `yaml:"tags"`
}

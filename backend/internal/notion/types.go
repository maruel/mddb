// Defines Notion API response types.

package notion

import (
	"encoding/json"
	"time"
)

// API response wrapper types.

// PaginatedResponse is the common structure for paginated API responses.
type PaginatedResponse[T any] struct {
	Object     string  `json:"object"`
	Results    []T     `json:"results"`
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

// SearchResponse is the response from the search endpoint.
type SearchResponse = PaginatedResponse[SearchResult]

// QueryResponse is the response from database query endpoint.
type QueryResponse = PaginatedResponse[Page]

// BlocksResponse is the response from block children endpoint.
type BlocksResponse = PaginatedResponse[Block]

// SearchResult represents an item in search results.
// Note: Notion API returns different structures for pages vs databases.
// Use the Object field to determine which fields are populated.
type SearchResult struct {
	Object         string    `json:"object"` // "page" or "database"
	ID             string    `json:"id"`
	CreatedTime    time.Time `json:"created_time"`
	LastEditedTime time.Time `json:"last_edited_time"`
	Parent         Parent    `json:"parent"`

	// For pages: properties contains PropertyValue
	// For databases: properties contains DBProperty (schema definitions)
	// The JSON structure differs, so we use json.RawMessage and parse based on Object.
	PropertiesRaw json.RawMessage `json:"properties,omitempty"`

	// For databases only
	Title       []RichText `json:"title,omitempty"`
	Description []RichText `json:"description,omitempty"`
}

// Parent represents the parent of a page or database.
type Parent struct {
	Type       string `json:"type"` // "database_id", "page_id", "workspace", "block_id"
	DatabaseID string `json:"database_id,omitempty"`
	PageID     string `json:"page_id,omitempty"`
	BlockID    string `json:"block_id,omitempty"`
	Workspace  bool   `json:"workspace,omitempty"`
}

// Database represents a Notion database.
type Database struct {
	Object         string                `json:"object"`
	ID             string                `json:"id"`
	CreatedTime    time.Time             `json:"created_time"`
	LastEditedTime time.Time             `json:"last_edited_time"`
	Title          []RichText            `json:"title"`
	Description    []RichText            `json:"description"`
	Properties     map[string]DBProperty `json:"properties"`
	Parent         Parent                `json:"parent"`
	URL            string                `json:"url"`
	Archived       bool                  `json:"archived"`
	IsInline       bool                  `json:"is_inline"`
}

// DBProperty represents a property definition in a database schema.
type DBProperty struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`

	// Type-specific configuration
	Title          *struct{}       `json:"title,omitempty"`
	RichText       *struct{}       `json:"rich_text,omitempty"`
	Number         *NumberConfig   `json:"number,omitempty"`
	Select         *SelectConfig   `json:"select,omitempty"`
	MultiSelect    *SelectConfig   `json:"multi_select,omitempty"`
	Date           *struct{}       `json:"date,omitempty"`
	Checkbox       *struct{}       `json:"checkbox,omitempty"`
	URL            *struct{}       `json:"url,omitempty"`
	Email          *struct{}       `json:"email,omitempty"`
	PhoneNumber    *struct{}       `json:"phone_number,omitempty"`
	Formula        *FormulaConfig  `json:"formula,omitempty"`
	Relation       *RelationConfig `json:"relation,omitempty"`
	Rollup         *RollupConfig   `json:"rollup,omitempty"`
	People         *struct{}       `json:"people,omitempty"`
	Files          *struct{}       `json:"files,omitempty"`
	CreatedTime    *struct{}       `json:"created_time,omitempty"`
	CreatedBy      *struct{}       `json:"created_by,omitempty"`
	LastEditedTime *struct{}       `json:"last_edited_time,omitempty"`
	LastEditedBy   *struct{}       `json:"last_edited_by,omitempty"`
	Status         *StatusConfig   `json:"status,omitempty"`
	UniqueID       *UniqueIDConfig `json:"unique_id,omitempty"`
}

// NumberConfig defines number property configuration.
type NumberConfig struct {
	Format string `json:"format"` // number, number_with_commas, percent, dollar, etc.
}

// SelectConfig defines select/multi_select property configuration.
type SelectConfig struct {
	Options []SelectOption `json:"options"`
}

// SelectOption represents a select option.
type SelectOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// StatusConfig defines status property configuration.
type StatusConfig struct {
	Options []StatusOption `json:"options"`
	Groups  []StatusGroup  `json:"groups"`
}

// StatusOption represents a status option.
type StatusOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// StatusGroup represents a group of status options.
type StatusGroup struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Color     string   `json:"color"`
	OptionIDs []string `json:"option_ids"`
}

// FormulaConfig defines formula property configuration.
type FormulaConfig struct {
	Expression string `json:"expression"`
}

// RelationConfig defines relation property configuration.
type RelationConfig struct {
	DatabaseID     string              `json:"database_id"`
	Type           string              `json:"type"` // "single_property" or "dual_property"
	SingleProperty *struct{}           `json:"single_property,omitempty"`
	DualProperty   *DualPropertyConfig `json:"dual_property,omitempty"`
}

// DualPropertyConfig defines dual property relation configuration.
type DualPropertyConfig struct {
	SyncedPropertyName string `json:"synced_property_name"`
	SyncedPropertyID   string `json:"synced_property_id"`
}

// RollupConfig defines rollup property configuration.
type RollupConfig struct {
	RelationPropertyName string `json:"relation_property_name"`
	RelationPropertyID   string `json:"relation_property_id"`
	RollupPropertyName   string `json:"rollup_property_name"`
	RollupPropertyID     string `json:"rollup_property_id"`
	Function             string `json:"function"` // count, count_values, sum, average, etc.
}

// UniqueIDConfig defines unique_id property configuration.
type UniqueIDConfig struct {
	Prefix string `json:"prefix,omitempty"`
}

// Page represents a Notion page (including database rows).
type Page struct {
	Object         string                   `json:"object"`
	ID             string                   `json:"id"`
	CreatedTime    time.Time                `json:"created_time"`
	LastEditedTime time.Time                `json:"last_edited_time"`
	Parent         Parent                   `json:"parent"`
	Archived       bool                     `json:"archived"`
	Properties     map[string]PropertyValue `json:"properties"`
	URL            string                   `json:"url"`
	Icon           *Icon                    `json:"icon,omitempty"`
	Cover          *File                    `json:"cover,omitempty"`
}

// Icon represents a page or database icon.
type Icon struct {
	Type     string `json:"type"` // "emoji", "external", "file"
	Emoji    string `json:"emoji,omitempty"`
	External *File  `json:"external,omitempty"`
	File     *File  `json:"file,omitempty"`
}

// PropertyValue represents a property value on a page.
type PropertyValue struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	// Value fields based on type
	Title          []RichText      `json:"title,omitempty"`
	RichText       []RichText      `json:"rich_text,omitempty"`
	Number         *float64        `json:"number,omitempty"`
	Select         *SelectValue    `json:"select,omitempty"`
	MultiSelect    []SelectValue   `json:"multi_select,omitempty"`
	Date           *DateValue      `json:"date,omitempty"`
	Checkbox       *bool           `json:"checkbox,omitempty"`
	URL            *string         `json:"url,omitempty"`
	Email          *string         `json:"email,omitempty"`
	PhoneNumber    *string         `json:"phone_number,omitempty"`
	Formula        *FormulaValue   `json:"formula,omitempty"`
	Relation       []RelationValue `json:"relation,omitempty"`
	Rollup         *RollupValue    `json:"rollup,omitempty"`
	People         []Person        `json:"people,omitempty"`
	Files          []FileValue     `json:"files,omitempty"`
	CreatedTime    *time.Time      `json:"created_time,omitempty"`
	CreatedBy      *Person         `json:"created_by,omitempty"`
	LastEditedTime *time.Time      `json:"last_edited_time,omitempty"`
	LastEditedBy   *Person         `json:"last_edited_by,omitempty"`
	Status         *StatusValue    `json:"status,omitempty"`
	UniqueID       *UniqueIDValue  `json:"unique_id,omitempty"`
}

// RichText represents formatted text content.
type RichText struct {
	Type        string       `json:"type"` // "text", "mention", "equation"
	Text        *TextContent `json:"text,omitempty"`
	Mention     *Mention     `json:"mention,omitempty"`
	Equation    *Equation    `json:"equation,omitempty"`
	Annotations *Annotations `json:"annotations,omitempty"`
	PlainText   string       `json:"plain_text"`
	Href        *string      `json:"href,omitempty"`
}

// TextContent represents plain text content.
type TextContent struct {
	Content string `json:"content"`
	Link    *Link  `json:"link,omitempty"`
}

// Link represents a hyperlink.
type Link struct {
	URL string `json:"url"`
}

// Mention represents a mention in rich text.
type Mention struct {
	Type        string       `json:"type"` // "user", "page", "database", "date", "link_preview"
	User        *Person      `json:"user,omitempty"`
	Page        *PageRef     `json:"page,omitempty"`
	Database    *DatabaseRef `json:"database,omitempty"`
	Date        *DateValue   `json:"date,omitempty"`
	LinkPreview *LinkPreview `json:"link_preview,omitempty"`
}

// PageRef is a reference to a page.
type PageRef struct {
	ID string `json:"id"`
}

// DatabaseRef is a reference to a database.
type DatabaseRef struct {
	ID string `json:"id"`
}

// LinkPreview represents a link preview mention.
type LinkPreview struct {
	URL string `json:"url"`
}

// Equation represents a LaTeX equation.
type Equation struct {
	Expression string `json:"expression"`
}

// Annotations represents text formatting.
type Annotations struct {
	Bold          bool   `json:"bold"`
	Italic        bool   `json:"italic"`
	Strikethrough bool   `json:"strikethrough"`
	Underline     bool   `json:"underline"`
	Code          bool   `json:"code"`
	Color         string `json:"color"`
}

// SelectValue represents a select property value.
type SelectValue struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// StatusValue represents a status property value.
type StatusValue struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// DateValue represents a date property value.
type DateValue struct {
	Start    string  `json:"start"`
	End      *string `json:"end,omitempty"`
	TimeZone *string `json:"time_zone,omitempty"`
}

// FormulaValue represents a formula result.
type FormulaValue struct {
	Type    string     `json:"type"` // "string", "number", "boolean", "date"
	String  *string    `json:"string,omitempty"`
	Number  *float64   `json:"number,omitempty"`
	Boolean *bool      `json:"boolean,omitempty"`
	Date    *DateValue `json:"date,omitempty"`
}

// RelationValue represents a relation to another page.
type RelationValue struct {
	ID string `json:"id"`
}

// RollupValue represents a rollup result.
type RollupValue struct {
	Type     string          `json:"type"` // "number", "date", "array", "unsupported", "incomplete"
	Number   *float64        `json:"number,omitempty"`
	Date     *DateValue      `json:"date,omitempty"`
	Array    []PropertyValue `json:"array,omitempty"`
	Function string          `json:"function"`
}

// Person represents a Notion user.
type Person struct {
	Object    string         `json:"object"`
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	AvatarURL *string        `json:"avatar_url,omitempty"`
	Type      string         `json:"type,omitempty"` // "person" or "bot"
	Person    *PersonDetails `json:"person,omitempty"`
}

// PersonDetails contains person-specific details.
type PersonDetails struct {
	Email string `json:"email"`
}

// FileValue represents a file property value.
type FileValue struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "file" or "external"
	File     *File  `json:"file,omitempty"`
	External *File  `json:"external,omitempty"`
}

// File represents a file reference.
type File struct {
	URL        string     `json:"url"`
	ExpiryTime *time.Time `json:"expiry_time,omitempty"`
}

// UniqueIDValue represents a unique_id property value.
type UniqueIDValue struct {
	Prefix *string `json:"prefix,omitempty"`
	Number int     `json:"number"`
}

// Block represents a Notion block.
type Block struct {
	Object         string    `json:"object"`
	ID             string    `json:"id"`
	Parent         Parent    `json:"parent"`
	Type           string    `json:"type"`
	CreatedTime    time.Time `json:"created_time"`
	LastEditedTime time.Time `json:"last_edited_time"`
	Archived       bool      `json:"archived"`
	HasChildren    bool      `json:"has_children"`

	// Block type content - only the matching type field will be populated
	Paragraph        *ParagraphBlock     `json:"paragraph,omitempty"`
	Heading1         *HeadingBlock       `json:"heading_1,omitempty"`
	Heading2         *HeadingBlock       `json:"heading_2,omitempty"`
	Heading3         *HeadingBlock       `json:"heading_3,omitempty"`
	BulletedListItem *ListItemBlock      `json:"bulleted_list_item,omitempty"`
	NumberedListItem *ListItemBlock      `json:"numbered_list_item,omitempty"`
	ToDo             *ToDoBlock          `json:"to_do,omitempty"`
	Toggle           *ToggleBlock        `json:"toggle,omitempty"`
	Code             *CodeBlock          `json:"code,omitempty"`
	Quote            *QuoteBlock         `json:"quote,omitempty"`
	Callout          *CalloutBlock       `json:"callout,omitempty"`
	Divider          *struct{}           `json:"divider,omitempty"`
	TableOfContents  *struct{}           `json:"table_of_contents,omitempty"`
	Breadcrumb       *struct{}           `json:"breadcrumb,omitempty"`
	ColumnList       *struct{}           `json:"column_list,omitempty"`
	Column           *struct{}           `json:"column,omitempty"`
	Image            *MediaBlock         `json:"image,omitempty"`
	Video            *MediaBlock         `json:"video,omitempty"`
	File             *MediaBlock         `json:"file,omitempty"`
	PDF              *MediaBlock         `json:"pdf,omitempty"`
	Bookmark         *BookmarkBlock      `json:"bookmark,omitempty"`
	Embed            *EmbedBlock         `json:"embed,omitempty"`
	LinkPreview      *LinkPreviewBlock   `json:"link_preview,omitempty"`
	Equation         *EquationBlock      `json:"equation,omitempty"`
	SyncedBlock      *SyncedBlockContent `json:"synced_block,omitempty"`
	Table            *TableBlock         `json:"table,omitempty"`
	TableRow         *TableRowBlock      `json:"table_row,omitempty"`
	ChildPage        *ChildPageBlock     `json:"child_page,omitempty"`
	ChildDatabase    *ChildDatabaseBlock `json:"child_database,omitempty"`
}

// ParagraphBlock represents a paragraph block.
type ParagraphBlock struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

// HeadingBlock represents a heading block.
type HeadingBlock struct {
	RichText     []RichText `json:"rich_text"`
	Color        string     `json:"color"`
	IsToggleable bool       `json:"is_toggleable"`
}

// ListItemBlock represents a list item block.
type ListItemBlock struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

// ToDoBlock represents a to-do block.
type ToDoBlock struct {
	RichText []RichText `json:"rich_text"`
	Checked  bool       `json:"checked"`
	Color    string     `json:"color"`
}

// ToggleBlock represents a toggle block.
type ToggleBlock struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

// CodeBlock represents a code block.
type CodeBlock struct {
	RichText []RichText `json:"rich_text"`
	Caption  []RichText `json:"caption"`
	Language string     `json:"language"`
}

// QuoteBlock represents a quote block.
type QuoteBlock struct {
	RichText []RichText `json:"rich_text"`
	Color    string     `json:"color"`
}

// CalloutBlock represents a callout block.
type CalloutBlock struct {
	RichText []RichText `json:"rich_text"`
	Icon     *Icon      `json:"icon,omitempty"`
	Color    string     `json:"color"`
}

// MediaBlock represents an image, video, file, or PDF block.
type MediaBlock struct {
	Type     string     `json:"type"` // "file" or "external"
	File     *File      `json:"file,omitempty"`
	External *File      `json:"external,omitempty"`
	Caption  []RichText `json:"caption,omitempty"`
}

// BookmarkBlock represents a bookmark block.
type BookmarkBlock struct {
	URL     string     `json:"url"`
	Caption []RichText `json:"caption"`
}

// EmbedBlock represents an embed block.
type EmbedBlock struct {
	URL string `json:"url"`
}

// LinkPreviewBlock represents a link preview block.
type LinkPreviewBlock struct {
	URL string `json:"url"`
}

// EquationBlock represents an equation block.
type EquationBlock struct {
	Expression string `json:"expression"`
}

// SyncedBlockContent represents synced block content.
type SyncedBlockContent struct {
	SyncedFrom *SyncedFrom `json:"synced_from,omitempty"`
}

// SyncedFrom indicates where a synced block is synced from.
type SyncedFrom struct {
	BlockID string `json:"block_id"`
}

// TableBlock represents a table block.
type TableBlock struct {
	TableWidth      int  `json:"table_width"`
	HasColumnHeader bool `json:"has_column_header"`
	HasRowHeader    bool `json:"has_row_header"`
}

// TableRowBlock represents a table row block.
type TableRowBlock struct {
	Cells [][]RichText `json:"cells"`
}

// ChildPageBlock represents a child page block.
type ChildPageBlock struct {
	Title string `json:"title"`
}

// ChildDatabaseBlock represents a child database block.
type ChildDatabaseBlock struct {
	Title string `json:"title"`
}

// Error represents a Notion API error response.
type Error struct {
	Object  string `json:"object"`
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

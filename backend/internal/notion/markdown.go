// Converts Notion blocks to Markdown.

package notion

import (
	"fmt"
	"strings"

	"github.com/maruel/ksid"
)

// MarkdownConverter converts Notion blocks to markdown with optional asset downloading.
type MarkdownConverter struct {
	assets *AssetDownloader
	nodeID ksid.ID
	mapper *Mapper // For resolving child page/database links
}

// NewMarkdownConverter creates a converter, optionally with asset downloading.
func NewMarkdownConverter(assets *AssetDownloader, nodeID ksid.ID) *MarkdownConverter {
	return &MarkdownConverter{
		assets: assets,
		nodeID: nodeID,
	}
}

// NewMarkdownConverterWithLinks creates a converter with asset downloading and link resolution.
func NewMarkdownConverterWithLinks(assets *AssetDownloader, nodeID ksid.ID, mapper *Mapper) *MarkdownConverter {
	return &MarkdownConverter{
		assets: assets,
		nodeID: nodeID,
		mapper: mapper,
	}
}

// Convert converts blocks to markdown, downloading assets if configured.
func (c *MarkdownConverter) Convert(blocks []Block) string {
	var sb strings.Builder
	c.blocksToMarkdownRecursive(blocks, &sb, 0)
	return sb.String()
}

// BlocksToMarkdown converts a slice of Notion blocks to markdown (without asset downloading).
func BlocksToMarkdown(blocks []Block) string {
	c := &MarkdownConverter{}
	return c.Convert(blocks)
}

// resolveChildLink resolves a child page/database Notion ID to an mddb node link.
func (c *MarkdownConverter) resolveChildLink(notionID string) string {
	if c.mapper == nil {
		return ""
	}
	if mddbID, ok := c.mapper.NotionToMddb[notionID]; ok {
		return mddbID.String()
	}
	return ""
}

// resolveMediaURL gets the URL for a media block, downloading if asset downloader is configured.
func (c *MarkdownConverter) resolveMediaURL(media *MediaBlock) string {
	if media == nil {
		return ""
	}

	var url string
	if media.File != nil {
		url = media.File.URL
	} else if media.External != nil {
		url = media.External.URL
	}

	// If we have an asset downloader, try to download and return local path
	if c.assets != nil && !c.nodeID.IsZero() {
		if localPath, err := c.assets.DownloadAsset(c.nodeID, url); err == nil && localPath != "" {
			return localPath
		}
		// On error, fall back to original URL
	}

	return url
}

// blocksToMarkdownRecursive converts blocks and their children to markdown.
func (c *MarkdownConverter) blocksToMarkdownRecursive(blocks []Block, sb *strings.Builder, depth int) {
	listState := &listState{}
	for i := range blocks {
		md := c.blockToMarkdown(&blocks[i], listState, depth)
		if md != "" {
			sb.WriteString(md)
		}
		// Recurse into children
		if len(blocks[i].Children) > 0 {
			// Special handling for tables
			if blocks[i].Type == "table" {
				renderTableChildren(blocks[i].Children, sb, blocks[i].Table)
			} else {
				c.blocksToMarkdownRecursive(blocks[i].Children, sb, depth+1)
				// Close toggle if needed
				if blocks[i].Type == "toggle" {
					sb.WriteString(strings.Repeat("  ", depth) + "</details>\n\n")
				}
			}
		}
	}
}

// renderTableChildren renders table rows with proper markdown table formatting.
func renderTableChildren(rows []Block, sb *strings.Builder, tableInfo *TableBlock) {
	for i := range rows {
		row := &rows[i]
		if row.Type == "table_row" && row.TableRow != nil {
			numCells := len(row.TableRow.Cells)
			cells := make([]string, 0, numCells)
			for _, cell := range row.TableRow.Cells {
				cells = append(cells, richTextToMarkdown(cell))
			}
			sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")

			// Add separator after header row
			if i == 0 && tableInfo != nil && tableInfo.HasColumnHeader {
				seps := make([]string, numCells)
				for j := range seps {
					seps[j] = "---"
				}
				sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")
			}
		}
	}
	sb.WriteString("\n")
}

// listState tracks list context for proper markdown formatting.
type listState struct {
	numberedCount int
	inBulleted    bool
	inNumbered    bool
}

// blockToMarkdown converts a single block to markdown.
func (c *MarkdownConverter) blockToMarkdown(block *Block, ls *listState, depth int) string {
	indent := strings.Repeat("  ", depth)

	// Reset list state for non-list blocks
	if block.Type != "bulleted_list_item" {
		ls.inBulleted = false
	}
	if block.Type != "numbered_list_item" {
		ls.inNumbered = false
		ls.numberedCount = 0
	}

	switch block.Type {
	case "paragraph":
		if block.Paragraph != nil {
			text := richTextToMarkdown(block.Paragraph.RichText)
			if text == "" {
				return "\n"
			}
			return indent + text + "\n\n"
		}

	case "heading_1":
		if block.Heading1 != nil {
			return "# " + richTextToMarkdown(block.Heading1.RichText) + "\n\n"
		}

	case "heading_2":
		if block.Heading2 != nil {
			return "## " + richTextToMarkdown(block.Heading2.RichText) + "\n\n"
		}

	case "heading_3":
		if block.Heading3 != nil {
			return "### " + richTextToMarkdown(block.Heading3.RichText) + "\n\n"
		}

	case "bulleted_list_item":
		if block.BulletedListItem != nil {
			prefix := ""
			if !ls.inBulleted {
				ls.inBulleted = true
				prefix = "\n"
			}
			return prefix + indent + "- " + richTextToMarkdown(block.BulletedListItem.RichText) + "\n"
		}

	case "numbered_list_item":
		if block.NumberedListItem != nil {
			prefix := ""
			if !ls.inNumbered {
				ls.inNumbered = true
				ls.numberedCount = 0
				prefix = "\n"
			}
			ls.numberedCount++
			return fmt.Sprintf("%s%s%d. %s\n", prefix, indent, ls.numberedCount, richTextToMarkdown(block.NumberedListItem.RichText))
		}

	case "to_do":
		if block.ToDo != nil {
			checkbox := "[ ]"
			if block.ToDo.Checked {
				checkbox = "[x]"
			}
			return indent + "- " + checkbox + " " + richTextToMarkdown(block.ToDo.RichText) + "\n"
		}

	case "toggle":
		if block.Toggle != nil {
			return indent + "<details>\n" + indent + "<summary>" + richTextToMarkdown(block.Toggle.RichText) + "</summary>\n\n"
		}

	case "code":
		if block.Code != nil {
			lang := block.Code.Language
			if lang == "plain text" {
				lang = ""
			}
			return "```" + lang + "\n" + richTextToPlain(block.Code.RichText) + "\n```\n\n"
		}

	case "quote":
		if block.Quote != nil {
			lines := strings.Split(richTextToMarkdown(block.Quote.RichText), "\n")
			var quoted []string
			for _, line := range lines {
				quoted = append(quoted, "> "+line)
			}
			return strings.Join(quoted, "\n") + "\n\n"
		}

	case "callout":
		if block.Callout != nil {
			emoji := ""
			if block.Callout.Icon != nil && block.Callout.Icon.Emoji != "" {
				emoji = block.Callout.Icon.Emoji + " "
			}
			return "> " + emoji + richTextToMarkdown(block.Callout.RichText) + "\n\n"
		}

	case "divider":
		return "---\n\n"

	case "image":
		if block.Image != nil {
			url := c.resolveMediaURL(block.Image)
			caption := richTextToPlain(block.Image.Caption)
			if caption == "" {
				caption = "image"
			}
			return fmt.Sprintf("![%s](%s)\n\n", caption, url)
		}

	case "video":
		if block.Video != nil {
			url := c.resolveMediaURL(block.Video)
			return fmt.Sprintf("[Video](%s)\n\n", url)
		}

	case "file", "pdf":
		var media *MediaBlock
		if block.Type == "file" {
			media = block.File
		} else {
			media = block.PDF
		}
		if media != nil {
			url := c.resolveMediaURL(media)
			return fmt.Sprintf("[File](%s)\n\n", url)
		}

	case "bookmark":
		if block.Bookmark != nil {
			caption := richTextToPlain(block.Bookmark.Caption)
			if caption == "" {
				caption = block.Bookmark.URL
			}
			return fmt.Sprintf("[%s](%s)\n\n", caption, block.Bookmark.URL)
		}

	case "embed":
		if block.Embed != nil {
			return fmt.Sprintf("[Embed](%s)\n\n", block.Embed.URL)
		}

	case "link_preview":
		if block.LinkPreview != nil {
			return fmt.Sprintf("[Link](%s)\n\n", block.LinkPreview.URL)
		}

	case "equation":
		if block.Equation != nil {
			return "$$\n" + block.Equation.Expression + "\n$$\n\n"
		}

	case "table_of_contents":
		return "[TOC]\n\n"

	case "breadcrumb":
		return "" // Skip breadcrumbs

	case "column_list", "column":
		return "" // Columns are structural, children handled separately

	case "synced_block":
		return "" // Content should be in children

	case "table":
		// Table content comes from children (table_row blocks)
		// Return a marker that we'll handle in the recursive processor
		return ""

	case "table_row":
		if block.TableRow != nil {
			var cells []string
			for _, cell := range block.TableRow.Cells {
				cells = append(cells, richTextToMarkdown(cell))
			}
			return "| " + strings.Join(cells, " | ") + " |\n"
		}

	case "child_page":
		if block.ChildPage != nil {
			link := c.resolveChildLink(block.ID)
			return fmt.Sprintf("📄 [%s](%s)\n\n", block.ChildPage.Title, link)
		}

	case "child_database":
		if block.ChildDatabase != nil {
			link := c.resolveChildLink(block.ID)
			return fmt.Sprintf("🗃️ [%s](%s)\n\n", block.ChildDatabase.Title, link)
		}
	}

	return ""
}

// richTextToMarkdown converts rich text to markdown with formatting.
func richTextToMarkdown(rt []RichText) string {
	parts := make([]string, 0, len(rt))
	for _, t := range rt {
		text := t.PlainText

		// Apply annotations
		if t.Annotations != nil {
			if t.Annotations.Code {
				text = "`" + text + "`"
			}
			if t.Annotations.Bold {
				text = "**" + text + "**"
			}
			if t.Annotations.Italic {
				text = "_" + text + "_"
			}
			if t.Annotations.Strikethrough {
				text = "~~" + text + "~~"
			}
			if t.Annotations.Underline {
				text = "<u>" + text + "</u>"
			}
		}

		// Apply link
		if t.Href != nil && *t.Href != "" {
			text = "[" + text + "](" + *t.Href + ")"
		}

		parts = append(parts, text)
	}
	return strings.Join(parts, "")
}

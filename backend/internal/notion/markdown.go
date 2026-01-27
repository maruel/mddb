// Converts Notion blocks to Markdown.

package notion

import (
	"fmt"
	"strings"
)

// BlocksToMarkdown converts a slice of Notion blocks to markdown.
func BlocksToMarkdown(blocks []Block) string {
	var sb strings.Builder
	listState := &listState{}

	for i := range blocks {
		md := blockToMarkdown(&blocks[i], listState, 0)
		if md != "" {
			sb.WriteString(md)
		}
	}

	return sb.String()
}

// listState tracks list context for proper markdown formatting.
type listState struct {
	numberedCount int
	inBulleted    bool
	inNumbered    bool
}

// blockToMarkdown converts a single block to markdown.
func blockToMarkdown(block *Block, ls *listState, depth int) string {
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
			url := ""
			if block.Image.File != nil {
				url = block.Image.File.URL
			} else if block.Image.External != nil {
				url = block.Image.External.URL
			}
			caption := richTextToPlain(block.Image.Caption)
			if caption == "" {
				caption = "image"
			}
			return fmt.Sprintf("![%s](%s)\n\n", caption, url)
		}

	case "video":
		if block.Video != nil {
			url := ""
			if block.Video.File != nil {
				url = block.Video.File.URL
			} else if block.Video.External != nil {
				url = block.Video.External.URL
			}
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
			url := ""
			if media.File != nil {
				url = media.File.URL
			} else if media.External != nil {
				url = media.External.URL
			}
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
		return "" // Table rows are children

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
			return fmt.Sprintf("📄 [%s]()\n\n", block.ChildPage.Title)
		}

	case "child_database":
		if block.ChildDatabase != nil {
			return fmt.Sprintf("🗃️ [%s]()\n\n", block.ChildDatabase.Title)
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

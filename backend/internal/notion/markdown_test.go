// Tests for the Notion block to Markdown converter.

package notion

import (
	"strings"
	"testing"
)

func TestBlocksToMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		blocks []Block
		want   string
	}{
		{
			"empty",
			[]Block{},
			"",
		},
		{
			"paragraph",
			[]Block{
				{Type: "paragraph", Paragraph: &ParagraphBlock{RichText: []RichText{{PlainText: "Hello World"}}}},
			},
			"Hello World\n\n",
		},
		{
			"headings",
			[]Block{
				{Type: "heading_1", Heading1: &HeadingBlock{RichText: []RichText{{PlainText: "H1"}}}},
				{Type: "heading_2", Heading2: &HeadingBlock{RichText: []RichText{{PlainText: "H2"}}}},
				{Type: "heading_3", Heading3: &HeadingBlock{RichText: []RichText{{PlainText: "H3"}}}},
			},
			"# H1\n\n## H2\n\n### H3\n\n",
		},
		{
			"bulleted list",
			[]Block{
				{Type: "bulleted_list_item", BulletedListItem: &ListItemBlock{RichText: []RichText{{PlainText: "Item 1"}}}},
				{Type: "bulleted_list_item", BulletedListItem: &ListItemBlock{RichText: []RichText{{PlainText: "Item 2"}}}},
			},
			"\n- Item 1\n- Item 2\n",
		},
		{
			"numbered list",
			[]Block{
				{Type: "numbered_list_item", NumberedListItem: &ListItemBlock{RichText: []RichText{{PlainText: "First"}}}},
				{Type: "numbered_list_item", NumberedListItem: &ListItemBlock{RichText: []RichText{{PlainText: "Second"}}}},
			},
			"\n1. First\n2. Second\n",
		},
		{
			"todo items",
			[]Block{
				{Type: "to_do", ToDo: &ToDoBlock{RichText: []RichText{{PlainText: "Unchecked"}}, Checked: false}},
				{Type: "to_do", ToDo: &ToDoBlock{RichText: []RichText{{PlainText: "Checked"}}, Checked: true}},
			},
			"- [ ] Unchecked\n- [x] Checked\n",
		},
		{
			"code block",
			[]Block{
				{Type: "code", Code: &CodeBlock{RichText: []RichText{{PlainText: "fmt.Println(\"Hello\")"}}, Language: "go"}},
			},
			"```go\nfmt.Println(\"Hello\")\n```\n\n",
		},
		{
			"quote",
			[]Block{
				{Type: "quote", Quote: &QuoteBlock{RichText: []RichText{{PlainText: "A wise quote"}}}},
			},
			"> A wise quote\n\n",
		},
		{
			"divider",
			[]Block{
				{Type: "divider", Divider: &struct{}{}},
			},
			"---\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BlocksToMarkdown(tt.blocks)
			if got != tt.want {
				t.Errorf("BlocksToMarkdown() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestRichTextToMarkdown(t *testing.T) {
	tests := []struct {
		name string
		rt   []RichText
		want string
	}{
		{"plain text", []RichText{{PlainText: "Hello"}}, "Hello"},
		{
			"bold",
			[]RichText{{PlainText: "bold", Annotations: &Annotations{Bold: true}}},
			"**bold**",
		},
		{
			"italic",
			[]RichText{{PlainText: "italic", Annotations: &Annotations{Italic: true}}},
			"_italic_",
		},
		{
			"code",
			[]RichText{{PlainText: "code", Annotations: &Annotations{Code: true}}},
			"`code`",
		},
		{
			"strikethrough",
			[]RichText{{PlainText: "strike", Annotations: &Annotations{Strikethrough: true}}},
			"~~strike~~",
		},
		{
			"link",
			[]RichText{{PlainText: "link", Href: ptrStr("https://example.com")}},
			"[link](https://example.com)",
		},
		{
			"bold and italic",
			[]RichText{{PlainText: "both", Annotations: &Annotations{Bold: true, Italic: true}}},
			"_**both**_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := richTextToMarkdown(tt.rt)
			if got != tt.want {
				t.Errorf("richTextToMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBlockToMarkdownImage(t *testing.T) {
	block := Block{
		Type: "image",
		Image: &MediaBlock{
			Type:     "external",
			External: &File{URL: "https://example.com/image.png"},
			Caption:  []RichText{{PlainText: "My image"}},
		},
	}

	result := BlocksToMarkdown([]Block{block})
	if !strings.Contains(result, "![My image](https://example.com/image.png)") {
		t.Errorf("expected image markdown, got %q", result)
	}
}

func TestBlockToMarkdownCallout(t *testing.T) {
	block := Block{
		Type: "callout",
		Callout: &CalloutBlock{
			RichText: []RichText{{PlainText: "Important note"}},
			Icon:     &Icon{Emoji: "💡"},
		},
	}

	result := BlocksToMarkdown([]Block{block})
	if !strings.Contains(result, "> 💡 Important note") {
		t.Errorf("expected callout markdown, got %q", result)
	}
}

func ptrStr(s string) *string {
	return &s
}

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, cleanup } from '@solidjs/testing-library';
import MarkdownPreview from './MarkdownPreview';

// Mock CSS module
vi.mock('./MarkdownPreview.module.css', () => ({
  default: {
    preview: 'preview',
  },
}));

afterEach(() => {
  cleanup();
});

describe('MarkdownPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders markdown content as HTML', () => {
    const { container } = render(() => <MarkdownPreview content="# Hello World" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview).toBeTruthy();
    expect(preview?.innerHTML).toContain('<h1>');
    expect(preview?.innerHTML).toContain('Hello World');
  });

  it('renders paragraphs correctly', () => {
    const { container } = render(() => <MarkdownPreview content="This is a paragraph." />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<p>');
    expect(preview?.innerHTML).toContain('This is a paragraph.');
  });

  it('renders links correctly', () => {
    const { container } = render(() => (
      <MarkdownPreview content="[Link Text](https://example.com)" />
    ));

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<a');
    expect(preview?.innerHTML).toContain('href="https://example.com"');
    expect(preview?.innerHTML).toContain('Link Text');
  });

  it('renders code blocks correctly', () => {
    const { container } = render(() => <MarkdownPreview content="```\ncode block\n```" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<code>');
    expect(preview?.innerHTML).toContain('code block');
  });

  it('renders inline code correctly', () => {
    const { container } = render(() => <MarkdownPreview content="Use `inline code` here" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<code>');
    expect(preview?.innerHTML).toContain('inline code');
  });

  it('renders lists correctly', () => {
    const { container } = render(() => <MarkdownPreview content="- Item 1\n- Item 2\n- Item 3" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<ul>');
    expect(preview?.innerHTML).toContain('<li>');
    expect(preview?.innerHTML).toContain('Item 1');
  });

  it('renders images without orgId unchanged', () => {
    const { container } = render(() => (
      <MarkdownPreview content="![Alt](https://example.com/image.png)" />
    ));

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="https://example.com/image.png"');
  });

  it('rewrites relative asset paths when orgId is provided', () => {
    const { container } = render(() => (
      <MarkdownPreview content="![Alt](assets/123/image.png)" orgId="org-456" />
    ));

    const preview = container.querySelector('[role="region"]');
    // Should rewrite assets/123/image.png to /assets/org-456/123/image.png
    expect(preview?.innerHTML).toContain('src="/assets/org-456/123/image.png"');
  });

  it('does not rewrite non-assets paths', () => {
    const { container } = render(() => (
      <MarkdownPreview content="![Alt](https://cdn.example.com/image.png)" orgId="org-456" />
    ));

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('src="https://cdn.example.com/image.png"');
  });

  it('renders empty content correctly', () => {
    const { container } = render(() => <MarkdownPreview content="" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview).toBeTruthy();
    // Empty content should produce empty or minimal HTML
    expect(preview?.innerHTML.trim()).toBe('');
  });

  it('has correct accessibility attributes', () => {
    const { container } = render(() => <MarkdownPreview content="Test" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.getAttribute('role')).toBe('region');
    expect(preview?.getAttribute('aria-label')).toBe('Markdown preview');
  });

  it('renders bold and italic text', () => {
    const { container } = render(() => (
      <MarkdownPreview content="**bold** and *italic* text" />
    ));

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<strong>bold</strong>');
    expect(preview?.innerHTML).toContain('<em>italic</em>');
  });

  it('renders blockquotes', () => {
    const { container } = render(() => <MarkdownPreview content="> This is a quote" />);

    const preview = container.querySelector('[role="region"]');
    expect(preview?.innerHTML).toContain('<blockquote>');
    expect(preview?.innerHTML).toContain('This is a quote');
  });

  it('handles complex asset paths correctly', () => {
    const { container } = render(() => (
      <MarkdownPreview content="![Alt](assets/page-id/subfolder/image.png)" orgId="my-org" />
    ));

    const preview = container.querySelector('[role="region"]');
    // Should preserve subfolder structure
    expect(preview?.innerHTML).toContain('src="/assets/my-org/page-id/subfolder/image.png"');
  });
});

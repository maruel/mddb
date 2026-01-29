// Component for rendering Markdown content with custom plugins.

import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';
import type { AssetUrlMap } from '../contexts/EditorContext';
import styles from './MarkdownPreview.module.css';

interface MarkdownPreviewProps {
  content: string;
  assetUrls?: AssetUrlMap;
}

const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
});

// Add core rule to detect checkbox syntax in list items
md.core.ruler.after('inline', 'task_list', (state) => {
  const tokens = state.tokens;
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (!token) continue;
    if (token.type === 'list_item_open') {
      // Look for inline content in this list item
      const inlineToken = tokens[i + 2];
      if (inlineToken && inlineToken.type === 'inline' && inlineToken.children) {
        const firstChild = inlineToken.children[0];
        if (firstChild && firstChild.type === 'text' && firstChild.content) {
          const match = firstChild.content.match(/^\[([ xX])\]\s*/);
          if (match && match[1]) {
            // Set attributes on list_item_open token
            token.attrSet('class', 'task-list-item');
            token.attrSet('data-checked', match[1].toLowerCase() === 'x' ? 'true' : 'false');
            // Remove checkbox syntax from text and add checkbox input
            firstChild.content = firstChild.content.slice(match[0].length);
            // Insert checkbox token at the beginning of inline children
            const checkboxToken = new state.Token('checkbox', '', 0);
            checkboxToken.attrSet('checked', match[1].toLowerCase() === 'x' ? 'true' : 'false');
            inlineToken.children.unshift(checkboxToken);
          }
        }
      }
    }
  }
  return true;
});

// Custom renderer for checkbox token
md.renderer.rules.checkbox = (tokens, idx) => {
  const token = tokens[idx];
  if (!token) return '';
  const isChecked = token.attrGet('checked') === 'true';
  return `<input type="checkbox" class="task-checkbox" disabled${isChecked ? ' checked' : ''}> `;
};

// Custom renderer for images to support signed asset URLs
const originalImageRenderer =
  md.renderer.rules.image ||
  function (tokens, idx, options, _env, self) {
    return self.renderToken(tokens, idx, options);
  };

md.renderer.rules.image = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  if (!token) {
    return originalImageRenderer(tokens, idx, options, env, self);
  }
  const srcIndex = token.attrIndex('src');
  const attrs = token.attrs;
  const assetUrls = env?.assetUrls as AssetUrlMap | undefined;
  const srcAttr = attrs?.[srcIndex];
  if (srcAttr && assetUrls) {
    const src = srcAttr[1];
    // If it's a local filename (not a URL or absolute path), look up signed URL
    if (!src.includes('://') && !src.startsWith('/')) {
      const signedUrl = assetUrls[src];
      if (signedUrl) {
        srcAttr[1] = signedUrl;
      }
    }
  }
  return originalImageRenderer(tokens, idx, options, env, self);
};

export default function MarkdownPreview(props: MarkdownPreviewProps) {
  const html = () => {
    const rawHtml = md.render(props.content, { assetUrls: props.assetUrls });
    return DOMPurify.sanitize(rawHtml);
  };

  return <div class={styles.preview} innerHTML={html()} role="region" aria-label="Markdown preview" />;
}

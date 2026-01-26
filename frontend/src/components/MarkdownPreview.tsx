// Component for rendering Markdown content with custom plugins.

import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';
import styles from './MarkdownPreview.module.css';

interface MarkdownPreviewProps {
  content: string;
  orgId?: string;
}

const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
});

// Custom renderer for images to support organization-aware asset URLs
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
  const orgId = env?.orgId;
  const srcAttr = attrs?.[srcIndex];
  if (srcAttr && orgId) {
    let src = srcAttr[1];
    // If it's a relative path starting with assets/, rewrite it
    if (src.startsWith('assets/')) {
      const parts = src.split('/');
      if (parts.length >= 2) {
        // Change assets/1/img.png to /assets/{orgId}/1/img.png
        src = `/assets/${orgId}/${parts.slice(1).join('/')}`;
        srcAttr[1] = src;
      }
    }
  }
  return originalImageRenderer(tokens, idx, options, env, self);
};

export default function MarkdownPreview(props: MarkdownPreviewProps) {
  const html = () => {
    const rawHtml = md.render(props.content, { orgId: props.orgId });
    return DOMPurify.sanitize(rawHtml);
  };

  return <div class={styles.preview} innerHTML={html()} role="region" aria-label="Markdown preview" />;
}

import MarkdownIt from 'markdown-it';
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
  const srcIndex = token.attrIndex('src');
  const attrs = token.attrs;
  const orgId = env?.orgId;
  if (attrs && srcIndex >= 0 && orgId) {
    let src = attrs[srcIndex][1];
    // If it's a relative path starting with assets/, rewrite it
    if (src.startsWith('assets/')) {
      const parts = src.split('/');
      if (parts.length >= 2) {
        // Change assets/1/img.png to /assets/{orgId}/1/img.png
        src = `/assets/${orgId}/${parts.slice(1).join('/')}`;
        attrs[srcIndex][1] = src;
      }
    }
  }
  return originalImageRenderer(tokens, idx, options, env, self);
};

export default function MarkdownPreview(props: MarkdownPreviewProps) {
  const html = () => md.render(props.content, { orgId: props.orgId });

  return (
    <div class={styles.preview} innerHTML={html()} role="region" aria-label="Markdown preview" />
  );
}

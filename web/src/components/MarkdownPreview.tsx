import MarkdownIt from 'markdown-it';
import styles from './MarkdownPreview.module.css';

interface MarkdownPreviewProps {
  content: string;
}

const md = new MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
});

export default function MarkdownPreview(props: MarkdownPreviewProps) {
  const html = () => md.render(props.content);

  return (
    <div
      class={styles.preview}
      innerHTML={html()}
      role="region"
      aria-label="Markdown preview"
    />
  );
}

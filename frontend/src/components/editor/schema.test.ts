import { describe, it, expect } from 'vitest';
import { schema } from './schema';
import { DOMSerializer, DOMParser } from 'prosemirror-model';

describe('Block Schema', () => {
  describe('Node Creation', () => {
    it('creates paragraph block by default', () => {
      const node = schema.nodes.block!.create();
      expect(node.attrs.type).toBe('paragraph');
      expect(node.attrs.indent).toBe(0);
    });

    it('creates heading block with level', () => {
      const node = schema.nodes.block!.create({ type: 'heading', level: 2 });
      expect(node.attrs.type).toBe('heading');
      expect(node.attrs.level).toBe(2);
    });

    it('creates bullet list block', () => {
      const node = schema.nodes.block!.create({ type: 'bullet', indent: 1 });
      expect(node.attrs.type).toBe('bullet');
      expect(node.attrs.indent).toBe(1);
    });

    it('creates task list block', () => {
      const node = schema.nodes.block!.create({ type: 'task', checked: true });
      expect(node.attrs.type).toBe('task');
      expect(node.attrs.checked).toBe(true);
    });

    it('creates code block with language', () => {
      const node = schema.nodes.block!.create({ type: 'code', language: 'typescript' });
      expect(node.attrs.type).toBe('code');
      expect(node.attrs.language).toBe('typescript');
    });
  });

  describe('DOM Serialization (toDOM)', () => {
    const serializer = DOMSerializer.fromSchema(schema);

    it('serializes paragraph', () => {
      const node = schema.nodes.block!.create({ type: 'paragraph' });
      const dom = serializer.serializeNode(node);
      expect(dom.nodeName).toBe('P');
      expect((dom as HTMLElement).dataset.type).toBe('paragraph');
    });

    it('serializes heading', () => {
      const node = schema.nodes.block!.create({ type: 'heading', level: 2 });
      const dom = serializer.serializeNode(node);
      expect(dom.nodeName).toBe('H2');
      expect((dom as HTMLElement).dataset.type).toBe('heading');
    });

    it('serializes bullet list', () => {
      const node = schema.nodes.block!.create({ type: 'bullet' });
      const dom = serializer.serializeNode(node);
      expect(dom.nodeName).toBe('DIV');
      expect((dom as HTMLElement).className).toContain('block-bullet');
    });

    it('serializes task list', () => {
      const node = schema.nodes.block!.create({ type: 'task', checked: true });
      const dom = serializer.serializeNode(node);
      expect((dom as HTMLElement).className).toContain('block-task');
      expect((dom as HTMLElement).dataset.checked).toBe('true');
    });

    it('serializes code block', () => {
      const node = schema.nodes.block!.create({ type: 'code', language: 'js' }, schema.text('code'));
      const dom = serializer.serializeNode(node);
      expect(dom.nodeName).toBe('PRE');
      expect((dom as HTMLElement).dataset.language).toBe('js');
    });
  });

  describe('DOM Parsing (parseDOM)', () => {
    const parser = DOMParser.fromSchema(schema);

    it('parses paragraph', () => {
      const div = document.createElement('div');
      div.innerHTML = '<p>hello</p>';
      const doc = parser.parse(div);
      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('paragraph');
    });

    it('parses heading', () => {
      const div = document.createElement('div');
      div.innerHTML = '<h2>Title</h2>';
      const doc = parser.parse(div);
      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('heading');
      expect(block.attrs.level).toBe(2);
    });

    it('parses task list item', () => {
      const div = document.createElement('div');
      div.innerHTML = '<li class="task-list-item" data-checked="true" data-indent="1">Task</li>';
      const doc = parser.parse(div);
      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('task');
      expect(block.attrs.checked).toBe(true);
      expect(block.attrs.indent).toBe(1);
    });

    it('parses code block class', () => {
      const div = document.createElement('div');
      div.innerHTML = '<pre class="language-js"><code>code</code></pre>';
      const doc = parser.parse(div);
      const block = doc.firstChild!;
      expect(block.attrs.type).toBe('code');
      expect(block.attrs.language).toBe('js');
    });
  });
});

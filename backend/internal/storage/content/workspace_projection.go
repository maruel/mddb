// Builds bounded discovery indexes and defines streaming content revisions and page-header parsing.

package content

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"github.com/maruel/ksid"
)

// MaxSnapshotMetadataBytes bounds front matter and table metadata before decoding.
const MaxSnapshotMetadataBytes = 64 << 10

// PageSnapshot contains a bounded body window and its full canonical content revision.
// Revision excludes independently editable icon, cover, and table metadata.
type PageSnapshot struct {
	Node         Node
	Revision     string
	ContentBytes int64
	NextOffset   int64
}

// PageWindow projects one canonical mutation snapshot using the same content window as file reads.
func PageWindow(node *Node, limit int) (out PageSnapshot, err error) {
	if err := validateContentWindow(0, limit); err != nil {
		return out, err
	}
	out.Node = *node
	out.Revision = node.Revision
	out.ContentBytes = int64(len(node.Content))
	out.Node.Content, out.NextOffset, err = contentWindow(node.Content, 0, limit)
	return out, err
}

func validateContentWindow(offset int64, limit int) error {
	if offset < 0 || limit < 4 || limit > 64<<10 {
		return errors.New("invalid content window")
	}
	return nil
}

func contentWindow(body string, offset int64, limit int) (window string, next int64, err error) {
	if offset > 0 && body != "" && !utf8.RuneStart(body[0]) {
		return "", 0, errors.New("content offset splits UTF-8")
	}
	end := min(len(body), limit)
	for end > 0 && end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	if !utf8.ValidString(body[:end]) {
		return "", 0, errors.New("document is not valid UTF-8")
	}
	return body[:end], offset + int64(end), nil
}

// Revision hashes a canonical parsed title and exactly bodyBytes of body.
// The streaming primitive owns the prefix format, cancellation, length validation,
// and digest encoding for snapshots and conditional mutations.
func Revision(ctx context.Context, title string, body io.Reader, bodyBytes int64) (string, error) {
	if bodyBytes < 0 || bodyBytes == math.MaxInt64 {
		return "", errors.New("invalid canonical content length")
	}
	h := sha256.New()
	hashLength(h, int64(len(title)))
	_, _ = io.WriteString(h, title)
	hashLength(h, bodyBytes)
	r := io.LimitReader(body, bodyBytes+1)
	buf := make([]byte, min(int64(32<<10), bodyBytes+1))
	var read int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := r.Read(buf)
		if n > 0 {
			read += int64(n)
			_, _ = h.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if read != bodyBytes {
		return "", errors.New("canonical content length changed")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashLength(h hash.Hash, n int64) {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(n)) //nolint:gosec // Lengths are validated nonnegative file sizes or string lengths.
	_, _ = h.Write(b[:])
}

// NodeSummaryPage contains only selected nodes; Total counts indexed identities.
type NodeSummaryPage struct {
	Nodes []*Node
	Total int
	Next  ksid.ID
}

func pageHeader(ctx context.Context, f *os.File) (parsed *page, bodyStart, fileSize int64, resultErr error) {
	st, err := f.Stat()
	if err != nil {
		return nil, 0, 0, err
	}
	r := bufio.NewReaderSize(f, 512)
	prefix, err := r.Peek(3)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, 0, 0, err
	}
	if !bytes.Equal(prefix, []byte("---")) {
		return ParseMarkdown(nil), 0, st.Size(), nil
	}
	var header []byte
	var pos int64
	lineStart := true
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, err
		}
		line, e := r.ReadSlice('\n')
		if len(header)+len(line) > MaxSnapshotMetadataBytes {
			return nil, 0, 0, errors.New("page front matter exceeds snapshot limit")
		}
		if pos > 0 && lineStart && bytes.HasPrefix(line, []byte("---")) {
			header = append(header, []byte("---")...)
			pos += 3
			// Bytes following the closing delimiter are body data, matching ParseMarkdown.
			tail := line[3:]
			for len(tail) > 0 && tail[0] == '\n' {
				tail = tail[1:]
				pos++
			}
			if len(tail) > 0 {
				return ParseMarkdown(header), pos, st.Size(), nil
			}
			for {
				b, x := r.ReadByte()
				if x == io.EOF {
					return ParseMarkdown(header), pos, st.Size(), nil
				}
				if x != nil {
					return nil, 0, 0, x
				}
				if b != '\n' {
					return ParseMarkdown(header), pos, st.Size(), nil
				}
				pos++
				if pos%4096 == 0 {
					if x := ctx.Err(); x != nil {
						return nil, 0, 0, x
					}
				}
			}
		}
		lineStart = len(line) > 0 && line[len(line)-1] == '\n'
		header = append(header, line...)
		pos += int64(len(line))
		if e == io.EOF {
			return ParseMarkdown(nil), 0, st.Size(), nil
		}
		if e != nil && !errors.Is(e, bufio.ErrBufferFull) {
			return nil, 0, 0, e
		}
	}
}

func insertID(ids []ksid.ID, id ksid.ID) []ksid.ID {
	at, ok := slices.BinarySearch(ids, id)
	if ok {
		return ids
	}
	return slices.Insert(ids, at, id)
}
func removeID(ids []ksid.ID, id ksid.ID) []ksid.ID {
	at, ok := slices.BinarySearch(ids, id)
	if !ok {
		return ids
	}
	return slices.Delete(ids, at, at+1)
}

// nodePathIndex builds private maps before publication. An ancestry-only legacy
// scan has nil discovery maps and cannot withdraw the published bounded index.
type nodePathIndex struct {
	parents      map[ksid.ID]ksid.ID
	children     map[ksid.ID][]ksid.ID
	discoverable map[ksid.ID]struct{}
}

func newNodePathIndex(discovery bool) *nodePathIndex {
	out := &nodePathIndex{parents: make(map[ksid.ID]ksid.ID)}
	if discovery {
		out.children = make(map[ksid.ID][]ksid.ID)
		out.discoverable = make(map[ksid.ID]struct{})
	}
	return out
}

func (idx *nodePathIndex) scan(check func() error, dir string, parent ksid.ID) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := check(); err != nil {
			return err
		}
		if !entry.IsDir() {
			continue
		}
		id, err := ksid.Parse(entry.Name())
		if err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		idx.parents[id] = parent
		if idx.children != nil {
			idx.children[parent] = insertID(idx.children[parent], id)
			_, p := os.Stat(filepath.Join(path, "index.md"))
			_, t := os.Stat(filepath.Join(path, "metadata.json"))
			if p != nil && !os.IsNotExist(p) {
				return p
			}
			if t != nil && !os.IsNotExist(t) {
				return t
			}
			if p == nil || t == nil {
				idx.discoverable[id] = struct{}{}
			}
		}
		if err := idx.scan(check, path, id); err != nil {
			return err
		}
	}
	return nil
}

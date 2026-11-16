package library

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// ErrTargetNotFound is returned when a target names no existing Resume
// Document or Markdown file.
var ErrTargetNotFound = errors.New("找不到 Resume Document")

// ErrAmbiguousTarget is returned when a name matches more than one Resume
// Document.
var ErrAmbiguousTarget = errors.New("Resume Document 名称不唯一")

// ErrExternalTarget is returned when a library-only operation is applied to an
// external Markdown file.
var ErrExternalTarget = errors.New("外部 Markdown 文件不能作为 Library 文档修改")

// ErrInvalidTarget is returned when a target exists but is not a usable
// Markdown file or library document.
var ErrInvalidTarget = errors.New("无效的 Resume Document 目标")

const (
	indexVersion = 1
	indexName    = "index.json"
	documentsDir = "documents"
	layoutsName  = "layouts.json"
)

// Document is the stable library metadata of one Resume Document.
type Document struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// AmbiguousTargetError describes a name shared by several Resume Documents.
type AmbiguousTargetError struct {
	Target    string
	Documents []Document
}

func (err *AmbiguousTargetError) Error() string {
	ids := make([]string, len(err.Documents))
	for index, document := range err.Documents {
		ids[index] = document.ID
	}
	return fmt.Sprintf("%v: 名称 %q 对应多个 Resume Document（%s），请改用 ID",
		ErrAmbiguousTarget, err.Target, strings.Join(ids, ", "))
}

// Unwrap reports ErrAmbiguousTarget.
func (err *AmbiguousTargetError) Unwrap() error { return ErrAmbiguousTarget }

// Store is the self-contained Resume Document library inside PathUserData.
type Store struct {
	dir string
	mu  sync.Mutex
}

type indexFile struct {
	Version   int        `json:"version"`
	Documents []Document `json:"documents"`
}

// Open opens the library inside mygo.App.Path(mygo.PathUserData).
func Open() (*Store, error) {
	mygo.App.SetName("tokicv")
	root, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, fmt.Errorf("定位 tokicv 用户数据目录: %w", err)
	}
	return OpenAt(filepath.Join(root, "library"))
}

// OpenAt opens a library rooted at an explicit directory. It creates nothing
// until the first write.
func OpenAt(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("Library 目录为空")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("解析 Library 目录 %q: %w", dir, err)
	}
	return &Store{dir: filepath.Clean(absolute)}, nil
}

// Dir returns the absolute library root.
func (s *Store) Dir() string { return s.dir }

// List returns the Resume Documents, most recently updated first.
func (s *Store) List() ([]Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	documents, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	sort.Slice(documents, func(i, j int) bool {
		if !documents[i].UpdatedAt.Equal(documents[j].UpdatedAt) {
			return documents[i].UpdatedAt.After(documents[j].UpdatedAt)
		}
		if documents[i].Name != documents[j].Name {
			return documents[i].Name < documents[j].Name
		}
		return documents[i].ID < documents[j].ID
	})
	result := make([]Document, len(documents))
	copy(result, documents)
	return result, nil
}

// Create adds a Resume Document with the exact source bytes.
func (s *Store) Create(name string, source []byte) (Document, error) {
	if err := validateName(name); err != nil {
		return Document{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	documents, err := s.loadLocked()
	if err != nil {
		return Document{}, err
	}
	id, err := newID(documents)
	if err != nil {
		return Document{}, err
	}
	now := time.Now().UTC()
	document := Document{
		ID:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.writeContentLocked(document.ID, source); err != nil {
		return Document{}, err
	}
	if err := s.saveLocked(append(documents, document)); err != nil {
		_ = os.Remove(s.contentPath(document.ID))
		return Document{}, err
	}
	return document, nil
}

// Import copies an external Markdown file into the library without changing
// the source file. An empty name uses the source file stem.
func (s *Store) Import(path, name string) (Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Document{}, fmt.Errorf("读取外部 Resume Document %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Document{}, fmt.Errorf("外部 Resume Document %q 不是普通文件", path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return Document{}, fmt.Errorf("外部 Resume Document %q 不是 .md 文件", path)
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("读取外部 Resume Document %q: %w", path, err)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return s.Create(name, source)
}

// Get returns the metadata of a Resume Document by exact ID.
func (s *Store) Get(id string) (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	documents, err := s.loadLocked()
	if err != nil {
		return Document{}, err
	}
	for _, document := range documents {
		if document.ID == id {
			return document, nil
		}
	}
	return Document{}, fmt.Errorf("%w: %q", ErrTargetNotFound, id)
}

// Show resolves a target and returns its document metadata and exact Markdown
// source bytes.
func (s *Store) Show(target string) (Document, []byte, error) {
	resolved, err := s.ResolveTarget(target)
	if err != nil {
		return Document{}, nil, err
	}
	source, err := resolved.Read()
	if err != nil {
		return Document{}, nil, err
	}
	return resolved.Document, source, nil
}

// Rename changes a library Resume Document name and keeps its ID and source.
func (s *Store) Rename(target, name string) (Document, error) {
	if err := validateName(name); err != nil {
		return Document{}, err
	}
	resolved, err := s.ResolveTarget(target)
	if err != nil {
		return Document{}, err
	}
	if resolved.Kind != TargetLibrary {
		return Document{}, fmt.Errorf("%w: %q", ErrExternalTarget, target)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	documents, err := s.loadLocked()
	if err != nil {
		return Document{}, err
	}
	for index := range documents {
		if documents[index].ID != resolved.Document.ID {
			continue
		}
		documents[index].Name = name
		documents[index].UpdatedAt = time.Now().UTC()
		if err := s.saveLocked(documents); err != nil {
			return Document{}, err
		}
		return documents[index], nil
	}
	return Document{}, fmt.Errorf("%w: %q", ErrTargetNotFound, target)
}

// Delete removes a library Resume Document and its content.
func (s *Store) Delete(target string) (Document, error) {
	resolved, err := s.ResolveTarget(target)
	if err != nil {
		return Document{}, err
	}
	if resolved.Kind != TargetLibrary {
		return Document{}, fmt.Errorf("%w: %q", ErrExternalTarget, target)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	documents, err := s.loadLocked()
	if err != nil {
		return Document{}, err
	}
	remaining := make([]Document, 0, len(documents))
	var deleted Document
	found := false
	for _, document := range documents {
		if document.ID == resolved.Document.ID {
			deleted = document
			found = true
			continue
		}
		remaining = append(remaining, document)
	}
	if !found {
		return Document{}, fmt.Errorf("%w: %q", ErrTargetNotFound, target)
	}
	contentPath := s.contentPath(deleted.ID)
	source, readErr := os.ReadFile(contentPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return Document{}, fmt.Errorf("读取 Resume Document %q 的内容以执行删除: %w", deleted.ID, readErr)
	}
	if err := os.Remove(contentPath); err != nil && !os.IsNotExist(err) {
		return Document{}, fmt.Errorf("删除 Resume Document %q 的内容: %w", deleted.ID, err)
	}
	if err := s.saveLocked(remaining); err != nil {
		if readErr == nil {
			if restoreErr := s.writeContentLocked(deleted.ID, source); restoreErr != nil {
				return Document{}, errors.Join(
					fmt.Errorf("写入 Library 索引: %w", err),
					fmt.Errorf("恢复 Resume Document %q 的内容: %w", deleted.ID, restoreErr),
				)
			}
		}
		return Document{}, fmt.Errorf("写入 Library 索引: %w", err)
	}
	return deleted, nil
}

func (s *Store) loadLocked() ([]Document, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, indexName))
	if os.IsNotExist(err) {
		return []Document{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 Library 索引: %w", err)
	}
	var index indexFile
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("解析 Library 索引 %q: %w", filepath.Join(s.dir, indexName), err)
	}
	if index.Version > indexVersion {
		return nil, fmt.Errorf("Library 索引版本 %d 高于支持的版本 %d", index.Version, indexVersion)
	}
	if index.Documents == nil {
		index.Documents = []Document{}
	}
	seen := make(map[string]bool, len(index.Documents))
	for _, document := range index.Documents {
		if document.ID == "" {
			return nil, errors.New("Library 索引包含空文档 ID")
		}
		if seen[document.ID] {
			return nil, fmt.Errorf("Library 索引包含重复文档 ID %q", document.ID)
		}
		seen[document.ID] = true
	}
	return index.Documents, nil
}

func (s *Store) saveLocked(documents []Document) error {
	if documents == nil {
		documents = []Document{}
	}
	data, err := json.MarshalIndent(indexFile{Version: indexVersion, Documents: documents}, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 Library 索引: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(filepath.Join(s.dir, indexName), data); err != nil {
		return fmt.Errorf("写入 Library 索引: %w", err)
	}
	return nil
}

func (s *Store) writeContentLocked(id string, source []byte) error {
	if err := os.MkdirAll(filepath.Join(s.dir, documentsDir), 0o755); err != nil {
		return fmt.Errorf("创建 Library 文档目录: %w", err)
	}
	if err := writeFileAtomic(s.contentPath(id), source); err != nil {
		return fmt.Errorf("写入 Resume Document %q: %w", id, err)
	}
	return nil
}

func (s *Store) contentPath(id string) string {
	return filepath.Join(s.dir, documentsDir, id+".md")
}

func (s *Store) readContent(id string) ([]byte, error) {
	data, err := os.ReadFile(s.contentPath(id))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: Resume Document %q 的内容文件缺失", ErrTargetNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("读取 Resume Document %q: %w", id, err)
	}
	return data, nil
}

func validateName(name string) error {
	if name == "" || strings.TrimSpace(name) == "" {
		return errors.New("Resume Document 名称不能为空")
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("Resume Document 名称 %q 不能以空白开头或结尾", name)
	}
	if strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("Resume Document 名称 %q 包含不可用字符", name)
	}
	return nil
}

func newID(documents []Document) (string, error) {
	existing := make(map[string]bool, len(documents))
	for _, document := range documents {
		existing[document.ID] = true
	}
	for {
		var value [8]byte
		if _, err := rand.Read(value[:]); err != nil {
			return "", fmt.Errorf("生成 Resume Document ID: %w", err)
		}
		id := "doc-" + hex.EncodeToString(value[:])
		if !existing[id] {
			return id, nil
		}
	}
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

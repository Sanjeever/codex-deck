package sessions

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const listPageSize = 40
const eventPageSize = 32
const matchPageSize = 100
const cachedDocuments = 8

type entry struct {
	path     string
	info     fs.FileInfo
	summary  Summary
	issues   []Issue
	doc      *document
	revision string
	used     uint64
}

type Service struct {
	ctx   context.Context
	root  string
	mu    sync.Mutex
	files map[string]*entry
	clock uint64
}

func NewService(ctx context.Context, codexHome string) *Service {
	root, err := filepath.Abs(filepath.Join(codexHome, "sessions"))
	if err != nil {
		root = filepath.Join(codexHome, "sessions")
	}
	return &Service{ctx: ctx, root: root, files: map[string]*entry{}}
}

// secureOpen rejects links and revalidates the opened identity. Callers can only
// use enumerated IDs; the bridge never accepts filenames from the frontend.
func (s *Service) secureOpen(path string) (*os.File, error) {
	rel, err := filepath.Rel(s.root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("会话路径越界")
	}
	check := func() error {
		current := s.root
		for _, part := range append([]string{""}, strings.Split(rel, string(filepath.Separator))...) {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("拒绝读取会话目录中的链接")
			}
		}
		resolvedRoot, err := filepath.EvalSymlinks(s.root)
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		inside, err := filepath.Rel(resolvedRoot, resolved)
		if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
			return fmt.Errorf("会话链接指向目录外")
		}
		return nil
	}
	if err := check(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("会话不是普通文件")
	}
	if err == nil {
		err = check()
	}
	if err == nil {
		var current fs.FileInfo
		current, err = os.Stat(path)
		if err == nil && !os.SameFile(info, current) {
			err = fmt.Errorf("会话文件在打开时被替换，请刷新")
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func sameVersion(a, b fs.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func (s *Service) label(path string) string {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return "会话目录"
	}
	return rel
}

func (s *Service) load(path string, needDocument bool) (*entry, *os.File, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, nil, err
	}
	f, err := s.secureOpen(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(s.label(path))))
	e := s.files[id]
	if e == nil || !sameVersion(e.info, info) || (needDocument && e.doc == nil) {
		d, readErr := readDocument(s.ctx, f, s.label(path))
		if readErr != nil {
			f.Close()
			return nil, nil, readErr
		}
		after, statErr := f.Stat()
		if statErr != nil {
			f.Close()
			return nil, nil, statErr
		}
		if !sameVersion(info, after) {
			f.Close()
			return nil, nil, fmt.Errorf("会话在读取期间发生变化，请重试")
		}
		s.clock++
		revision := strconv.FormatUint(s.clock, 10)
		if e != nil && sameVersion(e.info, info) {
			revision = e.revision
		}
		d.summary.ID = id
		e = &entry{path: path, info: info, summary: d.summary, issues: d.issues, doc: d, revision: revision}
		s.files[id] = e
	}
	s.clock++
	e.used = s.clock
	count := 0
	var oldest *entry
	for _, candidate := range s.files {
		if candidate.doc != nil {
			count++
			if candidate != e && (oldest == nil || candidate.used < oldest.used) {
				oldest = candidate
			}
		}
	}
	if count > cachedDocuments && oldest != nil {
		oldest.doc = nil
	}
	return e, f, nil
}

type pageCursor struct {
	Key    string `json:"key"`
	Offset int    `json:"offset"`
}

func cursor(key string, offset int) string {
	data, _ := json.Marshal(pageCursor{key, offset})
	return base64.RawURLEncoding.EncodeToString(data)
}
func parseCursor(value, key string, length int) (int, error) {
	if value == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var c pageCursor
	if err != nil || json.Unmarshal(data, &c) != nil || c.Key != key || c.Offset < 0 || c.Offset >= length {
		return 0, fmt.Errorf("分页已失效或记录已变化，请刷新后重试")
	}
	return c.Offset, nil
}
func key(parts ...string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

func localDate(value string) string {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return t.In(time.Local).Format("2006-01-02")
}
func newer(a, b string) bool {
	at, _ := time.Parse(time.RFC3339Nano, a)
	bt, _ := time.Parse(time.RFC3339Nano, b)
	return at.After(bt)
}

func (s *Service) List(query ListQuery) (ListPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := ListPage{Sessions: []Summary{}, Projects: []string{}, Dates: []string{}, Issues: []Issue{}, Source: s.root}
	if err := s.ctx.Err(); err != nil {
		return page, err
	}
	rootInfo, err := os.Lstat(s.root)
	if os.IsNotExist(err) {
		page.Missing = true
		s.files = map[string]*entry{}
		return page, nil
	}
	if err != nil {
		return page, fmt.Errorf("无法读取会话目录：%w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return page, fmt.Errorf("会话目录必须是普通目录，不能是链接")
	}
	seen := map[string]bool{}
	projects := map[string]bool{}
	dates := map[string]bool{}
	rows := []Summary{}
	revisions := []string{}
	search := strings.TrimSpace(query.Search)
	err = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			page.Issues = append(page.Issues, Issue{File: s.label(path), Message: "无法读取：" + walkErr.Error()})
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			page.Issues = append(page.Issues, Issue{File: s.label(path), Message: "拒绝扫描会话目录中的链接"})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".jsonl.zst") {
			page.Issues = append(page.Issues, Issue{File: s.label(path), Message: "暂不支持压缩会话 .jsonl.zst，该文件未参与展示或搜索"})
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		e, f, readErr := s.load(path, search != "")
		if readErr != nil {
			page.Issues = append(page.Issues, Issue{File: s.label(path), Message: readErr.Error()})
			return nil
		}
		defer f.Close()
		seen[e.summary.ID] = true
		revisions = append(revisions, e.summary.ID+e.revision)
		page.Issues = append(page.Issues, e.issues...)
		if e.summary.Project != "" {
			projects[e.summary.Project] = true
		}
		if date := localDate(e.summary.UpdatedAt); date != "" {
			dates[date] = true
		}
		if query.Project != "" && query.Project != e.summary.Project {
			return nil
		}
		if query.Date != "" && query.Date != localDate(e.summary.UpdatedAt) {
			return nil
		}
		summary := e.summary
		if search != "" {
			metadata := summary.Title + "\n" + summary.Project
			matched := strings.Contains(strings.ToLower(metadata), strings.ToLower(search))
			if matched {
				summary.Snippet = excerpt(metadata, search, 180)
			}
			for _, event := range e.doc.events {
				text, textErr := eventText(f, event)
				if textErr != nil {
					page.Issues = append(page.Issues, Issue{File: s.label(path), Message: textErr.Error()})
					break
				}
				if strings.Contains(strings.ToLower(event.Name+"\n"+text), strings.ToLower(search)) {
					matched = true
					summary.Snippet = excerpt(text, search, 180)
					summary.MatchEventID = event.ID
					break
				}
			}
			if !matched {
				return nil
			}
		}
		rows = append(rows, summary)
		return nil
	})
	if err != nil {
		return page, err
	}
	for id := range s.files {
		if !seen[id] {
			delete(s.files, id)
		}
	}
	for project := range projects {
		page.Projects = append(page.Projects, project)
	}
	sort.Strings(page.Projects)
	for date := range dates {
		page.Dates = append(page.Dates, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(page.Dates)))
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt == rows[j].UpdatedAt {
			return rows[i].ID < rows[j].ID
		}
		return newer(rows[i].UpdatedAt, rows[j].UpdatedAt)
	})
	sort.Strings(revisions)
	pageKey := key(query.Search, query.Project, query.Date, strings.Join(revisions, ","))
	start, err := parseCursor(query.Cursor, pageKey, len(rows))
	if err != nil {
		return page, err
	}
	end := min(start+listPageSize, len(rows))
	page.Sessions = rows[start:end]
	if end < len(rows) {
		page.NextCursor = cursor(pageKey, end)
	}
	return page, nil
}

func (s *Service) session(id string) (*entry, *os.File, error) {
	e := s.files[id]
	if e == nil {
		return nil, nil, fmt.Errorf("会话未找到，请刷新列表后重试")
	}
	return s.load(e.path, true)
}
func filtered(event Event, filter string) bool {
	switch filter {
	case "chat":
		return event.Kind == "user" || event.Kind == "assistant"
	case "error":
		return event.Status == "error"
	default:
		return true
	}
}

func (s *Service) Read(query ReadQuery) (ReadPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := ReadPage{Events: []Event{}, Turns: []Turn{}, Issues: []Issue{}}
	if query.Filter != "" && query.Filter != "all" && query.Filter != "chat" && query.Filter != "error" {
		return page, fmt.Errorf("未知事件筛选")
	}
	e, f, err := s.session(query.ID)
	if err != nil {
		return page, err
	}
	defer f.Close()
	page.Session = e.summary
	page.Revision = e.revision
	page.Turns = e.doc.turns
	page.Issues = e.issues
	indices := []int{}
	for i, event := range e.doc.events {
		if filtered(event.Event, query.Filter) {
			indices = append(indices, i)
		}
	}
	pageKey := key(query.ID, e.revision, query.Filter)
	start, err := parseCursor(query.Cursor, pageKey, len(indices))
	if err != nil {
		return page, err
	}
	if query.EventID != "" {
		found := false
		for i, index := range indices {
			if e.doc.events[index].ID == query.EventID {
				start = i
				found = true
				break
			}
		}
		if !found {
			return page, fmt.Errorf("定位事件不可用，请选择全部事件或刷新")
		}
	}
	end := min(start+eventPageSize, len(indices))
	for _, index := range indices[start:end] {
		if err := s.ctx.Err(); err != nil {
			return page, err
		}
		event := e.doc.events[index]
		text, err := eventText(f, event)
		if err != nil {
			return page, err
		}
		event.Text, event.TextCursor = chunk(text, 0)
		page.Events = append(page.Events, event.Event)
	}
	if end < len(indices) {
		page.NextCursor = cursor(pageKey, end)
	}
	return page, s.unchanged(e, f)
}

func chunk(text string, offset int) (string, int) {
	runes := []rune(text)
	end := min(offset+textChunkRunes, len(runes))
	if end < len(runes) {
		return string(runes[offset:end]), end
	}
	return string(runes[offset:end]), 0
}
func (s *Service) unchanged(e *entry, f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !sameVersion(e.info, info) {
		return fmt.Errorf("会话在读取期间发生变化，请刷新")
	}
	return nil
}

func (s *Service) Find(query MatchQuery) (MatchPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := MatchPage{Matches: []Match{}, Issues: []Issue{}}
	e, f, err := s.session(query.ID)
	if err != nil {
		return page, err
	}
	defer f.Close()
	page.Issues = e.issues
	search := strings.TrimSpace(query.Search)
	if search == "" {
		return page, nil
	}
	pageKey := key(query.ID, e.revision, search)
	start, err := parseCursor(query.Cursor, pageKey, len(e.doc.events))
	if err != nil {
		return page, err
	}
	for i := start; i < len(e.doc.events); i++ {
		if err := s.ctx.Err(); err != nil {
			return page, err
		}
		event := e.doc.events[i]
		text, err := eventText(f, event)
		if err != nil {
			return page, err
		}
		index := strings.Index(strings.ToLower(text), strings.ToLower(search))
		if index >= 0 || strings.Contains(strings.ToLower(event.Name), strings.ToLower(search)) {
			offset := 0
			if index >= 0 {
				offset = len([]rune(text[:index]))
			}
			page.Matches = append(page.Matches, Match{EventID: event.ID, Turn: event.Turn, Snippet: excerpt(text, search, 180), TextOffset: offset})
			if len(page.Matches) == matchPageSize {
				if i+1 < len(e.doc.events) {
					page.NextCursor = cursor(pageKey, i+1)
				}
				break
			}
		}
	}
	return page, s.unchanged(e, f)
}

func (s *Service) ReadText(query TextQuery) (TextPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, f, err := s.session(query.ID)
	if err != nil {
		return TextPage{}, err
	}
	defer f.Close()
	if e.revision != query.Revision {
		return TextPage{}, fmt.Errorf("会话已变化，请刷新后重新展开内容")
	}
	for _, event := range e.doc.events {
		if event.ID == query.EventID {
			text, err := eventText(f, event)
			if err != nil {
				return TextPage{}, err
			}
			if query.Offset < 0 || query.Offset > len([]rune(text)) {
				return TextPage{}, fmt.Errorf("内容位置无效")
			}
			value, next := chunk(text, query.Offset)
			return TextPage{Text: value, NextOffset: next}, s.unchanged(e, f)
		}
	}
	return TextPage{}, fmt.Errorf("事件未找到")
}

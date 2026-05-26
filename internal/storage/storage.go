package storage

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileInfo struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Size  int64     `json:"size"`
	MTime time.Time `json:"mtime"`
}

type Store struct {
	dir string
}

func New(dir string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	if err := ensureWritable(abs); err != nil {
		return nil, err
	}
	return &Store{dir: abs}, nil
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) List() ([]FileInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			ID:    EncodeID(entry.Name()),
			Name:  entry.Name(),
			Size:  info.Size(),
			MTime: info.ModTime(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].MTime.After(files[j].MTime)
	})
	return files, nil
}

func (s *Store) SaveMultipart(file multipart.File, header *multipart.FileHeader) (FileInfo, error) {
	defer file.Close()

	name := SafeName(header.Filename)
	if name == "" {
		return FileInfo{}, fmt.Errorf("文件名不能为空")
	}
	finalPath := s.uniquePath(name)
	tmpPath := finalPath + ".part"

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return FileInfo{}, err
	}
	copied, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return FileInfo{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return FileInfo{}, closeErr
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return FileInfo{}, err
	}

	info, err := os.Stat(finalPath)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{
		ID:    EncodeID(filepath.Base(finalPath)),
		Name:  filepath.Base(finalPath),
		Size:  copied,
		MTime: info.ModTime(),
	}, nil
}

func (s *Store) Open(id string) (*os.File, FileInfo, error) {
	name, err := DecodeID(id)
	if err != nil {
		return nil, FileInfo{}, err
	}
	path, err := s.pathFor(name)
	if err != nil {
		return nil, FileInfo{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, FileInfo{}, err
	}
	if info.IsDir() {
		return nil, FileInfo{}, errors.New("不能下载目录")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, FileInfo{}, err
	}
	return file, FileInfo{
		ID:    EncodeID(name),
		Name:  name,
		Size:  info.Size(),
		MTime: info.ModTime(),
	}, nil
}

func (s *Store) Delete(id string) error {
	name, err := DecodeID(id)
	if err != nil {
		return err
	}
	path, err := s.pathFor(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func SafeName(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	name = strings.Trim(name, ".")
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	if name == "" {
		return ""
	}
	return name
}

func EncodeID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

func DecodeID(id string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", fmt.Errorf("文件 ID 无效")
	}
	name := SafeName(string(data))
	if name == "" || name != string(data) {
		return "", fmt.Errorf("文件 ID 不安全")
	}
	return name, nil
}

func (s *Store) pathFor(name string) (string, error) {
	safe := SafeName(name)
	if safe == "" || safe != name {
		return "", fmt.Errorf("文件名不安全")
	}
	path := filepath.Join(s.dir, safe)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, s.dir+string(filepath.Separator)) && abs != s.dir {
		return "", fmt.Errorf("路径越界")
	}
	return abs, nil
}

func (s *Store) uniquePath(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	candidate := filepath.Join(s.dir, name)
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate
	}
	stamp := time.Now().Format("20060102-150405")
	for i := 1; ; i++ {
		next := filepath.Join(s.dir, fmt.Sprintf("%s-%s-%02d%s", base, stamp, i, ext))
		if _, err := os.Stat(next); errors.Is(err, os.ErrNotExist) {
			return next
		}
	}
}

func ensureWritable(dir string) error {
	testFile, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("目录不可写: %w", err)
	}
	name := testFile.Name()
	if err := testFile.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

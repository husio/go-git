package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type Repository struct {
	workdir string
	gitdir  string
	conf    string // TODO
}

func CreateRepository(dir string) (*Repository, error) {
	switch err := os.MkdirAll(path.Join(dir, ".git"), newDirPerm); {
	case errors.Is(err, os.ErrExist):
		return nil, fmt.Errorf("already a git repository: %w", err)
	case err == nil:
		// All good.
	default:
		return nil, fmt.Errorf("mkdir .git: %w", err)
	}

	repo, err := OpenRepository(dir)
	if err != nil {
		return nil, fmt.Errorf("new repository: %w", err)
	}
	if ok, err := isDir(repo.workdir); err != nil {
		return nil, fmt.Errorf("workdir is dir %q: %w", repo.workdir, err)
	} else if !ok {
		if err := os.MkdirAll(repo.workdir, newDirPerm); err != nil {
			return nil, fmt.Errorf("mkdir workdir: %w", err)
		}
	}
	if _, err := repo.DirPath(true, "branches"); err != nil {
		return nil, fmt.Errorf("mkdir branches: %w", err)
	}
	if _, err := repo.DirPath(true, "objects"); err != nil {
		return nil, fmt.Errorf("mkdir objects: %w", err)
	}
	if _, err := repo.DirPath(true, "refs", "tags"); err != nil {
		return nil, fmt.Errorf("mkdir refs/tags: %w", err)
	}
	if _, err := repo.DirPath(true, "refs", "heads"); err != nil {
		return nil, fmt.Errorf("mkdir refs/heads: %w", err)
	}
	if err := repo.WriteFile(true, []byte(defaultDescription), "description"); err != nil {
		return nil, fmt.Errorf("write description file: %w", err)
	}
	if err := repo.WriteFile(true, []byte(defaultHEAD), "HEAD"); err != nil {
		return nil, fmt.Errorf("write HEAD file: %w", err)
	}
	if err := repo.WriteFile(true, []byte(defaultConfig), "config"); err != nil {
		return nil, fmt.Errorf("write config file: %w", err)
	}
	return repo, nil
}

const (
	defaultDescription = "Unnamed repository.\n"
	defaultHEAD        = "ref: refs/heads/master\n"
	defaultConfig      = "[core]\nrepositoryformatversion = 0\nfilemode = false\nbare = false\n"
)

func FindRepository(repo string) (*Repository, error) {
	repo, err := filepath.Abs(repo)
	if err != nil {
		return nil, fmt.Errorf("abs filepath: %w", err)
	}
	for {
		if ok, err := isDir(path.Join(repo, ".git")); err == nil && ok {
			return OpenRepository(repo)
		}
		if repo == "." {
			return nil, fmt.Errorf("no .git directory: %w", os.ErrNotExist)
		}
		repo = filepath.Dir(repo)
	}
}

func OpenRepository(dir string) (*Repository, error) {
	gitdir := path.Join(dir, ".git")

	if ok, err := isDir(dir); err != nil {
		return nil, fmt.Errorf("is dir: %w", err)
	} else if !ok {
		return nil, fmt.Errorf("not a git directory: %q", dir)
	}

	// TODO read configuration

	r := &Repository{
		workdir: dir,
		gitdir:  gitdir,
	}
	return r, nil
}

// DirPath returns a directory path that is relative to this repository. If
// mkdir flag is set, directory is created if does not yet exist.
func (r *Repository) DirPath(mkdir bool, pathChunks ...string) (string, error) {
	full := path.Join(r.gitdir, path.Join(pathChunks...))
	ok, err := isDir(full)
	if err != nil {
		return "", fmt.Errorf("is dir: %w", err)
	}
	if !ok {
		if !mkdir {
			return "", fmt.Errorf("dir %q: %w", full, os.ErrNotExist)
		}
		if err := os.MkdirAll(full, newDirPerm); err != nil {
			return "", fmt.Errorf("mkdir %q: %w", full, err)
		}
	}
	return full, nil
}

func (r *Repository) WriteFile(mkdir bool, content []byte, pathChunks ...string) error {
	if len(pathChunks) > 1 {
		_, err := r.DirPath(mkdir, pathChunks[:len(pathChunks)-2]...)
		if err != nil {
			return fmt.Errorf("ensure directory: %w", err)
		}
	}
	full := path.Join(r.gitdir, path.Join(pathChunks...))
	if err := ioutil.WriteFile(full, content, 0644); err != nil {
		return fmt.Errorf("write contnt: %w", err)
	}
	return nil
}

func (r *Repository) ReadObject(sha []byte) (Object, error) {
	if len(sha) != 20 {
		return nil, fmt.Errorf("invalid hash length: %d", len(sha))
	}
	s := hex.EncodeToString(sha)
	full := path.Join(r.gitdir, "objects", s[:2], s[2:])
	fd, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	defer fd.Close()

	zrd, err := zlib.NewReader(fd)
	if err != nil {
		return nil, fmt.Errorf("zlib object reader: %w", err)
	}
	defer zrd.Close()

	rd := bufio.NewReader(zrd)

	kind, err := rd.ReadString(' ')
	if err != nil {
		return nil, fmt.Errorf("read object kind: %w", err)
	}
	kind = kind[:len(kind)-1]
	newObj, ok := objects[kind]
	if !ok {
		return nil, fmt.Errorf("unknown object kind: %q", kind)
	}

	ssize, err := rd.ReadString(0)
	if err != nil {
		return nil, fmt.Errorf("read object size: %w", err)
	}
	size, err := strconv.Atoi(ssize[:len(ssize)-1])
	if err != nil {
		return nil, fmt.Errorf("invalid object size: %w", err)
	}

	content, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("read object content: %w", err)
	}
	if size != len(content) {
		return nil, fmt.Errorf("bad object length %d != %d", len(content), size)
	}

	obj := newObj()
	if err := obj.Deserialize(content); err != nil {
		return nil, fmt.Errorf("deserialize %s object: %w", kind, err)
	}
	return obj, nil
}

func (r *Repository) WriteObject(kind string, content []byte) (sha []byte, werr error) {
	var b bytes.Buffer
	if _, err := fmt.Fprintf(&b, "%s %d\x00", kind, len(content)); err != nil {
		return nil, fmt.Errorf("build header: %w", err)
	}
	if _, err := b.Write(content); err != nil {
		return nil, fmt.Errorf("write content: %w", err)
	}
	raw := b.Bytes()

	sum := sha1.Sum(raw)
	sha = sum[:]
	s := hex.EncodeToString(sha)
	if _, err := r.DirPath(true, "objects", s[:2]); err != nil {
		return sha, fmt.Errorf("ensure object dir: %w", err)
	}
	full := path.Join(r.gitdir, "objects", s[:2], s[2:])

	fd, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return sha, fmt.Errorf("open object file: %w", err)
	}
	defer func() {
		if err := fd.Close(); err != nil {
			werr = fmt.Errorf("close object file: %w", err)
		}
	}()
	wr := zlib.NewWriter(fd)
	if _, err := wr.Write(raw); err != nil {
		return sha, fmt.Errorf("zlib object write: %w", err)
	}
	if err := wr.Close(); err != nil {
		return sha, fmt.Errorf("close zlib object writer: %w", err)
	}
	return sha, werr
}

func (r *Repository) ResolveRef(ref string) ([]byte, error) {
	for {
		if strings.HasPrefix(ref, "ref:") {
			raw, err := ioutil.ReadFile(ref[5:])
			if err != nil {
				return nil, fmt.Errorf("read ref %q: %w", ref[5:], err)
			}
			ref = string(raw)
		} else {
			return hex.DecodeString(ref)
		}
	}
}

const newDirPerm = 0770

var objects = map[string]func() Object{
	"commit": func() Object { return &CommitObject{} },
	"blob":   func() Object { return &BlobObject{} },
	"tree":   func() Object { return &TreeObject{} },
	"tag":    func() Object { panic("todo: tag") },
}

type Object interface {
	Deserialize([]byte) error
	Serialize() ([]byte, error)
}

type CommitObject struct {
	Header  map[string][]string
	Comment string
}

func (o *CommitObject) Deserialize(raw []byte) error {
	rd := bufio.NewReader(bytes.NewReader(raw))

	header := make(map[string][]string)
	var (
		key string
		buf = make([]byte, 0, 128)
	)

	// Header can be more complicated than what is here supported. For now
	// this is a good enough implemntation.
readHeader:
	for {
		switch c, err := rd.ReadByte(); {
		case errors.Is(err, io.EOF):
			if len(key) != 0 && len(buf) != 0 {
				header[key] = append(header[key], string(buf))
			}
			break readHeader
		case !errors.Is(err, nil):
			return err
		case c == ' ':
			if len(key) == 0 {
				key = string(buf)
				buf = buf[:0]
			} else {
				buf = append(buf, c)
			}
		case c == '\n':
			if next, err := rd.Peek(1); err == nil && next[0] == '\n' {
				// End of header.
				_, _ = rd.ReadByte()
				header[key] = append(header[key], string(buf))
				break readHeader
			} else {
				header[key] = append(header[key], string(buf))
				buf = buf[:0]
				key = ""
			}
		default:
			buf = append(buf, c)
		}
	}

	comment, err := ioutil.ReadAll(rd)
	if err != nil {
		return fmt.Errorf("comment: %w", err)
	}
	o.Header = header
	o.Comment = string(comment)
	return nil
}

func (o *CommitObject) Serialize() ([]byte, error) {
	panic("todo")
}

type TreeObject struct {
	Leafs []*TreeLeaf
}

type TreeLeaf struct {
	Mode os.FileMode
	Path string
	Sha  []byte
}

func (o *TreeObject) Deserialize(raw []byte) error {
	rd := bufio.NewReader(bytes.NewReader(raw))
	for {
		var leaf TreeLeaf
		switch mode, err := rd.ReadString(' '); {
		case errors.Is(err, nil):
			mode = mode[:len(mode)-1]
			n, err := strconv.Atoi(mode)
			if err != nil {
				return fmt.Errorf("invalid %q mode value: %w", mode, err)
			}
			leaf.Mode = os.FileMode(n)
		case errors.Is(err, io.EOF):
			return nil
		default:
			return fmt.Errorf("reading mode: %w", err)
		}

		path, err := rd.ReadString(0)
		if err != nil {
			return fmt.Errorf("read path: %w", err)
		}
		leaf.Path = path[:len(path)-1]

		sha := make([]byte, 20)
		if _, err := rd.Read(sha); err != nil {
			return fmt.Errorf("read sha: %w", err)
		}
		leaf.Sha = sha

		o.Leafs = append(o.Leafs, &leaf)
	}
}

func (o *TreeObject) Serialize() ([]byte, error) {
	var b bytes.Buffer
	for i, leaf := range o.Leafs {
		if _, err := fmt.Fprintf(&b, "%d %s\x00%s", leaf.Mode, leaf.Path, leaf.Sha); err != nil {
			return nil, fmt.Errorf("serialiez %d leaf: %w", i, err)
		}
	}
	return b.Bytes(), nil
}

type BlobObject struct {
	Data []byte
}

func (o *BlobObject) Deserialize(data []byte) error {
	o.Data = data
	return nil
}

func (o *BlobObject) Serialize() ([]byte, error) {
	return o.Data, nil
}

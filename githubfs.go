package githubfs

// Part 3 1.48
import (
	"context"
	"encoding/base64"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
)

type githubDir struct {
	tree *github.Tree
	mem.DirMap
}

//func (d *githubDir) Len() int
//func (d *githubDir) Names() []string
//func (d *githubDir) Files() []*mem.FileData
//func (d *githubDir) Add(*mem.FileData)
//func (d *githubDir) Remove(*mem.FileData)
const commitMessage string = "automatic commit"
const commitmsg string = "auto commit from git"

type githubFs struct {
	client *github.Client
	user   string
	repo   string
	branch *github.Branch
	tree   *github.Tree
	mu     sync.Mutex
}

func convstring(s string) *string {
	return &s
}

func createFile(name string) *File {
	fileData := CreateFile(name)
	file := NewFileHandle(fileData)
	return file
}

func NewGithubfs(client *github.Client, user string, repo string, branch string) (afero.Fs, error) {
	fs := &githubFs{
		client: client,
		user:   user,
		repo:   repo,
	}
	ctx := context.Background()
	var err error
	fs.branch, _, err = client.Repositories.GetBranch(ctx, user, repo, branch)
	if err != nil {
		return nil, err
	}
	//treeHash := b.Commit.Commit.Tree.GetSHA()
	err = fs.updateTree(fs.branch.Commit.Commit.Tree.GetSHA())
	if err != nil {
		return nil, err
	}
	return fs, nil
}

func (fs *githubFs) updateTree(sha string) (err error) {
	fs.tree, _, err = fs.client.Git.GetTree(context.TODO(), fs.user, fs.repo, sha, true)
	return err
}

func (fs *githubFs) Create(name string) (afero.File, error) {
	fileData := CreateFile(name)
	file := NewFileHandle(fileData)
	return file, nil
}

// Mkdir creates a directory in the filesystem, return an error if any
// happens.
func (fs *githubFs) Mkdir(name string, perm os.FileMode) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalName := strings.TrimPrefix(name, "/")
	parent := fs.findEntry(filepath.Dir(normalName))
	if normalName != "" && parent == nil {
		return afero.ErrFileNotFound
	}

	fs.tree.Entries = append(fs.tree.Entries, &github.TreeEntry{
		Type: convstring("tree"),
		Mode: convstring("040000"),
		Path: convstring(normalName),
	})
	return nil
}

// MkdirAll creates a directory path and all parents that does not exist
// yet.
func (fs *githubFs) MkdirAll(path string, perm os.FileMode) error {
	normalName := strings.TrimPrefix(path, "/")
	parentNames := strings.Split(filepath.Dir(normalName), FilePathSeparator)
	if len(parentNames) == 0 {
		return fs.Mkdir(path, perm)
	}

	for i := range parentNames {
		fs.mu.Lock()
		parentPath := strings.Join(parentNames[0:i+1], FilePathSeparator)
		parent := fs.findEntry(parentPath)
		fs.mu.Unlock()
		if parent == nil {
			err := fs.Mkdir(parentPath, perm)
			if err != nil {
				return err
			}
		}
	}
	return fs.Mkdir(path, perm)
}

func (fs *githubFs) findEntry(name string) *github.TreeEntry {
	normalName := strings.TrimPrefix(name, "/")
	for _, e := range fs.tree.Entries {
		if e.GetPath() == normalName {
			return e
		}
	}
	return nil
}

func (fs *githubFs) open(name string) (afero.File, *FileData, error) {
	normalName := strings.TrimPrefix(name, "/")
	entry := fs.findEntry(name)

	for _, e := range fs.tree.Entries {
		if e.GetPath() == normalName {
			entry = e
			break
		}
	}
	if entry == nil {
		return nil, nil, afero.ErrFileNotFound
	}
	if entry.GetType() == "blob" {
		fd := CreateFile(normalName)
		SetMode(fd, os.FileMode(int(0644)))
		f := NewFileHandle(fd)
		blob, _, err := fs.client.Git.GetBlob(context.TODO(), fs.user, fs.repo, entry.GetSHA())
		if err != nil {
			return nil, nil, err
		}
		b, _ := base64.StdEncoding.DecodeString(blob.GetContent())
		f.Write(b)
		f.Seek(0, 0)
		return f, fd, nil
	}

	dir := CreateDir(name)
	if normalName == "" {
		normalName = "."
	}
	for _, e := range fs.tree.Entries {
		if path.Dir(e.GetPath()) != normalName {
			continue
		}
		normalName := strings.TrimPrefix(e.GetPath(), path.Dir(e.GetPath())+"/")
		switch e.GetType() {
		case "blob":
			f := CreateFile(normalName)
			SetMode(f, os.FileMode(int(0644)))
			AddToMemDir(dir, f)

		case "tree":
			d := CreateDir(normalName)
			SetMode(d, os.FileMode(int(040000)))
			AddToMemDir(dir, d)
		default:
			continue
		}
	}
	return NewFileHandle(dir), dir, nil
}

// Open opens a file, returning it or an error, if any happens.
func (fs *githubFs) Open(name string) (afero.File, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	f, _, err := fs.open(name)
	return f, err
}

// OpenFile opens a file using the given flags and the given mode.
func (fs *githubFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	_, fd, err := fs.open(name)
	if fd != nil {
		SetMode(fd, perm)
	}
	return NewFileHandle(fd), err
}

func (fs *githubFs) Remove(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.remove(name)
}

// Remove removes a file identified by name, returning an error, if any
// happens.
func (fs *githubFs) remove(name string) error {
	normalName := strings.TrimPrefix(name, "/")
	entry := fs.findEntry(name)
	if entry == nil {
		return afero.ErrFileNotFound
	}
	resp, _, err := fs.client.Repositories.DeleteFile(context.TODO(), fs.user, fs.repo, normalName, &github.RepositoryContentFileOptions{
		Message: convstring(commitMessage),
		SHA:     convstring(entry.GetSHA()),
		Branch:  convstring(fs.branch.GetName()),
	})
	if err != nil {
		return err
	}

	return fs.updateTree(resp.Tree.GetSHA())
}

// RemoveAll removes a directory path and any children it contains. It
// does not fail if the path does not exist (return nil).
func (fs *githubFs) RemoveAll(path string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalName := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/")
	entry := fs.findEntry(path)
	if entry == nil {
		return afero.ErrFileNotFound
	}
	if entry.GetType() == "blob" {
		return fs.Remove(path)
	}

	for _, e := range fs.tree.Entries {
		if e.GetType() == "tree" {
			continue
		}
		if strings.HasPrefix(e.GetPath(), normalName+"/") {
			err := fs.remove(e.GetPath())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Rename renames a file.
func (fs *githubFs) Rename(oldname, newname string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	normalOld := strings.TrimPrefix(oldname, "/")
	normalNew := strings.TrimPrefix(newname, "/")
	err := fs.updateBranch()
	if err != nil {
		return err
	}
	err = fs.updateTree(fs.branch.Commit.Commit.Tree.GetSHA())
	if err != nil {
		return err
	}
	var entries []*github.TreeEntry
	for _, e := range fs.tree.Entries {
		if e.GetPath() == normalOld {
			e.Path = convstring(normalNew)
		}
		e.Content = nil
		e.URL = nil
		e.Size = nil
		entries = append(entries, e)
	}
	tree, _, err := fs.client.Git.CreateTree(context.TODO(), fs.user, fs.repo, "", entries)
	err = fs.updateTree(tree.GetSHA())
	if err != nil {
		return err
	}

	return fs.commit()
}

func (fs *githubFs) updateBranch() (err error) {
	fs.branch, _, err = fs.client.Repositories.GetBranch(context.TODO(), fs.user, fs.repo, fs.branch.GetName())
	return err
}

func (fs *githubFs) commit() error {
	commit, _, err := fs.client.Git.CreateCommit(context.TODO(), fs.user, fs.repo, &github.Commit{
		Message: convstring(commitmsg),
		Tree:    fs.tree,
		Parents: []*github.Commit{{SHA: fs.branch.GetCommit().SHA}},
	})

	if err != nil {
		return err
	}
	_, _, err = fs.client.Git.UpdateRef(context.TODO(), fs.user, fs.repo, &github.Reference{
		Ref: convstring("heads/" + fs.branch.GetName()),
		Object: &github.GitObject{
			SHA: commit.SHA,
		},
	}, false)

	return err
}

// Stat returns a FileInfo describing the named file, or an error, if any
// happens.
func (fs *githubFs) Stat(name string) (os.FileInfo, error) {
	f, err := fs.Open(name)
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

// The name of this FileSystem
func (fs *githubFs) Name() string {
	return "github"
}

//Chmod changes the mode of the named file to mode.
func (fs *githubFs) Chmod(name string, mode os.FileMode) error {
	// Not required as per our functionality.
	return nil
}

//Chtimes changes the access and modification times of the named file
func (fs *githubFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return nil
}

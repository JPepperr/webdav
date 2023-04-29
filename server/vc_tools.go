package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/net/webdav"
)

type VCHandler struct {
	fileSystemPath string
	repo           *git.Repository
	git_mutex      sync.Mutex
	webdav.Handler
}

func InitFs(pathFS, sevrverPrefix string) (string, *git.Repository, error) {
	if sevrverPrefix == "" {
		sevrverPrefix = "/root"
	}
	var repo *git.Repository
	err := os.Mkdir(pathFS, os.ModePerm|os.ModeDir)
	serverPath := path.Join(pathFS, sevrverPrefix)
	if err != nil {
		if !os.IsExist(err) {
			return "", nil, err
		}
		err = os.Mkdir(serverPath, os.ModePerm|os.ModeDir)
		if err != nil && !os.IsExist(err) {
			return "", nil, err
		}
		repo, err = git.PlainOpen(pathFS)
	} else {
		err = os.Mkdir(serverPath, os.ModePerm|os.ModeDir)
		if err != nil {
			return "", nil, err
		}

		repo, err = git.PlainInit(pathFS, false)
		if err != nil {
			return "", nil, err
		}

		initFileName := ".init"
		_, err = os.Create(path.Join(pathFS, initFileName))
		if err != nil {
			return "", nil, err
		}

		wt, err := repo.Worktree()
		if err != nil {
			return "", nil, err
		}

		err = wt.AddWithOptions(&git.AddOptions{
			Path: initFileName,
		})
		if err != nil {
			return "", nil, err
		}

		_, err = wt.Commit(time.Now().String(), &git.CommitOptions{
			All:               true,
			AllowEmptyCommits: false,
			Author:            getSignature(),
		})
		if err != nil {
			return "", nil, err
		}
	}

	return sevrverPrefix, repo, err
}

func GetBranchName(path string) string {
	hasher := md5.New()
	hasher.Write([]byte(path))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (h *VCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	status := http.StatusBadRequest
	switch r.Method {
	case "CHECKIN":
		status, err = h.handleCheckIn(w, r)
	default:
		h.Handler.ServeHTTP(w, r)
		return
	}

	if status != 0 {
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			w.Write([]byte(webdav.StatusText(status)))
		}
	}
	if h.Logger != nil {
		h.Logger(r, err)
	}
}

func (h *VCHandler) stripPrefix(p string) (string, int, error) {
	if h.Prefix == "" {
		return p, http.StatusOK, nil
	}
	if r := strings.TrimPrefix(p, h.Prefix); len(r) < len(p) {
		return r, http.StatusOK, nil
	}
	return p, http.StatusNotFound, errPrefixMismatch
}

func findETag(ctx context.Context, name string, fi os.FileInfo) (string, error) {
	if do, ok := fi.(webdav.ETager); ok {
		etag, err := do.ETag(ctx)
		if err != webdav.ErrNotImplemented {
			return etag, err
		}
	}
	return fmt.Sprintf(`"%x%x"`, fi.ModTime().UnixNano(), fi.Size()), nil
}

func (h *VCHandler) handleCheckIn(w http.ResponseWriter, r *http.Request) (status int, err error) {
	reqPath, status, err := h.stripPrefix(r.URL.Path)
	if err != nil {
		return status, err
	}
	ctx := r.Context()
	f, err := h.FileSystem.OpenFile(ctx, reqPath, os.O_RDONLY, 0)
	if err != nil {
		return http.StatusNotFound, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return http.StatusNotFound, err
	}
	if fi.IsDir() {
		return http.StatusMethodNotAllowed, nil
	}
	etag, err := findETag(ctx, reqPath, fi)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	w.Header().Set("ETag", etag)

	hash, err := h.checkIn(path.Join(h.fileSystemPath, reqPath))
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Header().Set("Version", hash)

	return http.StatusCreated, nil
}

func (h *VCHandler) checkIn(path string) (string, error) {
	brName := GetBranchName(path)
	createOption := true

	h.git_mutex.Lock()
	defer h.git_mutex.Unlock()

	wt, err := h.repo.Worktree()
	if err != nil {
		return "", err
	}

	_, err = h.repo.Storer.Reference(plumbing.NewBranchReferenceName(brName))
	if err == nil {
		createOption = false
	}
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(brName),
		Create: createOption,
		Keep:   true,
	})
	if err != nil {
		return "", err
	}

	err = wt.AddWithOptions(&git.AddOptions{
		Path: path,
	})
	if err != nil {
		return "", err
	}

	hash, err := wt.Commit(time.Now().String(), &git.CommitOptions{
		All:               true,
		AllowEmptyCommits: true,
		Author:            getSignature(),
	})
	if err != nil {
		return "", err
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Keep: true,
	})
	if err != nil {
		return "", err
	}

	return hash.String(), nil
}

func getSignature() *object.Signature {
	return &object.Signature{
		Name:  "root",
		Email: "root@root.com",
		When:  time.Now(),
	}
}

var (
	errPrefixMismatch = errors.New("webdav: prefix mismatch")
)

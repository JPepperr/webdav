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
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/zap"
	"golang.org/x/net/webdav"
)

type VCHandler struct {
	fileSystemPath           string
	versionControlSystemPath string
	reposCache               *lru.ARCCache[string, *git.Repository]
	webdav.Handler
}

func NewHandler(rootPath, fSPrefix, vCPrefix string, cacheSize int) (*VCHandler, error) {
	if fSPrefix == "" {
		fSPrefix = "/root"
	}
	if vCPrefix == "" {
		vCPrefix = "/vc_root"
	}
	fSPath := path.Join(rootPath, fSPrefix)
	vCPath := path.Join(rootPath, vCPrefix)

	err := os.Mkdir(rootPath, os.ModePerm|os.ModeDir)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	err = os.Mkdir(fSPath, os.ModePerm|os.ModeDir)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	err = os.Mkdir(vCPath, os.ModePerm|os.ModeDir)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	cache, err := lru.NewARC[string, *git.Repository](cacheSize)
	if err != nil {
		return nil, err
	}

	return &VCHandler{
		fSPath,
		vCPath,
		cache,
		webdav.Handler{
			Prefix:     "/",
			FileSystem: webdav.Dir(fSPath),
			LockSystem: webdav.NewMemLS(),
			Logger:     GetHandlerLoggingFunc(),
		},
	}, nil
}

func GetVCFileName(path string) string {
	hasher := md5.New()
	hasher.Write([]byte(path))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (h *VCHandler) getRepo(filePath string) (*git.Repository, error) {
	if cachedValue, ok := h.reposCache.Get(filePath); ok {
		Logger.Debug("CacheHit", zap.String("resource", filePath))
		return cachedValue, nil
	}
	name := GetVCFileName(filePath)
	vCDirPath := path.Join(h.versionControlSystemPath, name)

	var repo *git.Repository
	err := os.Mkdir(vCDirPath, os.ModePerm|os.ModeDir)
	if err != nil {
		if !os.IsExist(err) {
			return nil, err
		}
		repo, err = git.PlainOpen(vCDirPath)
	} else {
		vCFilePath, err := filepath.Abs(filepath.Join(vCDirPath, defaultVCFileName))
		if err != nil {
			return nil, err
		}

		err = os.Rename(filePath, vCFilePath)
		if err != nil {
			return nil, err
		}

		repo, err = git.PlainInit(vCDirPath, false)
		if err != nil {
			return nil, err
		}

		wt, err := repo.Worktree()
		if err != nil {
			return nil, err
		}

		err = wt.AddWithOptions(&git.AddOptions{
			Path: defaultVCFileName,
		})
		if err != nil {
			return nil, err
		}

		_, err = wt.Commit(time.Now().String(), &git.CommitOptions{
			All:               true,
			AllowEmptyCommits: false,
			Author:            getSignature(),
		})
		if err != nil {
			return nil, err
		}

		err = os.Symlink(vCFilePath, filePath)
		if err != nil {
			return nil, err
		}
	}
	Logger.Debug("CacheMiss", zap.String("resource", filePath))
	h.reposCache.Add(filePath, repo)
	return repo, err
}

func (h *VCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	status := http.StatusBadRequest
	switch r.Method {
	case MethodVersionControl:
		status, err = h.handleVersionControl(w, r)
	case MethodCheckout:
		status, err = h.handleCheckout(w, r)
	case MethodCheckin:
		status, err = h.handleCheckin(w, r)
	case MethodUncheckout:
		status, err = h.handleUncheckout(w, r)
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

func (h *VCHandler) lock(now time.Time, root string) (token string, err error) {
	token, err = h.LockSystem.Create(now, webdav.LockDetails{
		Root:      root,
		Duration:  -1,
		ZeroDepth: true,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (h *VCHandler) checkLocks(r *http.Request, path string) (release func(), err error) {
	now, token := time.Now(), ""
	if path != "" {
		token, err = h.lock(now, path)
		if err != nil {
			return nil, err
		}
	}

	return func() {
		if token != "" {
			h.LockSystem.Unlock(now, token)
		}
	}, nil
}

func (h *VCHandler) checkFile(w http.ResponseWriter, r *http.Request) (reqPath string, status int, err error) {
	reqPath, status, err = h.stripPrefix(r.URL.Path)
	if err != nil {
		return
	}

	ctx := r.Context()
	f, err := h.FileSystem.OpenFile(ctx, reqPath, os.O_RDONLY, 0)
	if err != nil {
		return "", http.StatusNotFound, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", http.StatusNotFound, err
	}
	if fi.IsDir() {
		return "", http.StatusMethodNotAllowed, errInvalidMethodForDir
	}

	etag, err := findETag(ctx, reqPath, fi)
	if err != nil {
		return "", http.StatusInternalServerError, err
	}
	w.Header().Set("ETag", etag)

	return reqPath, http.StatusOK, nil
}

func (h *VCHandler) handleVersionControl(w http.ResponseWriter, r *http.Request) (status int, err error) {
	reqPath, status, err := h.checkFile(w, r)
	if err != nil {
		return
	}
	hash, err := h.versionControl(path.Join(h.fileSystemPath, reqPath))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	w.Header().Set("Version", hash)
	return http.StatusOK, nil
}

func (h *VCHandler) handleCheckout(w http.ResponseWriter, r *http.Request) (status int, err error) {
	reqPath, status, err := h.checkFile(w, r)
	if err != nil {
		return
	}

	release, err := h.checkLocks(r, reqPath)
	if err != nil {
		if err == webdav.ErrLocked {
			return webdav.StatusLocked, nil
		}
		return http.StatusInternalServerError, err
	}
	defer release()

	err = h.checkout(path.Join(h.fileSystemPath, reqPath), r.Header.Get("Version"))
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return http.StatusNotAcceptable, nil
		}
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

func (h *VCHandler) handleUncheckout(w http.ResponseWriter, r *http.Request) (status int, err error) {
	reqPath, status, err := h.checkFile(w, r)
	if err != nil {
		return
	}

	release, err := h.checkLocks(r, reqPath)
	if err != nil {
		if err == webdav.ErrLocked {
			return webdav.StatusLocked, nil
		}
		return http.StatusInternalServerError, err
	}
	defer release()

	err = h.uncheckout(path.Join(h.fileSystemPath, reqPath))
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

func (h *VCHandler) handleCheckin(w http.ResponseWriter, r *http.Request) (status int, err error) {
	reqPath, status, err := h.checkFile(w, r)
	if err != nil {
		return
	}

	release, err := h.checkLocks(r, reqPath)
	if err != nil {
		if err == webdav.ErrLocked {
			return webdav.StatusLocked, nil
		}
		return http.StatusInternalServerError, err
	}
	defer release()

	hash, err := h.checkin(path.Join(h.fileSystemPath, reqPath))
	if err != nil {
		if errors.Is(err, errNeedUncheckout) {
			w.Header().Set("Allow", http.MethodGet+", "+MethodUncheckout+", "+MethodVersionControl)
			return http.StatusMethodNotAllowed, nil
		}
		return http.StatusInternalServerError, err
	}
	w.Header().Set("Version", hash)
	return http.StatusCreated, nil
}

func (h *VCHandler) versionControl(path string) (string, error) {
	repo, err := h.getRepo(path)
	if err != nil {
		return "", err
	}

	hashRef, err := repo.Head()
	if err != nil {
		return "", err
	}

	return hashRef.Hash().String(), nil
}

func (h *VCHandler) checkout(path, version string) error {
	repo, err := h.getRepo(path)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	bhash, err := repo.ResolveRevision(plumbing.Revision(version))
	if err != nil {
		return err
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Hash:   *bhash,
		Create: false,
		Force:  true,
	})

	return err
}

func (h *VCHandler) uncheckout(path string) error {
	repo, err := h.getRepo(path)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Create: false,
		Force:  true,
	})

	return err
}

func (h *VCHandler) checkin(path string) (string, error) {
	repo, err := h.getRepo(path)
	if err != nil {
		return "", err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	ref, err := repo.Head()
	if err != nil {
		return "", err
	}

	if ref.Name() != plumbing.Master {
		return "", errNeedUncheckout
	}

	err = wt.AddWithOptions(&git.AddOptions{
		Path: defaultVCFileName,
	})
	if err != nil {
		return "", err
	}

	hash, err := wt.Commit(time.Now().String(), &git.CommitOptions{
		All:               true,
		AllowEmptyCommits: false,
		Author:            getSignature(),
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
	MethodVersionControl = "VERSION-CONTROL"
	MethodCheckout       = "CHECKOUT"
	MethodCheckin        = "CHECKIN"
	MethodUncheckout     = "UNCHECKOUT"

	errPrefixMismatch      = errors.New("webdav: prefix mismatch")
	errInvalidMethodForDir = errors.New("webdav: method not allowed for collection")
	errNeedUncheckout      = errors.New("webdav: cannot checkin from checkouted version")

	defaultVCFileName = "init"
)

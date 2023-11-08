package git

import (
	"bufio"
	"fmt"
	"gopkg.in/src-d/go-git.v4/plumbing/format/gitignore"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/smecsia/welder/pkg/util"
	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
	"gopkg.in/src-d/go-git.v4/storage/filesystem/dotgit"
)

var (
	gitWorkTreeRefRegexp = regexp.MustCompile(`gitdir: (?P<root>.+)/.git/worktrees/(?P<name>.+)`)
)

const (
	refspecTag              = "+refs/tags/%s:refs/tags/%[1]s"
	refspecSingleBranch     = "+refs/heads/%s:refs/remotes/%s/%[1]s"
	refspecSingleBranchHEAD = "+HEAD:refs/remotes/%s/HEAD"
)

type Git interface {
	IsWorkTreeClean() (bool, string, error)
	HashShort() (string, error)
	Hash() (string, error)
	Branch() (string, error)
	CommitAndPush(msg string) error
	CreateTagAndPush(tagName string) error
	Root() string
	Alternates() ([]string, error)
	Worktrees() ([]string, error)
	Remotes() ([]Remote, error)
}

type Remote struct {
	Name string
	URLs []string
}

type GitImpl struct {
	RootPath   string
	PushBranch string
	Author     string
	Remote     string
}

func New(gitRoot string) Git {
	return &GitImpl{
		RootPath: gitRoot,
	}
}

func NewWithCfg(gitRoot string, author string, branch string, remote string) Git {
	return &GitImpl{
		RootPath:   gitRoot,
		Author:     author,
		PushBranch: branch,
		Remote:     remote,
	}
}

func TraverseToRoot() (Git, error) {
	startDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(startDir)
	root := filepath.Join(volume, "/")
	cwd := startDir
	_, err = os.Stat(filepath.Join(cwd, ".git"))
	for os.IsNotExist(err) && filepath.Dir(cwd) != root {
		cwd = filepath.Dir(cwd)
		_, err = os.Stat(filepath.Join(cwd, ".git"))
	}
	if filepath.Dir(cwd) == "/" {
		return nil, errors.Errorf("Could not traverse to Git root for the project in dir (started from %s)", startDir)
	}
	return New(cwd), nil
}

// GitRoot return root path
func (ctx *GitImpl) Root() string {
	return ctx.RootPath
}

// IsWorkTreeClean checks if a worktree is clean (no new changes have been produced)
func (ctx *GitImpl) IsWorkTreeClean() (bool, string, error) {
	_, wt, err := ctx.gitWorkTree()
	if err != nil {
		return false, "", err
	}

	s, err := wt.Status()
	if err != nil {
		return false, "", err
	}

	patterns, err := gitignore.ReadPatterns(osfs.New(ctx.RootPath), []string{})
	if err != nil {
		return false, s.String(), err
	}

	// workaround the issue with go git, as it does not work properly with subdirectories
	// trying all paths to exclude all marked as excluded from the root
	statusPaths := make([]string, 0)
	for statusPath := range s {
		statusPaths = util.AddIfNotExist(statusPaths, statusPath)
	}
	for _, p := range patterns {
		for statusPath := range s {
			if p.Match(strings.Split(statusPath, string(os.PathSeparator)), false) == gitignore.Exclude {
				for i, v := range statusPaths {
					if v == statusPath {
						// remove path from the list as it is excluded
						statusPaths = append(statusPaths[:i], statusPaths[i+1:]...)
						break
					}
				}
			}
		}
	}
	return len(statusPaths) == 0, strings.Join(statusPaths, "\n"), nil
}

// HashShort return first 7 letters of latest commit id
func (ctx *GitImpl) HashShort() (string, error) {
	hash, e := ctx.Hash()
	if len(hash) > 6 {
		return hash[:7], e
	} else {
		return hash, e
	}
}

// Hash returns full latest commit id
func (ctx *GitImpl) Hash() (string, error) {
	r, _, err := ctx.gitWorkTree()
	if err != nil {
		return "", err
	}
	plumbingHash, err := r.ResolveRevision(plumbing.Revision("HEAD"))
	if err != nil {
		return "", errors.Wrap(err, "unable to resolve latest git commit hash for repository")
	}
	return plumbingHash.String(), nil
}

func (ctx *GitImpl) Branch() (string, error) {
	r, _, err := ctx.gitWorkTree()
	if err != nil {
		return "", err
	}

	branches, err := r.Branches()
	if err != nil {
		return "", err
	}

	head, err := r.Head()
	if err != nil {
		return "", err
	}

	var name = ""
	err = branches.ForEach(func(branchRef *plumbing.Reference) error {
		if branchRef.Hash() == head.Hash() {
			name = strings.Replace(branchRef.Name().String(), "refs/heads/", "", 1)
			return nil
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	return name, nil

}

func (ctx *GitImpl) Remotes() ([]Remote, error) {
	r, _, err := ctx.gitWorkTree()
	if err != nil {
		return nil, err
	}
	remotes, err := r.Remotes()
	if err != nil {
		return nil, err
	}
	var res []Remote
	for _, remote := range remotes {
		res = append(res, Remote{Name: remote.Config().Name, URLs: remote.Config().URLs})
	}
	return res, nil
}

// CommitAndPush makes commit and pushes to master
func (ctx *GitImpl) CommitAndPush(msg string) error {
	r, wt, err := ctx.gitWorkTree()
	if err != nil {
		return errors.Wrapf(err, "failed to read worktree")
	}

	author := object.Signature{
		Name: ctx.Author,
		When: time.Now(),
	}
	opts := git.CommitOptions{All: true, Author: &author}
	hash, err := wt.Commit(msg, &opts)
	if err != nil {
		return errors.Wrapf(err, "failed to commit with msg=%q, author=%q", msg, author)
	}

	curUser, err := user.Current()
	if err != nil {
		return errors.Wrapf(err, "failed to detect username")
	}

	auth, err := ssh.NewSSHAgentAuth(curUser.Username)
	if err != nil {
		return errors.Wrapf(err, "failed to init SSH Aget Auth")
	}

	pushOpts := git.PushOptions{
		RemoteName: ctx.Remote,
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf(refspecSingleBranchHEAD, ctx.Remote))},
		Auth:       auth,
	}
	err = r.Push(&pushOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to push commit with hash=%q", hash)
	}
	return nil
}

// CreateTagAndPush creates new tag and pushes it to remote
func (ctx *GitImpl) CreateTagAndPush(tagName string) error {
	r, _, err := ctx.gitWorkTree()
	if err != nil {
		return err
	}
	ref, err := r.Tag(tagName)
	if err != nil {
		return errors.Wrapf(err, "failed to create tag")
	}
	curUser, err := user.Current()
	if err != nil {
		return errors.Wrapf(err, "failed to detect username")
	}

	auth, err := ssh.NewSSHAgentAuth(curUser.Username)
	if err != nil {
		return errors.Wrapf(err, "failed to init SSH Aget Auth")
	}

	refName := ref.Name()

	pushOpts := git.PushOptions{
		RemoteName: ctx.Remote,
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf(refspecTag, tagName))},
		Auth:       auth,
	}
	err = r.Push(&pushOpts)
	if err != nil {
		return errors.Wrapf(err, "failed to push tag %s", refName)
	}
	return nil
}

// Alternates returns list of alternates
func (ctx *GitImpl) Alternates() ([]string, error) {
	res := make([]string, 0)
	fs := osfs.New(path.Join(ctx.Root(), ".git"))
	dGit := dotgit.New(fs)

	alternates, err := dGit.Alternates()
	if err != nil {
		return res, errors.Wrapf(err, "could not get alternates")
	}
	for _, dGit := range alternates {
		// main alternate
		res = append(res, dGit.Fs().Root())
		// find all transients
		alternate := New(dGit.Fs().Root())
		transients, err := alternate.Alternates()
		if err != nil {
			continue
		}
		for _, transAlternate := range transients {
			res = append(res, transAlternate)
		}
	}

	return res, nil
}

// Worktrees returns list of defined worktrees
func (ctx *GitImpl) Worktrees() ([]string, error) {
	res := make([]string, 0)

	// add main tree of this worktree
	dotGitFile := path.Join(ctx.Root(), ".git")
	if info, err := os.Stat(dotGitFile); os.IsNotExist(err) {
		return res, nil
	} else if err == nil && info.Mode().IsRegular() { // this is one of worktrees itself
		file, err := os.Open(dotGitFile)
		if err != nil {
			return res, errors.Wrapf(err, "failed to read file %s", dotGitFile)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			gitDirExpr := scanner.Text()
			if !gitWorkTreeRefRegexp.Match([]byte(gitDirExpr)) {
				continue
			}
			match := util.MatchGroupsWithNames(gitWorkTreeRefRegexp, gitDirExpr)
			mainGitRoot := strings.TrimSpace(match["root"])
			// add main root
			res = append(res, mainGitRoot)

			// add all transients
			mainRoot := New(mainGitRoot)
			transients, err := mainRoot.Alternates()
			if err != nil {
				continue
			}
			for _, transWorktree := range transients {
				res = append(res, transWorktree)
			}
		}
	} else {
		return res, errors.Wrapf(err, "failed to check file %s", dotGitFile)
	}

	// add worktrees of main tree
	wtDir := path.Join(ctx.Root(), ".git", "worktrees")
	if _, err := os.Stat(wtDir); os.IsNotExist(err) || err.(*os.PathError).Err == syscall.ENOTDIR {
		return res, nil
	}
	files, err := ioutil.ReadDir(wtDir)
	if err != nil {
		return res, errors.Wrapf(err, "failed to list worktrees in %s", wtDir)
	}
	for _, f := range files {
		gitDirFile := path.Join(wtDir, f.Name(), "gitdir")
		if _, err := os.Stat(gitDirFile); os.IsNotExist(err) {
			return res, errors.Errorf("gitdir file does not exist in %s", f.Name())
		}
		gitDirBytes, err := ioutil.ReadFile(gitDirFile)
		if err != nil {
			return res, errors.Wrapf(err, "failed to read file %s", gitDirFile)
		}
		worktree := strings.TrimSpace(string(gitDirBytes))
		// add worktree itself
		res = append(res, worktree)

		// add transient worktrees
		wtRoot := New(worktree)
		transients, err := wtRoot.Alternates()
		if err != nil {
			continue
		}
		for _, transWorktree := range transients {
			res = append(res, transWorktree)
		}
	}
	return res, nil
}

func (ctx *GitImpl) gitWorkTree() (*git.Repository, *git.Worktree, error) {
	r, err := git.PlainOpen(ctx.RootPath)
	if err != nil {
		return nil, nil, err
	}
	wt, err := r.Worktree()
	if err != nil {
		return nil, nil, err
	}
	return r, wt, nil
}

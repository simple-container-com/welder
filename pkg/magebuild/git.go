package magebuild

import "fmt"

// IsWorkTreeClean checks if a worktree is clean (no new changes have been produced)
func (ctx *GoBuildContext) GitCheckWorkTree() error {
	if clean, status, err := ctx.git.IsWorkTreeClean(); err != nil {
		return err
	} else if !clean {
		return fmt.Errorf("git tree is not clean: \n%s", status)
	}
	return nil
}

// TagCommit makes tag based on current
func (ctx *GoBuildContext) TagCommit(tagName string) error {
	return ctx.git.CreateTagAndPush(tagName)
}

// CommitAndPush makes commit and pushes to master
func (ctx *GoBuildContext) GitCommitAndPush(msg string) error {
	return ctx.git.CommitAndPush(msg)
}

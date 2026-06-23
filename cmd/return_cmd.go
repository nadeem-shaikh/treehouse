package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kunchenguid/treehouse/internal/config"
	"github.com/kunchenguid/treehouse/internal/git"
	"github.com/kunchenguid/treehouse/internal/pool"
	"github.com/kunchenguid/treehouse/internal/ui"
)

var returnForce bool
var errReturnWorktreeUnmanaged = errors.New("return worktree unmanaged")

var returnCmd = &cobra.Command{
	Use:   "return [path]",
	Short: "Terminate lingering processes and return a worktree",
	RunE: func(cmd *cobra.Command, args []string) error {
		wtPath, err := resolveWorktreePath(args)
		if err != nil {
			return err
		}

		poolDir, err := resolveReturnPoolDir(wtPath, len(args) > 0)
		if err != nil {
			if errors.Is(err, errReturnWorktreeUnmanaged) {
				return fmt.Errorf("worktree %s is not managed by treehouse", wtPath)
			}
			return err
		}

		if !returnForce {
			dirty, _ := git.IsDirty(wtPath)
			unmerged, _ := git.UnmergedCommitCount(wtPath)
			if dirty || unmerged > 0 {
				if dirty {
					fmt.Fprintln(os.Stderr, "🌳 Worktree has uncommitted changes.")
				}
				if unmerged > 0 {
					fmt.Fprintf(os.Stderr, "🌳 Worktree has %d unmerged %s that would be discarded.\n", unmerged, plural("commit", unmerged))
				}

				// Default to keeping the worktree when commits would be lost.
				ok, err := ui.Confirm("Clean and return to pool?", unmerged == 0)
				if err != nil || !ok {
					fmt.Fprintln(os.Stderr, "🌳 Aborted.")
					return nil
				}
			}

			if err := git.DetachWorktree(wtPath); err != nil {
				return fmt.Errorf("failed to detach worktree HEAD: %w", err)
			}
		}

		killLingeringProcesses(wtPath)

		if err := pool.Release(poolDir, wtPath); err != nil {
			return fmt.Errorf("failed to return worktree: %w", err)
		}

		fmt.Fprintln(os.Stderr, "🌳 Worktree returned to pool.")
		return nil
	},
}

func init() {
	returnCmd.Flags().BoolVar(&returnForce, "force", false, "Clean, reset, and return without prompting")
	rootCmd.AddCommand(returnCmd)
}

func resolveWorktreePath(args []string) (string, error) {
	if len(args) > 0 {
		return filepath.Abs(args[0])
	}
	if env := os.Getenv("TREEHOUSE_DIR"); env != "" {
		return filepath.Abs(env)
	}
	return os.Getwd()
}

func resolveReturnPoolDir(wtPath string, explicitPath bool) (string, error) {
	pathPoolDir := filepath.Dir(filepath.Dir(wtPath))
	entry, err := pool.FindByPath(pathPoolDir, wtPath)
	if err != nil {
		return "", err
	}
	if entry != nil {
		return pathPoolDir, nil
	}

	var repoRoot string
	if explicitPath {
		repoRoot, err = git.FindMainRepoRootFrom(wtPath)
	} else {
		repoRoot, err = git.FindRepoRoot()
	}
	if err != nil {
		if explicitPath {
			return "", errReturnWorktreeUnmanaged
		}
		return "", fmt.Errorf("not in a git repository: %w", err)
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}

	fallbackPoolDir, err := config.ResolvePoolDir(repoRoot, cfg.Root)
	if err != nil {
		return "", err
	}

	entry, err = pool.FindByPath(fallbackPoolDir, wtPath)
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", errReturnWorktreeUnmanaged
	}
	return fallbackPoolDir, nil
}

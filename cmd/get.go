package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kunchenguid/treehouse/internal/config"
	"github.com/kunchenguid/treehouse/internal/git"
	"github.com/kunchenguid/treehouse/internal/pool"
	"github.com/kunchenguid/treehouse/internal/process"
	"github.com/kunchenguid/treehouse/internal/shell"
	"github.com/kunchenguid/treehouse/internal/ui"
)

var (
	getLease       bool
	getLeaseHolder string
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Acquire a worktree from the pool and open a subshell",
	Long: `Acquire a worktree from the pool and open a subshell in it.

Pass --lease for a non-interactive, durable acquire: treehouse reserves the
worktree, marks it leased in its persistent state, and prints only the worktree's
absolute path to stdout (all banners go to stderr). A leased worktree is never
handed out by a later get and never removed by prune, even with no process
running inside it, until you release it with 'treehouse return <path>'.`,
	RunE: getRunE,
}

func init() {
	getCmd.Flags().BoolVar(&getLease, "lease", false, "Durably lease a worktree without opening a subshell; print only its path to stdout")
	getCmd.Flags().StringVar(&getLeaseHolder, "lease-holder", "", "Optional label recorded as the lease holder (defaults to $TREEHOUSE_LEASE_HOLDER)")
	rootCmd.AddCommand(getCmd)
}

func getRunE(cmd *cobra.Command, args []string) error {
	repoRoot, err := git.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	poolDir, err := config.ResolvePoolDir(repoRoot, cfg.Root)
	if err != nil {
		return fmt.Errorf("failed to resolve pool directory: %w", err)
	}

	if err := config.EnsureGitignore(filepath.Dir(poolDir)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update .gitignore: %v\n", err)
	}

	if getLease {
		return getLeaseRunE(repoRoot, poolDir, cfg)
	}

	wtPath, err := pool.Acquire(repoRoot, poolDir, cfg.MaxTrees, cfg.Hooks.PostCreate)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "🌳 Entered worktree at %s. Type 'exit' to return.\n", ui.PrettyPath(wtPath))

	env := []string{
		"TREEHOUSE_DIR=" + wtPath,
	}
	_, err = shell.Spawn(wtPath, env)

	// Subshell exited — handle return
	if err := git.DetachWorktree(wtPath); err != nil {
		fmt.Fprintf(os.Stderr, "🌳 Warning: failed to detach worktree HEAD: %v\n", err)
	}

	dirty, _ := git.IsDirty(wtPath)
	unmerged, _ := git.UnmergedCommitCount(wtPath)
	if dirty || unmerged > 0 {
		if dirty {
			fmt.Fprintln(os.Stderr, "🌳 Worktree has uncommitted changes.")
		}
		if unmerged > 0 {
			fmt.Fprintf(os.Stderr, "🌳 Worktree has %d unmerged %s that would be discarded.\n", unmerged, plural("commit", unmerged))
		}

		// Default to keeping the worktree when commits would be lost: resetting a
		// detached HEAD discards them permanently.
		ok, promptErr := ui.Confirm("Clean worktree and return to pool?", unmerged == 0)
		if promptErr != nil || !ok {
			if dirty {
				fmt.Fprintln(os.Stderr, "🌳 Worktree left dirty. Use 'treehouse return --force' to clean it later.")
			} else {
				fmt.Fprintln(os.Stderr, "🌳 Worktree left with unmerged commits. Use 'treehouse return --force' to discard them.")
			}
			return nil
		}
	}

	if !killLingeringProcesses(wtPath) {
		return nil
	}

	if err := pool.Release(poolDir, wtPath); err != nil {
		fmt.Fprintf(os.Stderr, "🌳 Warning: failed to clean worktree: %v\n", err)
	} else {
		fmt.Fprintln(os.Stderr, "🌳 Worktree returned to pool.")
	}

	return nil
}

// getLeaseRunE performs a non-interactive, durable acquire. It reserves a
// worktree, marks it leased in persistent state, prints only the worktree path
// to stdout, and routes every human-facing message to stderr so that
// `path=$(treehouse get --lease)` works cleanly in scripts.
func getLeaseRunE(repoRoot, poolDir string, cfg config.Config) error {
	holder := getLeaseHolder
	if holder == "" {
		holder = os.Getenv("TREEHOUSE_LEASE_HOLDER")
	}

	wtPath, err := pool.AcquireLease(repoRoot, poolDir, cfg.MaxTrees, cfg.Hooks.PostCreate, holder)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "🌳 Leased worktree at %s. Run 'treehouse return %s' to release it.\n",
		ui.PrettyPath(wtPath), ui.PrettyPath(wtPath))
	// The bare path is the only thing on stdout, so callers can capture it.
	fmt.Fprintln(os.Stdout, wtPath)
	return nil
}

// killLingeringProcesses terminates any process whose cwd is within the given
// worktree, so detached tools (e.g. opencode servers that ignore SIGHUP) don't
// keep holding the worktree after it returns to the pool. It previews the
// targeted processes first and, when stdin is interactive, asks for
// confirmation before terminating. It returns false when the user declines,
// signalling the caller to leave the worktree as-is instead of returning it.
func killLingeringProcesses(wtPath string) bool {
	procs, err := process.FindTerminableWorktreeProcesses(wtPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "🌳 Warning: failed to scan for lingering processes: %v\n", err)
		return true
	}
	if len(procs) == 0 {
		return true
	}

	names := make([]string, len(procs))
	for i, p := range procs {
		names[i] = p.String()
	}
	fmt.Fprintf(os.Stderr, "🌳 Lingering processes in this worktree: %s\n", strings.Join(names, ", "))

	if ui.IsInteractive() {
		ok, promptErr := ui.Confirm("Terminate these processes?", true)
		if promptErr != nil || !ok {
			fmt.Fprintln(os.Stderr, "🌳 Left processes running; worktree not returned.")
			return false
		}
	}

	process.TerminateProcesses(procs, 2*time.Second)
	fmt.Fprintf(os.Stderr, "🌳 Terminated lingering processes: %s\n", strings.Join(names, ", "))
	return true
}

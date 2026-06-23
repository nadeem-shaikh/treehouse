package process

import (
	"os"
	"time"

	gopsutilprocess "github.com/shirou/gopsutil/v4/process"
)

// FindTerminableWorktreeProcesses returns the processes that
// TerminateWorktreeProcesses would target: those whose cwd is within the
// worktree, excluding the current process and its ancestors (so a caller
// running inside the worktree never targets its own process chain). Finding does
// not signal anything, so callers can preview or confirm before terminating.
func FindTerminableWorktreeProcesses(worktreePath string) ([]ProcessInfo, error) {
	procs, err := FindProcessesInWorktree(worktreePath)
	if err != nil {
		return nil, err
	}
	return filterProtectedProcesses(procs, int32(os.Getpid()), parentPID), nil
}

// TerminateProcesses terminates the given processes. On unix it sends SIGTERM,
// waits up to gracePeriod for them to exit, then SIGKILLs any survivors. On
// windows it uses TerminateProcess. Individual kill failures (e.g. a process
// already gone) are swallowed.
func TerminateProcesses(procs []ProcessInfo, gracePeriod time.Duration) {
	if len(procs) == 0 {
		return
	}

	pids := make([]int32, len(procs))
	for i, p := range procs {
		pids[i] = p.PID
	}

	terminate(pids, gracePeriod)
}

// TerminateWorktreeProcesses finds every process whose cwd is within the given
// worktree path (excluding the caller's own process chain) and terminates them.
//
// Returns the list of processes that were targeted. Errors only if the initial
// scan fails; individual kill failures (e.g. process already gone) are
// swallowed.
func TerminateWorktreeProcesses(worktreePath string, gracePeriod time.Duration) ([]ProcessInfo, error) {
	procs, err := FindTerminableWorktreeProcesses(worktreePath)
	if err != nil {
		return nil, err
	}
	TerminateProcesses(procs, gracePeriod)
	return procs, nil
}

func filterProtectedProcesses(procs []ProcessInfo, currentPID int32, lookupParent func(int32) (int32, error)) []ProcessInfo {
	protected := map[int32]struct{}{
		currentPID: {},
	}

	for pid := currentPID; pid > 0; {
		parent, err := lookupParent(pid)
		if err != nil {
			return nil
		}
		if parent <= 0 {
			break
		}
		if _, seen := protected[parent]; seen {
			break
		}
		protected[parent] = struct{}{}
		pid = parent
	}

	filtered := procs[:0]
	for _, proc := range procs {
		if _, skip := protected[proc.PID]; skip {
			continue
		}
		filtered = append(filtered, proc)
	}
	return filtered
}

func parentPID(pid int32) (int32, error) {
	proc, err := gopsutilprocess.NewProcess(pid)
	if err != nil {
		return 0, err
	}
	return proc.Ppid()
}

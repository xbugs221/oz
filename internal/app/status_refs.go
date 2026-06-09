// Package app resolves human-friendly status aliases for repository runtime state.
package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// StatusRef identifies one durable runtime object and its dynamic short alias.
type StatusRef struct {
	Alias string
	ID    string
}

// ListBatchRefs returns current-repository batch refs newest first.
func ListBatchRefs(repo string) ([]StatusRef, error) {
	return listStatusRefs(repo, batchesRoot, "b")
}

// ListRunRefs returns current-repository run refs newest first.
func ListRunRefs(repo string) ([]StatusRef, error) {
	return listStatusRefs(repo, runsRoot, "w")
}

// RunAliasForID returns the short workflow alias for a real run id.
func RunAliasForID(refs []StatusRef, runID string) string {
	for _, ref := range refs {
		if ref.ID == runID {
			return ref.Alias
		}
	}
	return ""
}

// ResolveStatusTarget maps wo status arguments to either a batch or workflow id.
func ResolveStatusTarget(repo string, args []string) (kind string, ref StatusRef, err error) {
	if len(args) == 0 {
		batches, err := ListBatchRefs(repo)
		if err != nil {
			return "", StatusRef{}, err
		}
		if len(batches) > 0 {
			return "batch", batches[0], nil
		}
		runs, err := ListRunRefs(repo)
		if err != nil {
			return "", StatusRef{}, err
		}
		if len(runs) > 0 {
			return "run", runs[0], nil
		}
		return "", StatusRef{}, fmt.Errorf("没有 wo run")
	}
	if len(args) != 1 {
		return "", StatusRef{}, fmt.Errorf("用法：wo status [-bN|-wN]")
	}
	arg := args[0]
	switch {
	case strings.HasPrefix(arg, "-b"):
		ref, err := resolveIndexedRef(repo, arg, "-b", ListBatchRefs)
		return "batch", ref, err
	case strings.HasPrefix(arg, "-w"):
		ref, err := resolveIndexedRef(repo, arg, "-w", ListRunRefs)
		return "run", ref, err
	default:
		return "", StatusRef{}, fmt.Errorf("用法：wo status [-bN|-wN]")
	}
}

// listStatusRefs reads runtime directories in newest-first order and assigns aliases.
func listStatusRefs(repo string, rootFn func(string) (string, error), prefix string) ([]StatusRef, error) {
	root, err := rootFn(repo)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refs := []StatusRef{}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		refs = append(refs, StatusRef{Alias: fmt.Sprintf("%s%d", prefix, len(refs)+1), ID: entry.Name()})
	}
	return refs, nil
}

// resolveIndexedRef resolves compact aliases such as -b1 or -w2.
func resolveIndexedRef(repo, arg, flag string, listFn func(string) ([]StatusRef, error)) (StatusRef, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(arg, flag))
	if err != nil || n < 1 {
		return StatusRef{}, fmt.Errorf("找不到 %s", strings.TrimPrefix(arg, "-"))
	}
	refs, err := listFn(repo)
	if err != nil {
		return StatusRef{}, err
	}
	if n > len(refs) {
		return StatusRef{}, fmt.Errorf("找不到 %s", strings.TrimPrefix(arg, "-"))
	}
	return refs[n-1], nil
}

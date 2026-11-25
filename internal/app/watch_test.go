package app

import "testing"

// As IntelliJ's GitRepositoryUpdater, the watch follows a repository's
// HEAD, index and refs, so Git status needs no polling; objects and logs,
// which change on every read, stay out.
func TestWatchFollowsGitMetadata(t *testing.T) {
	for rel, want := range map[string]bool{
		".git": false, ".git/refs": false, ".git/refs/heads/feature": false, ".git/info": false,
		"sub/.git": false, "sub/.git/refs/remotes": false,
		".git/objects": true, ".git/logs": true, "sub/.git/objects/ab": true,
		"node_modules": true, "node_modules/x/.git": true, "a.git": false, "src": false,
	} {
		if got := watchIgnored(rel, nil); got != want {
			t.Errorf("%s: ignored %v, want %v", rel, got, want)
		}
	}
}

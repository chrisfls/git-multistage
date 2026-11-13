package gitx

import "testing"

func TestLostObjectWrite(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name: "rename onto a concurrent object, as Windows CI reported it",
			stderr: "error: unable to write file .git/objects/e6/9de29bb2d1d6434b8b29ae775ad8c2e48c5391: Permission denied\n" +
				"fatal: unable to add (NULL) to database\n",
			want: true,
		},
		{
			name:   "absolute path with backslashes",
			stderr: `error: unable to write file D:\a\repo\.git\objects\e6\9de29bb2d1d6434b8b29ae775ad8c2e48c5391: Permission denied`,
			want:   true,
		},
		{
			name:   "sha256 object in a worktree's common directory",
			stderr: "error: unable to write file /r/.git/objects/47/3a0f4c3be8a93681a267e3b1e9a7dcda1185436fe141f7749120a303721813: Permission denied",
			want:   true,
		},
		{
			name:   "disk full is a real failure",
			stderr: "error: unable to write file .git/objects/e6/9de29bb2d1d6434b8b29ae775ad8c2e48c5391: No space left on device",
		},
		{
			name:   "unwritable object directory is a real failure",
			stderr: "error: insufficient permission for adding an object to repository database .git/objects",
		},
		{
			name:   "a file outside the object database",
			stderr: "error: unable to write file .git/index.lock: Permission denied",
		},
		{name: "no error output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lostObjectWrite(tt.stderr); got != tt.want {
				t.Fatalf("lostObjectWrite(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

package buildinfo

import "testing"

func TestCommitLabel(t *testing.T) {
	tests := []struct {
		name string
		info Info
		want string
	}{
		{name: "unstamped", info: Info{}, want: "unknown"},
		{name: "short revision", info: Info{Commit: "abc123"}, want: "abc123"},
		{
			name: "long revision is shortened",
			info: Info{Commit: "0123456789abcdef0123456789abcdef01234567"},
			want: "0123456789ab",
		},
		{
			name: "modified",
			info: Info{Commit: "0123456789abcdef0123456789abcdef01234567", Modified: true},
			want: "0123456789ab-modified",
		},
	}
	for _, tt := range tests {
		if got := tt.info.CommitLabel(); got != tt.want {
			t.Errorf("%s: CommitLabel %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCurrentFallsBackToDevVersion(t *testing.T) {
	if got := Current("").Version; got != "dev" {
		t.Errorf("version %q, want dev", got)
	}
	if got := Current("1.2.3").Version; got != "1.2.3" {
		t.Errorf("version %q, want 1.2.3", got)
	}
}

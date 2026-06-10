package github

import "testing"

func TestParseRepo(t *testing.T) {
	tests := []struct {
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"owner/repo", "owner", "repo", false},
		{"https://github.com/owner/repo", "owner", "repo", false},
		{"https://github.com/owner/repo.git", "owner", "repo", false},
		{"http://github.com/owner/repo", "owner", "repo", false},
		{"https://github.com/owner/repo/", "owner", "repo", false},
		{"invalid", "", "", true},
		{"", "", "", true},
		{"https://gitlab.com/owner/repo", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			owner, repo, err := ParseRepo(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRepo(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if owner != tt.wantOwner || repo != tt.wantRepo {
				t.Errorf("ParseRepo(%q) = (%q, %q), want (%q, %q)", tt.input, owner, repo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

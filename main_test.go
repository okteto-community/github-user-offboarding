package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireEnvs(t *testing.T) {
	t.Setenv("TEST_FOO", "bar")
	t.Setenv("TEST_BAZ", "qux")

	t.Run("all present", func(t *testing.T) {
		got, err := requireEnvs("TEST_FOO", "TEST_BAZ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["TEST_FOO"] != "bar" || got["TEST_BAZ"] != "qux" {
			t.Errorf("unexpected values: %v", got)
		}
	})

	t.Run("one missing", func(t *testing.T) {
		_, err := requireEnvs("TEST_FOO", "TEST_MISSING")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("all missing", func(t *testing.T) {
		_, err := requireEnvs("TEST_MISSING_A", "TEST_MISSING_B")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestParseLinkNext(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{
			name:   "next and last",
			header: `<https://api.github.com/orgs/foo/members?page=2>; rel="next", <https://api.github.com/orgs/foo/members?page=5>; rel="last"`,
			want:   "https://api.github.com/orgs/foo/members?page=2",
		},
		{
			name:   "last only",
			header: `<https://api.github.com/orgs/foo/members?page=5>; rel="last"`,
			want:   "",
		},
		{
			name:   "empty header",
			header: "",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLinkNext(tc.header); got != tc.want {
				t.Errorf("parseLinkNext(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestUsersToRemove(t *testing.T) {
	members := map[string]struct{}{
		"alice": {},
		"bob":   {},
	}

	tests := []struct {
		name          string
		users         []oktetoUser
		includeAdmins bool
		wantNames     []string
	}{
		{
			name: "removes user not in org",
			users: []oktetoUser{
				{ID: "u1", ExternalID: "alice", Name: "alice", Role: "Developer"},
				{ID: "u2", ExternalID: "charlie", Name: "charlie", Role: "Developer"},
			},
			wantNames: []string{"charlie"},
		},
		{
			name: "skips admin by default",
			users: []oktetoUser{
				{ID: "u1", ExternalID: "charlie", Name: "charlie", Role: "Admin"},
				{ID: "u2", ExternalID: "dave", Name: "dave", Role: "Developer"},
			},
			includeAdmins: false,
			wantNames:     []string{"dave"},
		},
		{
			name: "includes admin when flag set",
			users: []oktetoUser{
				{ID: "u1", ExternalID: "charlie", Name: "charlie", Role: "Admin"},
			},
			includeAdmins: true,
			wantNames:     []string{"charlie"},
		},
		{
			name: "skips user with no externalId",
			users: []oktetoUser{
				{ID: "u1", ExternalID: "", Name: "alice", Role: "Developer"},
			},
			wantNames: nil,
		},
		{
			name: "nothing to remove",
			users: []oktetoUser{
				{ID: "u1", ExternalID: "alice", Name: "alice", Role: "Developer"},
				{ID: "u2", ExternalID: "bob", Name: "bob", Role: "Developer"},
			},
			wantNames: nil,
		},
		{
			name: "case-insensitive match",
			users: []oktetoUser{
				{ID: "u1", ExternalID: "Alice", Name: "Alice", Role: "Developer"},
			},
			wantNames: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := usersToRemove(tc.users, members, tc.includeAdmins)
			if len(got) != len(tc.wantNames) {
				t.Fatalf("got %d users, want %d", len(got), len(tc.wantNames))
			}
			for i, u := range got {
				if u.Name != tc.wantNames[i] {
					t.Errorf("user[%d].Name = %q, want %q", i, u.Name, tc.wantNames[i])
				}
			}
		})
	}
}

func TestGetGitHubOrgMembers(t *testing.T) {
	page1 := []githubMember{{Login: "alice"}, {Login: "bob"}}
	page2 := []githubMember{{Login: "charlie"}}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			json.NewEncoder(w).Encode(page2)
		} else {
			w.Header().Set("Link", fmt.Sprintf(`<%s?page=2>; rel="next"`, srv.URL))
			json.NewEncoder(w).Encode(page1)
		}
	}))
	defer srv.Close()

	members, err := getGitHubOrgMembers("test-org", "test-token", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]struct{}{"alice": {}, "bob": {}, "charlie": {}}
	if len(members) != len(want) {
		t.Fatalf("got %d members, want %d", len(members), len(want))
	}
	for id := range want {
		if _, ok := members[id]; !ok {
			t.Errorf("missing member ID %s", id)
		}
	}
}

func TestGetGitHubOrgMembersError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := getGitHubOrgMembers("test-org", "bad-token", srv.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetOktetoUsers(t *testing.T) {
	want := []oktetoUser{
		{ID: "u1", ExternalID: "alice", Name: "alice", Email: "alice@example.com", Role: "Developer"},
		{ID: "u2", ExternalID: "bob", Name: "bob", Email: "bob@example.com", Role: "Admin"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v0/users" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	got, err := getOktetoUsers(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d users, want %d", len(got), len(want))
	}
	for i, u := range got {
		if u.ExternalID != want[i].ExternalID || u.Name != want[i].Name {
			t.Errorf("user[%d] = %+v, want %+v", i, u, want[i])
		}
	}
}

func TestGetOktetoUsersError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := getOktetoUsers(srv.URL, "test-token")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteOktetoUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path != "/api/v0/users/u1" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		if err := deleteOktetoUser(srv.URL, "test-token", "u1"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()

		if err := deleteOktetoUser(srv.URL, "test-token", "unknown"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

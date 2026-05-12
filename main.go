package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type githubMember struct {
	Login string `json:"login"`
}

type oktetoUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	LastSeen string `json:"lastSeen"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)

	env, err := requireEnvs("GH_TOKEN", "GH_ORG", "OKTETO_TOKEN", "OKTETO_URL")
	if err != nil {
		log.Fatal(err)
	}

	dryRun := os.Getenv("DRY_RUN") == "true"
	includeAdmins := os.Getenv("INCLUDE_ADMINS") == "true"

	log.Printf("fetching GitHub org members for %s", env["GH_ORG"])
	members, err := getGitHubOrgMembers(env["GH_ORG"], env["GH_TOKEN"])
	if err != nil {
		log.Fatalf("failed to fetch GitHub org members: %v", err)
	}
	log.Printf("found %d GitHub org members", len(members))

	log.Printf("fetching Okteto users from %s", env["OKTETO_URL"])
	users, err := getOktetoUsers(env["OKTETO_URL"], env["OKTETO_TOKEN"])
	if err != nil {
		log.Fatalf("failed to fetch Okteto users: %v", err)
	}
	log.Printf("found %d Okteto users", len(users))

	var toRemove []oktetoUser
	for _, u := range users {
		if !includeAdmins && u.Role == "Admin" {
			continue
		}
		if _, ok := members[strings.ToLower(u.Name)]; !ok {
			toRemove = append(toRemove, u)
		}
	}

	if len(toRemove) == 0 {
		log.Println("all Okteto users are active GitHub org members — nothing to do")
		return
	}

	log.Printf("%d user(s) to remove:", len(toRemove))
	for _, u := range toRemove {
		log.Printf("  - %-30s  email=%-35s  last_seen=%s", u.Name, u.Email, u.LastSeen)
	}

	if dryRun {
		log.Println("dry-run mode — no users deleted")
		return
	}

	removed, failed := 0, 0
	for _, u := range toRemove {
		if err := deleteOktetoUser(env["OKTETO_URL"], env["OKTETO_TOKEN"], u.ID); err != nil {
			log.Printf("ERROR: failed to remove %s: %v", u.Name, err)
			failed++
		} else {
			log.Printf("removed: %s (%s)", u.Name, u.Email)
			removed++
		}
	}

	log.Printf("done — removed: %d, failed: %d", removed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func requireEnvs(names ...string) (map[string]string, error) {
	result := make(map[string]string, len(names))
	var missing []string
	for _, name := range names {
		if val := os.Getenv(name); val != "" {
			result[name] = val
		} else {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	return result, nil
}

func getGitHubOrgMembers(org, token string) (map[string]struct{}, error) {
	members := make(map[string]struct{})
	nextURL := fmt.Sprintf("https://api.github.com/orgs/%s/members?per_page=100", org)
	for nextURL != "" {
		req, err := http.NewRequest(http.MethodGet, nextURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, body)
		}

		var page []githubMember
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("parsing GitHub response: %w", err)
		}
		for _, m := range page {
			members[strings.ToLower(m.Login)] = struct{}{}
		}
		nextURL = parseLinkNext(resp.Header.Get("Link"))
	}
	return members, nil
}

func getOktetoUsers(baseURL, token string) ([]oktetoUser, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v0/users", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Okteto API %d: %s", resp.StatusCode, body)
	}

	var users []oktetoUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("parsing Okteto response: %w", err)
	}
	return users, nil
}

func deleteOktetoUser(baseURL, token, id string) error {
	req, err := http.NewRequest(http.MethodDelete, strings.TrimRight(baseURL, "/")+"/api/v0/users/"+id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return nil
}

func parseLinkNext(header string) string {
	if m := linkNextRe.FindStringSubmatch(header); m != nil {
		return m[1]
	}
	return ""
}

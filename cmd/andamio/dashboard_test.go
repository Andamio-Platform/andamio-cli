package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/client"
	"github.com/Andamio-Platform/andamio-cli/internal/config"
)

// fullDashboardPayload mirrors the shape /api/v2/user/dashboard returns for an
// account that is simultaneously a teacher, a manager, a learner and a
// contributor — the case where the 1.0 rendering rules actually bite.
func fullDashboardPayload() map[string]interface{} {
	return map[string]interface{}{
		"user": map[string]interface{}{"alias": "teacher-01"},
		"counts": map[string]interface{}{
			"enrolled_courses":      float64(3),
			"completed_courses":     float64(2),
			"teaching_courses":      float64(4),
			"managing_projects":     float64(5),
			"contributing_projects": float64(6),
			"total_credentials":     float64(7),
		},
		"teacher": map[string]interface{}{
			"courses":               []interface{}{map[string]interface{}{"title": "Intro to Cardano"}},
			"total_pending_reviews": float64(2),
		},
		"student": map[string]interface{}{
			"enrolled_courses": []interface{}{
				map[string]interface{}{"title": "Secretly Enrolled Course"},
			},
		},
		"projects": map[string]interface{}{
			"managing":                  []interface{}{map[string]interface{}{"title": "Andamio Core"}},
			"total_pending_assessments": float64(1),
		},
	}
}

// The learner surface must not reappear in the tool's most-run command.
func TestPrintDashboard_OmitsLearnerContent(t *testing.T) {
	var buf strings.Builder
	printDashboard(&buf, fullDashboardPayload())
	got := buf.String()

	for _, forbidden := range []string{
		"Learning",
		"Secretly Enrolled Course",
		"enrolled",
		"completed",
		"contributing",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("dashboard renders learner/contributor content %q:\n%s", forbidden, got)
		}
	}
}

// The complement: pruning the learner surface must not have taken the roles
// 1.0 exists to serve with it.
func TestPrintDashboard_KeepsTheOneZeroRoles(t *testing.T) {
	var buf strings.Builder
	printDashboard(&buf, fullDashboardPayload())
	got := buf.String()

	for _, want := range []string{
		"teacher-01",            // identity
		"Teaching",              // section
		"Intro to Cardano",      // taught course
		"2 pending reviews",     // teacher's queue
		"Managing",              // section
		"Andamio Core",          // managed project
		"1 pending assessments", // manager's queue
		"4",                     // teaching count
		"5",                     // managing count
		"7",                     // credentials earned
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dashboard is missing 1.0 content %q:\n%s", want, got)
		}
	}
}

// A payload with no student section at all must render identically to one that
// has a student section — proof the omission is unconditional rather than a
// lucky branch.
func TestPrintDashboard_LearnerSectionMakesNoDifference(t *testing.T) {
	withStudent := fullDashboardPayload()

	withoutStudent := fullDashboardPayload()
	delete(withoutStudent, "student")

	var a, b strings.Builder
	printDashboard(&a, withStudent)
	printDashboard(&b, withoutStudent)

	if a.String() != b.String() {
		t.Errorf("presence of the student section changed the output:\n--- with ---\n%s\n--- without ---\n%s", a.String(), b.String())
	}
}

func TestPrintDashboard_EmptyPayloadDoesNotPanic(t *testing.T) {
	var buf strings.Builder
	printDashboard(&buf, map[string]interface{}{})
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("empty payload rendered content: %q", buf.String())
	}
}

// --- KTD9: the split between rendering and data --------------------------

// withTempHome points config.Load at a scratch directory holding a config that
// targets srv, so a command can be driven against an httptest server without
// touching the developer's real ~/.andamio/config.json.
func withTempHome(t *testing.T, baseURL string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".andamio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := map[string]string{"base_url": baseURL, "api_key": "test-key"}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// The dashboard payload's learner fields must survive untouched under
// --output json. Dropping the Learning section is a rendering decision; the
// CLI does not edit an API response it does not own.
func TestUserMe_JSONPassesGatewayPayloadThroughVerbatim(t *testing.T) {
	envelope := map[string]interface{}{"data": fullDashboardPayload()}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(envelope)
	}))
	defer srv.Close()

	withTempHome(t, srv.URL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	c := client.New(cfg)

	var got map[string]interface{}
	if err := c.Post(t.Context(), "/api/v2/user/dashboard", nil, &got); err != nil {
		t.Fatalf("POST dashboard: %v", err)
	}

	data, ok := got["data"].(map[string]interface{})
	if !ok {
		t.Fatal("response envelope has no data object")
	}
	student, ok := data["student"].(map[string]interface{})
	if !ok {
		t.Fatal("student section was stripped from the decoded payload; --output json must pass it through")
	}
	enrolled, ok := student["enrolled_courses"].([]interface{})
	if !ok || len(enrolled) != 1 {
		t.Errorf("enrolled_courses did not round-trip: %v", student["enrolled_courses"])
	}
	counts, _ := data["counts"].(map[string]interface{})
	if counts["enrolled_courses"] != float64(3) {
		t.Errorf("enrolled_courses count did not round-trip: %v", counts["enrolled_courses"])
	}
}

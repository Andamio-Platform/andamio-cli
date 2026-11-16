package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Andamio-Platform/andamio-cli/internal/cardano"
)

// project task verify-hash recomputes an on-chain task's hash from the user
// tasks list. For an on-chain row, assets[].name is the on-chain name bytes
// as a string, so the hex is those bytes encoded, always. The old "already
// hex?" guess left a name such as "CAFE" unencoded, and the task reported a
// hash mismatch that was not there.

func TestVerifyHashAssetNameHex(t *testing.T) {
	cases := []struct {
		source, name, want string
	}{
		{"chain_only", "XP", "5850"},
		{"merged", "CAFE", "43414645"},
		{"merged", "2024", "32303234"},
		{"", "CAFE", "43414645"}, // no source: an on-chain row
		{"chain_only", "", ""},   // policy-only token
		{"db_only", "XP", "5850"},
		{"db_only", "CAFE", "CAFE"}, // unchanged: needs a registry lookup
	}
	for _, tc := range cases {
		got, err := verifyHashAssetNameHex(tc.source, tc.name)
		if err != nil || got != tc.want {
			t.Errorf("verifyHashAssetNameHex(%q, %q) = %q, %v; want %q", tc.source, tc.name, got, err, tc.want)
		}
	}
	if _, err := verifyHashAssetNameHex("merged", "\x00\x14�\x10XP"); err == nil {
		t.Error("a name altered by invalid UTF-8 was hashed; want it reported as unverifiable")
	}
}

func TestProjectTaskVerifyHash_OnChainAssetNames(t *testing.T) {
	policy := strings.Repeat("ab", 28)
	hashFor := func(assetHex string) string {
		h, err := cardano.ComputeTaskHash(cardano.TaskData{
			ProjectContent: "Earn", ExpirationTime: 4102444800000, LovelaceAmount: 2000000,
			NativeAssets: []cardano.NativeAsset{{PolicyID: policy, TokenName: assetHex, Quantity: 5}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	row := func(index int, name, hash string) map[string]interface{} {
		return map[string]interface{}{
			"task_index": index, "task_hash": hash, "source": "merged",
			"on_chain_content": "4561726e", "expiration_posix": 4102444800000, "lovelace_amount": 2000000,
			"assets":  []interface{}{map[string]interface{}{"policy_id": policy, "name": name, "amount": "5"}},
			"content": map[string]interface{}{"title": "Earn"},
		}
	}
	resp, _ := json.Marshal(map[string]interface{}{"data": []interface{}{
		row(1, "XP", hashFor("5850")),
		row(2, "CAFE", hashFor("43414645")),
		row(3, "\x00\x14�\x10XP", hashFor("0014df105850")),
	}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/project/user/tasks/list") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	}))
	defer srv.Close()

	bin := buildTestBinary(t)
	stdout, stderr, code := runCLI(t, bin, srv.URL, "project", "task", "verify-hash", "proj-1", "--output", "json")
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, stderr)
	}
	var out struct {
		Results []struct {
			TaskIndex int    `json:"task_index"`
			Match     bool   `json:"match"`
			Error     string `json:"error"`
		} `json:"results"`
		Mismatches int `json:"mismatches"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if len(out.Results) != 3 {
		t.Fatalf("results = %+v", out.Results)
	}
	if !out.Results[0].Match || !out.Results[1].Match {
		t.Errorf("XP and CAFE rows should match: %+v", out.Results)
	}
	if out.Results[2].Match || !strings.Contains(out.Results[2].Error, "not valid UTF-8") {
		t.Errorf("altered name should be reported unverifiable, not matched: %+v", out.Results[2])
	}
}

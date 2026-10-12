// Package fixture is scanner input for schemasnapshot_test.go: one of each
// shape of json-tagged struct the CLI declares. It is parsed, never built.
package fixture

// TopLevel is the shape the scanner always handled.
type TopLevel struct {
	ID       string `json:"id"`
	internal string `json:"internal"` // unexported: not recorded
	Nested   struct {
		Value int `json:"value"`
	} `json:"nested"`
}

// NoTags has no json tags, so it is assumed never to be marshalled.
type NoTags struct {
	A string
}

func runCommand() {
	type localResult struct {
		OK bool `json:"ok"`
	}
	var session struct {
		Nonce string `json:"nonce"`
	}
	payload := struct {
		Version string `json:"version"`
	}{Version: "1"}
	rows := []struct {
		Name string `json:"name"`
	}{{Name: "a"}}
	ptr := &struct {
		Key string `json:"key"`
	}{}
	printJSON(struct {
		Inline string `json:"inline"`
	}{})
	_, _, _, _, _ = localResult{}, session, payload, rows, ptr
}

type Client struct{}

func (c *Client) Do() {
	type response struct {
		Code string `json:"code"`
	}
	_ = response{}
}

func printJSON(v any) {}

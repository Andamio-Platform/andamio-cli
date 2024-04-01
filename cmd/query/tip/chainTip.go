// Copyright 2024 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tip

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"

	"connectrpc.com/connect"
	sync "github.com/utxorpc/go-codegen/utxorpc/v1alpha/sync"
	utxorpc "github.com/utxorpc/go-sdk"
	"golang.org/x/net/http2"
)

func chainTip() {
	ctx := context.Background()
	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				// If you're also using this client for non-h2c traffic, you may want
				// to delegate to tls.Dial if the network isn't TCP or the addr isn't
				// in an allowlist.
				return net.Dial(network, addr)
			},
		},
	}
	baseUrl := "https://preview.utxorpc-v0.demeter.run"
	client := utxorpc.NewClient(httpClient, baseUrl)
	req := connect.NewRequest(&sync.FetchBlockRequest{})
	// set API key for demeter
	apiKey := os.Getenv("DEMETER_API_KEY")
	req.Header().Set("dmtr-api-key", apiKey)
	fmt.Println("connecting to utxorpc host:", baseUrl)
	chainSync, err := client.ChainSync.FetchBlock(ctx, req)
	if err != nil {
		fmt.Println(connect.CodeOf(err))
		if connectErr := new(connect.Error); errors.As(err, &connectErr) {
			fmt.Println(connectErr.Message())
			fmt.Println(connectErr.Details())
		}
		os.Exit(1)
	}
	fmt.Println("connected to utxorpc...")
	for i, blockRef := range chainSync.Msg.Block {
		fmt.Printf("Block[%d]:\n", i)
		fmt.Printf("Index: %d\n", blockRef.GetCardano().GetHeader().GetSlot())
		fmt.Printf("Hash: %x\n", blockRef.GetCardano().GetHeader().GetHash())
	}
}

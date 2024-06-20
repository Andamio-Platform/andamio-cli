// Copyright 2024 Andamio
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

package globalState

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/Andamio-Platform/andamio-cli/utils"
)

// Define the structs
type Asset struct {
	Unit   string `json:"unit"`
	Amount string `json:"amount"`
}

type DatumField struct {
	Bytes string       `json:"bytes,omitempty"`
	List  []DatumField `json:"list,omitempty"`
}

type DatumJSON struct {
	Constructor int          `json:"constructor"`
	Fields      []DatumField `json:"fields"`
}

type Datum struct {
	Type  string    `json:"type"`
	Hash  string    `json:"hash"`
	Bytes string    `json:"bytes"`
	JSON  DatumJSON `json:"json"`
}

type GlobalStateUTxO struct {
	TxHash          string  `json:"tx_hash"`
	Index           int     `json:"index"`
	Slot            int     `json:"slot"`
	Assets          []Asset `json:"assets"`
	Address         string  `json:"address"`
	Datum           Datum   `json:"datum"`
	ReferenceScript *string `json:"reference_script"` // Use *string to allow null values
	TxoutCbor       *string `json:"txout_cbor"`       // Use *string to allow null values
}

func globalState() {
	fmt.Println("Global state called")
	url := "https://dev.andamio.io/api/global-state/utxos"

	resp, err := http.Get(url)
	if err != nil {
		log.Fatalf("Failed to make GET request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	var globalStates []GlobalStateUTxO
	if err := json.Unmarshal(body, &globalStates); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	for _, instance := range globalStates {
		fmt.Printf("TxHash: %s\n", instance.TxHash)
		fmt.Printf("Index: %d\n", instance.Index)
		fmt.Printf("Slot: %d\n", instance.Slot)
		fmt.Printf("Address: %s\n", instance.Address)
		fmt.Printf("Assets:\n")
		for _, asset := range instance.Assets {
			if asset.Unit == "lovelace" {
				fmt.Printf("  - Unit: %s, Amount: %s\n", asset.Unit, asset.Amount)
			} else {
				unit, _ := utils.HexToString(asset.Unit)
				fmt.Printf("  - Unit: %s, Amount: %s\n", unit, asset.Amount)
			}
		}
		fmt.Printf("Datum:\n")
		fmt.Printf("  Type: %s\n", instance.Datum.Type)
		fmt.Printf("  Hash: %s\n", instance.Datum.Hash)
		fmt.Printf("  Bytes: %s\n", instance.Datum.Bytes)
		// fmt.Println("  JSON Fields:")
		// fmt.Print(instance.Datum.JSON)
		// Task: How to make Datum useful?
		fmt.Println("---------------------------------------------")
	}
}

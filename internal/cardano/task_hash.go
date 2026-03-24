package cardano

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// TaskData represents the fields used to compute a task hash.
// Matches the Aiken ProjectData type on-chain.
type TaskData struct {
	ProjectContent string // max 140 chars, NFC-normalized before hashing
	ExpirationTime uint64 // Unix milliseconds
	LovelaceAmount uint64 // micro-ADA
	NativeAssets   []NativeAsset
}

// NativeAsset represents a Cardano native asset attached to a task.
type NativeAsset struct {
	PolicyID  string // 56 hex chars (28 bytes)
	TokenName string // hex encoded (0-64 chars, 0-32 bytes)
	Quantity  uint64
}

// ComputeTaskHash computes the Blake2b-256 hash of task data encoded as Plutus Data CBOR.
// This matches @andamio/core's computeTaskHash and the on-chain Aiken hash_project_data.
//
// The encoding uses Plutus Data Constructor 0 (CBOR tag 121) with indefinite-length arrays:
//
//	tag(121) + 0x9f + [content_bytes, deadline_uint, lovelace_uint, tokens_list] + 0xff
func ComputeTaskHash(task TaskData) (string, error) {
	if err := validateTaskData(task); err != nil {
		return "", err
	}
	bytes := encodeTaskAsPlutusData(task)
	hash := Blake2b256(bytes)
	return hex.EncodeToString(hash), nil
}

// DebugTaskBytes returns the hex-encoded Plutus Data CBOR before hashing.
// Useful for comparing against @andamio/core's debugTaskBytes output.
func DebugTaskBytes(task TaskData) (string, error) {
	if err := validateTaskData(task); err != nil {
		return "", err
	}
	bytes := encodeTaskAsPlutusData(task)
	return hex.EncodeToString(bytes), nil
}

func validateTaskData(task TaskData) error {
	if utf8.RuneCountInString(task.ProjectContent) > 140 {
		return fmt.Errorf("project_content exceeds 140 characters (got %d)", utf8.RuneCountInString(task.ProjectContent))
	}
	for _, asset := range task.NativeAssets {
		if len(asset.PolicyID) != 56 {
			return fmt.Errorf("policyId must be 56 hex chars (got %d)", len(asset.PolicyID))
		}
		if _, err := hex.DecodeString(asset.PolicyID); err != nil {
			return fmt.Errorf("policyId contains invalid hex characters")
		}
		if len(asset.TokenName) > 64 || len(asset.TokenName)%2 != 0 {
			return fmt.Errorf("tokenName must be 0-64 hex chars with even length (got %d)", len(asset.TokenName))
		}
		if len(asset.TokenName) > 0 {
			if _, err := hex.DecodeString(asset.TokenName); err != nil {
				return fmt.Errorf("tokenName contains invalid hex characters")
			}
		}
	}
	return nil
}

func encodeTaskAsPlutusData(task TaskData) []byte {
	// NFC normalize content, then encode as UTF-8 bytes
	normalizedContent := norm.NFC.String(task.ProjectContent)
	contentBytes := []byte(normalizedContent)

	var buf []byte
	// Tag 121 (Plutus Data Constructor 0) + indefinite array start
	buf = append(buf, 0xd8, 121, 0x9f)
	// Field 1: project_content (ByteArray)
	buf = append(buf, encodeCBORBytes(contentBytes)...)
	// Field 2: expiration_time (unsigned int)
	buf = append(buf, encodeCBORUint(task.ExpirationTime)...)
	// Field 3: lovelace_amount (unsigned int)
	buf = append(buf, encodeCBORUint(task.LovelaceAmount)...)
	// Field 4: native_assets (List<FlatValue>)
	buf = append(buf, encodeTokensList(task.NativeAssets)...)
	// Break (end of indefinite array)
	buf = append(buf, 0xff)

	return buf
}

func encodeTokensList(assets []NativeAsset) []byte {
	if len(assets) == 0 {
		// Empty definite-length array
		return []byte{0x80}
	}

	// Indefinite-length array of FlatValue constructors
	buf := []byte{0x9f} // indefinite array start

	for _, asset := range assets {
		policyBytes, _ := hex.DecodeString(asset.PolicyID)
		var tokenBytes []byte
		if len(asset.TokenName) > 0 {
			tokenBytes, _ = hex.DecodeString(asset.TokenName)
		}

		// Each FlatValue is Constructor 0 with 3 fields
		buf = append(buf, 0xd8, 121, 0x9f) // tag 121, indefinite array
		buf = append(buf, encodeCBORBytes(policyBytes)...)
		buf = append(buf, encodeCBORBytes(tokenBytes)...)
		buf = append(buf, encodeCBORUint(asset.Quantity)...)
		buf = append(buf, 0xff) // break (end of FlatValue)
	}

	buf = append(buf, 0xff) // break (end of list)
	return buf
}

// encodeCBORUint encodes an unsigned integer as CBOR major type 0.
func encodeCBORUint(n uint64) []byte {
	if n < 24 {
		return []byte{byte(n)}
	} else if n < 256 {
		return []byte{0x18, byte(n)}
	} else if n < 65536 {
		return []byte{0x19, byte(n >> 8), byte(n)}
	} else if n < 4294967296 {
		buf := make([]byte, 5)
		buf[0] = 0x1a
		binary.BigEndian.PutUint32(buf[1:], uint32(n))
		return buf
	} else {
		buf := make([]byte, 9)
		buf[0] = 0x1b
		binary.BigEndian.PutUint64(buf[1:], n)
		return buf
	}
}

// encodeCBORBytes encodes a byte string as CBOR major type 2.
func encodeCBORBytes(data []byte) []byte {
	length := len(data)
	var header []byte
	if length < 24 {
		header = []byte{byte(0x40 + length)}
	} else if length < 256 {
		header = []byte{0x58, byte(length)}
	} else if length < 65536 {
		header = []byte{0x59, byte(length >> 8), byte(length)}
	} else {
		// Shouldn't happen for task data (max 140 chars content)
		header = []byte{0x59, byte(length >> 8), byte(length)}
	}
	result := make([]byte, len(header)+length)
	copy(result, header)
	copy(result[len(header):], data)
	return result
}

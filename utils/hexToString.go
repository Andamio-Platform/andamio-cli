package utils

import (
	"encoding/hex"
	"fmt"
)

func HexToString(hexStr string) (string, error) {
	// Ensure the hex string has at least 56 characters to remove
	if len(hexStr) <= 56 {
		return "", fmt.Errorf("hex string is too short")
	}

	// Remove the first 56 characters
	trimmedHexStr := hexStr[56:]

	// Convert the remaining hex characters to bytes
	bytes, err := hex.DecodeString(trimmedHexStr)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex string: %v", err)
	}

	// Convert bytes to a readable string
	readableStr := string(bytes)

	return readableStr, nil
}

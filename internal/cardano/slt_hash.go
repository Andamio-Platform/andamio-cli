package cardano

import (
	"encoding/hex"
)

// plutusChunkSize is the byte boundary at which Plutus's stringToBuiltinByteString
// switches from a definite-length CBOR byte string to an indefinite-length chunked one.
const plutusChunkSize = 64

// ComputeSltHash computes the Blake2b-256 hash of a list of SLT strings,
// matching @andamio/core's computeSltHash and the on-chain Plutus validator:
//
//	blake2b_256 $ serialiseData $ toBuiltinData $ map stringToBuiltinByteString slts
//
// The encoding is an indefinite-length CBOR array of byte strings, where
// strings longer than 64 bytes use Plutus chunked encoding.
func ComputeSltHash(slts []string) string {
	var buf []byte

	// Indefinite-length array start
	buf = append(buf, 0x9f)

	for _, slt := range slts {
		buf = append(buf, encodePlutusBuiltinByteString([]byte(slt))...)
	}

	// Break (end of indefinite array)
	buf = append(buf, 0xff)

	hash := Blake2b256(buf)
	return hex.EncodeToString(hash)
}

// encodePlutusBuiltinByteString encodes a byte slice matching Plutus's
// stringToBuiltinByteString:
//   - <= 64 bytes: regular CBOR byte string
//   - > 64 bytes: indefinite-length chunked byte string (64-byte chunks)
func encodePlutusBuiltinByteString(data []byte) []byte {
	if len(data) <= plutusChunkSize {
		return encodeCBORBytes(data)
	}

	// Indefinite-length byte string
	var buf []byte
	buf = append(buf, 0x5f) // indefinite byte string start

	for i := 0; i < len(data); i += plutusChunkSize {
		end := i + plutusChunkSize
		if end > len(data) {
			end = len(data)
		}
		buf = append(buf, encodeCBORBytes(data[i:end])...)
	}

	buf = append(buf, 0xff) // break
	return buf
}

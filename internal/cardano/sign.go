package cardano

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"

	"github.com/blinklabs-io/bursa"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// SignResult contains the output of signing a transaction.
type SignResult struct {
	SignedTx string `json:"signed_tx"`
	TxHash   string `json:"tx_hash"`
}

// LoadSigningKey loads a Cardano .skey file and returns the raw ed25519 private key and public key.
func LoadSigningKey(path string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	checkKeyFilePermissions(path)

	loaded, err := bursa.LoadKeyFromFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse .skey file: %w", err)
	}

	if len(loaded.SKey) < 32 {
		return nil, nil, fmt.Errorf("invalid signing key: expected at least 32 bytes, got %d", len(loaded.SKey))
	}

	// Bursa may return 64-byte extended keys or 32-byte seed keys.
	// ed25519.NewKeyFromSeed expects 32 bytes.
	seed := loaded.SKey
	if len(seed) > 32 {
		seed = seed[:32]
	}

	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)

	return privKey, pubKey, nil
}

// SignTransaction signs an unsigned Cardano transaction CBOR hex string.
// It extracts the body bytes without re-encoding, signs with Blake2b-256 + ed25519,
// merges the VKey witness into the existing witness set, and returns the signed tx + hash.
func SignTransaction(unsignedCBORHex string, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) (*SignResult, error) {
	txBytes, err := hex.DecodeString(unsignedCBORHex)
	if err != nil {
		return nil, fmt.Errorf("invalid CBOR hex: %w", err)
	}

	// Extract raw body bytes at index 0 of the top-level CBOR array.
	// CRITICAL: We must hash the original bytes, not re-encoded bytes.
	bodyBytes, err := extractRawArrayElement(txBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to extract transaction body: %w", err)
	}

	// Hash body with Blake2b-256
	bodyHash := blake2b256(bodyBytes)

	// Sign the hash
	signature := ed25519.Sign(privKey, bodyHash)

	// Verify before proceeding
	if !ed25519.Verify(pubKey, bodyHash, signature) {
		return nil, fmt.Errorf("signature verification failed — possible key corruption")
	}

	// Pre-flight: check required_signers if present
	checkRequiredSigners(bodyBytes, pubKey)

	// Decode the full transaction to merge witness
	signedTx, err := assembleSignedTx(txBytes, pubKey, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to assemble signed transaction: %w", err)
	}

	signedHex := hex.EncodeToString(signedTx)
	txHash := hex.EncodeToString(bodyHash)

	return &SignResult{
		SignedTx: signedHex,
		TxHash:   txHash,
	}, nil
}

// extractRawArrayElement extracts the raw CBOR bytes of element at `index`
// from a top-level CBOR array, without decoding/re-encoding the element itself.
func extractRawArrayElement(data []byte, index int) ([]byte, error) {
	// Use cbor.RawMessage to preserve exact bytes
	var rawElements []cbor.RawMessage
	if err := cbor.Unmarshal(data, &rawElements); err != nil {
		return nil, fmt.Errorf("failed to decode CBOR array: %w", err)
	}

	if index >= len(rawElements) {
		return nil, fmt.Errorf("CBOR array has %d elements, need index %d", len(rawElements), index)
	}

	return []byte(rawElements[index]), nil
}

// assembleSignedTx takes the original transaction bytes, adds the VKey witness,
// and returns the complete signed transaction CBOR.
func assembleSignedTx(txBytes []byte, pubKey ed25519.PublicKey, signature []byte) ([]byte, error) {
	// Decode into raw messages to preserve body, is_valid, and auxiliary_data bytes
	var rawElements []cbor.RawMessage
	if err := cbor.Unmarshal(txBytes, &rawElements); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %w", err)
	}

	if len(rawElements) < 4 {
		return nil, fmt.Errorf("invalid transaction: expected 4 elements, got %d", len(rawElements))
	}

	// Decode existing witness set (index 1) as a map
	var witnessMap map[uint]cbor.RawMessage
	if err := cbor.Unmarshal(rawElements[1], &witnessMap); err != nil {
		// If witness set is empty/null, start fresh
		witnessMap = make(map[uint]cbor.RawMessage)
	}

	// Build VKey witness: [vkey_bytes(32), signature_bytes(64)]
	vkeyWitness := [][]byte{[]byte(pubKey), signature}

	// Get existing VKey witnesses at map key 0, if any
	var existingVKeyWitnesses []cbor.RawMessage
	if existing, ok := witnessMap[0]; ok {
		if err := cbor.Unmarshal(existing, &existingVKeyWitnesses); err != nil {
			return nil, fmt.Errorf("failed to decode existing VKey witnesses: %w", err)
		}
	}

	// Encode our new witness
	newWitnessRaw, err := cbor.Marshal(vkeyWitness)
	if err != nil {
		return nil, fmt.Errorf("failed to encode VKey witness: %w", err)
	}

	// Append our witness, preserving existing ones as raw bytes
	allWitnesses := append(existingVKeyWitnesses, cbor.RawMessage(newWitnessRaw))

	// Encode the updated VKey witness set
	witnessSetEncoded, err := cbor.Marshal(allWitnesses)
	if err != nil {
		return nil, fmt.Errorf("failed to encode VKey witness set: %w", err)
	}
	witnessMap[0] = cbor.RawMessage(witnessSetEncoded)

	// Re-encode the full witness map
	witnessMapEncoded, err := cbor.Marshal(witnessMap)
	if err != nil {
		return nil, fmt.Errorf("failed to encode witness set: %w", err)
	}

	// Re-assemble: [original_body, updated_witnesses, is_valid, auxiliary_data]
	rawElements[1] = cbor.RawMessage(witnessMapEncoded)

	return cbor.Marshal(rawElements)
}

// blake2b256 computes the Blake2b-256 hash of data.
func blake2b256(data []byte) []byte {
	h, _ := blake2b.New256(nil) // nil key = unkeyed hash, never errors
	h.Write(data)
	return h.Sum(nil)
}

// checkRequiredSigners checks if the signing key matches the required_signers field
// in the transaction body. Prints a warning to stderr if there's a mismatch.
func checkRequiredSigners(bodyBytes []byte, pubKey ed25519.PublicKey) {
	// Decode body as a map to check for required_signers (key 14)
	var bodyMap map[uint]cbor.RawMessage
	if err := cbor.Unmarshal(bodyBytes, &bodyMap); err != nil {
		return // can't check, skip silently
	}

	requiredSignersRaw, ok := bodyMap[14]
	if !ok {
		return // no required_signers field
	}

	var requiredSigners [][]byte
	if err := cbor.Unmarshal(requiredSignersRaw, &requiredSigners); err != nil {
		return // can't decode, skip
	}

	// Compute our key hash: blake2b-224 of the verification key
	keyHash := blake2b224(pubKey)

	for _, required := range requiredSigners {
		if bytes.Equal(keyHash, required) {
			return // match found
		}
	}

	fmt.Fprintf(os.Stderr, "Warning: signing key does not match any required_signers in the transaction\n")
}

// blake2b224 computes the Blake2b-224 hash (used for Cardano key hashes).
func blake2b224(data []byte) []byte {
	h, _ := blake2b.New(28, nil) // 28 bytes = 224 bits
	h.Write(data)
	return h.Sum(nil)
}

// MessageSignResult contains the output of CIP-8 message signing.
type MessageSignResult struct {
	Signature string `json:"signature"` // COSE_Sign1 hex
	Key       string `json:"key"`       // COSE_Key hex
	KeyHash   string `json:"key_hash"`  // Blake2b-224 of pubKey, hex
}

// SignMessage produces a CIP-8/CIP-30 compatible message signature.
// It builds a COSE_Sign1 structure and a COSE_Key, matching the output
// of the CIP-30 wallet signData API.
func SignMessage(message []byte, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) (*MessageSignResult, error) {
	keyHash := blake2b224(pubKey)

	// Build protected headers as a CBOR map:
	// { 1: -8 (EdDSA), "address": keyHash }
	// Use canonical CBOR encoding for deterministic byte ordering (required for signature verification).
	protectedMap := map[interface{}]interface{}{
		uint(1):   int(-8), // algorithm: EdDSA
		"address": keyHash,
	}
	canonicalEnc, err := cbor.EncOptions{Sort: cbor.SortCanonical}.EncMode()
	if err != nil {
		return nil, fmt.Errorf("failed to create canonical encoder: %w", err)
	}
	protectedBytes, err := canonicalEnc.Marshal(protectedMap)
	if err != nil {
		return nil, fmt.Errorf("failed to encode protected headers: %w", err)
	}

	// Build SigStructure: ["Signature1", protected, external_aad, payload]
	sigStructure := []interface{}{
		"Signature1",
		protectedBytes,
		[]byte{}, // external_aad: empty
		message,  // payload: the nonce
	}
	sigStructureBytes, err := cbor.Marshal(sigStructure)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SigStructure: %w", err)
	}

	// Sign the SigStructure directly (CIP-8 signs the raw bytes, not a hash)
	signature := ed25519.Sign(privKey, sigStructureBytes)

	// Build COSE_Sign1: [protected, unprotected, payload, signature]
	// CIP-30 uses a 4-element array with Tag 18
	inner := []interface{}{
		protectedBytes,
		map[interface{}]interface{}{}, // unprotected headers: empty
		message,                       // payload
		signature,                     // signature
	}
	innerBytes, err := cbor.Marshal(inner)
	if err != nil {
		return nil, fmt.Errorf("failed to encode COSE_Sign1 inner: %w", err)
	}

	// Wrap in CBOR Tag 18 (COSE_Sign1)
	coseSign1Tagged, err := cbor.Marshal(cbor.Tag{Number: 18, Content: cbor.RawMessage(innerBytes)})
	if err != nil {
		return nil, fmt.Errorf("failed to encode COSE_Sign1 tag: %w", err)
	}

	// Build COSE_Key: { 1: 1 (OKP), 3: -8 (EdDSA), -1: 6 (Ed25519), -2: pubKey }
	coseKey := map[int]interface{}{
		1:  1,             // kty: OKP
		3:  -8,            // alg: EdDSA
		-1: 6,             // crv: Ed25519
		-2: []byte(pubKey), // x: public key bytes
	}
	coseKeyBytes, err := cbor.Marshal(coseKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode COSE_Key: %w", err)
	}

	return &MessageSignResult{
		Signature: hex.EncodeToString(coseSign1Tagged),
		Key:       hex.EncodeToString(coseKeyBytes),
		KeyHash:   hex.EncodeToString(keyHash),
	}, nil
}

// checkKeyFilePermissions warns if the .skey file has overly permissive permissions.
func checkKeyFilePermissions(path string) {
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}

	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s has permissions %o — consider restricting to 0600\n", path, mode)
	}
}

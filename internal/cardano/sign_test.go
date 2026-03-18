package cardano

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// buildTestTransaction creates a minimal valid Cardano transaction CBOR for testing.
// Structure: [body, witness_set, is_valid, auxiliary_data]
func buildTestTransaction(t *testing.T) ([]byte, []byte) {
	t.Helper()

	// Minimal tx body: map with inputs (key 0) and outputs (key 1)
	body := map[uint]interface{}{
		0: []interface{}{}, // inputs (empty for test)
		1: []interface{}{}, // outputs (empty for test)
		2: uint(200000),    // fee
	}

	bodyBytes, err := cbor.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}

	// Empty witness set
	witnessSet := map[uint]interface{}{}

	// Full transaction: [body, witness_set, true, null]
	tx := []interface{}{
		cbor.RawMessage(bodyBytes),
		witnessSet,
		true,
		nil,
	}

	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		t.Fatalf("failed to marshal transaction: %v", err)
	}

	return txBytes, bodyBytes
}

func TestExtractRawArrayElement(t *testing.T) {
	txBytes, originalBodyBytes := buildTestTransaction(t)

	// Extract body (index 0) from the transaction
	extractedBody, err := extractRawArrayElement(txBytes, 0)
	if err != nil {
		t.Fatalf("extractRawArrayElement failed: %v", err)
	}

	// The extracted bytes must exactly match the original body bytes.
	// This is the CRITICAL test: if CBOR re-encodes, hashes will differ.
	if hex.EncodeToString(extractedBody) != hex.EncodeToString(originalBodyBytes) {
		t.Errorf("extracted body bytes differ from original\n  got:  %s\n  want: %s",
			hex.EncodeToString(extractedBody),
			hex.EncodeToString(originalBodyBytes))
	}
}

func TestExtractRawArrayElement_OutOfBounds(t *testing.T) {
	txBytes, _ := buildTestTransaction(t)

	_, err := extractRawArrayElement(txBytes, 10)
	if err == nil {
		t.Fatal("expected error for out-of-bounds index")
	}
}

func TestExtractRawArrayElement_InvalidCBOR(t *testing.T) {
	_, err := extractRawArrayElement([]byte{0xFF, 0xFF}, 0)
	if err == nil {
		t.Fatal("expected error for invalid CBOR")
	}
}

func TestBlake2b256(t *testing.T) {
	// Known test vector: empty input
	hash := blake2b256([]byte{})
	expected := "0e5751c026e543b2e8ab2eb06099daa1d1e5df47778f7787faab45cdf12fe3a8"
	got := hex.EncodeToString(hash)
	if got != expected {
		t.Errorf("blake2b256 of empty input\n  got:  %s\n  want: %s", got, expected)
	}

	// Verify hash is 32 bytes
	if len(hash) != 32 {
		t.Errorf("expected 32-byte hash, got %d bytes", len(hash))
	}
}

func TestBlake2b224(t *testing.T) {
	hash := blake2b224([]byte{})
	// Verify hash is 28 bytes (224 bits)
	if len(hash) != 28 {
		t.Errorf("expected 28-byte hash, got %d bytes", len(hash))
	}
}

func TestSignTransaction_RoundTrip(t *testing.T) {
	txBytes, _ := buildTestTransaction(t)
	txHex := hex.EncodeToString(txBytes)

	// Generate a test key pair
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	result, err := SignTransaction(txHex, priv, pub)
	if err != nil {
		t.Fatalf("SignTransaction failed: %v", err)
	}

	// Verify result has non-empty fields
	if result.SignedTx == "" {
		t.Error("SignedTx is empty")
	}
	if result.TxHash == "" {
		t.Error("TxHash is empty")
	}

	// TxHash should be 64 hex chars (32 bytes)
	if len(result.TxHash) != 64 {
		t.Errorf("expected 64-char tx hash, got %d chars", len(result.TxHash))
	}

	// Signed tx should be valid hex
	signedBytes, err := hex.DecodeString(result.SignedTx)
	if err != nil {
		t.Fatalf("signed tx is not valid hex: %v", err)
	}

	// Signed tx should be valid CBOR with 4 elements
	var signedElements []cbor.RawMessage
	if err := cbor.Unmarshal(signedBytes, &signedElements); err != nil {
		t.Fatalf("signed tx is not valid CBOR array: %v", err)
	}
	if len(signedElements) != 4 {
		t.Errorf("expected 4 elements in signed tx, got %d", len(signedElements))
	}

	// Witness set should contain our VKey witness
	var witnessMap map[uint]cbor.RawMessage
	if err := cbor.Unmarshal(signedElements[1], &witnessMap); err != nil {
		t.Fatalf("failed to decode witness set: %v", err)
	}

	vkeyWitnessesRaw, ok := witnessMap[0]
	if !ok {
		t.Fatal("witness set missing VKey witnesses at key 0")
	}

	var vkeyWitnesses [][]cbor.RawMessage
	if err := cbor.Unmarshal(vkeyWitnessesRaw, &vkeyWitnesses); err != nil {
		t.Fatalf("failed to decode VKey witnesses: %v", err)
	}

	if len(vkeyWitnesses) != 1 {
		t.Errorf("expected 1 VKey witness, got %d", len(vkeyWitnesses))
	}
}

func TestSignTransaction_InvalidHex(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	_, err := SignTransaction("not-hex", priv, pub)
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestSignTransaction_InvalidCBOR(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	_, err := SignTransaction("ffff", priv, pub)
	if err == nil {
		t.Fatal("expected error for invalid CBOR")
	}
}

func TestAssembleSignedTx_PreservesExistingWitnesses(t *testing.T) {
	// Build a transaction with pre-existing script witnesses (key 1 in witness set)
	body := map[uint]interface{}{
		0: []interface{}{},
		1: []interface{}{},
		2: uint(200000),
	}
	bodyBytes, _ := cbor.Marshal(body)

	// Witness set with a script witness at key 1
	scriptWitness := []byte{0x01, 0x02, 0x03}
	scriptWitnessEncoded, _ := cbor.Marshal(scriptWitness)
	witnessSet := map[uint]cbor.RawMessage{
		1: cbor.RawMessage(scriptWitnessEncoded), // native scripts
	}

	tx := []interface{}{
		cbor.RawMessage(bodyBytes),
		witnessSet,
		true,
		nil,
	}
	txBytes, _ := cbor.Marshal(tx)

	// Sign it
	pub, _, _ := ed25519.GenerateKey(nil)
	signature := make([]byte, 64) // dummy signature

	signedTx, err := assembleSignedTx(txBytes, pub, signature)
	if err != nil {
		t.Fatalf("assembleSignedTx failed: %v", err)
	}

	// Decode and verify both witnesses exist
	var signedElements []cbor.RawMessage
	cbor.Unmarshal(signedTx, &signedElements)

	var resultWitnesses map[uint]cbor.RawMessage
	cbor.Unmarshal(signedElements[1], &resultWitnesses)

	if _, ok := resultWitnesses[0]; !ok {
		t.Error("VKey witnesses (key 0) missing from signed tx")
	}
	if _, ok := resultWitnesses[1]; !ok {
		t.Error("Script witnesses (key 1) were removed — should have been preserved")
	}
}


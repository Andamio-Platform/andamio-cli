package cardano

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"strings"

	"filippo.io/edwards25519"
	"github.com/blinklabs-io/bursa"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// SignResult contains the output of signing a transaction.
type SignResult struct {
	SignedTx string `json:"signed_tx"`
	TxHash   string `json:"tx_hash"`
}

// SigningKey wraps a loaded Cardano signing key. Bursa writes two distinct
// on-disk key shapes and they are NOT interchangeable at the crypto layer:
//
//   - Standard ("PaymentSigningKeyShelley_ed25519"): a 32-byte seed. RFC 8032
//     derives the actual (scalar, prefix) pair from it via SHA-512 — this is
//     what crypto/ed25519's Sign/NewKeyFromSeed already implement correctly.
//   - Extended/BIP32 ("PaymentExtendedSigningKeyShelley_ed25519_bip32", as
//     written by 'wallet create' for every mnemonic-derived wallet): the
//     on-disk bytes already ARE the derived scalar (kL, 32B) and nonce
//     prefix (kR, 32B) — there is no seed to hash. Feeding kL through
//     NewKeyFromSeed re-hashes it via SHA-512 as if it were a fresh seed,
//     silently producing a completely different (and wrong) keypair than
//     the one that actually owns the wallet's address. That mismatch is
//     invisible locally — SignTransaction's own ed25519.Verify self-check
//     still passes, because the wrongly-derived priv/pub pair is at least
//     internally consistent — and only surfaces on-chain as
//     MissingVKeyWitnessesUTXOW when the network checks the signature
//     against the address's real key hash.
//
// Extended keys need their own signing path (signExtended) that uses kL/kR
// directly instead of routing through crypto/ed25519.
type SigningKey struct {
	extended bool
	scalar   []byte // kL, 32 bytes — extended keys only
	prefix   []byte // kR, 32 bytes — extended keys only
	std      ed25519.PrivateKey
	PubKey   ed25519.PublicKey
}

// Sign produces a 64-byte Ed25519 signature over message, using whichever
// derivation this key actually requires.
func (k *SigningKey) Sign(message []byte) []byte {
	if k.extended {
		return signExtended(k.scalar, k.prefix, k.PubKey, message)
	}
	return ed25519.Sign(k.std, message)
}

// NewStandardSigningKey wraps an already-loaded standard (non-extended)
// Ed25519 key pair as a SigningKey. For callers that hold a raw keypair
// directly rather than a .skey path — tests, mainly.
func NewStandardSigningKey(priv ed25519.PrivateKey) *SigningKey {
	return &SigningKey{std: priv, PubKey: priv.Public().(ed25519.PublicKey)}
}

// LoadSigningKey loads a Cardano .skey file (standard or BIP32-extended)
// and returns a SigningKey ready to sign with the correct algorithm for
// its actual key shape.
func LoadSigningKey(path string) (*SigningKey, error) {
	checkKeyFilePermissions(path)

	loaded, err := bursa.LoadKeyFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .skey file: %w", err)
	}

	// Dispatch on the key's declared type, not on SKey's byte length:
	// Bursa's LoadKeyFromFile returns a 64-byte SKey for BOTH a genuine
	// plain-seed key (Go's ed25519.NewKeyFromSeed output is seed(32) ||
	// pubkey(32) = 64 bytes) and would return the same shape for other
	// non-extended types — 64 bytes alone doesn't imply extended. Only a
	// real BIP32-extended key decodes to a 96-byte SKey (privKey(64:
	// kL||kR) || chainCode(32)), via decodeExtendedCborKey — and the
	// on-disk "type" field is what actually says which one it is.
	if strings.HasSuffix(loaded.Type, "_bip32") {
		if len(loaded.VKey) != 32 || len(loaded.SKey) < 64 {
			return nil, fmt.Errorf("invalid extended signing key %q: unexpected key shape (vkey=%dB, skey=%dB)", loaded.Type, len(loaded.VKey), len(loaded.SKey))
		}
		return &SigningKey{
			extended: true,
			scalar:   append([]byte{}, loaded.SKey[0:32]...),
			prefix:   append([]byte{}, loaded.SKey[32:64]...),
			PubKey:   ed25519.PublicKey(loaded.VKey),
		}, nil
	}

	if len(loaded.SKey) < 32 {
		return nil, fmt.Errorf("invalid signing key: expected at least 32 bytes, got %d", len(loaded.SKey))
	}

	// Standard (non-extended) key: a plain 32-byte seed.
	seed := loaded.SKey
	if len(seed) > 32 {
		seed = seed[:32]
	}
	privKey := ed25519.NewKeyFromSeed(seed)
	return &SigningKey{
		std:    privKey,
		PubKey: privKey.Public().(ed25519.PublicKey),
	}, nil
}

// signExtended implements BIP32-Ed25519 ("extended key") signing: kL/kR are
// used directly rather than being re-derived from a seed via SHA-512. This
// is the standard extended-Ed25519 scheme (Khovratovich & Law), the same
// algorithm cardano-crypto/cardano-serialization-lib use for HD wallet
// keys:
//
//	r     = SHA512(kR || message)              (mod L)
//	R     = r*B                                (point, encoded 32B)
//	hram  = SHA512(R || A || message)          (mod L), A = public key
//	S     = r + hram*kL                        (mod L)
//	sig   = R || S
//
// kL (32 bytes) is a clamped scalar, not necessarily < L, but scalar
// multiplication on a group of order L is periodic mod L, so reducing it
// mod L first (via SetUniformBytes on a zero-padded 64-byte buffer) is
// safe and required before it can be used in edwards25519.Scalar
// arithmetic, which only accepts canonical (< L) values.
func signExtended(scalar, prefix, pubKey, message []byte) []byte {
	kLBuf := make([]byte, 64)
	copy(kLBuf, scalar)
	kL, err := edwards25519.NewScalar().SetUniformBytes(kLBuf)
	if err != nil {
		panic(fmt.Sprintf("signExtended: reducing kL mod L: %v", err))
	}

	rHash := sha512.Sum512(append(append([]byte{}, prefix...), message...))
	r, err := edwards25519.NewScalar().SetUniformBytes(rHash[:])
	if err != nil {
		panic(fmt.Sprintf("signExtended: reducing r mod L: %v", err))
	}

	R := new(edwards25519.Point).ScalarBaseMult(r)
	RBytes := R.Bytes()

	hramInput := append(append(append([]byte{}, RBytes...), pubKey...), message...)
	hramHash := sha512.Sum512(hramInput)
	hram, err := edwards25519.NewScalar().SetUniformBytes(hramHash[:])
	if err != nil {
		panic(fmt.Sprintf("signExtended: reducing hram mod L: %v", err))
	}

	S := edwards25519.NewScalar().MultiplyAdd(hram, kL, r)

	sig := make([]byte, 64)
	copy(sig[:32], RBytes)
	copy(sig[32:], S.Bytes())
	return sig
}

// SignTransaction signs an unsigned Cardano transaction CBOR hex string.
// It extracts the body bytes without re-encoding, signs with Blake2b-256 + ed25519,
// merges the VKey witness into the existing witness set, and returns the signed tx + hash.
func SignTransaction(unsignedCBORHex string, key *SigningKey) (*SignResult, error) {
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
	bodyHash := Blake2b256(bodyBytes)

	// Sign the hash
	signature := key.Sign(bodyHash)

	// Verify before proceeding
	if !ed25519.Verify(key.PubKey, bodyHash, signature) {
		return nil, fmt.Errorf("signature verification failed — possible key corruption")
	}

	// Pre-flight: check required_signers if present
	checkRequiredSigners(bodyBytes, key.PubKey)

	// Decode the full transaction to merge witness
	signedTx, err := assembleSignedTx(txBytes, key.PubKey, signature)
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

// Blake2b256 computes the Blake2b-256 hash of data.
func Blake2b256(data []byte) []byte {
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
func SignMessage(message []byte, key *SigningKey) (*MessageSignResult, error) {
	pubKey := key.PubKey
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
	signature := key.Sign(sigStructureBytes)

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

package script

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// Test Helpers
// =============================================================================

// buildScript constructs a Bitcoin script from opcodes and data
func buildScript(elements []byte) []byte {
	return elements
}

// buildEnvelope creates a minimal inscription envelope script
// Format: OP_FALSE OP_IF "ord" <fields...> OP_ENDIF
func buildEnvelope(fields ...[]byte) []byte {
	var script []byte

	// OP_FALSE
	script = append(script, OP_FALSE)

	// OP_IF
	script = append(script, OP_IF)

	// "ord" protocol identifier
	script = append(script, OP_DATA_3)
	script = append(script, []byte("ord")...)

	// Add fields
	for _, field := range fields {
		script = append(script, field...)
	}

	// OP_ENDIF
	script = append(script, OP_ENDIF)

	return script
}

// pushData creates a pushdata instruction for data
func pushData(data []byte) []byte {
	var instr []byte
	if len(data) <= OP_DATA_75 {
		instr = append(instr, byte(len(data)))
	} else if len(data) <= 0xff {
		instr = append(instr, OP_PUSHDATA1)
		instr = append(instr, byte(len(data)))
	} else if len(data) <= 0xffff {
		instr = append(instr, OP_PUSHDATA2)
		instr = append(instr, byte(len(data)))
		instr = append(instr, byte(len(data)>>8))
	} else {
		instr = append(instr, OP_PUSHDATA4)
		// Little-endian length
		instr = append(instr, byte(len(data)))
		instr = append(instr, byte(len(data)>>8))
		instr = append(instr, byte(len(data)>>16))
		instr = append(instr, byte(len(data)>>24))
	}
	instr = append(instr, data...)
	return instr
}

// pushOpcode creates an instruction for a direct opcode value (OP_1, OP_2, etc.)
func pushOpcode(opcode byte) []byte {
	return []byte{opcode}
}

func firstExtractPkScriptForNFT(pkScript []byte) (nft *NFTData, hasNFT bool) {
	nfts := ExtractPkScriptForNFTJubilee(pkScript)
	if len(nfts) == 0 {
		return nil, false
	} else {
		return nfts[0], true
	}
}

// TestEmptyScript tests that empty scripts return no inscription
func TestEmptyScript(t *testing.T) {
	script := []byte{}
	nft, hasNFT := ExtractPkScriptForNFT(script)
	if hasNFT {
		t.Errorf("Empty script should not have NFT, got: %+v", nft)
	}
}

// TestMinimalEnvelope tests a minimal valid inscription envelope
func TestMinimalEnvelope(t *testing.T) {
	// OP_FALSE OP_IF "ord" OP_ENDIF
	script := buildEnvelope()
	nft, hasNFT := ExtractPkScriptForNFT(script)

	if !hasNFT {
		t.Fatal("Minimal envelope should be parsed as NFT")
	}

	// Verify basic structure
	if nft == nil {
		t.Fatal("NFT data should not be nil")
	}
}

// TestEnvelopeWithContentType tests parsing content type tag
func TestEnvelopeWithContentType(t *testing.T) {
	// OP_FALSE OP_IF "ord" OP_DATA_1 1 "text/plain;charset=utf-8" OP_DATA_1 0 "ord" OP_ENDIF
	script := buildEnvelope(
		pushData([]byte{1}), // content type tag
		pushData([]byte("text/plain;charset=utf-8")),
		pushData([]byte{}), // body tag
		pushData([]byte("ord")),
	)

	nft, hasNFT := ExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Envelope with content type should be parsed as NFT")
	}

	if string(nft.ContentType) != "text/plain;charset=utf-8" {
		t.Errorf("ContentType = %s, want %s", string(nft.ContentType), "text/plain;charset=utf-8")
	}

	if string(nft.ContentBody) != "ord" {
		t.Errorf("ContentBody = %s, want %s", string(nft.ContentBody), "ord")
	}
}

// TestEnvelopeWithContentEncoding tests parsing content encoding tag
func TestEnvelopeWithContentEncoding(t *testing.T) {
	script := buildEnvelope(
		pushData([]byte{1}), // content type tag
		pushData([]byte("text/plain;charset=utf-8")),
		pushData([]byte{9}), // content encoding tag
		pushData([]byte("br")),
		pushData([]byte{}), // body tag
		pushData([]byte("ord")),
	)

	nft, hasNFT := firstExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Envelope with content encoding should be parsed as NFT")
	}

	if string(nft.ContentEncoding) != "br" {
		t.Errorf("ContentEncoding = %s, want %s", string(nft.ContentEncoding), "br")
	}
}

// TestEnvelopeWithMetadata tests parsing metadata tag
func TestEnvelopeWithMetadata(t *testing.T) {
	metadata := []byte{0x44, 0x00, 0x01, 0x02, 0x03}
	script := buildEnvelope(
		pushData([]byte{5}), // metadata tag
		pushData(metadata),
	)

	nft, hasNFT := firstExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Envelope with metadata should be parsed as NFT")
	}

	if !bytes.Equal(nft.Metadata, metadata) {
		t.Errorf("Metadata = %x, want %x", nft.Metadata, metadata)
	}
}

// TestEnvelopeWithPointer tests parsing pointer tag
func TestEnvelopeWithPointer(t *testing.T) {
	pointer := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	script := buildEnvelope(
		pushData([]byte{2}), // pointer tag
		pushData(pointer),
	)

	nft, hasNFT := firstExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Envelope with pointer should be parsed as NFT")
	}

	if nft.Pointer != 1 {
		t.Errorf("Pointer = %d, want 1", nft.Pointer)
	}
}

// TestEnvelopeWithParent tests parsing parent tag
func TestEnvelopeWithParent(t *testing.T) {
	// Valid parent: 32-byte txid + 1-byte index
	parent := make([]byte, 33)
	for i := 0; i < 32; i++ {
		parent[i] = 0xff
	}
	parent[32] = 0x01

	script := buildEnvelope(
		pushData([]byte{3}), // parent tag
		pushData(parent),
	)

	nft, hasNFT := firstExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Envelope with parent should be parsed as NFT")
	}

	if len(nft.Parents) != 1 {
		t.Errorf("Parents count = %d, want 1", len(nft.Parents))
	}
}

// TestEnvelopeWithBody tests parsing body tag
func TestEnvelopeWithBody(t *testing.T) {
	// OP_FALSE OP_IF "ord" OP_DATA_1 1 "text/plain;charset=utf-8" OP_DATA_1 0 "ord" OP_ENDIF
	script := buildEnvelope(
		pushData([]byte{0}), // not body tag
		pushData([]byte("ord")),
	)

	nft, hasNFT := firstExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Envelope with content type should be parsed as NFT")
	}

	if string(nft.ContentBody) != "" || !nft.IsUnrecognizedEven {
		t.Errorf("ContentBody = %s, want %s", string(nft.ContentBody), "ord")
		data, _ := json.Marshal(nft)
		t.Logf("nft: %s", strings.ReplaceAll(string(data), ",", "\n"))
	}
}

// TestBodyParsing tests various body content scenarios
func TestBodyParsing(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantBody string
	}{
		{"empty body", []byte{}, ""},
		{"short body", []byte("ord"), "ord"},
		{"longer body", []byte("hello world"), "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := buildEnvelope(
				pushData([]byte{}), // body tag
				pushData(tt.body),
			)

			nft, hasNFT := ExtractPkScriptForNFT(script)
			if !hasNFT {
				t.Fatal("Envelope with body should be parsed as NFT")
			}

			if string(nft.ContentBody) != tt.wantBody {
				t.Errorf("ContentBody = %s, want %s", string(nft.ContentBody), tt.wantBody)
			}
		})
	}
}

// TestInvalidProtocolIdentifier tests that wrong protocol IDs are rejected
func TestInvalidProtocolIdentifier(t *testing.T) {
	var script []byte
	script = append(script, OP_FALSE)
	script = append(script, OP_IF)
	script = append(script, OP_DATA_3)
	script = append(script, []byte("foo")...) // Wrong protocol
	script = append(script, OP_ENDIF)

	nft, hasNFT := ExtractPkScriptForNFT(script)
	if hasNFT {
		t.Errorf("Wrong protocol ID should not be parsed as NFT, got: %+v", nft)
	}
}

// TestNoOP_FALSE tests scripts without OP_FALSE
func TestNoOP_FALSE(t *testing.T) {
	// OP_IF "ord" OP_ENDIF (missing OP_FALSE)
	var script []byte
	script = append(script, OP_IF)
	script = append(script, OP_DATA_3)
	script = append(script, []byte("ord")...)
	script = append(script, OP_ENDIF)

	nft, hasNFT := ExtractPkScriptForNFT(script)
	if hasNFT {
		t.Errorf("Script without OP_FALSE should not be parsed as NFT, got: %+v", nft)
	}
}

// TestNoOP_IF tests scripts without OP_IF
func TestNoOP_IF(t *testing.T) {
	// OP_FALSE "ord" OP_ENDIF (missing OP_IF)
	var script []byte
	script = append(script, OP_FALSE)
	script = append(script, OP_DATA_3)
	script = append(script, []byte("ord")...)
	script = append(script, OP_ENDIF)

	nft, hasNFT := ExtractPkScriptForNFT(script)
	if hasNFT {
		t.Errorf("Script without OP_IF should not be parsed as NFT, got: %+v", nft)
	}
}

// TestNoOP_ENDIF tests scripts without OP_ENDIF
func TestNoOP_ENDIF(t *testing.T) {
	// OP_FALSE OP_IF "ord" (missing OP_ENDIF)
	script := buildEnvelope()
	// Remove OP_ENDIF
	script = script[:len(script)-1]

	nft, hasNFT := ExtractPkScriptForNFT(script)
	if hasNFT {
		t.Errorf("Script without OP_ENDIF should not be parsed as NFT, got: %+v", nft)
	}
}

// TestPUSHDATA2EmptyStringAsBodyTag tests the specific case from Rust tests
// where PUSHDATA2 is used to push an empty string as the body tag
func TestPUSHDATA2EmptyStringAsBodyTag(t *testing.T) {
	var script []byte

	// OP_FALSE
	script = append(script, OP_FALSE)

	// PUSHDATA2 0x0000 (empty string using PUSHDATA2)
	script = append(script, OP_PUSHDATA2)
	script = append(script, 0x00)
	script = append(script, 0x00)

	// OP_IF
	script = append(script, OP_IF)

	// Push "ord" using PUSHDATA2
	script = append(script, OP_PUSHDATA2)
	script = append(script, 0x03)
	script = append(script, 0x00)
	script = append(script, []byte("ord")...)

	// Push pointer tag (0x02)
	script = append(script, OP_PUSHDATA2)
	script = append(script, 0x01)
	script = append(script, 0x00)
	script = append(script, 0x02)

	// Push pointer value (1, 1)
	script = append(script, 0x01)
	script = append(script, 0x01)

	// Push content type tag (0x01)
	script = append(script, OP_PUSHDATA2)
	script = append(script, 0x01)
	script = append(script, 0x00)
	script = append(script, 0x01)

	// Push "text/plain"
	script = append(script, OP_PUSHDATA2)
	script = append(script, 0x0a)
	script = append(script, 0x00)
	script = append(script, []byte("text/plain")...)

	// PUSHDATA2 empty string as body tag
	script = append(script, OP_PUSHDATA2)
	script = append(script, 0x00)
	script = append(script, 0x00)

	// Push body content "Hello"
	script = append(script, 0x05)
	script = append(script, []byte("Hello")...)

	// OP_ENDIF
	script = append(script, OP_ENDIF)

	nft, hasNFT := firstExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("PUSHDATA2 empty string body tag should be parsed as NFT")
	}

	if string(nft.ContentType) != "text/plain" {
		t.Errorf("ContentType = %s, want text/plain", string(nft.ContentType))
	}

	if string(nft.ContentBody) != "Hello" {
		t.Errorf("ContentBody = %s, want Hello", string(nft.ContentBody))
	}

	if nft.Pointer != 1 {
		t.Errorf("Pointer = %d, want 1", nft.Pointer)
	}
}

// TestJubileeEmptyScript tests that empty scripts return no inscriptions
func TestJubileeEmptyScript(t *testing.T) {
	script := []byte{}
	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 0 {
		t.Errorf("Empty script should return no NFTs, got: %d", len(nfts))
	}
}

// TestJubileeMinimalEnvelope tests minimal valid inscription
func TestJubileeMinimalEnvelope(t *testing.T) {
	script := buildEnvelope()
	nfts := ExtractPkScriptForNFTJubilee(script)

	if len(nfts) != 1 {
		t.Fatalf("Minimal envelope should return 1 NFT, got: %d", len(nfts))
	}

	nft := nfts[0]
	if nft == nil {
		t.Fatal("NFT data should not be nil")
	}
}

// TestJubileeStutteringDetection tests stutter detection
func TestJubileeStutteringDetection(t *testing.T) {
	tests := []struct {
		name        string
		script      []byte
		wantStutter bool
		description string
	}{
		{
			"double false",
			append([]byte{OP_FALSE, OP_FALSE}, buildEnvelope()...),
			true,
			"OP_FALSE OP_FALSE OP_IF ...",
		},
		{
			"false if false",
			func() []byte {
				var s []byte
				s = append(s, OP_FALSE)
				s = append(s, OP_IF)
				s = append(s, OP_FALSE)
				s = append(s, OP_IF)
				s = append(s, OP_DATA_3)
				s = append(s, []byte("ord")...)
				s = append(s, OP_ENDIF)
				s = append(s, OP_ENDIF)
				return s
			}(),
			true,
			"OP_FALSE OP_IF OP_FALSE OP_IF ord OP_ENDIF OP_ENDIF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nfts := ExtractPkScriptForNFTJubilee(tt.script)
			if len(nfts) != 1 {
				t.Fatalf("%s: should return 1 NFT, got: %d", tt.description, len(nfts))
			}

			if nfts[0].IsStutter != tt.wantStutter {
				t.Errorf("IsStutter = %v, want %v", nfts[0].IsStutter, tt.wantStutter)
			}
		})
	}
}

// TestJubileeMultipleInscriptions tests parsing multiple inscriptions in one script
func TestJubileeMultipleInscriptions(t *testing.T) {
	var script []byte

	// First inscription: "foo"
	script = append(script, OP_FALSE)
	script = append(script, OP_IF)
	script = append(script, OP_DATA_3)
	script = append(script, []byte("ord")...)
	script = append(script, OP_DATA_1)
	script = append(script, 0x01) // content type
	script = append(script, OP_DATA_24)
	script = append(script, []byte("text/plain;charset=utf-8")...)
	script = append(script, 0x00) // body tag
	script = append(script, OP_DATA_3)
	script = append(script, []byte("foo")...)
	script = append(script, OP_ENDIF)

	// Second inscription: "bar"
	script = append(script, OP_FALSE)
	script = append(script, OP_IF)
	script = append(script, OP_DATA_3)
	script = append(script, []byte("ord")...)
	script = append(script, OP_DATA_1)
	script = append(script, 0x01) // content type
	script = append(script, OP_DATA_24)
	script = append(script, []byte("text/plain;charset=utf-8")...)
	script = append(script, 0x00) // body tag
	script = append(script, OP_DATA_3)
	script = append(script, []byte("bar")...)
	script = append(script, OP_ENDIF)

	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 2 {
		t.Fatalf("Multiple inscriptions should return 2 NFTs, got: %d", len(nfts))
	}

	if string(nfts[0].ContentBody) != "foo" {
		t.Errorf("First body = %s, want foo", string(nfts[0].ContentBody))
	}

	if string(nfts[1].ContentBody) != "bar" {
		t.Errorf("Second body = %s, want bar", string(nfts[1].ContentBody))
	}
}

// TestJubileeDuplicateField tests duplicate field detection
func TestJubileeDuplicateField(t *testing.T) {
	// Duplicate metadata fields (allowed in Jubilee)
	script := buildEnvelope(
		pushData([]byte{5}), // metadata tag
		pushData([]byte{0x00}),
		pushData([]byte{5}), // duplicate metadata tag
		pushData([]byte{0x01}),
	)

	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 1 {
		t.Fatalf("Should return 1 NFT, got: %d", len(nfts))
	}

	nft := nfts[0]
	if !nft.IsDuplicateField {
		t.Error("Duplicate metadata should set IsDuplicateField")
	}

	// Jubilee allows duplicate metadata to be concatenated
	expectedMetadata := []byte{0x00, 0x01}
	if !bytes.Equal(nft.Metadata, expectedMetadata) {
		t.Errorf("Metadata = %x, want %x", nft.Metadata, expectedMetadata)
	}
}

// TestJubileeIncompleteField tests incomplete field detection
func TestJubileeIncompleteField(t *testing.T) {
	// Tag without value
	script := buildEnvelope(
		pushData([]byte{99}), // unknown tag
		// Missing value
	)

	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 1 {
		t.Fatalf("Should return 1 NFT, got: %d", len(nfts))
	}

	nft := nfts[0]
	if !nft.IsIncompleteField {
		t.Error("Incomplete field should set IsIncompleteField")
	}
}

// TestJubileeUnrecognizedEvenField tests unrecognized even field detection
func TestJubileeUnrecognizedEvenField(t *testing.T) {
	// Tag 22 is even and not pointer (2)
	script := buildEnvelope(
		pushData([]byte{22}),
		pushData([]byte{0x00}),
	)

	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 1 {
		t.Fatalf("Should return 1 NFT, got: %d", len(nfts))
	}

	nft := nfts[0]
	if !nft.IsUnrecognizedEven {
		t.Error("Unrecognized even field should set IsUnrecognizedEven")
	}
}

// TestJubileePushnumOpcodes tests pushnum opcode detection
func TestJubileePushnumOpcodes(t *testing.T) {
	// Use OP_1 through OP_16 for tag value
	script := buildEnvelope(
		pushData([]byte{}), // body tag
		pushOpcode(OP_1),   // OP_1 as data
	)

	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 1 {
		t.Fatalf("Should return 1 NFT, got: %d", len(nfts))
	}

	nft := nfts[0]
	if !nft.IsPushnum {
		t.Error("Using OP_1 should set IsPushnum")
	}

	// Verify the data is parsed correctly
	if len(nft.ContentBody) != 1 || nft.ContentBody[0] != 1 {
		t.Errorf("ContentBody = %x, want [0x01]", nft.ContentBody)
	}
}

// TestJubileeMultipleParents tests parsing multiple parent inscriptions
func TestJubileeMultipleParents(t *testing.T) {
	// Create two valid parent IDs
	parent1 := make([]byte, 36)
	for i := 0; i < 32; i++ {
		parent1[i] = 0xff
	}
	parent1[32] = 0x04
	parent1[33] = 0x03
	parent1[34] = 0x02
	parent1[35] = 0x01

	parent2 := make([]byte, 36)
	for i := 0; i < 32; i++ {
		parent2[i] = 0xff
	}
	parent2[32] = 0x04
	parent2[33] = 0x03
	parent2[34] = 0x02
	parent2[35] = 0x00

	script := buildEnvelope(
		pushData([]byte{3}), // parent tag
		pushData(parent1),
		pushData([]byte{3}), // second parent tag
		pushData(parent2),
	)

	nfts := ExtractPkScriptForNFTJubilee(script)
	if len(nfts) != 1 {
		t.Fatalf("Should return 1 NFT, got: %d", len(nfts))
	}

	nft := nfts[0]
	if !nft.IsDuplicateField {
		t.Error("Duplicate parent tags should set IsDuplicateField")
	}

	if len(nft.Parents) != 2 {
		t.Errorf("Parents count = %d, want 2", len(nft.Parents))
	}

	if !nft.HasParent {
		t.Error("HasParent should be true")
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

// TestGetNFTIdFromScript tests the NFT ID parsing utility
func TestGetNFTIdFromScript(t *testing.T) {
	tests := []struct {
		name      string
		script    []byte
		wantID    string
		wantValid bool
	}{
		{
			"32 bytes (txid only)",
			[]byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
				0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
				0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
				0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f,
			},
			"1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100i0",
			true,
		},
		{
			"33 bytes (txid + 1 byte index)",
			append([]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			}, 0x01),
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffi1",
			true,
		},
		{
			"36 bytes (txid + 4 byte index)",
			append([]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			}, 0x01, 0x02, 0x03, 0x04),
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffi67305985",
			true,
		},
		{
			"too short",
			[]byte{0x00, 0x01},
			"",
			false,
		},
		{
			"too long",
			make([]byte, 40),
			"",
			false,
		},
		{
			"35 bytes with trailing zero",
			append([]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			}, 0x01, 0x02, 0x00),
			"",
			false,
		},
		{
			"36 bytes with trailing zero",
			append([]byte{
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
				0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			}, 0x01, 0x02, 0x00, 0x00),
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffi513",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := GetNFTIdFromScript(tt.script)
			if ok != tt.wantValid {
				t.Errorf("GetNFTIdFromScript() valid = %v, want %v", ok, tt.wantValid)
			}
			if ok && id != tt.wantID {
				t.Errorf("GetNFTIdFromScript() = %s, want %s", id, tt.wantID)
			}
		})
	}
}

// TestGetFlags tests the NFTData.GetFlags method
func TestGetFlags(t *testing.T) {
	nft := &NFTData{
		IsStutter:          true,
		IsPushnum:          true,
		HasMetadata:        true,
		HasPointer:         true,
		IsUnrecognizedEven: true,
	}

	flag := nft.GetFlags()

	// Verify individual bits are set correctly
	if nft.IsStutter && (flag&(1<<7)) == 0 {
		t.Error("IsStutter bit should be set")
	}
	if nft.IsPushnum && (flag&(1<<8)) == 0 {
		t.Error("IsPushnum bit should be set")
	}
	if nft.HasMetadata && (flag&(1<<1)) == 0 {
		t.Error("HasMetadata bit should be set")
	}
	if nft.HasPointer && (flag&(1<<5)) == 0 {
		t.Error("HasPointer bit should be set")
	}
	if nft.IsUnrecognizedEven && (flag&(1<<11)) == 0 {
		t.Error("IsUnrecognizedEven bit should be set")
	}
}

// =============================================================================
// Real-World Test Cases
// =============================================================================

// TestRealInscriptionFromNftTxt tests parsing real inscriptions from nft.txt
func TestRealInscriptionFromNftTxt(t *testing.T) {
	// First line from nft.txt - a real BRC-20 inscription
	scriptHex := "205e9d5715b5a8df9d418c882f5ec2c059785b241082457bbf6de652f89fab816cac0063036f7264010118746578742f706c61696e3b636861727365743d7574662d38010320f3ae2fc4cb5c28d6a3d73958f4d22555e4b746cfacfc21544bc25bf22cf7d05e00357b2270223a226272632d3230222c226f70223a226d696e74222c227469636b223a227761696e222c22616d74223a2235383838227d680063036f7264010118746578742f706c61696e3b636861727365743d7574662d38010320f3ae2fc4cb5c28d6a3d73958f4d22555e4b746cfacfc21544bc25bf22cf7d05e010202322900357b2270223a226272632d3230222c226f70223a226d696e74222c227469636b223a227761696e222c22616d74223a2235383838227d680063036f7264010118746578742f706c61696e3b636861727365743d7574662d38010320f3ae2fc4cb5c28d6a3d73958f4d22555e4b746cfacfc21544bc25bf22cf7d05e010202425000357b2270223a226272632d3230222c226f70223a226d696e74222c227469636b223a227761696e222c22616d74223a2235383838227d68"

	script, err := hex.DecodeString(scriptHex)
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	nft, hasNFT := ExtractPkScriptForNFT(script)
	if !hasNFT {
		t.Fatal("Real BRC-20 inscription should be parsed")
	}

	// Verify it's recognized as text content
	if string(nft.ContentType) != "text/plain;charset=utf-8" {
		t.Errorf("ContentType = %s, want text/plain;charset=utf-8", string(nft.ContentType))
	}

	// Body should contain JSON BRC-20 data
	if !bytes.Contains(nft.ContentBody, []byte("brc-20")) {
		t.Error("Body should contain brc-20 identifier")
	}
}

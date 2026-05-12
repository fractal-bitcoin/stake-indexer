# Bitcoin Ordinals Inscription Decoder - Planning Document

## Project Overview

This is a Golang reimplementation of the Bitcoin Ordinals inscription decoding logic from the Rust `ord` project. The decoder extracts NFT/inscription data from Bitcoin transaction tapscripts.

## Architecture

### Core Components

```
parser/script/
├── envelope.rs          # Rust reference: Envelope parsing logic
├── inscription.rs       # Rust reference: Inscription structure & parsing
├── inscription.go       # Golang: Original strict implementation
├── inscription_jubilee.go # Golang: Jubilee permissive implementation
├── model.go             # Data structures (NFTData, AddressData)
├── opcode.go            # Bitcoin script opcodes
├── utils.go             # Helper functions (GetOpcodeFormScript)
├── script.go            # Script type detection utilities
└── nft_test.go          # Unit tests (to be improved)
```

## Inscription Envelope Format

The inscription is embedded in Bitcoin tapscript using the following format:

```
OP_FALSE          [optional, may stutter]
OP_IF
  OP_PUSHDATA "ord"
  <tag>           # Optional field tags
  <value>         # Tag values
  ...
  OP_PUSHDATA ""  # Empty tag = body content marker
  <body_data>     # Body content (may be chunked)
  ...
OP_ENDIF
```

### Tag Definitions

| Tag ID | Name             | Description                                  |
|--------|------------------|----------------------------------------------|
| 0      | CONTENT          | Body content (empty tag marker)              |
| 1      | CONTENT_TYPE     | MIME type (e.g., "text/plain;charset=utf-8") |
| 2      | POINTER          | Offset to output being inscribed             |
| 3      | PARENT           | Parent inscription ID(s)                     |
| 5      | METADATA         | CBOR-encoded metadata                        |
| 7      | METAPROTOCOL     | Metaprotocol identifier                      |
| 9      | CONTENT_ENCODING | Content encoding (e.g., "br" for Brotli)     |
| 11     | DELEGATE         | Delegated inscription ID                     |
| 66     | UNBOUND          | Makes inscription unbound/cursed             |

### Data Structures

#### Rust `Envelope<T>`
```rust
pub struct Envelope<T> {
    pub input: u32,      // Input index in transaction
    pub offset: u32,     // Envelope offset within input
    pub payload: T,      // Inscription data
    pub pushnum: bool,   // Used OP_PUSHNUM opcodes
    pub stutter: bool,   // Had stuttering (repeated OP_FALSE)
}
```

#### Rust `Inscription`
```rust
pub struct Inscription {
    pub body: Option<Vec<u8>>,
    pub content_encoding: Option<Vec<u8>>,
    pub content_type: Option<Vec<u8>>,
    pub delegate: Option<Vec<u8>>,
    pub duplicate_field: bool,
    pub incomplete_field: bool,
    pub metadata: Option<Vec<u8>>,
    pub metaprotocol: Option<Vec<u8>>,
    pub parents: Vec<Vec<u8>>,
    pub pointer: Option<Vec<u8>>,
    pub rune: Option<Vec<u8>>,
    pub unrecognized_even_field: bool,
}
```

#### Golang `NFTData`
```go
type NFTData struct {
    // Flags
    IsStrip            bool
    IsKeyVerify        bool
    IsCursed           bool
    IsStutter          bool
    IsPushnum          bool
    IsUnrecognizedEven bool
    IsDuplicateField   bool
    IsIncompleteField  bool
    IsReinscription    bool

    // Tag presence flags
    HasPointer         bool
    HasParent          bool
    HasDeligate        bool
    HasMetaProtocal    bool
    HasMetadata        bool
    HasContentEncoding bool

    // Data fields
    Pointer         uint64
    Parents         [][]byte
    Deligate        []byte
    MetaProtocol    []byte
    Metadata        []byte
    ContentEncoding []byte
    ContentType     []byte
    ContentBody     []byte
    TapScriptPk     []byte

    // Computed
    ParentsId       []string
}
```

## Parsing Logic

### Main Flow

1. **Scan for OP_FALSE** - Marks potential inscription start
2. **Verify OP_IF** - Conditional block start
3. **Verify "ord"** - Protocol identifier (3 bytes)
4. **Parse Fields** - Read tag/value pairs until:
   - Empty tag (`[]`) found → marks body start
   - OP_ENDIF found → complete envelope
5. **Parse Body** - Read all following data until OP_ENDIF

### Key Parsing Rules

1. **Stuttering Detection**: Repeated OP_FALSE or OP_FALSE OP_IF OP_FALSE
2. **Even Tag Check**: Even-numbered tags (except pointer=2) make inscription unbound
3. **Duplicate Field**: Same tag appearing twice sets duplicate_field flag
4. **Incomplete Field**: Tag without value sets incomplete_field flag
5. **Pushnum Detection**: Using OP_1 through OP_16 for data pushes sets pushnum flag

### Two Implementations

#### `ExtractPkScriptForNFT` (Original)
- Strict parsing
- Returns `(nft *NFTData, hasNFT bool)`
- Breaks on first error
- No duplicate fields allowed

#### `ExtractPkScriptForNFTJubilee` (Jubilee)
- Permissive parsing
- Returns `[]*NFTData` (multiple inscriptions)
- Continues on recoverable errors
- Allows some duplicate fields (metadata, parents)

## Test Strategy

### Current Test Coverage

The existing `nft_test.go` has:
- `TestNFTDecode` - Basic decoding from nft.txt
- `TestNFTJubileeDecode` - Jubilee decoding from nft.txt

### Test Categories to Add

#### 1. Basic Envelope Tests
- Valid minimal envelope
- Empty envelope
- Envelope with no fields

#### 2. Field Tag Tests
- Content type parsing
- Content encoding parsing
- Metadata parsing
- Parent parsing
- Delegate parsing
- Pointer parsing
- Metaprotocol parsing

#### 3. Edge Cases
- Duplicate field detection
- Incomplete field detection
- Unrecognized even field
- Stuttering detection
- Pushnum opcode usage

#### 4. Body Parsing Tests
- Empty body
- Single chunk body
- Multi-chunk body (>520 bytes)
- No body (only tags)

#### 5. Invalid Script Tests
- Wrong protocol identifier
- No OP_ENDIF
- No OP_IF
- Malformed opcodes

#### 6. Multiple Inscriptions
- Two envelopes in one script
- Multiple envelopes with different fields

#### 7. PUSHDATA Variants
- OP_PUSHDATA1 usage
- OP_PUSHDATA2 usage
- OP_PUSHDATA4 usage
- Empty PUSHDATA2 as body tag (from Rust test)

## Key Differences from Rust

1. **Opcode Parsing**: Golang uses `GetOpcodeFormScript` instead of Rust's iterator-based `Instructions`
2. **Error Handling**: Golang uses explicit break/return patterns vs Rust's Result types
3. **Multiple Inscriptions**: Jubilee version returns array instead of Vec

## Future Enhancements

1. Add rune support (Tag::Rune from Rust)
2. Add media type detection
3. Add inscription ID generation
4. Add compression/decompression support
5. Add jubilee height awareness

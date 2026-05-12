package script

import (
	"encoding/binary"
)

// false if ord 1 1 type 0 content endif
// false false if ord ...
// false if false if ord ...
// false if ord false if ord ...
// false if ord 1 1 false if ord ...
// false if ord 1 1 type false if ord ...
// false if ord 1 1 type 0 false if ord ...
// false if ord 1 1 type 0 content false if ord ...
// false if ord 1 1 type 1 0 content endif
func ExtractPkScript_next(pkScript []byte) (data []byte, size uint, op, finish, ok bool) {
	size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript)
	if data == nil {
		return nil, size, false, false, false
	}

	if isPush {
		return data, size, isOpcode, false, true
	}

	if data[0] == OP_ENDIF { // found
		return nil, size, true, true, true
	}
	// check invalid OP_CODE
	return nil, size, true, false, true
}

func ExtractPkScriptForNFTJubilee(pkScript []byte) (nfts []*NFTData) {
	length := len(pkScript)
	if length == 0 {
		return
	}

	isPubKeyVerify := false
	var tapScriptPk []byte
	if pkScript[0] == 32 && length > 1+32+1+1 {
		tapScriptPk = append(tapScriptPk, pkScript[:35]...)
		if pkScript[33] == OP_CHECKSIGVERIFY && pkScript[34] >= OP_1 && pkScript[34] <= OP_8 {
			isPubKeyVerify = true
		}
	}

	p := uint(0)
	e := uint(length)

	prefix0 := OP_TRUE
	prefix1 := OP_TRUE
	for p < e {

		// check stutter
		var stutter bool
		if prefix0 == OP_FALSE || (prefix1 == OP_FALSE && prefix0 == OP_IF) {
			stutter = true
		}

		{ // check OP_FALSE
			size, data, isPush, _ := GetOpcodeFormScript(pkScript[p:])
			if data == nil {
				break
			}
			p += size // consume OP_CODE derectly, notice: OP_FALSE is a PUSH and a OPCODE
			if !isPush || len(data) != 0 {
				// skip if not OP_FALSE
				prefix0 = OP_TRUE
				prefix1 = OP_TRUE
				continue
			}
		}

		// roll
		prefix1 = prefix0
		prefix0 = OP_FALSE

		// min envlope lenght == 6, OP_IF OP_PUSH_DATA3 'ord' OP_ENDIF
		if p+6 > e {
			break
		}

		{ // check OP_IF
			size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript[p:])
			if data == nil {
				break
			}
			if isPush || !isOpcode || size != 1 || data[0] != OP_IF {
				continue // skip if not OP_IF
			}
			p += size // consume OP_CODE after make sure it's "IF"
		}

		// roll
		prefix1 = prefix0
		prefix0 = OP_IF

		{ // check OP_PUSH_DATA_3 magic ord
			size, data, isPush, _ := GetOpcodeFormScript(pkScript[p:])
			if data == nil {
				break
			}
			if !isPush || len(data) != 3 || data[0] != 'o' || data[1] != 'r' || data[2] != 'd' {
				continue // skip if not 'ord'
			}
			p += size // consume OP_CODE after make sure it's "ord"
		}

		offset := p

		tags := make(map[string]bool)
		nft := &NFTData{IsStutter: stutter} // initialize a new NFT container
		nft.TapScriptPk = tapScriptPk
		nft.IsKeyVerify = isPubKeyVerify

		// reset
		prefix0 = OP_TRUE
		prefix1 = OP_TRUE

		// parse nft
		for offset < e {
			// check tag name
			dataTag, sizeTag, opTag, finishTag, okTag := ExtractPkScript_next(pkScript[offset:])
			if !okTag {
				break
			}

			offset += sizeTag // consume OP_CODE
			if finishTag {    // found
				nfts = append(nfts, nft)
				break
			}
			if dataTag == nil { // not push
				break
			}

			// body start
			if len(dataTag) == 0 { // ORDINALS_TAG_CONTENT
				for offset < e {
					data, size, op, finish, ok := ExtractPkScript_next(pkScript[offset:])
					if !ok {
						break
					}
					offset += size // consume OP_CODE
					if finish {    // found
						nfts = append(nfts, nft)
						break
					}
					if data == nil {
						break
					} else {
						if op { // is number
							if size == 1 && len(data) == 0 {
							} else {
								nft.IsPushnum = true
							}
						}
						nft.ContentBody = append(nft.ContentBody, data...)
					}
				}
				// content parsing is done; restart from outer loop
				break
			}

			if opTag { // is number, should after content check
				nft.IsPushnum = true
			}

			// other tag
			if dataTag[0]%2 == 0 && !(len(dataTag) == 1 && dataTag[0] == ORDINALS_TAG_POINTER) {
				nft.IsUnrecognizedEven = true
			}

			isDupTag := false
			tagName := string(dataTag)
			if _, ok := tags[tagName]; ok {
				isDupTag = true
				nft.IsDuplicateField = true
			}
			tags[tagName] = true

			// get tag value
			// if size == 1 && (data[0] == OP_1 || data[0] == OP_DATA_1 ) {
			data, size, op, finish, ok := ExtractPkScript_next(pkScript[offset:])
			if !ok {
				break
			}
			offset += size // consume OP_CODE
			if finish {    // found
				nft.IsIncompleteField = true
				nfts = append(nfts, nft)
				break
			}
			if data == nil {
				break
			} else {
				if op { // is number
					if size == 1 && len(data) == 0 {
					} else {
						nft.IsPushnum = true
					}
				}

				if len(dataTag) != 1 {
					continue
				}

				if !isDupTag {
					if dataTag[0] == ORDINALS_TAG_CONTENTTYPE {
						nft.ContentType = append([]byte{}, data...)

					} else if dataTag[0] == ORDINALS_TAG_CONTENTENCODING {
						nft.ContentEncoding = append([]byte{}, data...)
						nft.HasContentEncoding = true
					} else if dataTag[0] == ORDINALS_TAG_DELEGATE {
						if _, ok := GetNFTIdFromScript(data); ok {
							nft.Deligate = append([]byte{}, data...)
							nft.HasDeligate = true
						}
					} else if dataTag[0] == ORDINALS_TAG_PARENT {
						if id, ok := GetNFTIdFromScript(data); ok {
							nft.ParentsId = append(nft.ParentsId, id)
							nft.Parents = append(nft.Parents, data)
							nft.HasParent = true
						}
					} else if dataTag[0] == ORDINALS_TAG_METADATA {
						nft.Metadata = append([]byte{}, data...)
						nft.HasMetadata = true
					} else if dataTag[0] == ORDINALS_TAG_METAPROTOCOL {
						nft.MetaProtocol = append([]byte{}, data...)
						nft.HasMetaProtocal = true
					} else if dataTag[0] == ORDINALS_TAG_POINTER {
						var pointer [8]byte
						copy(pointer[:], data)
						nft.Pointer = binary.LittleEndian.Uint64(pointer[:])
						nft.HasPointer = true
					}
				} else {
					if dataTag[0] == ORDINALS_TAG_METADATA {
						nft.Metadata = append(nft.Metadata, data...)
						nft.HasMetadata = true
					} else if dataTag[0] == ORDINALS_TAG_PARENT {
						if id, ok := GetNFTIdFromScript(data); ok {
							nft.ParentsId = append(nft.ParentsId, id)
							nft.Parents = append(nft.Parents, data)
							nft.HasParent = true
						}
					}
				}
			}

		} // for parse nft tags/body

		// restart
		p = offset
	}

	return nfts
}

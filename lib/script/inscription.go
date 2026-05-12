package script

// false if ord 1 1 type 0 content endif
// false false if ord ...
// false if false if ord ...
// false if ord false if ord ...
// false if ord 1 1 false if ord ...
// false if ord 1 1 type false if ord ...
// false if ord 1 1 type 0 false if ord ...
// false if ord 1 1 type 0 content false if ord ...
// false if ord 1 1 type 1 0 content endif
func ExtractPkScriptForNFT(pkScript []byte) (nft *NFTData, hasNFT bool) {
	length := len(pkScript)
	if length == 0 {
		return
	}

	nft = &NFTData{}
	tags := make(map[string]bool)

	if pkScript[0] == 32 && length > 1+32+1+1 {
		nft.TapScriptPk = append(nft.TapScriptPk, pkScript[:35]...)
		if pkScript[33] == OP_CHECKSIGVERIFY && pkScript[34] >= OP_1 && pkScript[34] <= OP_8 {
			nft.IsKeyVerify = true
		}
	}

	p := uint(0)
	e := uint(length)

	for p < e {
		// check OP_FALSE
		size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript[p:])
		if data == nil {
			break
		}
		p += size // consume OP_CODE, notice: OP_FALSE is a PUSH and a OPCODE
		if !isPush || len(data) != 0 {
			// fmt.Println("skip not OP_FALSE")
			// skip if not OP_FALSE
			continue
		}

		// min envlope lenght == 6, OP_IF OP_PUSH_DATA3 'ord' OP_ENDIF
		if p+6 > e {
			break
		}

		// check OP_IF
		size, data, isPush, isOpcode = GetOpcodeFormScript(pkScript[p:])
		if data == nil {
			break
		}
		p += size // consume OP_CODE
		if isPush || !isOpcode || size != 1 || data[0] != OP_IF {
			// fmt.Println("skip not OP_IF")
			// skip if not OP_IF
			continue
		}

		// check OP_PUSH_DATA_3 magic ord
		size, data, isPush, isOpcode = GetOpcodeFormScript(pkScript[p:])
		if data == nil {
			break
		}
		p += size // consume OP_CODE
		if !isPush || len(data) != 3 || data[0] != 'o' || data[1] != 'r' || data[2] != 'd' {
			// skip if not 'ord'
			// fmt.Println("skip not ord", isPush, size, data)
			continue
		}

		offset := p

		// parse nft
		for offset < e {
			// check tag name
			size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript[offset:])
			if data == nil {
				break
			}
			offset += size // consume OP_CODE
			if !isPush {
				if size == 1 && data[0] == OP_ENDIF { // found
					return nft, true
				}
				// check invalid OP_CODE
				break
			}

			// body start
			if len(data) == 0 { // ORDINALS_TAG_CONTENT
				for offset < e {
					size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript[offset:])
					if data == nil {
						break
					}
					offset += size // consume OP_CODE
					if isPush {
						// official need fix minimal push check
						if isOpcode {
							if len(data) == 0 { // ? fixme: not sure if ord accept it
								continue
							}
							break
						}

						// append content type data
						nft.ContentBody = append(nft.ContentBody, data...)
					} else {
						if data[0] == OP_ENDIF { // found
							return nft, true
						}
						// check invalid OP_CODE
						break
					}
				}
				break
			}

			// other tag
			if data[0]%2 == 0 {
				break // error: invalid tag
			}

			// official need fix minimal push check
			if isOpcode {
				break
			}

			tagName := string(data)
			if _, ok := tags[tagName]; ok {
				break // error: dup tag
			}
			tags[tagName] = true

			// fixme: minimal pushdata content type
			// if size == 1 && (data[0] == OP_1 || data[0] == OP_DATA_1 ) {
			if len(data) == 1 && data[0] == OP_DATA_1 {
				size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript[offset:])
				if data == nil {
					break
				}
				offset += size // consume OP_CODE
				if isPush {
					// official need fix minimal push check
					if isOpcode {
						if len(data) == 0 { // OP_0 is push and opcode, and ord accept it
							continue
						}
						break
					}

					// append content type data
					nft.ContentType = append(nft.ContentType, data...)
				} else {
					if data[0] == OP_ENDIF { // found
						return nft, true
					}
					// check invalid OP_CODE
					break
				}
			} else {
				// skip valid tag
				size, data, isPush, isOpcode := GetOpcodeFormScript(pkScript[offset:])
				if data == nil {
					break
				}
				offset += size // consume OP_CODE
				// skip valid tag
				if !isPush {
					// check invalid OP_CODE
					break
				} else {
					// official need fix minimal push check
					if isOpcode {
						if len(data) == 0 { // OP_0 is push and opcode, and ord accept it
							continue
						}
						break
					}
				}
			}
		} // for parse nft tags/body

		// restart
		p = offset
	}

	return nil, false
}

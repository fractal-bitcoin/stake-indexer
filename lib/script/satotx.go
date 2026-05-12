package script

func ExtractPkScriptForTxo(pkScript, scriptType []byte) (txo *AddressData) {
	txo = &AddressData{}

	if len(pkScript) == 0 {
		return txo
	}

	if isPubkeyHash(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2PKH
		copy(txo.AddressPkh[:], pkScript[3:23])
		return txo
	}

	if isPayToWitnessPubKeyHash(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2WPKH
		copy(txo.AddressPkh[:], GetHash160(pkScript))
		return txo
	}

	if isPayToWitnessScriptHash(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2WSH
		copy(txo.AddressPkh[:], GetHash160(pkScript))
		return txo
	}

	if isPayToTaproot(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2TR
		copy(txo.AddressPkh[:], GetHash160(pkScript))
		return txo
	}

	if isPayToAnchor(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2A
		copy(txo.AddressPkh[:], GetHash160(pkScript))
		return txo
	}

	if isPayToScriptHash(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2SH
		copy(txo.AddressPkh[:], GetHash160(pkScript))
		return txo
	}

	if isPubkey(scriptType) {
		txo.HasAddress = true
		txo.CodeType = CodeType_P2PK
		copy(txo.AddressPkh[:], GetHash160(pkScript))
		return txo
	}

	// if isMultiSig(scriptType) {
	// 	return pkScript[:]
	// }

	if IsOpreturn(scriptType) {
		return txo
	}

	return txo
}

func GetLockingScriptType(pkScript []byte) (scriptType []byte) {
	length := len(pkScript)
	if length == 0 {
		return
	}
	scriptType = make([]byte, 0)

	lenType := 0
	p := uint(0)
	e := uint(length)

	for p < e && lenType < 32 {
		c := pkScript[p]
		if 0 < c && c < 0x4f {
			cnt, cntsize := SafeDecodeVarIntForScript(pkScript[p:])
			p += cnt + cntsize
			if p > e {
				break
			}
		} else {
			p += 1
		}
		scriptType = append(scriptType, c)
		lenType += 1
	}
	return
}

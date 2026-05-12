package script

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

var nftScripts []string

func init() {
	dat, err := os.ReadFile("testdata/nft.txt")
	if err != nil {
		nftScripts = nil
		return
	}
	nftScripts = strings.Split(string(dat), "\n")
}

func TestNFTDecode(t *testing.T) {
	for line, scriptHex := range nftScripts {
		if len(scriptHex) == 0 {
			continue
		}
		script, err := hex.DecodeString(scriptHex)
		if err != nil {
			t.Logf("ignore line: %d, %s", line, scriptHex)
			continue
		}

		if nft, hasnft := ExtractPkScriptForNFT(script); hasnft {
			data, _ := json.Marshal(nft)
			t.Logf("scriptLen: %s, nft: %s", scriptHex, strings.ReplaceAll(string(data), ",", "\n"))
		}
	}
}

func TestNFTJubileeDecode(t *testing.T) {
	for line, scriptHex := range nftScripts {
		if len(scriptHex) == 0 {
			continue
		}
		script, err := hex.DecodeString(scriptHex)
		if err != nil {
			t.Logf("ignore line: %d, %s", line, scriptHex)
			continue
		}

		for _, nft := range ExtractPkScriptForNFTJubilee(script) {
			data, _ := json.Marshal(nft)
			t.Logf("scriptLen: %s, nft: %s", scriptHex, strings.ReplaceAll(string(data), ",", "\n"))
		}
	}
}

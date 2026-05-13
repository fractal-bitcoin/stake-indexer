package node

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"stake_indexer/internal/component/log"
	"strings"

	"github.com/spf13/viper"
	"github.com/ybbus/jsonrpc"
	"go.uber.org/zap"
)

var rpcClient jsonrpc.RPCClient

func Init(configFile string) error {
	vp := viper.New()
	vp.SetConfigFile(configFile)
	if err := vp.ReadInConfig(); err != nil {
		return fmt.Errorf("read chain config failed: %w", err)
	}

	rpcAddress := vp.GetString("rpc")
	rpcAuth := vp.GetString("rpc_auth")
	rpcClient = jsonrpc.NewClientWithOpts(rpcAddress, &jsonrpc.RPCClientOpts{
		CustomHeaders: map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(rpcAuth)),
		},
	})
	return nil
}

type BlockIndexInfo struct {
	Height     uint32 `json:"height"`
	HashHex    string `json:"hash"`
	TxCnt      uint32 `json:"txs"`
	FileIdx    int    `json:"file"`
	FileOffset int64  `json:"pos"`
}

func GetRawBlock(blockHash string) ([]byte, error) {
	result, err := GetBlockDetail(blockHash, 0)
	if err != nil {
		logger.Log.Error("GetRawBlock err", zap.Error(err))
		return nil, err
	}

	rawBlockString, ok := result.(string)
	if !ok {
		logger.Log.Error("invalid block data format")
		return nil, fmt.Errorf("invalid block data format")
	}

	rawBlock, err := hex.DecodeString(rawBlockString)
	if err != nil {
		logger.Log.Error("raw block decode failed", zap.Error(err))
		return nil, err
	}

	return rawBlock, nil
}

func GetBlockDetail(blockHash string, verbose int) (interface{}, error) {
	params := []interface{}{blockHash, verbose}

	response, err := rpcClient.Call("getblock", params)
	if err != nil {
		logger.Log.Error("RPC call failed", zap.Error(err))
		return nil, err
	}

	if response.Error != nil {
		logger.Log.Error("RPC returned error",
			zap.Any("code", response.Error.Code),
			zap.String("message", response.Error.Message))
		return nil, fmt.Errorf("RPC error: %v", response.Error)
	}

	return response.Result, nil
}

func GetBlockIndexRangeRPC(startHeight, endHeight uint32) ([]*BlockIndexInfo, bool) {
	if endHeight < startHeight {
		logger.Log.Info("invalid: end height must be greater than or equal to start height",
			zap.Uint32("startHeight", startHeight),
			zap.Uint32("endHeight", endHeight))
		return nil, false
	}

	if endHeight-startHeight > 10000 {
		logger.Log.Info("invalid: range too large, maximum 10000 blocks",
			zap.Uint32("startHeight", startHeight),
			zap.Uint32("endHeight", endHeight))
		return nil, false
	}

	response, err := rpcClient.Call("getblockindexrange", []interface{}{startHeight, endHeight})
	if err != nil {
		logger.Log.Info("RPC call failed",
			zap.Error(err),
			zap.Uint32("startHeight", startHeight),
			zap.Uint32("endHeight", endHeight))
		return nil, false
	}

	if response.Error != nil {
		if response.Error.Code == -8 && strings.Contains(strings.ToLower(response.Error.Message), "start height exceeds current chain height") {
			return make([]*BlockIndexInfo, 0), true
		}
		logger.Log.Info("RPC returned error",
			zap.Uint32("startHeight", startHeight),
			zap.Uint32("endHeight", endHeight),
			zap.Any("error", response.Error))
		return nil, false
	}

	blockIndexInfos := make([]*BlockIndexInfo, 0, endHeight-startHeight)

	js, err := json.Marshal(response.Result)
	if err != nil {
		logger.Log.Info("invalid json",
			zap.Uint32("startHeight", startHeight),
			zap.Uint32("endHeight", endHeight),
			zap.Any("result_type", fmt.Sprintf("%T", response.Result)))
		return nil, false
	}

	if err = json.Unmarshal(js, &blockIndexInfos); err != nil {
		logger.Log.Info("invalid array",
			zap.Uint32("startHeight", startHeight),
			zap.Uint32("endHeight", endHeight),
			zap.Error(err))
		return nil, false
	}

	return blockIndexInfos, true
}

func GetBlockCountRPC() (uint32, error) {
	response, err := rpcClient.Call("getblockcount", []interface{}{})
	if err != nil {
		return 0, fmt.Errorf("rpc getblockcount failed: %w", err)
	}
	if response.Error != nil {
		return 0, fmt.Errorf("rpc getblockcount error: %v", response.Error)
	}

	switch v := response.Result.(type) {
	case json.Number:
		n, convErr := v.Int64()
		if convErr != nil {
			return 0, fmt.Errorf("parse getblockcount json number failed: %w", convErr)
		}
		if n < 0 {
			return 0, nil
		}
		return uint32(n), nil
	case float64:
		if v < 0 {
			return 0, nil
		}
		return uint32(v), nil
	case int64:
		if v < 0 {
			return 0, nil
		}
		return uint32(v), nil
	case int:
		if v < 0 {
			return 0, nil
		}
		return uint32(v), nil
	default:
		return 0, fmt.Errorf("unexpected getblockcount result type: %T", response.Result)
	}
}

func GetBlockHashRPC(height uint32) (string, error) {
	response, err := rpcClient.Call("getblockhash", []interface{}{height})
	if err != nil {
		return "", fmt.Errorf("rpc getblockhash failed: %w", err)
	}
	if response.Error != nil {
		return "", fmt.Errorf("rpc getblockhash error: %v", response.Error)
	}
	blockHash, ok := response.Result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected getblockhash result type: %T", response.Result)
	}
	return strings.TrimSpace(blockHash), nil
}

func GetRawMemPoolRPC(blockHash string) ([]string, error) {
	params := []interface{}{}
	blockHash = strings.TrimSpace(blockHash)
	if blockHash != "" {
		params = append(params, blockHash)
	}

	response, err := rpcClient.Call("getrawtxmempool", params)
	if err != nil {
		return nil, fmt.Errorf("rpc getrawtxmempool failed: %w", err)
	}
	if response.Error != nil {
		return nil, fmt.Errorf("rpc getrawtxmempool error: %v", response.Error)
	}

	var rawTxHexes []string
	js, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("marshal getrawtxmempool result failed: %w", err)
	}
	if err := json.Unmarshal(js, &rawTxHexes); err != nil {
		return nil, fmt.Errorf("unmarshal getrawtxmempool result failed: %w", err)
	}

	return rawTxHexes, nil
}

func GetRawTxHexRPC(txID string) (string, error) {
	txID = strings.TrimSpace(txID)
	if txID == "" {
		return "", fmt.Errorf("txid is empty")
	}

	response, err := rpcClient.Call("getrawtransaction", []interface{}{txID})
	if err != nil {
		return "", fmt.Errorf("rpc getrawtransaction failed: %w", err)
	}
	if response.Error != nil {
		return "", fmt.Errorf("rpc getrawtransaction error: %v", response.Error)
	}
	rawHex, ok := response.Result.(string)
	if !ok {
		return "", fmt.Errorf("unexpected getrawtransaction result type: %T", response.Result)
	}
	return strings.TrimSpace(rawHex), nil
}

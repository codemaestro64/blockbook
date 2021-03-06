package dcr

import (
	"blockbook/bchain"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"blockbook/bchain/coins/btc"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

type DecredRPC struct {
	*btc.BitcoinRPC
	client      http.Client
	rpcURL      string
	rpcUser     string
	rpcPassword string
}

// NewDecredRPC returns new DecredRPC instance.
func NewDecredRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	var c btc.Configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotate(err, "Invalid configuration file")
	}

	transport := &http.Transport{
		Dial:                (&net.Dialer{KeepAlive: 600 * time.Second}).Dial,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // necessary to not to deplete ports
	}

	d := &DecredRPC{
		BitcoinRPC:  b.(*btc.BitcoinRPC),
		client:      http.Client{Timeout: time.Duration(c.RPCTimeout) * time.Second, Transport: transport},
		rpcURL:      c.RPCURL,
		rpcUser:     c.RPCUser,
		rpcPassword: c.RPCPass,
	}

	d.BitcoinRPC.RPCMarshaler = btc.JSONMarshalerV1{}
	d.BitcoinRPC.ChainConfig.SupportsEstimateSmartFee = false

	return d, nil
}

// Initialize initializes DecredRPC instance.
func (d *DecredRPC) Initialize() error {
	chainInfo, err := d.GetChainInfo()
	if err != nil {
		return err
	}

	chainName := chainInfo.Chain
	glog.Info("Chain name ", chainName)

	params := GetChainParams(chainName)

	// always create parser
	d.BitcoinRPC.Parser = NewDecredParser(params, d.BitcoinRPC.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		d.BitcoinRPC.Testnet = false
		d.BitcoinRPC.Network = "livenet"
	} else {
		d.BitcoinRPC.Testnet = true
		d.BitcoinRPC.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type GenericCmd struct {
	ID     int           `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params,omitempty"`
}

type GetBlockChainInfoResult struct {
	Error  Error `json:"error"`
	Result struct {
		Chain                string  `json:"chain"`
		Blocks               int64   `json:"blocks"`
		Headers              int64   `json:"headers"`
		SyncHeight           int64   `json:"syncheight"`
		BestBlockHash        string  `json:"bestblockhash"`
		Difficulty           uint32  `json:"difficulty"`
		VerificationProgress float64 `json:"verificationprogress"`
		ChainWork            string  `json:"chainwork"`
		InitialBlockDownload bool    `json:"initialblockdownload"`
		MaxBlockSize         int64   `json:"maxblocksize"`
	} `json:"result"`
}

type GetNetworkInfoResult struct {
	Error  Error `json:"error"`
	Result struct {
		Version         int32   `json:"version"`
		ProtocolVersion int32   `json:"protocolversion"`
		TimeOffset      int64   `json:"timeoffset"`
		Connections     int32   `json:"connections"`
		RelayFee        float64 `json:"relayfee"`
	} `json:"result"`
}

type GetInfoChainResult struct {
	Error  Error `json:"error"`
	Result struct {
		Version         int32   `json:"version"`
		ProtocolVersion int32   `json:"protocolversion"`
		Blocks          int64   `json:"blocks"`
		TimeOffset      int64   `json:"timeoffset"`
		Connections     int32   `json:"connections"`
		Proxy           string  `json:"proxy"`
		Difficulty      float64 `json:"difficulty"`
		TestNet         bool    `json:"testnet"`
		RelayFee        float64 `json:"relayfee"`
		Errors          string  `json:"errors"`
	}
}

type GetBestBlockResult struct {
	Error  Error `json:"error"`
	Result struct {
		Hash   string `json:"hash"`
		Height int64  `json:"height"`
	} `json:"result"`
}

type GetBlockHashResult struct {
	Error  Error  `json:"error"`
	Result string `json:"result"`
}

type GetBlockResult struct {
	Error  Error `json:"error"`
	Result struct {
		Hash          string      `json:"hash"`
		Confirmations int64       `json:"confirmations"`
		Size          int32       `json:"size"`
		Height        int64       `json:"height"`
		Version       json.Number `json:"version"`
		MerkleRoot    string      `json:"merkleroot"`
		StakeRoot     string      `json:"stakeroot"`
		RawTx         []RawTx     `json:"rawtx"`
		Tx            []string    `json:"tx,omitempty"`
		STx           []string    `json:"stx,omitempty"`
		Time          int64       `json:"time"`
		Nonce         json.Number `json:"nonce"`
		VoteBits      uint16      `json:"votebits"`
		FinalState    string      `json:"finalstate"`
		Voters        uint16      `json:"voters"`
		FreshStake    uint8       `json:"freshstake"`
		Revocations   uint8       `json:"revocations"`
		PoolSize      uint32      `json:"poolsize"`
		Bits          string      `json:"bits"`
		SBits         float64     `json:"sbits"`
		ExtraData     string      `json:"extradata"`
		StakeVersion  uint32      `json:"stakeversion"`
		Difficulty    float64     `json:"difficulty"`
		ChainWork     string      `json:"chainwork"`
		PreviousHash  string      `json:"previousblockhash"`
		NextHash      string      `json:"nextblockhash,omitempty"`
	} `json:"result"`
}

type GetBlockHeaderResult struct {
	Error  Error `json:"error"`
	Result struct {
		Hash          string      `json:"hash"`
		Confirmations int64       `json:"confirmations"`
		Version       json.Number `json:"version"`
		MerkleRoot    string      `json:"merkleroot"`
		StakeRoot     string      `json:"stakeroot"`
		VoteBits      uint16      `json:"votebits"`
		FinalState    string      `json:"finalstate"`
		Voters        uint16      `json:"voters"`
		FreshStake    uint8       `json:"freshstake"`
		Revocations   uint8       `json:"revocations"`
		PoolSize      uint32      `json:"poolsize"`
		Bits          string      `json:"bits"`
		SBits         float64     `json:"sbits"`
		Height        uint32      `json:"height"`
		Size          uint32      `json:"size"`
		Time          int64       `json:"time"`
		Nonce         uint32      `json:"nonce"`
		ExtraData     string      `json:"extradata"`
		StakeVersion  uint32      `json:"stakeversion"`
		Difficulty    float64     `json:"difficulty"`
		ChainWork     string      `json:"chainwork"`
		PreviousHash  string      `json:"previousblockhash,omitempty"`
		NextHash      string      `json:"nextblockhash,omitempty"`
	} `json:"result"`
}

type ScriptSig struct {
	Asm string `json:"asm"`
	Hex string `json:"hex"`
}

type Vin struct {
	Coinbase    string     `json:"coinbase"`
	Stakebase   string     `json:"stakebase"`
	Txid        string     `json:"txid"`
	Vout        uint32     `json:"vout"`
	Tree        int8       `json:"tree"`
	Sequence    uint32     `json:"sequence"`
	AmountIn    float64    `json:"amountin"`
	BlockHeight uint32     `json:"blockheight"`
	BlockIndex  uint32     `json:"blockindex"`
	ScriptSig   *ScriptSig `json:"scriptsig"`
}

type ScriptPubKeyResult struct {
	Asm       string   `json:"asm"`
	Hex       string   `json:"hex,omitempty"`
	ReqSigs   int32    `json:"reqSigs,omitempty"`
	Type      string   `json:"type"`
	Addresses []string `json:"addresses,omitempty"`
	CommitAmt *float64 `json:"commitamt,omitempty"`
}

type Vout struct {
	Value        float64            `json:"value"`
	N            uint32             `json:"n"`
	Version      uint16             `json:"version"`
	ScriptPubKey ScriptPubKeyResult `json:"scriptPubKey"`
}

type RawTx struct {
	Hex           string `json:"hex"`
	Txid          string `json:"txid"`
	Version       int32  `json:"version"`
	LockTime      uint32 `json:"locktime"`
	Vin           []Vin  `json:"vin"`
	Vout          []Vout `json:"vout"`
	Expiry        uint32 `json:"expiry"`
	BlockHash     string `json:"blockhash,omitempty"`
	BlockHeight   int64  `json:"blockheight,omitempty"`
	BlockIndex    uint32 `json:"blockindex,omitempty"`
	Confirmations int64  `json:"confirmations,omitempty"`
	Time          int64  `json:"time,omitempty"`
	Blocktime     int64  `json:"blocktime,omitempty"`
}

type GetTransactionResult struct {
	Error  Error `json:"error"`
	Result struct {
		RawTx
	} `json:"result"`
}

type EstimateSmartFeeResult struct {
	Error  Error `json:"error"`
	Result struct {
		FeeRate float64  `json:"feerate"`
		Errors  []string `json:"errors"`
		Blocks  int64    `json:"blocks"`
	} `json:"result"`
}

type EstimateFeeResult struct {
	Error  Error       `json:"error"`
	Result json.Number `json:"result"`
}

type SendRawTransactionResult struct {
}

type DecodeRawTransactionResult struct {
	Error  Error `json:"error"`
	Result struct {
		Txid     string `json:"txid"`
		Version  int32  `json:"version"`
		Locktime uint32 `json:"locktime"`
		Expiry   uint32 `json:"expiry"`
		Vin      []Vin  `json:"vin"`
		Vout     []Vout `json:"vout"`
	} `json:"result"`
}

func (d *DecredRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	blockchainInfoRequest := GenericCmd{
		ID:     1,
		Method: "getblockchaininfo",
	}
	blockchainInfoResult := GetBlockChainInfoResult{}
	err := d.Call(blockchainInfoRequest, &blockchainInfoResult)
	if err != nil {
		return nil, err
	}
	if blockchainInfoResult.Error.Message != "" {
		return nil, fmt.Errorf("Error fetching blockchain info: %s", blockchainInfoResult.Error.Message)
	}

	infoChainRequest := GenericCmd{
		ID:     2,
		Method: "getinfo",
	}
	infoChainResult := &GetInfoChainResult{}
	err = d.Call(infoChainRequest, infoChainResult)
	if err != nil {
		return nil, err
	}
	if infoChainResult.Error.Message != "" {
		return nil, fmt.Errorf("Error fetching network info: %s", infoChainResult.Error.Message)
	}

	chainInfo := &bchain.ChainInfo{
		Chain:           blockchainInfoResult.Result.Chain,
		Blocks:          int(blockchainInfoResult.Result.Blocks),
		Headers:         int(blockchainInfoResult.Result.Headers),
		Bestblockhash:   blockchainInfoResult.Result.BestBlockHash,
		Difficulty:      strconv.Itoa(int(blockchainInfoResult.Result.Difficulty)),
		SizeOnDisk:      blockchainInfoResult.Result.SyncHeight,
		Version:         strconv.Itoa(int(infoChainResult.Result.Version)),
		Subversion:      "",
		ProtocolVersion: strconv.Itoa(int(infoChainResult.Result.ProtocolVersion)),
		Timeoffset:      float64(infoChainResult.Result.TimeOffset),
		Warnings:        "",
	}
	return chainInfo, nil
}

func (d *DecredRPC) getBestBlock() (*GetBestBlockResult, error) {
	bestBlockRequest := GenericCmd{
		ID:     1,
		Method: "getbestblock",
	}
	bestBlockResult := &GetBestBlockResult{}
	err := d.Call(bestBlockRequest, bestBlockResult)
	if err != nil {
		return nil, err
	}
	if bestBlockResult.Error.Message != "" {
		return nil, fmt.Errorf("Error fetching best block: %s", bestBlockResult.Error.Message)
	}

	return bestBlockResult, err
}

func (d *DecredRPC) GetBestBlockHash() (string, error) {
	bestBlock, err := d.getBestBlock()
	if err != nil {
		return "", err
	}

	return bestBlock.Result.Hash, nil
}

func (d *DecredRPC) GetBestBlockHeight() (uint32, error) {
	bestBlock, err := d.getBestBlock()
	if err != nil {
		return 0, err
	}

	return uint32(bestBlock.Result.Height), err
}

func (d *DecredRPC) GetBlockHash(height uint32) (string, error) {
	blockHashRequest := GenericCmd{
		ID:     1,
		Method: "getblockhash",
		Params: []interface{}{height},
	}
	blockHashResult := GetBlockHashResult{}
	err := d.Call(blockHashRequest, &blockHashResult)
	if err != nil {
		return "", err
	}
	if blockHashResult.Error.Message != "" {
		return "", fmt.Errorf("Error fetching block hash: %s", blockHashResult.Error.Message)
	}

	return blockHashResult.Result, err
}

func (d *DecredRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	blockHeaderRequest := GenericCmd{
		ID:     1,
		Method: "getblockheader",
		Params: []interface{}{hash},
	}

	blockHeader := &GetBlockHeaderResult{}
	err := d.Call(blockHeaderRequest, blockHeader)
	if err != nil {
		return nil, err
	}
	if blockHeader.Error.Message != "" {
		return nil, fmt.Errorf("Error fetching block info: %s", blockHeader.Error.Message)
	}

	header := &bchain.BlockHeader{
		Hash:          blockHeader.Result.Hash,
		Prev:          blockHeader.Result.PreviousHash,
		Next:          blockHeader.Result.NextHash,
		Height:        blockHeader.Result.Height,
		Confirmations: int(blockHeader.Result.Confirmations),
		Size:          int(blockHeader.Result.Size),
		Time:          blockHeader.Result.Time / 1000,
	}

	return header, nil
}

func (d *DecredRPC) GetBlockHeaderByHeight(height uint32) (*bchain.BlockHeader, error) {
	return nil, nil
}

func (d *DecredRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	requestHash := hash
	if requestHash == "" {
		getHashRequest := GenericCmd{
			ID:     1,
			Method: "getblockhash",
			Params: []interface{}{height},
		}
		getHashResult := &GetBlockHashResult{}
		err := d.Call(getHashRequest, getHashResult)
		if err != nil {
			return nil, err
		}
		if getHashResult.Error.Message != "" {
			return nil, fmt.Errorf("Error fetching block hash: %s", getHashResult.Error.Message)
		}
		requestHash = getHashResult.Result
	}

	block, err := d.getBlock(requestHash)
	if err != nil {
		return nil, err
	}

	header := bchain.BlockHeader{
		Hash:          block.Result.Hash,
		Prev:          block.Result.PreviousHash,
		Next:          block.Result.NextHash,
		Height:        uint32(block.Result.Height),
		Confirmations: int(block.Result.Confirmations),
		Size:          int(block.Result.Size),
		Time:          block.Result.Time,
	}

	bchainBlock := &bchain.Block{
		BlockHeader: header,
	}

	for _, txId := range block.Result.Tx {
		if block.Result.Height == 0 {
			continue
		}

		tx, err := d.GetTransaction(txId)
		if err != nil {
			return nil, err
		}

		bchainBlock.Txs = append(bchainBlock.Txs, *tx)

	}

	return bchainBlock, nil
}

func (d *DecredRPC) getBlock(hash string) (*GetBlockResult, error) {
	blockRequest := GenericCmd{
		ID:     1,
		Method: "getblock",
		Params: []interface{}{hash},
	}
	block := &GetBlockResult{}
	err := d.Call(blockRequest, block)
	if err != nil {
		return nil, err
	}
	if block.Error.Message != "" {
		return nil, fmt.Errorf("Error fetching block info: %s", block.Error.Message)
	}

	return block, err
}

func (d *DecredRPC) decodeRawTransaction(txHex string) (*bchain.Tx, error) {
	decodeRawTxRequest := GenericCmd{
		ID:     1,
		Method: "decoderawtransaction",
		Params: []interface{}{txHex},
	}
	decodeRawTxResult := &DecodeRawTransactionResult{}
	err := d.Call(decodeRawTxRequest, &decodeRawTxResult)
	if err != nil {
		return nil, err
	}
	if decodeRawTxResult.Error.Message != "" {
		return nil, fmt.Errorf("Error decoding raw tx: %s", decodeRawTxResult.Error.Message)
	}

	tx := &bchain.Tx{
		Hex:      txHex,
		Txid:     decodeRawTxResult.Result.Txid,
		Version:  decodeRawTxResult.Result.Version,
		LockTime: decodeRawTxResult.Result.Locktime,
	}

	return tx, nil
}

func (d *DecredRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	block, err := d.getBlock(hash)
	if err != nil {
		return nil, err
	}

	header := bchain.BlockHeader{
		Hash:          block.Result.Hash,
		Prev:          block.Result.PreviousHash,
		Next:          block.Result.NextHash,
		Height:        uint32(block.Result.Height),
		Confirmations: int(block.Result.Confirmations),
		Size:          int(block.Result.Size),
		Time:          int64(block.Result.Time),
	}

	bInfo := &bchain.BlockInfo{
		BlockHeader: header,
		MerkleRoot:  block.Result.MerkleRoot,
		Version:     block.Result.Version,
		Nonce:       block.Result.Nonce,
		Bits:        block.Result.Bits,
		Difficulty:  json.Number(strconv.FormatFloat(block.Result.Difficulty, 'e', -1, 64)),
		Txids:       block.Result.Tx,
	}

	return bInfo, nil
}

func (d *DecredRPC) GetMempoolTransactions() ([]string, error) {
	return nil, nil
}

func (d *DecredRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	r, err := d.getRawTransaction(txid)
	if err != nil {
		return nil, err
	}

	tx, err := d.Parser.ParseTxFromJson(r)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}

	return tx, nil
}

func (d *DecredRPC) getRawTransaction(txid string) (json.RawMessage, error) {
	if txid == "" {
		return nil, bchain.ErrTxidMissing
	}

	verbose := 1
	getTxRequest := GenericCmd{
		ID:     1,
		Method: "getrawtransaction",
		Params: []interface{}{txid, &verbose},
	}
	getTxResult := &GetTransactionResult{}
	err := d.Call(getTxRequest, &getTxResult)
	if err != nil {
		return nil, err
	}
	if getTxResult.Error.Message != "" {
		return nil, fmt.Errorf("Error fetching transaction: %s", getTxResult.Error.Message)
	}

	bytes, err := json.Marshal(getTxResult.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}

	return json.RawMessage(bytes), nil
}

func (d *DecredRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return nil, nil
}

func (d *DecredRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	return d.getRawTransaction(tx.Txid)
}

func (d *DecredRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	estimateSmartFeeRequest := GenericCmd{
		ID:     1,
		Method: "estimatesmartfee",
		Params: []interface{}{blocks},
	}
	estimateSmartFeeResult := EstimateSmartFeeResult{}

	err := d.Call(estimateSmartFeeRequest, &estimateSmartFeeResult)
	if err != nil {
		return *big.NewInt(0), nil
	}
	if estimateSmartFeeResult.Error.Message != "" {
		return *big.NewInt(0), fmt.Errorf("Error fetching smart fee estimate: %s", estimateSmartFeeResult.Error.Message)
	}

	return *big.NewInt(int64(estimateSmartFeeResult.Result.FeeRate)), nil
}

func (d *DecredRPC) EstimateFee(blocks int) (big.Int, error) {
	estimateFeeRequest := GenericCmd{
		ID:     1,
		Method: "estimatefee",
		Params: []interface{}{blocks},
	}

	estimateFeeResult := EstimateFeeResult{}
	err := d.Call(estimateFeeRequest, &estimateFeeResult)
	if err != nil {
		return *big.NewInt(0), err
	}

	r, err := d.Parser.AmountToBigInt(estimateFeeResult.Result)
	if err != nil {
		return r, err
	}

	return r, nil
}

func (d *DecredRPC) SendRawTransaction(tx string) (string, error) {
	sendRawTxRequest := &GenericCmd{
		ID:     1,
		Method: "sendrawtransaction",
		Params: []interface{}{tx},
	}

	var res string
	err := d.Call(sendRawTxRequest, res)
	if err != nil {
		return "", err
	}

	return res, nil
}

// Call calls Backend RPC interface, using RPCMarshaler interface to marshall the request
func (d *DecredRPC) Call(req interface{}, res interface{}) error {
	httpData, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", d.rpcURL, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(d.rpcUser, d.rpcPassword)
	httpRes, err := d.client.Do(httpReq)
	// in some cases the httpRes can contain data even if it returns error
	// see http://devs.cloudimmunity.com/gotchas-and-common-mistakes-in-go-golang/
	if httpRes != nil {
		defer httpRes.Body.Close()
	}
	if err != nil {
		return err
	}

	// if server returns HTTP error code it might not return json with response
	// handle both cases
	if httpRes.StatusCode != 200 {
		err = safeDecodeResponse(httpRes.Body, &res)
		if err != nil {
			return errors.Errorf("%v %v", httpRes.Status, err)
		}
		return nil
	}
	return safeDecodeResponse(httpRes.Body, &res)
}

func safeDecodeResponse(body io.ReadCloser, res *interface{}) (err error) {
	var data []byte
	defer func() {
		if r := recover(); r != nil {
			glog.Error("unmarshal json recovered from panic: ", r, "; data: ", string(data))
			debug.PrintStack()
			if len(data) > 0 && len(data) < 2048 {
				err = errors.Errorf("Error: %v", string(data))
			} else {
				err = errors.New("Internal error")
			}
		}
	}()
	data, err = ioutil.ReadAll(body)
	if err != nil {
		return err
	}

	error := json.Unmarshal(data, res)
	return error
}

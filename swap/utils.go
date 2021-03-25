package swap

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcom "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	contractabi "github.com/binance-chain/bsc-eth-swap/abi"
	"github.com/binance-chain/bsc-eth-swap/common"
	"github.com/binance-chain/bsc-eth-swap/model"
	"github.com/binance-chain/bsc-eth-swap/util"

)

func buildSwapPairInstance(pairs []model.SwapPair) (map[ethcom.Address]*SwapPairIns, error) {
	swapPairInstances := make(map[ethcom.Address]*SwapPairIns, len(pairs))

	for _, pair := range pairs {

		lowBound := big.NewInt(0)
		_, ok := lowBound.SetString(pair.LowBound, 10)
		if !ok {
			panic(fmt.Sprintf("invalid lowBound amount: %s", pair.LowBound))
		}
		upperBound := big.NewInt(0)
		_, ok = upperBound.SetString(pair.UpperBound, 10)
		if !ok {
			panic(fmt.Sprintf("invalid upperBound amount: %s", pair.LowBound))
		}

		swapPairInstances[ethcom.HexToAddress(pair.ERC20Addr)] = &SwapPairIns{
			Symbol:     pair.Symbol,
			Name:       pair.Name,
			Decimals:   pair.Decimals,
			LowBound:   lowBound,
			UpperBound: upperBound,
			BEP20Addr:  ethcom.HexToAddress(pair.BEP20Addr),
			ERC20Addr:  ethcom.HexToAddress(pair.ERC20Addr),
		}

		util.Logger.Infof("Load swap pair, symbol %s, bep20 address %s, erc20 address %s", pair.Symbol, pair.BEP20Addr, pair.ERC20Addr)
	}

	return swapPairInstances, nil
}

func GetKeyConfig(cfg *util.Config) (*util.KeyConfig, error) {
	if cfg.KeyManagerConfig.KeyType == common.AWSPrivateKey {
		result, err := util.GetSecret(cfg.KeyManagerConfig.AWSSecretName, cfg.KeyManagerConfig.AWSRegion)
		if err != nil {
			return nil, err
		}

		keyConfig := util.KeyConfig{}
		err = json.Unmarshal([]byte(result), &keyConfig)
		if err != nil {
			return nil, err
		}
		return &keyConfig, nil
	} else {
		return &util.KeyConfig{
			HMACKey:        cfg.KeyManagerConfig.LocalHMACKey,
			AdminApiKey:    cfg.KeyManagerConfig.LocalAdminApiKey,
			AdminSecretKey: cfg.KeyManagerConfig.LocalAdminSecretKey,
			BSCPrivateKey:  cfg.KeyManagerConfig.LocalBSCPrivateKey,
			ETHPrivateKey:  cfg.KeyManagerConfig.LocalETHPrivateKey,
		}, nil
	}
}

func abiEncodeFillETH2BSCSwap(ethTxHash ethcom.Hash, erc20Addr ethcom.Address, toAddress ethcom.Address, amount *big.Int, abi *abi.ABI) ([]byte, error) {
	data, err := abi.Pack("fillETH2BSCSwap", ethTxHash, erc20Addr, toAddress, amount)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func abiEncodeERC20Transfer(recipient ethcom.Address, amount *big.Int, abi *abi.ABI) ([]byte, error) {
	data, err := abi.Pack("transfer", recipient, amount)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func abiEncodeFillBSC2ETHSwap(ethTxHash ethcom.Hash, erc20Addr ethcom.Address, toAddress ethcom.Address, amount *big.Int, abi *abi.ABI) ([]byte, error) {
	data, err := abi.Pack("fillBSC2ETHSwap", ethTxHash, erc20Addr, toAddress, amount)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func abiEncodeCreateSwapPair(registerTxHash ethcom.Hash, erc20Addr ethcom.Address, name, symbol string, decimals uint8, abi *abi.ABI) ([]byte, error) {
	data, err := abi.Pack("createSwapPair", registerTxHash, erc20Addr, name, symbol, decimals)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func buildSignedTransaction(contract ethcom.Address, ethClient *ethclient.Client, txInput []byte, privateKey *ecdsa.PrivateKey) (*types.Transaction, error) {
	txOpts := bind.NewKeyedTransactor(privateKey)

	nonce, err := ethClient.PendingNonceAt(context.Background(), txOpts.From)
	if err != nil {
		return nil, err
	}
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}
	value := big.NewInt(0)
	msg := ethereum.CallMsg{From: txOpts.From, To: &contract, GasPrice: gasPrice, Value: value, Data: txInput}
	gasLimit, err := ethClient.EstimateGas(context.Background(), msg)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
	}

	rawTx := types.NewTransaction(nonce, contract, value, gasLimit, gasPrice, txInput)
	signedTx, err := txOpts.Signer(types.HomesteadSigner{}, txOpts.From, rawTx)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func buildNativeCoinTransferTx(contract ethcom.Address, ethClient *ethclient.Client, value *big.Int, privateKey *ecdsa.PrivateKey) (*types.Transaction, error) {
	txOpts := bind.NewKeyedTransactor(privateKey)

	nonce, err := ethClient.PendingNonceAt(context.Background(), txOpts.From)
	if err != nil {
		return nil, err
	}
	gasPrice, err := ethClient.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, err
	}
	msg := ethereum.CallMsg{From: txOpts.From, To: &contract, GasPrice: gasPrice, Value: value}
	gasLimit, err := ethClient.EstimateGas(context.Background(), msg)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
	}

	rawTx := types.NewTransaction(nonce, contract, value, gasLimit, gasPrice, nil)
	signedTx, err := txOpts.Signer(types.HomesteadSigner{}, txOpts.From, rawTx)
	if err != nil {
		return nil, err
	}

	return signedTx, nil
}

func queryDeployedBEP20ContractAddr(erc20Addr ethcom.Address, bscSwapAgentAddr ethcom.Address, txRecipient *types.Receipt, bscClient *ethclient.Client) (ethcom.Address, error) {
	swapAgentInstance, err := contractabi.NewBSCSwapAgent(bscSwapAgentAddr, bscClient)
	if err != nil {
		return ethcom.Address{}, err
	}
	if len(txRecipient.Logs) != 2 {
		return ethcom.Address{}, fmt.Errorf("Expected tx logs length in recipient is 2, actual it is %d", len(txRecipient.Logs))
	}
	createSwapEvent, err := swapAgentInstance.ParseSwapPairCreated(*txRecipient.Logs[1])
	if err != nil || createSwapEvent == nil {
		return ethcom.Address{}, err
	}

	util.Logger.Debugf("Deployed bep20 contact %s for register erc20 %s", createSwapEvent.Bep20Addr.String(), erc20Addr.String())
	return createSwapEvent.Bep20Addr, nil
}

func BuildKeys(privateKeyStr string) (*ecdsa.PrivateKey, *ecdsa.PublicKey, error) {
	if strings.HasPrefix(privateKeyStr, "0x") {
		privateKeyStr = privateKeyStr[2:]
	}
	priKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return nil, nil, err
	}
	publicKey, ok := priKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("get public key error")
	}
	return priKey, publicKey, nil
}

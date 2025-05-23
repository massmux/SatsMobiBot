package telegram

// lightning to chain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/BoltzExchange/boltz-client/v2/pkg/boltz"
	"github.com/btcsuite/btcd/btcec/v2"
	"os"
)

const endpoint = "wss://api.boltz.exchange/v2/ws"
const invoiceAmount = 10000
const destinationAddress = "<address to which the swap should be claimed>"

// Swap from Lightning to BTC mainchain
// var toCurrency = boltz.CurrencyBtc
var toCurrency = boltz.CurrencyLiquid

var boltz_network = boltz.MainNet

func printJson(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println(string(b))
}

func reverseSwap() error {
	ourKeys, err := btcec.NewPrivateKey()
	if err != nil {
		return err
	}

	preimage := make([]byte, 32)
	_, err = rand.Read(preimage)
	if err != nil {
		return err
	}
	preimageHash := sha256.Sum256(preimage)

	boltzApi := &boltz.Api{URL: endpoint}

	swap, err := boltzApi.CreateReverseSwap(boltz.CreateReverseSwapRequest{
		From:           boltz.CurrencyBtc,
		To:             toCurrency,
		ClaimPublicKey: ourKeys.PubKey().SerializeCompressed(),
		PreimageHash:   preimageHash[:],
		InvoiceAmount:  invoiceAmount,
	})
	if err != nil {
		return fmt.Errorf("Could not create swap: %s", err)
	}

	boltzPubKey, err := btcec.ParsePubKey(swap.RefundPublicKey)
	if err != nil {
		return err
	}

	tree := swap.SwapTree.Deserialize()
	if err := tree.Init(toCurrency, false, ourKeys, boltzPubKey); err != nil {
		return err
	}

	if err := tree.Check(boltz.ReverseSwap, swap.TimeoutBlockHeight, preimageHash[:]); err != nil {
		return err
	}

	fmt.Println("Swap created")
	printJson(swap)

	boltzWs := boltzApi.NewWebsocket()
	if err := boltzWs.Connect(); err != nil {
		return fmt.Errorf("Could not connect to Boltz websocket: %w", err)
	}

	if err := boltzWs.Subscribe([]string{swap.Id}); err != nil {
		return err
	}

	for update := range boltzWs.Updates {
		parsedStatus := boltz.ParseEvent(update.Status)

		printJson(update)

		switch parsedStatus {
		case boltz.SwapCreated:
			fmt.Println("Waiting for invoice to be paid")
			break

		case boltz.TransactionMempool:
			lockupTransaction, err := boltz.NewTxFromHex(toCurrency, update.Transaction.Hex, nil)
			if err != nil {
				return err
			}

			vout, _, err := lockupTransaction.FindVout(boltz_network, swap.LockupAddress)
			if err != nil {
				return err
			}

			satPerVbyte := float64(2)
			claimTransaction, _, err := boltz.ConstructTransaction(
				boltz_network,
				boltz.CurrencyBtc,
				[]boltz.OutputDetails{
					{
						SwapId:            swap.Id,
						SwapType:          boltz.ReverseSwap,
						Address:           destinationAddress,
						LockupTransaction: lockupTransaction,
						Vout:              vout,
						Preimage:          preimage,
						PrivateKey:        ourKeys,
						SwapTree:          tree,
						Cooperative:       true,
					},
				},
				satPerVbyte,
				boltzApi,
			)
			if err != nil {
				return fmt.Errorf("could not create claim transaction: %w", err)
			}

			txHex, err := claimTransaction.Serialize()
			if err != nil {
				return fmt.Errorf("could not serialize claim transaction: %w", err)
			}

			txId, err := boltzApi.BroadcastTransaction(toCurrency, txHex)
			if err != nil {
				return fmt.Errorf("could not broadcast transaction: %w", err)
			}

			fmt.Printf("Broadcast claim transaction: %s\n", txId)
			break

		case boltz.InvoiceSettled:
			fmt.Println("Swap succeeded", swap.Id)
			if err := boltzWs.Close(); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func main() {
	if err := reverseSwap(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

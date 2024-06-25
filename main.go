package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	rpcURL     = "https://rpc-testnet.unit0.dev"
	maxRetries = 5
)

func main() {
	data, err := ioutil.ReadFile("pk.txt")
	if err != nil {
		log.Fatalf("Failed to read pk.txt: %v", err)
	}
	privateKeyStrings := strings.Split(string(data), "\n")

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to rpc url: %v", err)
	}

	for _, privateKeyString := range privateKeyStrings {
		privateKeyString = strings.TrimSpace(strings.ReplaceAll(privateKeyString, "\r", ""))
		if privateKeyString == "" {
			continue // skip empty lines
		}

		privateKey, err := crypto.HexToECDSA(privateKeyString)
		if err != nil {
			log.Fatalf("Failed to load private key: %v", err)
		}

		publicKey := privateKey.Public()
		publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			log.Fatalf("Failed to cast public key to ECDSA")
		}

		fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

		// Get balance
		balance, err := client.BalanceAt(context.Background(), fromAddress, nil)
		if err != nil {
			log.Fatalf("Failed to get balance: %v", err)
		}

		balanceInUNIT0 := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(math.Pow10(18)))
		fmt.Printf("Balance wallet %s : %f UNIT0\n", fromAddress.Hex(), balanceInUNIT0)

		gasLimit := uint64(22000) // gas limit

		// Set custom gas price
		gasPrice := big.NewInt(900000) // 1 Gwei in Wei

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("How many wallets do you want to generate: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		numWallets, err := strconv.Atoi(input)
		if err != nil {
			log.Fatalf("Invalid number of wallets: %v", err)
		}

		rand.Seed(time.Now().UnixNano())
		value := big.NewInt(427596) // 0.00000000000427596 UNIT0 in Wei

		for i := 0; i < numWallets; i++ {
			newPrivateKey, err := crypto.GenerateKey()
			if err != nil {
				log.Fatalf("Failed to generate new private key: %v", err)
			}

			newAddress := crypto.PubkeyToAddress(newPrivateKey.PublicKey)

			// Fetch the nonce before each transaction
			var nonce uint64
			var retryCount int
			for retryCount = 0; retryCount < maxRetries; retryCount++ {
				nonce, err = client.PendingNonceAt(context.Background(), fromAddress)
				if err != nil {
					if strings.Contains(err.Error(), "502 Bad Gateway") {
						fmt.Println("502 Bad Gateway encountered, retrying in 5 seconds...")
						time.Sleep(5 * time.Second)
						continue
					}
					log.Fatalf("Failed to get the nonce: %v", err)
				}
				break
			}

			if retryCount == maxRetries {
				log.Fatalf("Max retries exceeded for fetching nonce")
			}

			// create tx
			tx := types.NewTransaction(nonce, newAddress, value, gasLimit, gasPrice, nil)

			// sign tx
			signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(88817)), privateKey)
			if err != nil {
				log.Fatalf("Failed to sign the transaction: %v", err)
			}

			// send tx
			err = client.SendTransaction(context.Background(), signedTx)
			if err != nil {
				// handle specific errors
				switch {
				case strings.Contains(err.Error(), "Replacement transaction underpriced"):
					fmt.Println("Got an error :(, Retry transaction...")
					time.Sleep(time.Duration(math.Pow(2, float64(retryCount))) * time.Second)
					continue
				case strings.Contains(err.Error(), "Nonce too low"):
					fmt.Println("Nonce too low, retrying with new nonce...")
					continue
				case strings.Contains(err.Error(), "Upfront cost exceeds account balance"):
					fmt.Println("Your wallet has low Balance")
					continue
				case strings.Contains(err.Error(), "502 Bad Gateway"):
					fmt.Println("Got an error 502 Bad Gateway. Retrying in 3 seconds...")
					time.Sleep(3 * time.Second)
					continue
				case strings.Contains(err.Error(), "Known transaction"):
					fmt.Println("Got an error, retrying in 3 seconds...")
					time.Sleep(3 * time.Second)
					continue
				default:
					log.Fatalf("Failed to send the transaction: %v", err)
				}
			}

			fmt.Printf("Transaction %d sent to %s , tx link : https://explorer-testnet.unit0.dev/tx/%s\n", i+1, newAddress.Hex(), signedTx.Hash().Hex())
		}
		fmt.Println("========================================")
	}
}

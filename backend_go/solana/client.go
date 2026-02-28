package solana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/portto/solana-go-sdk/client"
	"github.com/portto/solana-go-sdk/common"
	"github.com/portto/solana-go-sdk/rpc"
	"github.com/portto/solana-go-sdk/types"
)

// Memo program ID (SPL Memo)
var memoProgramID = common.PublicKeyFromString("MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr")

type Client struct {
	rpc     *client.Client
	keypair *types.Account
}

func New(rpcURL string, keypairJSON string) (*Client, error) {
	ep := rpc.DevnetRPCEndpoint
	if rpcURL != "" {
		ep = rpcURL
	}
	c := client.NewClient(ep)

	sc := &Client{rpc: c}
	if keypairJSON != "" {
		data, err := base64.StdEncoding.DecodeString(keypairJSON)
		if err != nil {
			// Try raw JSON array
			var arr []byte
			if jerr := json.Unmarshal([]byte(keypairJSON), &arr); jerr != nil {
				return nil, fmt.Errorf("invalid keypair: %w", err)
			}
			data = arr
		}
		if len(data) == 0 {
			var arr []byte
			_ = json.Unmarshal([]byte(keypairJSON), &arr)
			data = arr
		}
		if len(data) < 64 {
			return nil, fmt.Errorf("keypair too short")
		}
		acc, err := types.AccountFromBytes(data)
		if err != nil {
			return nil, err
		}
		sc.keypair = &acc
	}
	return sc, nil
}

// SubmitMemo writes decision hash to Solana devnet and returns TX signature
func (c *Client) SubmitMemo(ctx context.Context, memoText string) (string, error) {
	if c.keypair == nil {
		return "dev-stub-no-keypair", nil
	}

	inst := types.Instruction{
		ProgramID: memoProgramID,
		Accounts:  []types.AccountMeta{},
		Data:      []byte(memoText),
	}
	blockhash, err := c.rpc.GetLatestBlockhash(ctx)
	if err != nil {
		return "", err
	}
	blockhashStr := blockhash.Blockhash
	msg := types.NewMessage(types.NewMessageParam{
		FeePayer:        c.keypair.PublicKey,
		RecentBlockhash: blockhashStr,
		Instructions:    []types.Instruction{inst},
	})

	tx, err := types.NewTransaction(types.NewTransactionParam{
		Message: msg,
		Signers: []types.Account{*c.keypair},
	})
	if err != nil {
		return "", err
	}

	sig, err := c.rpc.SendTransaction(ctx, tx)
	if err != nil {
		return "", err
	}
	return sig, nil
}

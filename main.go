package main

import (
	"dgamingfoundation/hackatom-relayer/zoneA"
	"dgamingfoundation/hackatom-relayer/zoneB"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/store/state"
	"github.com/cosmos/cosmos-sdk/x/ibc/23-commitment/merkle"
	rpcclient "github.com/tendermint/tendermint/rpc/client"

	sdk "github.com/cosmos/cosmos-sdk/types"
	cli "github.com/cosmos/cosmos-sdk/x/ibc/client/utils"
	ibc "github.com/cosmos/cosmos-sdk/x/ibc/keeper"
	"github.com/dgamingfoundation/hackatom-zone/x/nftapp/types"
	"github.com/spf13/viper"
)

func main() {
	viper.Set("trust-node", true)

	cliA := zoneA.GetCLIContext()
	cliB := zoneB.GetCLIContext()
	packet, _, err := GetRelayPacket(cliA, cliB)
	if err != nil {
		panic(fmt.Errorf("failed to GetRelayPacket: %v", err))
	}

	var st types.SellTokenPacket
	if err := json.Unmarshal(packet.Commit(), &st); err != nil {
		panic(fmt.Errorf("failed to unmarshal SellTokenPacket: %v", err))
	}

	fmt.Printf("%+v\n", st)
}

func GetRelayPacket(cliCtxSource, cliCtx context.CLIContext) (ibc.Packet, ibc.Proof, error) {
	keeper := ibc.DummyKeeper()
	cdc := cliCtx.Codec

	connid := viper.GetString(cli.FlagConnectionID)
	chanid := viper.GetString(cli.FlagChannelID)

	obj := keeper.Channel.Object(connid, chanid)

	seqbz, _, err := query(cliCtx, obj.Seqrecv.Key())
	if err != nil {
		return nil, nil, err
	}
	seq, err := state.DecodeInt(seqbz, state.Dec)
	if err != nil {
		return nil, nil, err
	}

	sentbz, _, err := query(cliCtxSource, obj.Seqsend.Key())
	if err != nil {
		return nil, nil, err
	}
	sent, err := state.DecodeInt(sentbz, state.Dec)
	if err != nil {
		return nil, nil, err
	}

	if seq == sent {
		return nil, nil, errors.New("no packet detected")
	}

	var packet SellTokenPacket
	fmt.Println(obj.Packets.Value(seq + 1).Key())
	packetbz, proof, err := query(cliCtxSource, obj.Packets.Value(seq+1).Key())
	if err != nil {
		return nil, nil, err
	}
	cdc.MustUnmarshalBinaryBare(packetbz, &packet)

	return packet, proof, nil
}

type SellTokenPacket struct {
	Token *BaseNFT `json:"token"`
	Price sdk.Coin `json:"price"`
}

func (m SellTokenPacket) Timeout() uint64 {
	return math.MaxUint64
}

func (m SellTokenPacket) Commit() []byte {
	data, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Errorf("failed to marshal SellTokenPacket packet: %v", err))
	}

	return data
}

// BaseNFT non fungible token definition
type BaseNFT struct {
	ID          string         `json:"id,omitempty"`       // id of the token; not exported to clients
	Owner       sdk.AccAddress `json:"owner,string"`       // account address that owns the NFT
	Name        string         `json:"name,string"`        // name of the token
	Description string         `json:"description,string"` // unique description of the NFT
	Image       string         `json:"image,string"`       // image path
	TokenURI    string         `json:"token_uri,string"`   // optional extra properties available fo querying
}

// Copied from client/context/query.go
func query(ctx context.CLIContext, key []byte) ([]byte, merkle.Proof, error) {
	node, err := ctx.GetNode()
	if err != nil {
		return nil, merkle.Proof{}, err
	}

	opts := rpcclient.ABCIQueryOptions{
		Height: ctx.Height,
		Prove:  true,
	}

	result, err := node.ABCIQueryWithOptions("/store/ibc/key", key, opts)
	if err != nil {
		return nil, merkle.Proof{}, err
	}

	resp := result.Response
	if !resp.IsOK() {
		return nil, merkle.Proof{}, errors.New(resp.Log)
	}

	return resp.Value, merkle.Proof{
		Key:   key,
		Proof: resp.Proof,
	}, nil
}

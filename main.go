package main

import (
	"bytes"
	"dgamingfoundation/hackatom-relayer/zoneA"
	"dgamingfoundation/hackatom-relayer/zoneB"
	"encoding/json"
	"errors"
	"fmt"
	app "github.com/dgamingfoundation/hackatom-marketplace"
	"io/ioutil"
	"net/http"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/store/state"
	"github.com/cosmos/cosmos-sdk/types/rest"
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

	fmt.Printf("------------%+v\n", st)

	//curl
	err = sendTokenToHub(st)
	if err != nil {
		panic(err)
	}
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

	var packet types.SellTokenPacket
	fmt.Println(obj.Packets.Value(seq + 1).Key())
	packetbz, proof, err := query(cliCtxSource, obj.Packets.Value(seq+1).Key())
	if err != nil {
		return nil, nil, err
	}
	cdc.MustUnmarshalBinaryBare(packetbz, &packet)

	return &packet, proof, nil
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

type putOnMarketNFTReq struct {
	BaseReq rest.BaseReq  `json:"base_req"`
	Owner   string        `json:"owner"`
	Token   types.BaseNFT `json:"token"`
	Price   sdk.Coin      `json:"price"`

	// User data
	Name     string `json:"name"`
	Password string `json:"password"`
}

func sendTokenToHub(st types.SellTokenPacket) error {
	hubURL := "http://localhost:1317/hh/nft/sell"
	ownerName := "validator1"
	ownerPassword := "12345678"
	ownerAddr := "cosmos16y2vaas25ea8n353tfve45rwvt4sx0gl627pzn"

	reqObject := putOnMarketNFTReq{
		rest.BaseReq{
			From:          ownerAddr,
			ChainID:       "hhchain",
			Sequence:      3,
			AccountNumber: 0,
		},
		ownerAddr,
		*st.Token,
		st.Price,
		ownerName,
		ownerPassword,
	}

	cdc := app.MakeCodec()
	reqBytes, err := cdc.MarshalJSON(reqObject)
	if err != nil {
		return err
	}
	fmt.Println(string(reqBytes))

	req, err := http.NewRequest("POST", hubURL, bytes.NewBuffer(reqBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Println("response Status:", resp.Status)
	fmt.Println("response Headers:", resp.Header)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))

	return nil
}

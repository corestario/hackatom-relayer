package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/types/rest"
	"github.com/cosmos/cosmos-sdk/x/ibc/23-commitment/merkle"
	app "github.com/dgamingfoundation/hackatom-marketplace"

	cliCtx "github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/store/state"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/ibc/client/cli"
	ibc "github.com/cosmos/cosmos-sdk/x/ibc/keeper"
	"github.com/dgamingfoundation/hackatom-zone/x/nftapp/types"
	"github.com/spf13/viper"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
)

// Relayer queries for new IBC packets in zones A and B and relays them respectively.
// Currently "relaying" means just issuing transactions with data from IBC packets
// (which is a bit of a cheat, obviously, because I guess we should call ibc/Keeper.Receive()
// on the receiving app).
type Relayer struct {
	contextA      cliCtx.CLIContext
	contextB      cliCtx.CLIContext
	hubRestURL    string
	ownerName     string
	ownerPassword string
	ownerAddr     string
}

func NewRelayer(
	contextA cliCtx.CLIContext,
	contextB cliCtx.CLIContext,
	hubRestURL string,
	ownerName string,
	ownerPassword string,
	ownerAddr string,
) *Relayer {
	return &Relayer{
		contextA:      contextA,
		contextB:      contextB,
		hubRestURL:    hubRestURL,
		ownerName:     ownerName,
		ownerPassword: ownerPassword,
		ownerAddr:     ownerAddr,
	}
}

func (m *Relayer) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			packet, _, err := m.GetRelayPacket()
			if err != nil {
				log.Println("Failed to GetRelayPacket: ", err)
				continue
			}
			if packet == nil {
				log.Println("Packet is nil, skipping")
				continue
			}

			var st types.SellTokenPacket
			if err := json.Unmarshal(packet.Commit(), &st); err != nil {
				log.Println("Failed to unmarshal packet: ", err)
				continue
			}

			if st.Token == nil {
				log.Println("Token is nil, skipping current packet")
				continue
			}
			if err := m.sendTokenToHub(st); err != nil {
				log.Println("Failed to send token to hub: ", err)
				continue
			}
		case <-ctx.Done():
			log.Println("Stopping Relayer")
			return
		}
	}
}

func (m *Relayer) GetRelayPacket() (ibc.Packet, ibc.Proof, error) {
	keeper := ibc.DummyKeeper()
	cdc := m.contextA.Codec

	connID := viper.GetString(cli.FlagConnectionID)
	chanID := viper.GetString(cli.FlagChannelID)

	obj := keeper.Channel.Object(connID, chanID)

	seqBz, _, err := m.query(m.contextB, obj.Seqrecv.Key())
	if err != nil {
		return nil, nil, err
	}
	seq, err := state.DecodeInt(seqBz, state.Dec)
	if err != nil {
		return nil, nil, err
	}

	sentBz, _, err := m.query(m.contextA, obj.Seqsend.Key())
	if err != nil {
		return nil, nil, err
	}
	sent, err := state.DecodeInt(sentBz, state.Dec)
	if err != nil {
		return nil, nil, err
	}

	if seq == sent {
		return nil, nil, errors.New("no packet detected")
	}

	var packet types.SellTokenPacket
	packetBz, proof, err := m.query(m.contextA, obj.Packets.Value(seq).Key())
	if err != nil {
		return nil, nil, err
	}
	cdc.MustUnmarshalBinaryBare(packetBz, &packet)

	return &packet, proof, nil
}

func (m *Relayer) query(ctx cliCtx.CLIContext, key []byte) ([]byte, merkle.Proof, error) {
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

func (m *Relayer) sendTokenToHub(st types.SellTokenPacket) error {
	seqNum, err := m.getSequenceNumber()
	if err != nil {
		return fmt.Errorf("failed to getSequenceNumber: %v", err)
	}

	var reqBytes []byte
	reqObject := putOnMarketNFTReq{
		BaseReq: rest.BaseReq{
			From:          m.ownerAddr,
			ChainID:       "hhchain",
			Sequence:      seqNum,
			AccountNumber: 0,
		},
		Owner:    m.ownerAddr,
		Token:    *st.Token,
		Price:    st.Price,
		Name:     m.ownerName,
		Password: m.ownerPassword,
	}

	cdc := app.MakeCodec()
	reqBytes, err = cdc.MarshalJSON(reqObject)
	if err != nil {
		return fmt.Errorf("error while getting sell token: %v", err)
	}

	req, err := http.NewRequest("POST", m.hubRestURL+"/hh/nft/sell", bytes.NewBuffer(reqBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	body, _ := ioutil.ReadAll(resp.Body)
	log.Println("Hub response: ", body)
	_ = resp.Body.Close()

	return nil
}

func (m *Relayer) getSequenceNumber() (uint64, error) {
	req, err := http.NewRequest("GET", m.hubRestURL+"/auth/accounts/"+m.ownerAddr, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}

	var respData struct {
		Value map[string]interface{} `json:"value"`
	}
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %v", err)
	}
	if err := json.Unmarshal(respBytes, &respData); err != nil {
		return 0, fmt.Errorf("failed to unmarshal response")
	}

	seqNum, ok := respData.Value["sequence"].(string)
	if !ok {
		return 0, errors.New("sequence is missing")
	}

	seqNumUint64, err := strconv.ParseUint(seqNum, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse sequence number `%s`: %v", seqNum, err)
	}

	return seqNumUint64, nil
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

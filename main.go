package main

import (
	"dgamingfoundation/hackatom-relayer/zoneA"
	"dgamingfoundation/hackatom-relayer/zoneB"
	"encoding/json"
	"fmt"

	cli "github.com/cosmos/cosmos-sdk/x/ibc/client/utils"
	"github.com/dgamingfoundation/hackatom-zone/x/nftapp/types"
	"github.com/spf13/viper"
)

func main() {
	viper.Set("trust-node", true)

	cliA := zoneA.GetCLIContext()
	cliB := zoneB.GetCLIContext()
	packet, _, err := cli.GetRelayPacket(cliA, cliB)
	if err != nil {
		panic(fmt.Errorf("failed to GetRelayPacket: %v", err))
	}

	var st types.SellTokenPacket
	if err := json.Unmarshal(packet.Commit(), &st); err != nil {
		panic(fmt.Errorf("failed to unmarshal SellTokenPacket: %v", err))
	}

	fmt.Printf("%+v\n", st)
}

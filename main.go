package main

import (
	"context"
	"dgamingfoundation/hackatom-relayer/lib"
	"dgamingfoundation/hackatom-relayer/zoneA"
	"dgamingfoundation/hackatom-relayer/zoneB"

	"github.com/spf13/viper"
)

func main() {
	viper.Set("trust-node", true)
	cliA := zoneA.GetCLIContext()
	cliB := zoneB.GetCLIContext()

	relayer := lib.NewRelayer(
		cliA,
		cliB,
		"http://localhost:1417",
		"validator1",
		"12345678",
		"cosmos16y2vaas25ea8n353tfve45rwvt4sx0gl627pzn",
	)

	relayer.Run(context.Background()) // Blocks
}

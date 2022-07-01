package app

import (
	"github.com/cosmos/cosmos-sdk/simapp"
)

// Re-export simapp's GenesisState for compatibility
type GenesisState = simapp.GenesisState

// NewDefaultGenesisState generates the default state for the application.
func NewDefaultGenesisState() GenesisState {
	encodingConfig := MakeEncodingConfig()
	return ModuleBasics.DefaultGenesis(encodingConfig.Marshaler)
}

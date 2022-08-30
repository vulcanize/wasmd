package benchmarks

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	dbm "github.com/cosmos/cosmos-sdk/db"
	"github.com/cosmos/cosmos-sdk/simapp"
	"github.com/cosmos/cosmos-sdk/simapp/helpers"
	simappparams "github.com/cosmos/cosmos-sdk/simapp/params"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/CosmWasm/wasmd/app"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

func setup(db dbm.DBConnection, invCheckPeriod uint, opts ...wasm.Option) *app.WasmApp {
	encodingConfig := app.MakeEncodingConfig()
	wasmApp := app.NewWasmApp(log.NewNopLogger(), db, nil, map[int64]bool{}, app.DefaultNodeHome, invCheckPeriod, encodingConfig, wasm.EnableAllProposals, app.EmptyBaseAppOptions{}, opts)
	return wasmApp
}

// SetupWithGenesisAccounts initializes a new WasmApp with the provided genesis
// accounts and possible balances.
func SetupWithGenesisAccounts(b testing.TB, db dbm.DBConnection, genAccs []authtypes.GenesisAccount, balances ...banktypes.Balance) *app.WasmApp {
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey(context.TODO())
	require.NoError(b, err)

	// create validator set with single validator
	validator := tmtypes.NewValidator(pubKey, 1)
	valSet := tmtypes.NewValidatorSet([]*tmtypes.Validator{validator})

	wasmApp := setup(db, 0)
	genesisState := app.NewDefaultGenesisState()
	genesisState = simapp.GenesisStateWithValSet(b, wasmApp.AppCodec(), genesisState, valSet, genAccs, balances...)

	stateBytes, err := json.MarshalIndent(genesisState, "", " ")
	if err != nil {
		panic(err)
	}

	wasmApp.InitChain(
		abci.RequestInitChain{
			Validators:      []abci.ValidatorUpdate{},
			ConsensusParams: app.DefaultConsensusParams,
			AppStateBytes:   stateBytes,
		},
	)

	wasmApp.Commit()
	wasmApp.BeginBlock(abci.RequestBeginBlock{Header: tmproto.Header{Height: wasmApp.LastBlockHeight() + 1}})

	return wasmApp
}

type AppInfo struct {
	App          *app.WasmApp
	MinterKey    *secp256k1.PrivKey
	MinterAddr   sdk.AccAddress
	ContractAddr string
	Denom        string
	AccNum       uint64
	SeqNum       uint64
	TxConfig     client.TxConfig
}

func InitializeWasmApp(b testing.TB, db dbm.DBConnection, numAccounts int) AppInfo {
	// constants
	minter := secp256k1.GenPrivKey()
	addr := sdk.AccAddress(minter.PubKey().Address())
	denom := "uatom"

	// genesis setup (with a bunch of random accounts)
	genAccs := make([]authtypes.GenesisAccount, numAccounts+1)
	bals := make([]banktypes.Balance, numAccounts+1)
	genAccs[0] = &authtypes.BaseAccount{
		Address: addr.String(),
	}
	bals[0] = banktypes.Balance{
		Address: addr.String(),
		Coins:   sdk.NewCoins(sdk.NewInt64Coin(denom, 100000000000)),
	}
	for i := 0; i <= numAccounts; i++ {
		acct := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address()).String()
		if i == 0 {
			acct = addr.String()
		}
		genAccs[i] = &authtypes.BaseAccount{
			Address: acct,
		}
		bals[i] = banktypes.Balance{
			Address: acct,
			Coins:   sdk.NewCoins(sdk.NewInt64Coin(denom, 100000000000)),
		}
	}
	wasmApp := SetupWithGenesisAccounts(b, db, genAccs, bals...)

	// add wasm contract
	height := int64(2)
	txGen := simappparams.MakeTestEncodingConfig().TxConfig
	wasmApp.BeginBlock(abci.RequestBeginBlock{Header: tmproto.Header{Height: height, Time: time.Now()}})

	// upload the code
	cw20Code, err := ioutil.ReadFile("./testdata/cw20_base.wasm")
	require.NoError(b, err)
	storeMsg := wasmtypes.MsgStoreCode{
		Sender:       addr.String(),
		WASMByteCode: cw20Code,
	}
	storeTx, err := helpers.GenSignedMockTx(txGen, []sdk.Msg{&storeMsg}, nil, 55123123, "", []uint64{0}, []uint64{0}, minter)
	require.NoError(b, err)
	_, res, err := wasmApp.SimDeliver(txGen.TxEncoder(), storeTx)
	require.NoError(b, err)
	codeID := uint64(1)

	// instantiate the contract
	initialBalances := make([]balance, numAccounts+1)
	for i := 0; i <= numAccounts; i++ {
		acct := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address()).String()
		if i == 0 {
			acct = addr.String()
		}
		initialBalances[i] = balance{
			Address: acct,
			Amount:  1000000000,
		}
	}
	init := cw20InitMsg{
		Name:            "Cash Money",
		Symbol:          "CASH",
		Decimals:        2,
		InitialBalances: initialBalances,
	}
	initBz, err := json.Marshal(init)
	require.NoError(b, err)
	initMsg := wasmtypes.MsgInstantiateContract{
		Sender: addr.String(),
		Admin:  addr.String(),
		CodeID: codeID,
		Label:  "Demo contract",
		Msg:    initBz,
	}
	gasWanted := 500000 + 10000*uint64(numAccounts)
	initTx, err := helpers.GenSignedMockTx(txGen, []sdk.Msg{&initMsg}, nil, gasWanted, "", []uint64{0}, []uint64{1}, minter)
	require.NoError(b, err)
	_, res, err = wasmApp.SimDeliver(txGen.TxEncoder(), initTx)
	require.NoError(b, err)

	// TODO: parse contract address better
	evt := res.Events[len(res.Events)-1]
	attr := evt.Attributes[0]
	contractAddr := string(attr.Value)

	wasmApp.EndBlock(abci.RequestEndBlock{Height: height})
	wasmApp.Commit()

	return AppInfo{
		App:          wasmApp,
		MinterKey:    minter,
		MinterAddr:   addr,
		ContractAddr: contractAddr,
		Denom:        denom,
		AccNum:       0,
		SeqNum:       2,
		TxConfig:     simappparams.MakeTestEncodingConfig().TxConfig,
	}
}

func GenSequenceOfTxs(b testing.TB, info *AppInfo, msgGen func(*AppInfo) ([]sdk.Msg, error), numToGenerate int) []sdk.Tx {
	fees := sdk.Coins{sdk.NewInt64Coin(info.Denom, 0)}
	txs := make([]sdk.Tx, numToGenerate)

	for i := 0; i < numToGenerate; i++ {
		msgs, err := msgGen(info)
		require.NoError(b, err)
		txs[i], err = helpers.GenSignedMockTx(
			info.TxConfig,
			msgs,
			fees,
			1234567,
			"",
			[]uint64{info.AccNum},
			[]uint64{info.SeqNum},
			info.MinterKey,
		)
		require.NoError(b, err)
		info.SeqNum += 1
	}

	return txs
}

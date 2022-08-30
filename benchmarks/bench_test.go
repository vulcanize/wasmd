package benchmarks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abci "github.com/tendermint/tendermint/abci/types"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	dbm "github.com/cosmos/cosmos-sdk/db"
	"github.com/cosmos/cosmos-sdk/db/memdb"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

func BenchmarkTxSending(b *testing.B) {
	cases := map[string]struct {
		db          func(*testing.B) dbm.DBConnection
		txBuilder   func(*testing.B, *AppInfo) []sdk.Tx
		blockSize   int
		numAccounts int
	}{
		"basic send - memdb": {
			db:          buildMemDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(bankSendMsg),
			numAccounts: 50,
		},
		"cw20 transfer - memdb": {
			db:          buildMemDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(cw20TransferMsg),
			numAccounts: 50,
		},
		"basic send - leveldb": {
			db:          buildBadgerDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(bankSendMsg),
			numAccounts: 50,
		},
		"cw20 transfer - leveldb": {
			db:          buildBadgerDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(cw20TransferMsg),
			numAccounts: 50,
		},
		"basic send - leveldb - 8k accounts": {
			db:          buildBadgerDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(bankSendMsg),
			numAccounts: 8000,
		},
		"cw20 transfer - leveldb - 8k accounts": {
			db:          buildBadgerDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(cw20TransferMsg),
			numAccounts: 8000,
		},
		"basic send - leveldb - 8k accounts - huge blocks": {
			db:          buildBadgerDB,
			blockSize:   1000,
			txBuilder:   buildTxFromMsg(bankSendMsg),
			numAccounts: 8000,
		},
		"cw20 transfer - leveldb - 8k accounts - huge blocks": {
			db:          buildBadgerDB,
			blockSize:   1000,
			txBuilder:   buildTxFromMsg(cw20TransferMsg),
			numAccounts: 8000,
		},
		"basic send - leveldb - 80k accounts": {
			db:          buildBadgerDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(bankSendMsg),
			numAccounts: 80000,
		},
		"cw20 transfer - leveldb - 80k accounts": {
			db:          buildBadgerDB,
			blockSize:   20,
			txBuilder:   buildTxFromMsg(cw20TransferMsg),
			numAccounts: 80000,
		},
	}

	for name, tc := range cases {
		b.Run(name, func(b *testing.B) {
			db := tc.db(b)
			defer db.Close()
			appInfo := InitializeWasmApp(b, db, tc.numAccounts)
			txs := tc.txBuilder(b, &appInfo)

			// number of Tx per block for the benchmarks
			blockSize := tc.blockSize
			height := int64(3)
			txEncoder := appInfo.TxConfig.TxEncoder()

			b.ResetTimer()

			for i := 0; i < b.N/blockSize; i++ {
				appInfo.App.BeginBlock(abci.RequestBeginBlock{Header: tmproto.Header{Height: height, Time: time.Now()}})

				for j := 0; j < blockSize; j++ {
					idx := i*blockSize + j

					_, _, err := appInfo.App.SimCheck(txEncoder, txs[idx])
					if err != nil {
						panic("something is broken in checking transaction")
					}
					_, _, err = appInfo.App.SimDeliver(txEncoder, txs[idx])
					require.NoError(b, err)
				}

				appInfo.App.EndBlock(abci.RequestEndBlock{Height: height})
				appInfo.App.Commit()
				height++
			}
		})
	}
}

func bankSendMsg(info *AppInfo) ([]sdk.Msg, error) {
	// Precompute all txs
	rcpt := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	coins := sdk.Coins{sdk.NewInt64Coin(info.Denom, 100)}
	sendMsg := banktypes.NewMsgSend(info.MinterAddr, rcpt, coins)
	return []sdk.Msg{sendMsg}, nil
}

func cw20TransferMsg(info *AppInfo) ([]sdk.Msg, error) {
	rcpt := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	transfer := cw20ExecMsg{Transfer: &transferMsg{
		Recipient: rcpt.String(),
		Amount:    765,
	}}
	transferBz, err := json.Marshal(transfer)
	if err != nil {
		return nil, err
	}

	sendMsg := &wasmtypes.MsgExecuteContract{
		Sender:   info.MinterAddr.String(),
		Contract: info.ContractAddr,
		Msg:      transferBz,
	}
	return []sdk.Msg{sendMsg}, nil
}

func buildTxFromMsg(builder func(info *AppInfo) ([]sdk.Msg, error)) func(b *testing.B, info *AppInfo) []sdk.Tx {
	return func(b *testing.B, info *AppInfo) []sdk.Tx {
		return GenSequenceOfTxs(b, info, builder, b.N)
	}
}

func buildMemDB(b *testing.B) dbm.DBConnection {
	return memdb.NewDB()
}

func buildBadgerDB(b *testing.B) dbm.DBConnection {
	db, err := dbm.NewDB("testing", dbm.BadgerDBBackend, b.TempDir())
	require.NoError(b, err)
	return db
}

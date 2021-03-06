// nolint
package distribution

import (
	"github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	"github.com/cosmos/cosmos-sdk/x/distribution/tags"
	"github.com/cosmos/cosmos-sdk/x/distribution/types"
)

type (
	Keeper = keeper.Keeper
	Hooks  = keeper.Hooks

	MsgSetWithdrawAddress          = types.MsgSetWithdrawAddress
	MsgWithdrawDelegatorReward     = types.MsgWithdrawDelegatorReward
	MsgWithdrawValidatorCommission = types.MsgWithdrawValidatorCommission

	GenesisState = types.GenesisState

	// expected keepers
	StakingKeeper       = types.StakingKeeper
	BankKeeper          = types.BankKeeper
	FeeCollectionKeeper = types.FeeCollectionKeeper
)

const (
	DefaultCodespace = types.DefaultCodespace
	CodeInvalidInput = types.CodeInvalidInput
	StoreKey         = types.StoreKey
	TStoreKey        = types.TStoreKey
	RouterKey        = types.RouterKey
	QuerierRoute     = types.QuerierRoute
)

var (
	ErrNilDelegatorAddr = types.ErrNilDelegatorAddr
	ErrNilWithdrawAddr  = types.ErrNilWithdrawAddr
	ErrNilValidatorAddr = types.ErrNilValidatorAddr

	TagValidator = tags.Validator
	TagDelegator = tags.Delegator

	NewMsgSetWithdrawAddress          = types.NewMsgSetWithdrawAddress
	NewMsgWithdrawDelegatorReward     = types.NewMsgWithdrawDelegatorReward
	NewMsgWithdrawValidatorCommission = types.NewMsgWithdrawValidatorCommission

	NewKeeper         = keeper.NewKeeper
	DefaultParamspace = keeper.DefaultParamspace

	RegisterCodec       = types.RegisterCodec
	DefaultGenesisState = types.DefaultGenesisState
	ValidateGenesis     = types.ValidateGenesis
	InitialFeePool      = types.InitialFeePool
)

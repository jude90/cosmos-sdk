package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/multisig"

	"github.com/cosmos/cosmos-sdk/codec"

	"github.com/tendermint/go-amino"
	"github.com/tendermint/tendermint/libs/common"

	"github.com/cosmos/cosmos-sdk/client/context"
	"github.com/cosmos/cosmos-sdk/client/keys"
	ckeys "github.com/cosmos/cosmos-sdk/crypto/keys"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authtxb "github.com/cosmos/cosmos-sdk/x/auth/client/txbuilder"
)

// CompleteAndBroadcastTxCli implements a utility function that facilitates
// sending a series of messages in a signed transaction given a TxBuilder and a
// QueryContext. It ensures that the account exists, has a proper number and
// sequence set. In addition, it builds and signs a transaction with the
// supplied messages. Finally, it broadcasts the signed transaction to a node.
//
// NOTE: Also see CompleteAndBroadcastTxREST.
func CompleteAndBroadcastTxCli(txBldr authtxb.TxBuilder, cliCtx context.CLIContext, msgs []sdk.Msg) error {
	txBldr, err := prepareTxBuilder(txBldr, cliCtx)
	if err != nil {
		return err
	}

	name, err := cliCtx.GetFromName()
	if err != nil {
		return err
	}

	if txBldr.GetSimulateAndExecute() || cliCtx.Simulate {
		txBldr, err = EnrichCtxWithGas(txBldr, cliCtx, name, msgs)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "estimated gas = %v\n", txBldr.GetGas())
	}
	if cliCtx.Simulate {
		return nil
	}

	passphrase, err := keys.GetPassphrase(name)
	if err != nil {
		return err
	}

	// build and sign the transaction
	txBytes, err := txBldr.BuildAndSign(name, passphrase, msgs)
	if err != nil {
		return err
	}

	// broadcast to a Tendermint node
	_, err = cliCtx.BroadcastTx(txBytes)
	return err
}

// EnrichCtxWithGas calculates the gas estimate that would be consumed by the
// transaction and set the transaction's respective value accordingly.
func EnrichCtxWithGas(txBldr authtxb.TxBuilder, cliCtx context.CLIContext, name string, msgs []sdk.Msg) (authtxb.TxBuilder, error) {
	_, adjusted, err := simulateMsgs(txBldr, cliCtx, name, msgs)
	if err != nil {
		return txBldr, err
	}
	return txBldr.WithGas(adjusted), nil
}

// CalculateGas simulates the execution of a transaction and returns
// both the estimate obtained by the query and the adjusted amount.
func CalculateGas(queryFunc func(string, common.HexBytes) ([]byte, error), cdc *amino.Codec, txBytes []byte, adjustment float64) (estimate, adjusted uint64, err error) {
	// run a simulation (via /app/simulate query) to
	// estimate gas and update TxBuilder accordingly
	rawRes, err := queryFunc("/app/simulate", txBytes)
	if err != nil {
		return
	}
	estimate, err = parseQueryResponse(cdc, rawRes)
	if err != nil {
		return
	}
	adjusted = adjustGasEstimate(estimate, adjustment)
	return
}

// PrintUnsignedStdTx builds an unsigned StdTx and prints it to os.Stdout.
// Don't perform online validation or lookups if offline is true.
func PrintUnsignedStdTx(w io.Writer, txBldr authtxb.TxBuilder, cliCtx context.CLIContext, msgs []sdk.Msg, offline bool) (err error) {
	var stdTx auth.StdTx
	if offline {
		stdTx, err = buildUnsignedStdTxOffline(txBldr, cliCtx, msgs)
	} else {
		stdTx, err = buildUnsignedStdTx(txBldr, cliCtx, msgs)
	}
	if err != nil {
		return
	}
	json, err := cliCtx.Codec.MarshalJSON(stdTx)
	if err == nil {
		fmt.Fprintf(w, "%s\n", json)
	}
	return
}

func retrieveMultisigKeyFromKeybase(kb ckeys.Keybase, name string) (*multisig.PubKeyMultisigThreshold, error) {
	info, err := kb.Get(name)
	if err != nil {
		return nil, err
	}

	if info.GetType() != ckeys.TypeOffline {
		return nil, fmt.Errorf(
			"%q must be an offline key: %s", name, info.GetType())
	}

	multisigPub, ok := info.GetPubKey().(*multisig.PubKeyMultisigThreshold)
	if !ok {
		return nil, fmt.Errorf("%q must be a multisig public key", name)
	}

	return multisigPub, nil
}

func multisigKeyContainsPubKey(multisigKey *multisig.PubKeyMultisigThreshold, pubkey crypto.PubKey) bool {
	for _, subkey := range multisigKey.PubKeys {
		if subkey.Equals(pubkey) {
			return true
		}
	}
	return false
}

func MultisigSignStdTx(txBldr authtxb.TxBuilder, cliCtx context.CLIContext,
	offlinePubName string, name string, stdTx auth.StdTx,
	offline bool) (signedStdTx auth.StdTx, err error) {

	keybase, err := keys.GetKeyBase()
	if err != nil {
		return signedStdTx, err
	}

	// retrieve keys from keybase
	multisigKey, err := retrieveMultisigKeyFromKeybase(keybase, offlinePubName)
	if err != nil {
		return signedStdTx, err
	}

	info, err := keybase.Get(name)
	if err != nil {
		return signedStdTx, err
	}

	pub := info.GetPubKey()
	if !multisigKeyContainsPubKey(multisigKey, info.GetPubKey()) {
		return signedStdTx, fmt.Errorf(
			"%q does not contain %s", offlinePubName, pub.Address().String())
	}

	// check whether the address is a signer
	if err := assertTxSigner(sdk.AccAddress(multisigKey.Address()), stdTx.GetSigners()); err != nil {
		return signedStdTx, err
	}

	addr := multisigKey.Address()

	if !offline && txBldr.GetAccountNumber() == 0 {
		accNum, err := cliCtx.GetAccountNumber(addr)
		if err != nil {
			return signedStdTx, err
		}
		txBldr = txBldr.WithAccountNumber(accNum)
	}

	if !offline && txBldr.GetSequence() == 0 {
		accSeq, err := cliCtx.GetAccountSequence(addr)
		if err != nil {
			return signedStdTx, err
		}
		txBldr = txBldr.WithSequence(accSeq)
	}

	passphrase, err := keys.GetPassphrase(name)
	if err != nil {
		return signedStdTx, err
	}
	return txBldr.MultisigSignStdTx(name, passphrase, stdTx)
}

// SignStdTx appends a signature to a StdTx and returns a copy of a it. If appendSig
// is false, it replaces the signatures already attached with the new signature.
// Don't perform online validation or lookups if offline is true.
func SignStdTx(txBldr authtxb.TxBuilder, cliCtx context.CLIContext, name string, stdTx auth.StdTx, appendSig bool, offline bool) (auth.StdTx, error) {
	var signedStdTx auth.StdTx

	keybase, err := keys.GetKeyBase()
	if err != nil {
		return signedStdTx, err
	}

	info, err := keybase.Get(name)
	if err != nil {
		return signedStdTx, err
	}

	addr := info.GetPubKey().Address()

	// check whether the address is a signer
	if err := assertTxSigner(sdk.AccAddress(addr), stdTx.GetSigners()); err != nil {
		return signedStdTx, err
	}

	if !offline && txBldr.GetAccountNumber() == 0 {
		accNum, err := cliCtx.GetAccountNumber(addr)
		if err != nil {
			return signedStdTx, err
		}
		txBldr = txBldr.WithAccountNumber(accNum)
	}

	if !offline && txBldr.GetSequence() == 0 {
		accSeq, err := cliCtx.GetAccountSequence(addr)
		if err != nil {
			return signedStdTx, err
		}
		txBldr = txBldr.WithSequence(accSeq)
	}

	passphrase, err := keys.GetPassphrase(name)
	if err != nil {
		return signedStdTx, err
	}

	return txBldr.SignStdTx(name, passphrase, stdTx, appendSig)
}

// GetTxEncoder return tx encoder from global sdk configuration if ones is defined.
// Otherwise returns encoder with default logic.
func GetTxEncoder(cdc *codec.Codec) (encoder sdk.TxEncoder) {
	encoder = sdk.GetConfig().GetTxEncoder()
	if encoder == nil {
		encoder = auth.DefaultTxEncoder(cdc)
	}
	return
}

// nolint
// SimulateMsgs simulates the transaction and returns the gas estimate and the adjusted value.
func simulateMsgs(txBldr authtxb.TxBuilder, cliCtx context.CLIContext, name string, msgs []sdk.Msg) (estimated, adjusted uint64, err error) {
	txBytes, err := txBldr.BuildWithPubKey(name, msgs)
	if err != nil {
		return
	}
	estimated, adjusted, err = CalculateGas(cliCtx.Query, cliCtx.Codec, txBytes, txBldr.GetGasAdjustment())
	return
}

func adjustGasEstimate(estimate uint64, adjustment float64) uint64 {
	return uint64(adjustment * float64(estimate))
}

func parseQueryResponse(cdc *amino.Codec, rawRes []byte) (uint64, error) {
	var simulationResult sdk.Result
	if err := cdc.UnmarshalBinaryLengthPrefixed(rawRes, &simulationResult); err != nil {
		return 0, err
	}
	return simulationResult.GasUsed, nil
}

func prepareTxBuilder(txBldr authtxb.TxBuilder, cliCtx context.CLIContext) (authtxb.TxBuilder, error) {
	if err := cliCtx.EnsureAccountExists(); err != nil {
		return txBldr, err
	}

	from, err := cliCtx.GetFromAddress()
	if err != nil {
		return txBldr, err
	}

	// TODO: (ref #1903) Allow for user supplied account number without
	// automatically doing a manual lookup.
	if txBldr.GetAccountNumber() == 0 {
		accNum, err := cliCtx.GetAccountNumber(from)
		if err != nil {
			return txBldr, err
		}
		txBldr = txBldr.WithAccountNumber(accNum)
	}

	// TODO: (ref #1903) Allow for user supplied account sequence without
	// automatically doing a manual lookup.
	if txBldr.GetSequence() == 0 {
		accSeq, err := cliCtx.GetAccountSequence(from)
		if err != nil {
			return txBldr, err
		}
		txBldr = txBldr.WithSequence(accSeq)
	}
	return txBldr, nil
}

// buildUnsignedStdTx builds a StdTx as per the parameters passed in the
// contexts. Gas is automatically estimated if gas wanted is set to 0.
func buildUnsignedStdTx(txBldr authtxb.TxBuilder, cliCtx context.CLIContext, msgs []sdk.Msg) (stdTx auth.StdTx, err error) {
	txBldr, err = prepareTxBuilder(txBldr, cliCtx)
	if err != nil {
		return
	}
	return buildUnsignedStdTxOffline(txBldr, cliCtx, msgs)
}

func buildUnsignedStdTxOffline(txBldr authtxb.TxBuilder, cliCtx context.CLIContext, msgs []sdk.Msg) (stdTx auth.StdTx, err error) {
	if txBldr.GetSimulateAndExecute() {
		var name string
		name, err = cliCtx.GetFromName()
		if err != nil {
			return
		}

		txBldr, err = EnrichCtxWithGas(txBldr, cliCtx, name, msgs)
		if err != nil {
			return
		}
		fmt.Fprintf(os.Stderr, "estimated gas = %v\n", txBldr.GetGas())
	}
	stdSignMsg, err := txBldr.Build(msgs)
	if err != nil {
		return
	}
	return auth.NewStdTx(stdSignMsg.Msgs, stdSignMsg.Fee, nil, stdSignMsg.Memo), nil
}

func assertTxSigner(user sdk.AccAddress, signers []sdk.AccAddress) error {
	for _, s := range signers {
		if bytes.Equal(user.Bytes(), s.Bytes()) {
			return nil
		}
	}
	return fmt.Errorf("The generated transaction's intended signer does not match the given signer: %s",
		user.String())
}

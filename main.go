package main

import (
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/simapp"
	"github.com/cosmos/cosmos-sdk/simapp/params"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/offchain"
	"io/ioutil"
	"log"
	"net/http"
)

const headerContentTypeJSON = "application/json"
const headerContentTypeProto = "application/protobuf"

func makeCodec() params.EncodingConfig {
	conf := simapp.MakeTestEncodingConfig()
	offchain.RegisterInterfaces(conf.InterfaceRegistry)
	offchain.RegisterLegacyAminoCodec(conf.Amino)
	return conf
}

func writeError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)

	sdkErr, ok := err.(errors.Error)
	if !ok {
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	b, err := json.Marshal(sdkErr)
	if err != nil {
		panic(err)
	}
	_, _ = w.Write(b)
}

func verify(conf params.EncodingConfig) http.HandlerFunc {
	verifier := offchain.NewVerifier(conf.TxConfig.SignModeHandler())
	return func(writer http.ResponseWriter, request *http.Request) {
		var (
			tx sdk.Tx
			err error
		)
		b, err := ioutil.ReadAll(request.Body)
		if err != nil {
			writeError(writer, err)
			return
		}
		contentHeader := request.Header.Get("Content-Type")
		switch contentHeader {
		case headerContentTypeJSON:
			tx, err = conf.TxConfig.TxJSONDecoder()(b)
		case headerContentTypeProto:
			tx, err = conf.TxConfig.TxJSONDecoder()(b)
		default:
			writeError(writer, fmt.Errorf("unknown content type: %s", contentHeader))
			return
		}
		if err != nil {
			writeError(writer, err)
			return
		}

		err = verifier.Verify(tx)
		if err != nil {
			writeError(writer, err)
			return
		}
		_, _ = writer.Write([]byte("valid data:" + string(tx.GetMsgs()[0].(*offchain.MsgSignData).Data)))
	}
}

func sign(conf params.EncodingConfig) http.HandlerFunc {
	var x  struct {
		PrivateKey []byte `json:"private_key"`
		Data string `json:"data"`
	}
	signer := offchain.NewSigner(conf.TxConfig)
	return func(writer http.ResponseWriter, request *http.Request) {
		err := json.NewDecoder(request.Body).Decode(&x)
		if err != nil {
			writeError(writer, err)
			return
		}
		privKey := &secp256k1.PrivKey{Key: x.PrivateKey}
		signedTx, err := signer.Sign(privKey, []sdk.Msg{
			offchain.NewMsgSignData(sdk.AccAddress(privKey.PubKey().Address()), []byte(x.Data)),
		})
		if err != nil {
			writeError(writer, err)
			return
		}

		b, err := conf.TxConfig.TxJSONEncoder()(signedTx)
		if err != nil {
			writeError(writer, err)
			return
		}
		_, _ = writer.Write(b)
	}
}

func main() {
	conf := makeCodec()
	router := &http.ServeMux{}
	router.HandleFunc("/verify", verify(conf))
	router.HandleFunc("/sign", sign(conf))
	log.Fatal(http.ListenAndServe(":8080", router))
}
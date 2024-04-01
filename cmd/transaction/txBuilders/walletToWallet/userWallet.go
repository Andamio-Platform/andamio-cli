package walletToWallet

import (
	"encoding/hex"
	"strings"

	"github.com/Salvionied/apollo/serialization"
	"github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Key"
	"github.com/blinklabs-io/bursa"
)

type UserWallet struct {
	UserPKH  serialization.PubKeyHash
	UserVkey Key.VerificationKey
	UserSkey Key.SigningKey
}

var (
	userWallet = &UserWallet{}
)

func WalletSetup() error {

	mnemonics := "tourist minimum increase mushroom relax sing tree until scrap gas board sign immense kid genre"

	err := SetUserWallet(toMnemonic(mnemonics))
	if err != nil {
		return err
	}

	return nil
}

func SetUserWallet(mnemonic string) error {
	rootKey, err := bursa.GetRootKeyFromMnemonic(mnemonic)
	if err != nil {
		return err
	}

	accountKey := bursa.GetAccountKey(rootKey, 0)
	paymentKey := bursa.GetPaymentKey(accountKey, 0)
	UserAddress, err := Address.DecodeAddress(bursa.GetAddress(accountKey, "preprod", 0).String())
	if err != nil {
		return err
	}

	vKeyBytes, err := hex.DecodeString(bursa.GetPaymentVKey(paymentKey).CborHex)
	if err != nil {
		return err

	}

	sKeyBytes, err := hex.DecodeString(bursa.GetPaymentSKey(paymentKey).CborHex)
	if err != nil {
		return err
	}

	vKeyBytes = vKeyBytes[2:]
	sKeyBytes = sKeyBytes[2:]
	sKeyBytes = append(sKeyBytes[:64], sKeyBytes[96:]...)

	userWallet = &UserWallet{
		UserPKH:  serialization.PubKeyHash(UserAddress.PaymentPart),
		UserVkey: Key.VerificationKey{Payload: vKeyBytes},
		UserSkey: Key.SigningKey{Payload: sKeyBytes},
	}

	return nil
}

func toMnemonic(seedPhrase string) (mnemonic string) {
	words := strings.Fields(seedPhrase)
	mnemonic = strings.Join(words, " ")
	return mnemonic
}

func GetUserWallet() *UserWallet {
	return userWallet
}

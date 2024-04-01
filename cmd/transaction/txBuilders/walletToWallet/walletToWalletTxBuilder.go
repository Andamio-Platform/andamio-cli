package walletToWallet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/Salvionied/apollo"
	"github.com/Salvionied/apollo/serialization"
	"github.com/Salvionied/apollo/txBuilding/Backend/BlockFrostChainContext"
	"github.com/Salvionied/apollo/txBuilding/Utils"
)

type TxFile struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	CborHex     string `json:"cborHex"`
}

func WalletToWalletTxBuilder(originAddress string, destinationAddress string, amount float64, submitOpt int64) (string, error) {

	defer func() {
		if err := recover(); err != nil {
			log.Printf("Panic occurred: %v", err)
			return
		}
	}()

	apiURL := "https://cardano-preprod.blockfrost.io/api"
	networkID := 0
	apiKey := "preprod9zzl4g8Xa3faU50a1OVDZdPeQ92ZsdcT"

	bfc := BlockFrostChainContext.NewBlockfrostChainContext(apiURL, networkID, apiKey)

	apolloBE := apollo.New(&bfc)

	apolloBE = apolloBE.SetWalletFromBech32(originAddress).SetWalletAsChangeAddress()

	originAddressInApolloAddressType := apolloBE.GetWallet().GetAddress()

	userUtxos := bfc.Utxos(*originAddressInApolloAddressType)

	apolloBE, err := apolloBE.
		AddLoadedUTxOs(userUtxos...).
		PayToAddressBech32(destinationAddress, int(amount)).
		AddRequiredSigner(serialization.PubKeyHash(originAddressInApolloAddressType.PaymentPart)).
		Complete()

	if err != nil {
		log.Println(err)
		return "", err
	}

	err = WalletSetup()
	if err != nil {
		return "", err
	}

	userWallet := GetUserWallet()

	apolloBE, err = apolloBE.SignWithSkey(userWallet.UserVkey, userWallet.UserSkey)
	if err != nil {
		log.Println(err)
		return "", err
	}

	tx := apolloBE.GetTx()

	txHex := Utils.ToCbor(tx)

	switch submitOpt {
	case 0: // blockfrost
		txID, err := bfc.SubmitTx(*tx)
		if err != nil {
			log.Println(err)
			return "", err
		}

		if txID.Payload != nil {
			log.Println("Transaction submitted successfully")
			return hex.EncodeToString(txID.Payload), nil

		} else {
			return "", fmt.Errorf("transaction submission failed")
		}

	case 1: // local node / cardano-cli

		submitTxViaCardanoCLI(&txHex)

	default:
		return "", fmt.Errorf("invalid submit option")
	}

	return "", nil
}

func submitTxViaCardanoCLI(txHex *string) {

	err := createTxBodyFile(txHex)
	if err != nil {
		log.Println(err)
		return
	}

	fmt.Println("Submitting transaction via cardano-cli")

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.Command("sh", "echo 'Hello'")
	cmd.Start()
	cmd.Wait()
	// cmd := exec.Command("cardano-cli transaction submit --testnet-magic 1 --tx-file output/txBody.cbor")

	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	fmt.Println(stdoutBuf)
	if stdoutBuf.Len() == 0 {
		log.Println(stdoutBuf.String())
	}

	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	fmt.Println(stderrBuf)
	if stderrBuf.Len() > 0 {
		log.Println(stderrBuf.String())
	}

	cmd.Start()

}

func createTxBodyFile(txBody *string) error {

	txFile := &TxFile{
		Type:        "Witnessed Tx BabbageEra",
		Description: "Ledger Cddl Format",
		CborHex:     *txBody,
	}

	json_data, err := json.MarshalIndent(txFile, "", "\t")
	if err != nil {
		os.Exit(1)
		return err
	}

	err = ensureDBDirectoryExists("output", 0755)
	if err != nil {
		return err
	}

	file, err := os.Create("output/txBody.cbor")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(json_data)
	if err != nil {
		return err
	}

	return nil

}

func ensureDBDirectoryExists(path string, perm os.FileMode) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, perm)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		err = os.Chmod(path, perm)
		if err != nil {
			return fmt.Errorf("failed to change directory permissions: %v", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check directory existence: %v", err)
	}
	return nil
}

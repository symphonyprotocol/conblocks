package block

import "encoding/gob"
import "bytes"
import "log"
import "github.com/symphonyprotocol/sutil/elliptic"
import "crypto/sha256"
import "github.com/boltdb/bolt"


// Transaction represents a Bitcoin transaction
type Transaction struct {
	ID        []byte
	Nonce     int64
	From      string
	To        string
	Amount    int64
	Signature []byte
}

// Serialized Transaction
func (tx Transaction) Serialize() []byte {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return encoded.Bytes()
}
//  Deserializes Transaction
func DeserializeTransction(d []byte) *Transaction {
	var transaction Transaction

	decoder := gob.NewDecoder(bytes.NewReader(d))
	err := decoder.Decode(&transaction)
	if err != nil {
		log.Panic(err)
	}

	return &transaction
}

func (tx *Transaction) Sign(privKey *elliptic.PrivateKey){
	transbytes := tx.Serialize()
	sign_bytes, _ := elliptic.SignCompact(elliptic.S256(), privKey,  transbytes, true)
	tx.Signature = sign_bytes
}

func (tx *Transaction) Verify() bool{
	trans := NewTransaction(tx.Nonce, tx.Amount, tx.From, tx.To)
	transbytes := trans.Serialize()
	recover_pubkey, compressed, err := elliptic.RecoverCompact(elliptic.S256(), tx.Signature, transbytes)
	if err != nil || !compressed{
		return false
	}else{
		address := recover_pubkey.ToAddressCompressed()
		return address == tx.From
	}
}

func (tx *Transaction) SetID() {
	var encoded bytes.Buffer
	var hash [32]byte

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}
	hash = sha256.Sum256(encoded.Bytes())
	tx.ID = hash[:]
}


func NewTransaction(nonce, amount int64, from, to string) *Transaction{
	trans := Transaction{
		Nonce : nonce,
		From : from,
		To : to,
		Amount: amount,
		Signature: []byte(""),
	}
	trans.SetID()
	return &trans
}

func GetMaxUnpackNonce(transactions []* Transaction) int64{
	var nonce int64 = -1
	for _, trans := range transactions{
		if trans.Nonce > nonce{
			nonce = trans.Nonce
		}
	}
	return nonce
}

func SendTo(from, to string, amount int64, wif string){
	_, validFrom := elliptic.LoadAddress(from)
	_, validTo := elliptic.LoadAddress(to)
	prikey, _ := elliptic.LoadWIF(wif)
	private_key, _ := elliptic.PrivKeyFromBytes(elliptic.S256(), prikey)

	var trans * Transaction
	
	if !validFrom{
		log.Panic("ERROR: Sender address is not valid")
	}
	if !validTo{
		log.Panic("ERROR: Recipient address is not valid")
	}

	account := GetAccount(from)
	
	bc := LoadBlockchain()
	db := bc.GetDB()
	defer db.Close()

	unpacktransactions := bc.FindUnpackTransaction(from)
	if len(unpacktransactions) == 0{
		trans = NewTransaction(account.Nonce + 1, amount, from,to)
	}else{
		nonce := GetMaxUnpackNonce(unpacktransactions)
		trans = NewTransaction(nonce + 1, amount, from,to)
	}

	trans.Sign(private_key)

	bc.SaveTransaction(trans)

}

func Mine(address string) []* Transaction{
	bc := LoadBlockchain()
	db := bc.GetDB()
	defer db.Close()

	var transactions []* Transaction

	unpacktransactions := bc.FindAllUnpackTransaction()
	if len(unpacktransactions) > 0{
		for key := range unpacktransactions{
			transactions = unpacktransactions[key]
			break
		}

	}else{
		log.Panic("no transaction can be mine")
	}

	flag := make(chan struct{})

	bc.MineBlock(transactions, func(block *Block){

		err := bc.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(blocksBucket))
			err := b.Put(block.Header.Hash, block.Serialize())
			if err != nil {
				log.Panic(err)
			}
	
			err = b.Put([]byte("l"), block.Header.Hash)
			if err != nil {
				log.Panic(err)
			}
	
			bc.tip = block.Header.Hash
	
			return nil
		})
		if err != nil {
			log.Panic(err)
		}

		for _, v := range transactions{
			bc.db.Update(func(tx *bolt.Tx) error {
				b := tx.Bucket([]byte(packageBucket))
				err = b.Delete(v.ID)
				return err
			})
		}

		//for test
		// bc.AcceptNewBlock(block)


		flag <- struct{}{}
	})
	<- flag

	return transactions
}


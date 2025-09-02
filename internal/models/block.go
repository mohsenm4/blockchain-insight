package models

type Block struct {
	Number       uint64
	Hash         string
	Miner        string
	TxCount      int
	Timestamp    uint64
	Transactions []Transaction
}

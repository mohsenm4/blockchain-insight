package models

type Block struct {
	Number       uint64        `json:"number"`
	Hash         string        `json:"hash"`
	Miner        string        `json:"miner"`
	TxCount      int           `json:"tx_count"`
	Timestamp    uint64        `json:"timestamp"`
	Transactions []Transaction `json:"transactions"`
}

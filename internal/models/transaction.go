package models

import "time"

type Transaction struct {
	Hash   string    `json:"hash"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Value  string    `json:"value"`
	Time   time.Time `json:"time"`
	GasFee string    `json:"gas_fee"`
}

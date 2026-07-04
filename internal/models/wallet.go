package models

import (
    "time"

    "github.com/google/uuid"
)

type Wallet struct {
    ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
    Balance   int64     `gorm:"not null;default:0" json:"balance"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type Transaction struct {
    ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
    WalletID  uuid.UUID `gorm:"type:uuid;not null;index" json:"wallet_id"`
    Type      string    `gorm:"type:varchar(10);not null" json:"type"`
    Amount    int64     `gorm:"not null" json:"amount"`
    Balance   int64     `gorm:"not null" json:"balance"`
    CreatedAt time.Time `json:"created_at"`
}

type OperationType string

const (
    Deposit  OperationType = "DEPOSIT"
    Withdraw OperationType = "WITHDRAW"
)

type WalletRequest struct {
    WalletID      uuid.UUID     `json:"walletId" binding:"required"`
    OperationType OperationType `json:"operationType" binding:"required,oneof=DEPOSIT WITHDRAW"`
    Amount        int64         `json:"amount" binding:"required,min=1"`
}

type WalletResponse struct {
    WalletID uuid.UUID `json:"walletId"`
    Balance  int64     `json:"balance"`
}
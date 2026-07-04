package repository

import (
    "context"
    "errors"

    "github.com/google/uuid"
    "gorm.io/gorm"
    "wallet/internal/models"
)

var (
    ErrWalletNotFound      = errors.New("wallet not found")
    ErrInsufficientBalance = errors.New("insufficient balance")
)

type WalletRepository interface {
    GetByID(ctx context.Context, id uuid.UUID) (*models.Wallet, error)
    Create(ctx context.Context, wallet *models.Wallet) error
    UpdateBalance(ctx context.Context, id uuid.UUID, amount int64) (*models.Wallet, error)
    AddTransaction(ctx context.Context, tx *models.Transaction) error
    GetBalance(ctx context.Context, id uuid.UUID) (int64, error)
}

type walletRepository struct {
    db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) WalletRepository {
    return &walletRepository{db: db}
}

func (r *walletRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Wallet, error) {
    var wallet models.Wallet
    err := r.db.WithContext(ctx).Where("id = ?", id).First(&wallet).Error
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, ErrWalletNotFound
        }
        return nil, err
    }
    return &wallet, nil
}

func (r *walletRepository) Create(ctx context.Context, wallet *models.Wallet) error {
    return r.db.WithContext(ctx).Create(wallet).Error
}

func (r *walletRepository) UpdateBalance(ctx context.Context, id uuid.UUID, amount int64) (*models.Wallet, error) {
    var wallet models.Wallet

    err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        err := tx.Raw(`
            SELECT id, balance 
            FROM wallets 
            WHERE id = ? 
            FOR UPDATE
        `, id).Scan(&wallet).Error
        
        if err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) {
                return ErrWalletNotFound
            }
            return err
        }

        newBalance := wallet.Balance + amount
        if newBalance < 0 {
            return ErrInsufficientBalance
        }

        result := tx.Exec(`
            UPDATE wallets 
            SET balance = ?, updated_at = NOW() 
            WHERE id = ?
        `, newBalance, id)

        if result.Error != nil {
            return result.Error
        }

        wallet.Balance = newBalance
        return nil
    })

    if err != nil {
        return nil, err
    }

    return &wallet, nil
}

func (r *walletRepository) AddTransaction(ctx context.Context, tx *models.Transaction) error {
    return r.db.WithContext(ctx).Create(tx).Error
}

func (r *walletRepository) GetBalance(ctx context.Context, id uuid.UUID) (int64, error) {
    var wallet models.Wallet
    err := r.db.WithContext(ctx).Select("balance").Where("id = ?", id).First(&wallet).Error
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return 0, ErrWalletNotFound
        }
        return 0, err
    }
    return wallet.Balance, nil
}

package service

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"wallet/internal/models"
	"wallet/internal/repository"
)

type WalletService interface {
	ProcessTransaction(ctx context.Context, req *models.WalletRequest) (*models.WalletResponse, error)
	GetBalance(ctx context.Context, walletID uuid.UUID) (int64, error)
	CreateWallet(ctx context.Context) (*models.WalletResponse, error)
}

type walletService struct {
	walletRepo  repository.WalletRepository
	logger      *zap.Logger
	mu          *sync.RWMutex
	walletCache map[uuid.UUID]int64
}

func NewWalletService(walletRepo repository.WalletRepository, logger *zap.Logger) WalletService {
	return &walletService{
		walletRepo:  walletRepo,
		logger:      logger,
		mu:          &sync.RWMutex{},
		walletCache: make(map[uuid.UUID]int64),
	}
}

func (s *walletService) ProcessTransaction(ctx context.Context, req *models.WalletRequest) (*models.WalletResponse, error) {
	var amount int64
	if req.OperationType == models.Deposit {
		amount = req.Amount
	} else {
		amount = -req.Amount
	}

	wallet, err := s.walletRepo.UpdateBalance(ctx, req.WalletID, amount)
	if err != nil {
		s.logger.Error("failed to update balance",
			zap.String("wallet_id", req.WalletID.String()),
			zap.Int64("amount", amount),
			zap.Error(err),
		)
		return nil, err
	}

	transaction := &models.Transaction{
		WalletID: req.WalletID,
		Type:     string(req.OperationType),
		Amount:   req.Amount,
		Balance:  wallet.Balance,
	}

	if err := s.walletRepo.AddTransaction(ctx, transaction); err != nil {
		s.logger.Error("failed to record transaction",
			zap.String("wallet_id", req.WalletID.String()),
			zap.Error(err),
		)
	}

	s.mu.Lock()
	s.walletCache[req.WalletID] = wallet.Balance
	s.mu.Unlock()

	return &models.WalletResponse{
		WalletID: req.WalletID,
		Balance:  wallet.Balance,
	}, nil
}

func (s *walletService) GetBalance(ctx context.Context, walletID uuid.UUID) (int64, error) {
	s.mu.RLock()
	if balance, ok := s.walletCache[walletID]; ok {
		s.mu.RUnlock()
		return balance, nil
	}
	s.mu.RUnlock()

	balance, err := s.walletRepo.GetBalance(ctx, walletID)
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	s.walletCache[walletID] = balance
	s.mu.Unlock()

	return balance, nil
}

func (s *walletService) CreateWallet(ctx context.Context) (*models.WalletResponse, error) {
	wallet := &models.Wallet{
		Balance: 0,
	}

	if err := s.walletRepo.Create(ctx, wallet); err != nil {
		s.logger.Error("failed to create wallet", zap.Error(err))
		return nil, err
	}

	return &models.WalletResponse{
		WalletID: wallet.ID,
		Balance:  wallet.Balance,
	}, nil
}

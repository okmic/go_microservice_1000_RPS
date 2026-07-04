package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"wallet/internal/models"
	"wallet/internal/repository"
	"wallet/internal/service"
)

type WalletHandler struct {
	walletService service.WalletService
	logger        *zap.Logger
}

func NewWalletHandler(walletService service.WalletService, logger *zap.Logger) *WalletHandler {
	return &WalletHandler{
		walletService: walletService,
		logger:        logger,
	}
}

func (h *WalletHandler) ProcessTransaction(c *gin.Context) {
	var req models.WalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := h.walletService.ProcessTransaction(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to process transaction", zap.Error(err))
		
		switch {
		case errors.Is(err, repository.ErrWalletNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		case errors.Is(err, repository.ErrInsufficientBalance):
			c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient balance"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *WalletHandler) GetBalance(c *gin.Context) {
	walletIDStr := c.Param("walletId")
	walletID, err := uuid.Parse(walletIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid wallet id"})
		return
	}

	balance, err := h.walletService.GetBalance(c.Request.Context(), walletID)
	if err != nil {
		h.logger.Error("failed to get balance", zap.Error(err))
		
		if errors.Is(err, repository.ErrWalletNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "wallet not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"walletId": walletID,
		"balance":  balance,
	})
}

func (h *WalletHandler) CreateWallet(c *gin.Context) {
	response, err := h.walletService.CreateWallet(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to create wallet", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusCreated, response)
}
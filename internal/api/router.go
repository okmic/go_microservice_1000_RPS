package api

import (
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
    "wallet/internal/api/handlers"
    "wallet/internal/api/middlewares"
)

func SetupRouter(
    walletHandler *handlers.WalletHandler,
    logger *zap.Logger,
) *gin.Engine {
    router := gin.New()
    router.Use(gin.Recovery())
    router.Use(middlewares.Logger(logger))

    api := router.Group("/api/v1")
    {
        wallets := api.Group("/wallets")
        {
            wallets.POST("/", walletHandler.CreateWallet)
            wallets.POST("/transaction", walletHandler.ProcessTransaction)
            wallets.GET("/:walletId", walletHandler.GetBalance)
        }
    }

    return router
}

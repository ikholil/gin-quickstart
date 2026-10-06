package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gin-quickstart/internal/model"
	"gin-quickstart/internal/repository"
	"gin-quickstart/internal/service"
)

var errPayloadTooLarge = errors.New("request body exceeds limit")

type Server struct {
	service *service.Service
}

func New(service *service.Service) *Server {
	return &Server{service: service}
}

func (s *Server) Router() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "ok"}})
	})
	router.GET("/readyz", func(c *gin.Context) {
		if err := s.service.Ping(c.Request.Context()); err != nil {
			log.Printf("readiness check failed: %v", err)
			c.JSON(http.StatusServiceUnavailable, errorEnvelope{
				Error: apiError{Code: "not_ready", Message: "service is not ready"},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "ready"}})
	})

	api := router.Group("/api/v1")
	api.POST("/auth/register", s.register)
	api.POST("/auth/login", s.login)
	api.GET("/categories", s.listCategories)
	api.GET("/categories/:id", s.getCategory)
	api.GET("/products", s.listProducts)
	api.GET("/products/:id", s.getProduct)

	customer := api.Group("")
	customer.Use(s.authenticate())
	customer.GET("/cart", s.getCart)
	customer.POST("/cart/items", s.addCartItem)
	customer.PATCH("/cart/items/:productID", s.setCartItem)
	customer.DELETE("/cart/items/:productID", s.removeCartItem)
	customer.POST("/orders", s.checkout)
	customer.GET("/orders", s.listCustomerOrders)
	customer.GET("/orders/:id", s.getCustomerOrder)

	admin := api.Group("/admin")
	admin.Use(s.authenticate(), s.requireAdmin())
	admin.GET("/categories", s.listAdminCategories)
	admin.GET("/categories/:id", s.getAdminCategory)
	admin.POST("/categories", s.createCategory)
	admin.PATCH("/categories/:id", s.updateCategory)
	admin.GET("/products", s.listAdminProducts)
	admin.GET("/products/:id", s.getAdminProduct)
	admin.POST("/products", s.createProduct)
	admin.PATCH("/products/:id", s.updateProduct)
	admin.GET("/orders", s.listAdminOrders)
	admin.GET("/orders/:id", s.getAdminOrder)
	admin.POST("/orders/:id/cancel", s.cancelOrder)
	return router
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeData(c *gin.Context, status int, value any) {
	c.JSON(status, gin.H{"data": value})
}

func writePage[T any](c *gin.Context, status int, page model.Page[T]) {
	c.JSON(status, gin.H{
		"data": page.Items,
		"pagination": gin.H{
			"page":      page.Page,
			"page_size": page.PageSize,
			"total":     page.Total,
		},
	})
}

func writeError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "an internal error occurred"
	switch {
	case errors.Is(err, errPayloadTooLarge):
		status, code, message = http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 1 MiB"
	case errors.Is(err, service.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "invalid_input", err.Error()
	case errors.Is(err, service.ErrInvalidCredentials):
		status, code, message = http.StatusUnauthorized, "invalid_credentials", "invalid email or password"
	case errors.Is(err, service.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, "unauthorized", "authentication is required"
	case errors.Is(err, service.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "access is forbidden"
	case errors.Is(err, repository.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, repository.ErrConflict):
		status, code, message = http.StatusConflict, "conflict", "the request conflicts with the current resource state"
	default:
		log.Printf("request failed: %v", err)
	}
	c.JSON(status, errorEnvelope{Error: apiError{Code: code, Message: message}})
}

func decodeJSON(c *gin.Context, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return errPayloadTooLarge
		}
		return fmt.Errorf("%w: malformed JSON body", service.ErrInvalidInput)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: request must contain one JSON value", service.ErrInvalidInput)
	}
	return nil
}

func pagination(c *gin.Context) (int, int, error) {
	page, pageSize := 1, 20
	if raw := c.Query("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 1_000_000_000 {
			return 0, 0, fmt.Errorf("%w: page must be a positive integer", service.ErrInvalidInput)
		}
		page = value
	}
	if raw := c.Query("page_size"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, fmt.Errorf("%w: page_size must be between 1 and 100", service.ErrInvalidInput)
		}
		pageSize = value
	}
	return page, pageSize, nil
}

const maxInt = int(^uint(0) >> 1)

func paginated(c *gin.Context) (int, int, error) {
	page, pageSize, err := pagination(c)
	if err != nil {
		return 0, 0, err
	}
	if page-1 > maxInt/pageSize {
		return 0, 0, fmt.Errorf("%w: page offset is too large", service.ErrInvalidInput)
	}
	return page, pageSize, nil
}

func (s *Server) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") {
			writeError(c, service.ErrUnauthorized)
			c.Abort()
			return
		}
		userID, role, err := s.service.ParseToken(header[1])
		if err != nil {
			writeError(c, service.ErrUnauthorized)
			c.Abort()
			return
		}
		c.Set("user_id", userID)
		c.Set("role", role)
		c.Next()
	}
}

func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != "admin" {
			writeError(c, service.ErrForbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}

func userID(c *gin.Context) string {
	id, _ := c.Get("user_id")
	value, _ := id.(string)
	return value
}

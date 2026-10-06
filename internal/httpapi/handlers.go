package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gin-quickstart/internal/model"
	"gin-quickstart/internal/service"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) register(c *gin.Context) {
	var request credentialsRequest
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	user, err := s.service.Register(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, user)
}

func (s *Server) login(c *gin.Context) {
	var request credentialsRequest
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	result, err := s.service.Login(c.Request.Context(), request.Email, request.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, result)
}

func (s *Server) listCategories(c *gin.Context) {
	page, pageSize, err := paginated(c)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.service.Categories(c.Request.Context(), page, pageSize)
	if err != nil {
		writeError(c, err)
		return
	}
	writePage(c, http.StatusOK, result)
}

func (s *Server) listAdminCategories(c *gin.Context) {
	page, pageSize, err := paginated(c)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.service.AdminCategories(c.Request.Context(), page, pageSize)
	if err != nil {
		writeError(c, err)
		return
	}
	writePage(c, http.StatusOK, result)
}

func (s *Server) getCategory(c *gin.Context) {
	category, err := s.service.Category(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, category)
}

func (s *Server) getAdminCategory(c *gin.Context) {
	category, err := s.service.AdminCategory(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, category)
}

func (s *Server) listProducts(c *gin.Context) {
	page, pageSize, err := paginated(c)
	if err != nil {
		writeError(c, err)
		return
	}
	query, categoryID := strings.TrimSpace(c.Query("q")), strings.TrimSpace(c.Query("category_id"))
	if len(query) > 200 || len(categoryID) > 100 {
		writeError(c, service.ErrInvalidInput)
		return
	}
	result, err := s.service.Products(c.Request.Context(), model.ProductFilter{
		Query: query, CategoryID: categoryID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writePage(c, http.StatusOK, result)
}

func (s *Server) getProduct(c *gin.Context) {
	product, err := s.service.Product(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, product)
}

func (s *Server) listAdminProducts(c *gin.Context) {
	page, pageSize, err := paginated(c)
	if err != nil {
		writeError(c, err)
		return
	}
	query, categoryID := strings.TrimSpace(c.Query("q")), strings.TrimSpace(c.Query("category_id"))
	if len(query) > 200 || len(categoryID) > 100 {
		writeError(c, service.ErrInvalidInput)
		return
	}
	result, err := s.service.AdminProducts(c.Request.Context(), model.ProductFilter{
		Query: query, CategoryID: categoryID, Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writePage(c, http.StatusOK, result)
}

func (s *Server) getAdminProduct(c *gin.Context) {
	product, err := s.service.AdminProduct(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, product)
}

func (s *Server) createCategory(c *gin.Context) {
	var request struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	category, err := s.service.CreateCategory(c.Request.Context(), request.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, category)
}

func (s *Server) updateCategory(c *gin.Context) {
	var request struct {
		Name   *string `json:"name"`
		Active *bool   `json:"active"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	category, err := s.service.UpdateCategory(c.Request.Context(), c.Param("id"), request.Name, request.Active)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, category)
}

func (s *Server) createProduct(c *gin.Context) {
	var request struct {
		CategoryID  string `json:"category_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PriceCents  *int64 `json:"price_cents"`
		Stock       *int64 `json:"stock"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	if request.PriceCents == nil || request.Stock == nil {
		writeError(c, service.ErrInvalidInput)
		return
	}
	product, err := s.service.CreateProduct(c.Request.Context(), model.Product{
		CategoryID: request.CategoryID, Name: request.Name, Description: request.Description,
		PriceCents: *request.PriceCents, Stock: *request.Stock,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, product)
}

func (s *Server) updateProduct(c *gin.Context) {
	var request model.ProductPatch
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	product, err := s.service.UpdateProduct(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, product)
}

func (s *Server) getCart(c *gin.Context) {
	cart, err := s.service.Cart(c.Request.Context(), userID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, cart)
}

func (s *Server) addCartItem(c *gin.Context) {
	var request struct {
		ProductID string `json:"product_id"`
		Quantity  int64  `json:"quantity"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	if err := s.service.AddCartItem(c.Request.Context(), userID(c), request.ProductID, request.Quantity); err != nil {
		writeError(c, err)
		return
	}
	cart, err := s.service.Cart(c.Request.Context(), userID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, cart)
}

func (s *Server) setCartItem(c *gin.Context) {
	var request struct {
		Quantity int64 `json:"quantity"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	if err := s.service.SetCartItem(c.Request.Context(), userID(c), c.Param("productID"), request.Quantity); err != nil {
		writeError(c, err)
		return
	}
	cart, err := s.service.Cart(c.Request.Context(), userID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, cart)
}

func (s *Server) removeCartItem(c *gin.Context) {
	if err := s.service.RemoveCartItem(c.Request.Context(), userID(c), c.Param("productID")); err != nil {
		writeError(c, err)
		return
	}
	cart, err := s.service.Cart(c.Request.Context(), userID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, cart)
}

func (s *Server) checkout(c *gin.Context) {
	var request struct {
		ShippingAddress model.Address `json:"shipping_address"`
	}
	if err := decodeJSON(c, &request); err != nil {
		writeError(c, err)
		return
	}
	order, err := s.service.Checkout(c.Request.Context(), userID(c), request.ShippingAddress)
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusCreated, order)
}

func (s *Server) listCustomerOrders(c *gin.Context) {
	page, pageSize, err := paginated(c)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.service.Orders(c.Request.Context(), userID(c), page, pageSize)
	if err != nil {
		writeError(c, err)
		return
	}
	writePage(c, http.StatusOK, result)
}

func (s *Server) getCustomerOrder(c *gin.Context) {
	order, err := s.service.Order(c.Request.Context(), c.Param("id"), userID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, order)
}

func (s *Server) listAdminOrders(c *gin.Context) {
	page, pageSize, err := paginated(c)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := s.service.Orders(c.Request.Context(), "", page, pageSize)
	if err != nil {
		writeError(c, err)
		return
	}
	writePage(c, http.StatusOK, result)
}

func (s *Server) getAdminOrder(c *gin.Context) {
	order, err := s.service.Order(c.Request.Context(), c.Param("id"), "")
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, order)
}

func (s *Server) cancelOrder(c *gin.Context) {
	order, err := s.service.CancelOrder(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeData(c, http.StatusOK, order)
}

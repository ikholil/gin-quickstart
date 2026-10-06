package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"gin-quickstart/internal/database"
	"gin-quickstart/internal/httpapi"
	"gin-quickstart/internal/repository"
	"gin-quickstart/internal/service"
)

func newRouter(t *testing.T) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	app := service.New(repository.New(db), []byte("0123456789abcdef0123456789abcdef"), time.Minute)
	if err := app.EnsureAdmin(context.Background(), "admin@example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	return httpapi.New(app).Router()
}

func requestJSON(t *testing.T, handler http.Handler, method, path, token string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &body)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func responseData(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response %q: %v", response.Body.String(), err)
	}
	return envelope.Data
}

func TestAuthAndAdminProtectedCatalogRoutes(t *testing.T) {
	router := newRouter(t)

	if response := requestJSON(t, router, http.MethodGet, "/healthz", "", nil); response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", response.Code)
	}
	if response := requestJSON(t, router, http.MethodGet, "/readyz", "", nil); response.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, want 200", response.Code)
	}
	if response := requestJSON(t, router, http.MethodGet, "/api/v1/admin/orders", "", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin status = %d, want 401", response.Code)
	}

	register := requestJSON(t, router, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": "customer@example.com", "password": "correct horse battery",
	})
	if register.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body=%s", register.Code, register.Body.String())
	}
	if bytes.Contains(register.Body.Bytes(), []byte("password_hash")) {
		t.Fatal("registration response leaked password hash")
	}
	customerLogin := requestJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "customer@example.com", "password": "correct horse battery",
	})
	if customerLogin.Code != http.StatusOK {
		t.Fatalf("customer login status = %d, body=%s", customerLogin.Code, customerLogin.Body.String())
	}
	customerToken, ok := responseData(t, customerLogin)["access_token"].(string)
	if !ok || customerToken == "" {
		t.Fatalf("login response has no access token: %s", customerLogin.Body.String())
	}
	if response := requestJSON(t, router, http.MethodPost, "/api/v1/admin/categories", customerToken,
		map[string]string{"name": "Forbidden"}); response.Code != http.StatusForbidden {
		t.Fatalf("customer admin status = %d, want 403", response.Code)
	}

	adminLogin := requestJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "admin@example.com", "password": "a sufficiently long password",
	})
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("admin login status = %d, body=%s", adminLogin.Code, adminLogin.Body.String())
	}
	adminToken, ok := responseData(t, adminLogin)["access_token"].(string)
	if !ok || adminToken == "" {
		t.Fatalf("admin login has no access token: %s", adminLogin.Body.String())
	}

	category := requestJSON(t, router, http.MethodPost, "/api/v1/admin/categories", adminToken,
		map[string]string{"name": "Books"})
	if category.Code != http.StatusCreated {
		t.Fatalf("create category status = %d, body=%s", category.Code, category.Body.String())
	}
	categoryID, ok := responseData(t, category)["id"].(string)
	if !ok || categoryID == "" {
		t.Fatalf("category response has no id: %s", category.Body.String())
	}
	product := requestJSON(t, router, http.MethodPost, "/api/v1/admin/products", adminToken, map[string]any{
		"category_id": categoryID, "name": "Book", "description": "",
		"price_cents": 1299, "stock": 4,
	})
	if product.Code != http.StatusCreated {
		t.Fatalf("create product status = %d, body=%s", product.Code, product.Body.String())
	}
	productID, ok := responseData(t, product)["id"].(string)
	if !ok || productID == "" {
		t.Fatalf("product response has no id: %s", product.Body.String())
	}
	updated := requestJSON(t, router, http.MethodPatch, "/api/v1/admin/products/"+productID, adminToken,
		map[string]any{"price_cents": 0, "description": ""})
	if updated.Code != http.StatusOK {
		t.Fatalf("update product status = %d, body=%s", updated.Code, updated.Body.String())
	}
	updatedProduct := responseData(t, updated)
	if updatedProduct["price_cents"] != float64(0) || updatedProduct["description"] != "" {
		t.Fatalf("zero and empty patch values were not applied: %s", updated.Body.String())
	}
	archivedProduct := requestJSON(t, router, http.MethodPatch, "/api/v1/admin/products/"+productID, adminToken,
		map[string]any{"active": false})
	if archivedProduct.Code != http.StatusOK || responseData(t, archivedProduct)["active"] != false {
		t.Fatalf("archive product status = %d, body=%s", archivedProduct.Code, archivedProduct.Body.String())
	}
	if response := requestJSON(t, router, http.MethodGet, "/api/v1/products/"+productID, "", nil); response.Code != http.StatusNotFound {
		t.Fatalf("public archived product status = %d, want 404", response.Code)
	}
	if response := requestJSON(t, router, http.MethodGet, "/api/v1/admin/products/"+productID, adminToken, nil); response.Code != http.StatusOK {
		t.Fatalf("admin archived product detail status = %d, body=%s", response.Code, response.Body.String())
	}
	adminProducts := requestJSON(t, router, http.MethodGet, "/api/v1/admin/products?page=1&page_size=1", adminToken, nil)
	if adminProducts.Code != http.StatusOK {
		t.Fatalf("admin product list status = %d, body=%s", adminProducts.Code, adminProducts.Body.String())
	}
	var archivedList struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(adminProducts.Body.Bytes(), &archivedList); err != nil {
		t.Fatal(err)
	}
	if len(archivedList.Data) != 1 || archivedList.Data[0]["active"] != false {
		t.Fatalf("admin product list omitted archived product: %s", adminProducts.Body.String())
	}
	reactivated := requestJSON(t, router, http.MethodPatch, "/api/v1/admin/products/"+productID, adminToken,
		map[string]any{"active": true})
	if reactivated.Code != http.StatusOK {
		t.Fatalf("reactivate product status = %d, body=%s", reactivated.Code, reactivated.Body.String())
	}
	archivedCategory := requestJSON(t, router, http.MethodPatch, "/api/v1/admin/categories/"+categoryID, adminToken,
		map[string]any{"active": false})
	if archivedCategory.Code != http.StatusOK {
		t.Fatalf("archive category status = %d, body=%s", archivedCategory.Code, archivedCategory.Body.String())
	}
	if response := requestJSON(t, router, http.MethodGet, "/api/v1/categories/"+categoryID, "", nil); response.Code != http.StatusNotFound {
		t.Fatalf("public archived category status = %d, want 404", response.Code)
	}
	adminCategory := requestJSON(t, router, http.MethodGet, "/api/v1/admin/categories/"+categoryID, adminToken, nil)
	if adminCategory.Code != http.StatusOK || responseData(t, adminCategory)["active"] != false {
		t.Fatalf("admin archived category detail status = %d, body=%s", adminCategory.Code, adminCategory.Body.String())
	}
	adminCategories := requestJSON(t, router, http.MethodGet, "/api/v1/admin/categories?page=1&page_size=1", adminToken, nil)
	if adminCategories.Code != http.StatusOK {
		t.Fatalf("admin category list status = %d, body=%s", adminCategories.Code, adminCategories.Body.String())
	}
	reopenedCategory := requestJSON(t, router, http.MethodPatch, "/api/v1/admin/categories/"+categoryID, adminToken,
		map[string]any{"active": true})
	if reopenedCategory.Code != http.StatusOK {
		t.Fatalf("reactivate category status = %d, body=%s", reopenedCategory.Code, reopenedCategory.Body.String())
	}

	list := requestJSON(t, router, http.MethodGet, "/api/v1/products?q=Book&page=1&page_size=1", "", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("product list status = %d, body=%s", list.Code, list.Body.String())
	}
	var listEnvelope struct {
		Data       []map[string]any `json:"data"`
		Pagination struct {
			Page     int   `json:"page"`
			PageSize int   `json:"page_size"`
			Total    int64 `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(listEnvelope.Data) != 1 || listEnvelope.Pagination.Page != 1 ||
		listEnvelope.Pagination.PageSize != 1 || listEnvelope.Pagination.Total != 1 {
		t.Fatalf("unexpected list response: %s", list.Body.String())
	}

	added := requestJSON(t, router, http.MethodPost, "/api/v1/cart/items", customerToken, map[string]any{
		"product_id": productID, "quantity": 2,
	})
	if added.Code != http.StatusOK {
		t.Fatalf("add cart item status = %d, body=%s", added.Code, added.Body.String())
	}
	checkedOut := requestJSON(t, router, http.MethodPost, "/api/v1/orders", customerToken, map[string]any{
		"shipping_address": map[string]string{
			"recipient_name": "Customer", "line1": "1 Main St", "city": "Example", "country_code": "US",
		},
	})
	if checkedOut.Code != http.StatusCreated {
		t.Fatalf("checkout status = %d, body=%s", checkedOut.Code, checkedOut.Body.String())
	}
	orderID, ok := responseData(t, checkedOut)["id"].(string)
	if !ok || orderID == "" {
		t.Fatalf("checkout response has no order id: %s", checkedOut.Body.String())
	}
	if response := requestJSON(t, router, http.MethodGet, "/api/v1/orders/"+orderID, customerToken, nil); response.Code != http.StatusOK {
		t.Fatalf("customer order read status = %d, body=%s", response.Code, response.Body.String())
	}
	cancelled := requestJSON(t, router, http.MethodPost, "/api/v1/admin/orders/"+orderID+"/cancel", adminToken, nil)
	if cancelled.Code != http.StatusOK || responseData(t, cancelled)["state"] != "cancelled" {
		t.Fatalf("admin cancellation status = %d, body=%s", cancelled.Code, cancelled.Body.String())
	}
	refreshedProduct := requestJSON(t, router, http.MethodGet, "/api/v1/products/"+productID, "", nil)
	if refreshedProduct.Code != http.StatusOK || responseData(t, refreshedProduct)["stock"] != float64(4) {
		t.Fatalf("cancellation did not restore inventory: %s", refreshedProduct.Body.String())
	}
}

func TestStrictJSONAndPaginationValidation(t *testing.T) {
	router := newRouter(t)
	unknown := requestJSON(t, router, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": "customer@example.com", "password": "correct horse battery", "role": "admin",
	})
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", unknown.Code)
	}
	invalidPage := requestJSON(t, router, http.MethodGet, "/api/v1/products?page_size=101", "", nil)
	if invalidPage.Code != http.StatusBadRequest {
		t.Fatalf("invalid page_size status = %d, want 400", invalidPage.Code)
	}
	var errorBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(unknown.Body.Bytes(), &errorBody); err != nil {
		t.Fatal(err)
	}
	if errorBody.Error.Code != "invalid_input" {
		t.Fatalf("error code = %q, want invalid_input", errorBody.Error.Code)
	}
}

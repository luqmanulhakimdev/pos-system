package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/luqmanulhakimdev/pos-system/internal/domain"
)

var (
	ErrInvalidCheckout    = errors.New("invalid checkout request")
	ErrProductUnavailable = errors.New("product unavailable")
)

type RequestedItem struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

type CheckoutRequest struct {
	CashierID  int64
	CustomerID *int64
	Items      []RequestedItem
}

type ProductSnapshot struct {
	ID           int64
	Name         string
	SKU          string
	PriceMinor   int64
	Currency     string
	InventoryQty int64
}

type OrderRecord struct {
	OrderNumber   string
	CustomerID    *int64
	CashierID     int64
	SubtotalMinor int64
	Currency      string
}

type CheckoutTransaction interface {
	LockProductAndInventory(context.Context, int64) (ProductSnapshot, error)
	CreateOrder(context.Context, OrderRecord) (int64, error)
	CreateOrderItem(context.Context, int64, domain.OrderItem) error
	SetInventoryQuantity(context.Context, int64, int64) error
	AppendStockMovement(context.Context, int64, int64, int64, domain.MovementType, int64, string) error
	AppendAuditLog(context.Context, int64, string, string, string, map[string]any) error
}

type CheckoutStore interface {
	WithinTransaction(context.Context, func(CheckoutTransaction) error) error
}

type Checkout struct {
	store CheckoutStore
	now   func() time.Time
}

func NewCheckout(store CheckoutStore) *Checkout {
	return &Checkout{store: store, now: time.Now}
}

func (c *Checkout) Execute(ctx context.Context, request CheckoutRequest) (domain.Order, error) {
	if c.store == nil || request.CashierID <= 0 || len(request.Items) == 0 || len(request.Items) > 100 {
		return domain.Order{}, ErrInvalidCheckout
	}
	if request.CustomerID != nil && *request.CustomerID <= 0 {
		return domain.Order{}, ErrInvalidCheckout
	}
	quantities := make(map[int64]int64, len(request.Items))
	for _, item := range request.Items {
		if item.ProductID <= 0 || item.Quantity <= 0 || quantities[item.ProductID] > math.MaxInt64-item.Quantity {
			return domain.Order{}, ErrInvalidCheckout
		}
		quantities[item.ProductID] += item.Quantity
	}
	productIDs := make([]int64, 0, len(quantities))
	for id := range quantities {
		productIDs = append(productIDs, id)
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })

	orderNumber, err := newOrderNumber(c.now())
	if err != nil {
		return domain.Order{}, fmt.Errorf("generate order number: %w", err)
	}
	var result domain.Order
	err = c.store.WithinTransaction(ctx, func(tx CheckoutTransaction) error {
		items := make([]domain.OrderItem, 0, len(productIDs))
		products := make(map[int64]ProductSnapshot, len(productIDs))
		for _, productID := range productIDs {
			product, err := tx.LockProductAndInventory(ctx, productID)
			if err != nil {
				return err
			}
			if product.ID != productID || product.InventoryQty < 0 {
				return ErrProductUnavailable
			}
			quantity := quantities[productID]
			if quantity > product.InventoryQty {
				return domain.ErrInsufficientStock
			}
			item, err := domain.NewOrderItem(product.ID, product.Name, product.SKU, quantity, product.PriceMinor, product.Currency)
			if err != nil {
				return err
			}
			items = append(items, item)
			products[productID] = product
		}
		order, err := domain.NewOrder(items, domain.OrderPending)
		if err != nil {
			return err
		}
		orderID, err := tx.CreateOrder(ctx, OrderRecord{
			OrderNumber:   orderNumber,
			CustomerID:    request.CustomerID,
			CashierID:     request.CashierID,
			SubtotalMinor: order.SubtotalMinor,
			Currency:      order.Currency,
		})
		if err != nil {
			return err
		}
		for _, item := range order.Items {
			product := products[item.ProductID]
			updated, err := domain.ApplyMovement(domain.Inventory{ProductID: item.ProductID, Quantity: product.InventoryQty}, domain.StockMovement{
				ProductID: item.ProductID,
				Type:      domain.MovementSale,
				Delta:     -item.Quantity,
				Reason:    "checkout " + orderNumber,
			})
			if err != nil {
				return err
			}
			if err := tx.CreateOrderItem(ctx, orderID, item); err != nil {
				return err
			}
			if err := tx.SetInventoryQuantity(ctx, item.ProductID, updated.Quantity); err != nil {
				return err
			}
			if err := tx.AppendStockMovement(ctx, item.ProductID, orderID, request.CashierID, domain.MovementSale, -item.Quantity, "checkout"); err != nil {
				return err
			}
		}
		if err := tx.AppendAuditLog(ctx, request.CashierID, "order.created", "order", fmt.Sprint(orderID), map[string]any{
			"order_number":   orderNumber,
			"subtotal_minor": order.SubtotalMinor,
			"currency":       order.Currency,
		}); err != nil {
			return err
		}
		order.ID = orderID
		result = order
		return nil
	})
	if err != nil {
		return domain.Order{}, err
	}
	return result, nil
}

func newOrderNumber(now time.Time) (string, error) {
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "POS-" + now.UTC().Format("20060102") + "-" + strings.ToUpper(hex.EncodeToString(entropy[:])), nil
}

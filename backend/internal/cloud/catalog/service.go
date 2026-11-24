package catalog

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"luckytv/backend/internal/cloud/store"
)

var ErrProductUnavailable = errors.New("sandbox product is unavailable")

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) EnsureSandboxProducts() error {
	products := []store.Product{
		{ID: "sandbox-points-100", Name: "沙箱点数 100", Currency: "CNY", AmountCNY: 100, Points: 100, Enabled: true},
		{ID: "sandbox-points-500", Name: "沙箱点数 500", Currency: "CNY", AmountCNY: 500, Points: 500, Enabled: true},
	}
	for _, product := range products {
		var current store.Product
		err := s.db.First(&current, "id = ?", product.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.db.Create(&product).Error; err != nil {
				return fmt.Errorf("create sandbox product: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read sandbox product: %w", err)
		}
	}
	return nil
}

func (s *Service) List() ([]store.Product, error) {
	products := make([]store.Product, 0)
	if err := s.db.Where("enabled = ?", true).Order("points ASC").Find(&products).Error; err != nil {
		return nil, fmt.Errorf("list sandbox products: %w", err)
	}
	return products, nil
}

func (s *Service) FindInTx(tx *gorm.DB, productID string) (store.Product, error) {
	var product store.Product
	err := tx.Where("id = ? AND enabled = ?", productID, true).First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Product{}, ErrProductUnavailable
	}
	if err != nil {
		return store.Product{}, fmt.Errorf("read sandbox product: %w", err)
	}
	return product, nil
}

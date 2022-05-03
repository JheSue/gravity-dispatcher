package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/BrobridgeOrg/gravity-sdk/config_store"
	"github.com/BrobridgeOrg/gravity-sdk/core"
	"github.com/BrobridgeOrg/gravity-sdk/product"
	"github.com/nats-io/nats.go"
)

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrProductExistsAlready = errors.New("product exists already")
	ErrInvalidProductName   = errors.New("invalid product name")
)

type ProductManager struct {
	client      *core.Client
	configStore *config_store.ConfigStore
}

func NewProductManager(client *core.Client, domain string) *ProductManager {

	pm := &ProductManager{
		client: client,
	}

	pm.configStore = config_store.NewConfigStore(client,
		config_store.WithDomain(domain),
		config_store.WithCatalog("PRODUCT"),
	)

	err := pm.configStore.Init()
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return pm
}

func (pm *ProductManager) CreateProduct(productSetting *product.ProductSetting) (*product.ProductSetting, error) {

	// Attempt to get product information
	_, err := pm.configStore.Get(productSetting.Name)
	if err != nats.ErrKeyNotFound {
		return nil, ErrProductExistsAlready
	}

	if err == nats.ErrInvalidKey {
		return nil, ErrInvalidProductName
	}

	productSetting.CreatedAt = time.Now()
	productSetting.UpdatedAt = time.Now()

	data, _ := json.Marshal(productSetting)

	// Write to KV store
	_, err = pm.configStore.Put(productSetting.Name, data)
	if err != nil {

		switch err {
		case nats.ErrInvalidKey:
			return nil, ErrInvalidProductName
		}

		return nil, err
	}

	return productSetting, nil
}

func (pm *ProductManager) DeleteProduct(name string) error {

	// Check whether specific product exist or not
	_, err := pm.GetProduct(name)
	if err != nil {
		return err
	}

	err = pm.configStore.Delete(name)
	if err != nil {
		return err
	}

	return nil
}

func (pm *ProductManager) UpdateProduct(name string, productSetting *product.ProductSetting) (*product.ProductSetting, error) {

	// Check whether specific product exist or not
	_, err := pm.GetProduct(name)
	if err != nil {
		return nil, err
	}

	productSetting.UpdatedAt = time.Now()

	data, _ := json.Marshal(productSetting)

	// Write to KV store
	_, err = pm.configStore.Put(name, data)
	if err != nil {

		switch err {
		case nats.ErrInvalidKey:
			return nil, ErrInvalidProductName
		}

		return nil, err
	}

	return productSetting, nil
}

func (pm *ProductManager) PurgeProduct(name string) error {

	// Attempt to get product information
	setting, err := pm.GetProduct(name)
	if err != nil {
		return err
	}

	js, err := pm.client.GetJetStream()
	if err != nil {
		return err
	}

	// Purge stream
	err = js.PurgeStream(setting.Stream)
	if err != nil {
		return err
	}

	return nil
}

func (pm *ProductManager) GetProduct(name string) (*product.ProductSetting, error) {

	// Attempt to get product information
	kv, err := pm.configStore.Get(name)
	if err != nil {
		switch err {
		case nats.ErrInvalidKey:
			fallthrough
		case nats.ErrKeyNotFound:
			return nil, ErrProductNotFound
		}

		return nil, err
	}

	// Parsing value
	var productSetting product.ProductSetting
	err = json.Unmarshal(kv.Value(), &productSetting)
	if err != nil {
		return nil, err
	}

	return &productSetting, nil
}

func (pm *ProductManager) ListProducts() ([]*product.ProductSetting, error) {

	// Getting all entries
	keys, _ := pm.configStore.Keys()

	entries := make([]nats.KeyValueEntry, len(keys))
	for i, key := range keys {

		entry, err := pm.configStore.Get(key)
		if err != nil {
			fmt.Printf("Can not get product \"%s\" information\n", key)
			continue
		}

		entries[i] = entry
	}

	products := make([]*product.ProductSetting, len(entries))
	for i, entry := range entries {

		var p product.ProductSetting
		err := json.Unmarshal(entry.Value(), &p)
		if err != nil {
			fmt.Printf("Product \"%s\" Invalid setting format\n", entry.Key())
		}

		products[i] = &p
	}

	return products, nil
}

package store

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func ValidateStore(storeURL string) (name string, hasShopifyPayments bool, latency time.Duration, err error) {
	start := time.Now()
	if !strings.HasSuffix(storeURL, ".myshopify.com") && !strings.Contains(storeURL, ".") {
		return "", false, 0, fmt.Errorf("invalid store URL")
	}
	if !strings.HasPrefix(storeURL, "http") {
		storeURL = "https://" + storeURL
	}
	u, err := url.Parse(storeURL)
	if err != nil {
		return "", false, 0, err
	}
	productsURL := u.Scheme + "://" + u.Host + "/products.json?limit=1"
	resp, err := http.Get(productsURL)
	if err != nil {
		return "", false, 0, err
	}
	defer resp.Body.Close()
	latency = time.Since(start)
	if resp.StatusCode != 200 {
		return "", false, latency, fmt.Errorf("store returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	name = u.Host
	hasShopifyPayments = strings.Contains(bodyStr, "shopify") || strings.Contains(bodyStr, "Shopify")
	return name, hasShopifyPayments, latency, nil
}

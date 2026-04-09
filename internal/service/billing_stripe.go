package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type StripeClient struct {
	SecretKey     string
	WebhookSecret string
	PriceIDPro    string
	HTTPClient    *http.Client
}

func (c *StripeClient) CreateCheckoutSession(ctx context.Context, in CheckoutInput) (CheckoutSession, error) {
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("success_url", in.SuccessURL)
	form.Set("cancel_url", in.CancelURL)
	clientRefID := in.UserID.String()
	if in.OrganizationID != nil {
		clientRefID = in.OrganizationID.String()
		form.Set("subscription_data[metadata][organization_id]", in.OrganizationID.String())
	}
	form.Set("client_reference_id", clientRefID)
	form.Set("customer_email", in.UserEmail)
	form.Set("line_items[0][price]", c.PriceIDPro)
	form.Set("line_items[0][quantity]", "1")
	form.Set("subscription_data[metadata][user_id]", in.UserID.String())
	form.Set("subscription_data[metadata][plan]", PlanPro)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return CheckoutSession{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return CheckoutSession{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return CheckoutSession{}, fmt.Errorf("stripe checkout failed status=%d body=%s", res.StatusCode, string(body))
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return CheckoutSession{}, err
	}
	return CheckoutSession{ID: out.ID, URL: out.URL}, nil
}

func (c *StripeClient) ParseSubscriptionWebhook(payload []byte, signature string) (WebhookSubscriptionEvent, error) {
	if !verifyStripeSignature(payload, signature, c.WebhookSecret) {
		return WebhookSubscriptionEvent{}, fmt.Errorf("signature mismatch")
	}
	var evt struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID               string `json:"id"`
				Status           string `json:"status"`
				Customer         string `json:"customer"`
				CurrentPeriodEnd int64  `json:"current_period_end"`
				Items            struct {
					Data []struct {
						Quantity int64 `json:"quantity"`
					} `json:"data"`
				} `json:"items"`
				Metadata map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&evt); err != nil {
		return WebhookSubscriptionEvent{}, err
	}
	if evt.Type != "customer.subscription.created" && evt.Type != "customer.subscription.updated" && evt.Type != "customer.subscription.deleted" {
		return WebhookSubscriptionEvent{Type: evt.Type}, nil
	}
	var periodEnd *time.Time
	if evt.Data.Object.CurrentPeriodEnd > 0 {
		t := time.Unix(evt.Data.Object.CurrentPeriodEnd, 0).UTC()
		periodEnd = &t
	}
	seatCount := int64(1)
	if len(evt.Data.Object.Items.Data) > 0 && evt.Data.Object.Items.Data[0].Quantity > 0 {
		seatCount = evt.Data.Object.Items.Data[0].Quantity
	}
	return WebhookSubscriptionEvent{
		Type:                 evt.Type,
		StripeSubscriptionID: evt.Data.Object.ID,
		StripeCustomerID:     evt.Data.Object.Customer,
		SeatCount:            seatCount,
		Status:               evt.Data.Object.Status,
		CurrentPeriodEnd:     periodEnd,
		Metadata:             evt.Data.Object.Metadata,
	}, nil
}

func (c *StripeClient) UpdateSubscriptionQuantity(ctx context.Context, subscriptionID string, quantity int64) error {
	if quantity < 1 {
		quantity = 1
	}
	itemID, err := c.getSubscriptionItemID(ctx, subscriptionID)
	if err != nil {
		return err
	}
	form := url.Values{}
	form.Set("items[0][id]", itemID)
	form.Set("items[0][quantity]", strconv.FormatInt(quantity, 10))
	form.Set("proration_behavior", "none")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.stripe.com/v1/subscriptions/"+url.PathEscape(subscriptionID), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return fmt.Errorf("stripe update subscription failed status=%d body=%s", res.StatusCode, string(body))
	}
	return nil
}

func (c *StripeClient) ListInvoices(ctx context.Context, customerID string, limit int64, startingAfter string) (StripeInvoiceList, error) {
	if limit <= 0 {
		limit = 20
	}
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("limit", strconv.FormatInt(limit, 10))
	if strings.TrimSpace(startingAfter) != "" {
		form.Set("starting_after", strings.TrimSpace(startingAfter))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stripe.com/v1/invoices?"+form.Encode(), nil)
	if err != nil {
		return StripeInvoiceList{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	resBody, statusCode, err := c.do(req)
	if err != nil {
		return StripeInvoiceList{}, err
	}
	if statusCode >= 400 {
		return StripeInvoiceList{}, fmt.Errorf("stripe list invoices failed status=%d body=%s", statusCode, string(resBody))
	}
	var out struct {
		Data    []stripeInvoice `json:"data"`
		HasMore bool            `json:"has_more"`
	}
	if err := json.Unmarshal(resBody, &out); err != nil {
		return StripeInvoiceList{}, err
	}
	list := StripeInvoiceList{Data: make([]StripeInvoice, 0, len(out.Data)), HasMore: out.HasMore}
	for _, inv := range out.Data {
		list.Data = append(list.Data, inv.toModel())
	}
	return list, nil
}

func (c *StripeClient) GetInvoice(ctx context.Context, invoiceID string) (StripeInvoice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stripe.com/v1/invoices/"+url.PathEscape(invoiceID), nil)
	if err != nil {
		return StripeInvoice{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	resBody, statusCode, err := c.do(req)
	if err != nil {
		return StripeInvoice{}, err
	}
	if statusCode >= 400 {
		return StripeInvoice{}, fmt.Errorf("stripe get invoice failed status=%d body=%s", statusCode, string(resBody))
	}
	var out stripeInvoice
	if err := json.Unmarshal(resBody, &out); err != nil {
		return StripeInvoice{}, err
	}
	return out.toModel(), nil
}

func (c *StripeClient) getSubscriptionItemID(ctx context.Context, subscriptionID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.stripe.com/v1/subscriptions/"+url.PathEscape(subscriptionID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("stripe get subscription failed status=%d body=%s", res.StatusCode, string(body))
	}
	var out struct {
		Items struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if len(out.Items.Data) == 0 || strings.TrimSpace(out.Items.Data[0].ID) == "" {
		return "", fmt.Errorf("subscription item not found")
	}
	return out.Items.Data[0].ID, nil
}

func verifyStripeSignature(payload []byte, signature, secret string) bool {
	parts := strings.Split(signature, ",")
	var ts, sig string
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == "t" {
			ts = kv[1]
		}
		if kv[0] == "v1" {
			sig = kv[1]
		}
	}
	if ts == "" || sig == "" || secret == "" {
		return false
	}
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		if delta := time.Since(time.Unix(n, 0)); delta > 5*time.Minute || delta < -5*time.Minute {
			return false
		}
	}
	signedPayload := ts + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

func (c *StripeClient) do(req *http.Request) ([]byte, int, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return body, res.StatusCode, nil
}

type stripeInvoice struct {
	ID                string `json:"id"`
	Customer          string `json:"customer"`
	AmountDue         int64  `json:"amount_due"`
	Currency          string `json:"currency"`
	Status            string `json:"status"`
	HostedInvoiceURL  string `json:"hosted_invoice_url"`
	InvoicePDF        string `json:"invoice_pdf"`
	Created           int64  `json:"created"`
	StatusTransitions struct {
		PaidAt int64 `json:"paid_at"`
	} `json:"status_transitions"`
	TotalTaxAmounts []struct {
		Amount int64 `json:"amount"`
	} `json:"total_tax_amounts"`
	PaymentIntent struct {
		PaymentMethod string `json:"payment_method"`
		Status        string `json:"status"`
	} `json:"payment_intent"`
	DefaultPaymentMethod struct {
		ID string `json:"id"`
	} `json:"default_payment_method"`
	Lines struct {
		Data []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Amount      int64  `json:"amount"`
			Currency    string `json:"currency"`
			Quantity    int64  `json:"quantity"`
			Period      struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"period"`
		} `json:"data"`
	} `json:"lines"`
}

func (s stripeInvoice) toModel() StripeInvoice {
	out := StripeInvoice{
		ID:               s.ID,
		CustomerID:       s.Customer,
		AmountDue:        s.AmountDue,
		Currency:         s.Currency,
		Status:           s.Status,
		HostedInvoiceURL: s.HostedInvoiceURL,
		InvoicePDF:       s.InvoicePDF,
		PaymentStatus:    s.PaymentIntent.Status,
		PaymentMethod:    s.PaymentIntent.PaymentMethod,
		LineItems:        make([]InvoiceLineItem, 0, len(s.Lines.Data)),
	}
	if out.PaymentMethod == "" {
		out.PaymentMethod = s.DefaultPaymentMethod.ID
	}
	if s.Created > 0 {
		out.CreatedAt = time.Unix(s.Created, 0).UTC()
	}
	if s.StatusTransitions.PaidAt > 0 {
		paidAt := time.Unix(s.StatusTransitions.PaidAt, 0).UTC()
		out.PaidAt = &paidAt
	}
	for _, tax := range s.TotalTaxAmounts {
		out.TaxAmount += tax.Amount
	}
	for _, line := range s.Lines.Data {
		item := InvoiceLineItem{
			ID:          line.ID,
			Description: line.Description,
			Amount:      line.Amount,
			Currency:    line.Currency,
			Quantity:    line.Quantity,
		}
		if line.Period.Start > 0 {
			start := time.Unix(line.Period.Start, 0).UTC()
			item.PeriodStart = &start
		}
		if line.Period.End > 0 {
			end := time.Unix(line.Period.End, 0).UTC()
			item.PeriodEnd = &end
		}
		out.LineItems = append(out.LineItems, item)
	}
	return out
}

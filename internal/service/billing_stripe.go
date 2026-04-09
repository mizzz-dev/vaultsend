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

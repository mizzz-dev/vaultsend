package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Subscription struct {
	ID                   uuid.UUID
	UserID               *uuid.UUID
	OrganizationID       *uuid.UUID
	StripeCustomerID     *string
	StripeSubscriptionID string
	Plan                 string
	Status               string
	CurrentPeriodEnd     *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type UpsertSubscriptionParams struct {
	UserID               *uuid.UUID
	OrganizationID       *uuid.UUID
	StripeCustomerID     *string
	StripeSubscriptionID string
	Plan                 string
	Status               string
	CurrentPeriodEnd     *time.Time
}

func (q *Queries) UpsertSubscription(ctx context.Context, arg UpsertSubscriptionParams) (Subscription, error) {
	const query = `
INSERT INTO subscriptions (user_id, organization_id, stripe_customer_id, stripe_subscription_id, plan, status, current_period_end)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (stripe_subscription_id) DO UPDATE
SET user_id = EXCLUDED.user_id,
    organization_id = EXCLUDED.organization_id,
    stripe_customer_id = EXCLUDED.stripe_customer_id,
    plan = EXCLUDED.plan,
    status = EXCLUDED.status,
    current_period_end = EXCLUDED.current_period_end,
    updated_at = now()
RETURNING id, user_id, organization_id, stripe_customer_id, stripe_subscription_id, plan, status, current_period_end, created_at, updated_at`
	var out Subscription
	err := q.db.QueryRow(ctx, query,
		arg.UserID,
		arg.OrganizationID,
		arg.StripeCustomerID,
		arg.StripeSubscriptionID,
		arg.Plan,
		arg.Status,
		arg.CurrentPeriodEnd,
	).Scan(
		&out.ID,
		&out.UserID,
		&out.OrganizationID,
		&out.StripeCustomerID,
		&out.StripeSubscriptionID,
		&out.Plan,
		&out.Status,
		&out.CurrentPeriodEnd,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

func (q *Queries) GetLatestSubscriptionByUserID(ctx context.Context, userID uuid.UUID) (Subscription, error) {
	const query = `
SELECT id, user_id, organization_id, stripe_customer_id, stripe_subscription_id, plan, status, current_period_end, created_at, updated_at
FROM subscriptions
WHERE user_id = $1
ORDER BY updated_at DESC
LIMIT 1`
	var out Subscription
	err := q.db.QueryRow(ctx, query, userID).Scan(
		&out.ID,
		&out.UserID,
		&out.OrganizationID,
		&out.StripeCustomerID,
		&out.StripeSubscriptionID,
		&out.Plan,
		&out.Status,
		&out.CurrentPeriodEnd,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	return out, err
}

func (q *Queries) GetLatestSubscriptionByOrgID(ctx context.Context, orgID uuid.UUID) (Subscription, error) {
	const query = `
SELECT id, user_id, organization_id, stripe_customer_id, stripe_subscription_id, plan, status, current_period_end, created_at, updated_at
FROM subscriptions
WHERE organization_id = $1
ORDER BY updated_at DESC
LIMIT 1`
	var out Subscription
	err := q.db.QueryRow(ctx, query, orgID).Scan(
		&out.ID,
		&out.UserID,
		&out.OrganizationID,
		&out.StripeCustomerID,
		&out.StripeSubscriptionID,
		&out.Plan,
		&out.Status,
		&out.CurrentPeriodEnd,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	return out, err
}

func (q *Queries) UpsertOrgSubscription(ctx context.Context, arg UpsertSubscriptionParams) (Subscription, error) {
	arg.UserID = nil
	return q.UpsertSubscription(ctx, arg)
}

func (q *Queries) CountShipmentsByUserSince(ctx context.Context, ownerUserID uuid.UUID, since time.Time) (int64, error) {
	const query = `SELECT COUNT(1) FROM shipments WHERE owner_user_id = $1 AND created_at >= $2`
	var total int64
	if err := q.db.QueryRow(ctx, query, ownerUserID, since).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (q *Queries) SumStorageBytesByUser(ctx context.Context, ownerUserID uuid.UUID) (int64, error) {
	const query = `
SELECT COALESCE(SUM(f.size_bytes), 0)
FROM files f
JOIN shipments s ON s.id = f.shipment_id
WHERE s.owner_user_id = $1
  AND s.status NOT IN ('deleted', 'revoked')`
	var total int64
	if err := q.db.QueryRow(ctx, query, ownerUserID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (q *Queries) CountShipmentsByOrgSince(ctx context.Context, organizationID uuid.UUID, since time.Time) (int64, error) {
	const query = `SELECT COUNT(1) FROM shipments WHERE organization_id = $1 AND created_at >= $2`
	var total int64
	if err := q.db.QueryRow(ctx, query, organizationID, since).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (q *Queries) SumStorageBytesByOrg(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	const query = `
SELECT COALESCE(SUM(f.size_bytes), 0)
FROM files f
JOIN shipments s ON s.id = f.shipment_id
WHERE s.organization_id = $1
  AND s.status NOT IN ('deleted', 'revoked')`
	var total int64
	if err := q.db.QueryRow(ctx, query, organizationID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

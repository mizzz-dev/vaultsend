package middleware

import (
	"context"

	"github.com/google/uuid"
)

type orgCtxKey string

const authOrgKey orgCtxKey = "auth_org"

type OrgContext struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
}

func WithOrgContext(ctx context.Context, org OrgContext) context.Context {
	return context.WithValue(ctx, authOrgKey, org)
}

func OrgFromContext(ctx context.Context) (*OrgContext, bool) {
	v := ctx.Value(authOrgKey)
	if v == nil {
		return nil, false
	}
	org, ok := v.(OrgContext)
	if !ok {
		return nil, false
	}
	return &org, true
}

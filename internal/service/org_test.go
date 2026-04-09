package service

import (
	"context"
	"testing"

	"github.com/example/vaultsend/internal/store"
	"github.com/google/uuid"
)

type fakeOrgStore struct {
	org     store.Organization
	members map[uuid.UUID]store.OrganizationMember
}

type fakeOrgBilling struct {
	seatLimit int64
	usage     int64
	syncCalls int
}

func (f *fakeOrgBilling) GetSeatLimit(ctx context.Context, orgID uuid.UUID) (int64, error) {
	return f.seatLimit, nil
}
func (f *fakeOrgBilling) GetCurrentSeatUsage(ctx context.Context, orgID uuid.UUID) (int64, error) {
	return f.usage, nil
}
func (f *fakeOrgBilling) SyncSeatCountWithStripe(ctx context.Context, orgID uuid.UUID) error {
	f.syncCalls++
	return nil
}

func (f *fakeOrgStore) CreateOrg(ctx context.Context, arg store.CreateOrgParams) (store.Organization, error) {
	id := uuid.New()
	f.org = store.Organization{ID: id, Name: arg.Name, OwnerUserID: arg.OwnerUserID}
	if f.members == nil {
		f.members = map[uuid.UUID]store.OrganizationMember{}
	}
	f.members[arg.OwnerUserID] = store.OrganizationMember{OrganizationID: id, UserID: arg.OwnerUserID, Role: "owner"}
	return f.org, nil
}
func (f *fakeOrgStore) GetOrgByID(ctx context.Context, orgID uuid.UUID) (store.Organization, error) {
	return f.org, nil
}
func (f *fakeOrgStore) GetUserOrgs(ctx context.Context, userID uuid.UUID) ([]store.Organization, error) {
	return []store.Organization{f.org}, nil
}
func (f *fakeOrgStore) AddMember(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, role string) (store.OrganizationMember, error) {
	m := store.OrganizationMember{OrganizationID: orgID, UserID: userID, Role: role}
	f.members[userID] = m
	return m, nil
}
func (f *fakeOrgStore) RemoveMember(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) error {
	delete(f.members, userID)
	return nil
}
func (f *fakeOrgStore) GetOrgMembers(ctx context.Context, orgID uuid.UUID) ([]store.OrganizationMember, error) {
	out := []store.OrganizationMember{}
	for _, m := range f.members {
		out = append(out, m)
	}
	return out, nil
}
func (f *fakeOrgStore) GetOrganizationMember(ctx context.Context, orgID uuid.UUID, userID uuid.UUID) (store.OrganizationMember, error) {
	m, ok := f.members[userID]
	if !ok {
		return store.OrganizationMember{}, store.ErrNotFound
	}
	return m, nil
}

func TestOrgCreateAddMemberAndAuthz(t *testing.T) {
	fs := &fakeOrgStore{members: map[uuid.UUID]store.OrganizationMember{}}
	bs := &fakeOrgBilling{seatLimit: 2, usage: 1}
	svc := &OrgService{Store: fs, Billing: bs}
	owner := uuid.New()
	org, err := svc.CreateOrg(context.Background(), owner, "Team A")
	if err != nil {
		t.Fatal(err)
	}
	member := uuid.New()
	if _, err := svc.AddMember(context.Background(), owner, org.ID, member, "member"); err != nil {
		t.Fatal(err)
	}
	if bs.syncCalls != 1 {
		t.Fatalf("expected stripe sync once, got %d", bs.syncCalls)
	}
	ship := store.Shipment{ID: uuid.New(), OrganizationID: &org.ID}
	if err := svc.AuthorizeShipmentAction(context.Background(), member, ship, "read"); err != nil {
		t.Fatal(err)
	}
	if err := svc.AuthorizeShipmentAction(context.Background(), member, ship, "delete"); err == nil {
		t.Fatal("member should not delete")
	}
}

func TestOrgAddMember_SeatLimitExceeded(t *testing.T) {
	fs := &fakeOrgStore{members: map[uuid.UUID]store.OrganizationMember{}}
	bs := &fakeOrgBilling{seatLimit: 1, usage: 1}
	svc := &OrgService{Store: fs, Billing: bs}
	owner := uuid.New()
	org, err := svc.CreateOrg(context.Background(), owner, "Team B")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddMember(context.Background(), owner, org.ID, uuid.New(), "member")
	if err == nil {
		t.Fatal("expected seat limit error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "SEAT_LIMIT" || !apiErr.UpgradeRequired {
		t.Fatalf("unexpected err: %#v", err)
	}
}

func TestOrgRemoveMember_SyncSeatCount(t *testing.T) {
	fs := &fakeOrgStore{members: map[uuid.UUID]store.OrganizationMember{}}
	bs := &fakeOrgBilling{seatLimit: 5, usage: 2}
	svc := &OrgService{Store: fs, Billing: bs}
	owner := uuid.New()
	org, err := svc.CreateOrg(context.Background(), owner, "Team C")
	if err != nil {
		t.Fatal(err)
	}
	member := uuid.New()
	fs.members[member] = store.OrganizationMember{OrganizationID: org.ID, UserID: member, Role: "member"}
	if err := svc.RemoveMember(context.Background(), owner, org.ID, member); err != nil {
		t.Fatal(err)
	}
	if bs.syncCalls != 1 {
		t.Fatalf("expected stripe sync once, got %d", bs.syncCalls)
	}
}

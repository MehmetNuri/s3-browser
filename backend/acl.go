package main

import (
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// AclGrant is one line of an access control list. Grantee is a canonical
// user ID, a group URI or an e-mail address depending on Type.
type AclGrant struct {
	Type       string `json:"type"` // CanonicalUser | Group | AmazonCustomerByEmail
	Grantee    string `json:"grantee"`
	Name       string `json:"name"` // display name, when the service reports one
	Permission string `json:"permission"`
}

// Acl is the access control list of a bucket or object.
type Acl struct {
	OwnerID   string     `json:"ownerId"`
	OwnerName string     `json:"ownerName"`
	Grants    []AclGrant `json:"grants"`
}

const (
	groupAllUsers           = "http://acs.amazonaws.com/groups/global/AllUsers"
	groupAuthenticatedUsers = "http://acs.amazonaws.com/groups/global/AuthenticatedUsers"
	groupLogDelivery        = "http://acs.amazonaws.com/groups/s3/LogDelivery"
	maxAclGrants            = 100
)

var aclPermissions = map[string]bool{"FULL_CONTROL": true, "READ": true, "WRITE": true, "READ_ACP": true, "WRITE_ACP": true}

func aclFromGrants(owner *types.Owner, grants []types.Grant) Acl {
	acl := Acl{Grants: []AclGrant{}}
	if owner != nil {
		acl.OwnerID, acl.OwnerName = aws.ToString(owner.ID), aws.ToString(owner.DisplayName)
	}
	for _, g := range grants {
		if g.Grantee == nil {
			continue
		}
		grant := AclGrant{Type: string(g.Grantee.Type), Name: aws.ToString(g.Grantee.DisplayName), Permission: string(g.Permission)}
		switch g.Grantee.Type {
		case types.TypeGroup:
			grant.Grantee = aws.ToString(g.Grantee.URI)
		case types.TypeAmazonCustomerByEmail:
			grant.Grantee = aws.ToString(g.Grantee.EmailAddress)
		default:
			grant.Grantee = aws.ToString(g.Grantee.ID)
		}
		acl.Grants = append(acl.Grants, grant)
	}
	return acl
}

// grantsToPolicy builds the policy a PUT needs; the owner is kept from the GET.
func grantsToPolicy(acl Acl) (*types.AccessControlPolicy, error) {
	if acl.OwnerID == "" {
		return nil, errors.New(T("aclOwnerUnknown"))
	}
	if len(acl.Grants) > maxAclGrants {
		return nil, errors.New(T("tooManyGrants", maxAclGrants))
	}
	policy := &types.AccessControlPolicy{Owner: &types.Owner{ID: aws.String(acl.OwnerID)}}
	for _, g := range acl.Grants {
		grantee := strings.TrimSpace(g.Grantee)
		if grantee == "" || !validHeaderValue(grantee) || !aclPermissions[g.Permission] {
			return nil, errors.New(T("invalidGrant"))
		}
		target := &types.Grantee{Type: types.Type(g.Type)}
		switch g.Type {
		case string(types.TypeGroup):
			target.URI = aws.String(grantee)
		case string(types.TypeAmazonCustomerByEmail):
			target.EmailAddress = aws.String(grantee)
		case string(types.TypeCanonicalUser):
			target.ID = aws.String(grantee)
		default:
			return nil, errors.New(T("invalidGrant"))
		}
		policy.Grants = append(policy.Grants, types.Grant{Grantee: target, Permission: types.Permission(g.Permission)})
	}
	return policy, nil
}

func (a *App) GetObjectAcl(bucket, key string) (Acl, error) {
	c, err := a.cli()
	if err != nil {
		return Acl{}, err
	}
	out, err := c.GetObjectAcl(a.ctx, &s3.GetObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return Acl{}, describeErr(err)
	}
	return aclFromGrants(out.Owner, out.Grants), nil
}

// SetObjectAcl replaces the grants of an object. The owner always keeps full
// control, so a mistaken list cannot lock the account out.
func (a *App) SetObjectAcl(bucket, key string, acl Acl) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	policy, err := grantsToPolicy(withOwnerControl(acl))
	if err != nil {
		return err
	}
	_, err = c.PutObjectAcl(a.ctx, &s3.PutObjectAclInput{Bucket: aws.String(bucket), Key: aws.String(key), AccessControlPolicy: policy})
	return describeErr(err)
}

func (a *App) GetBucketAcl(bucket string) (Acl, error) {
	c, err := a.cli()
	if err != nil {
		return Acl{}, err
	}
	out, err := c.GetBucketAcl(a.ctx, &s3.GetBucketAclInput{Bucket: aws.String(bucket)})
	if err != nil {
		return Acl{}, describeErr(err)
	}
	return aclFromGrants(out.Owner, out.Grants), nil
}

func (a *App) SetBucketAcl(bucket string, acl Acl) error {
	c, err := a.cli()
	if err != nil {
		return err
	}
	policy, err := grantsToPolicy(withOwnerControl(acl))
	if err != nil {
		return err
	}
	_, err = c.PutBucketAcl(a.ctx, &s3.PutBucketAclInput{Bucket: aws.String(bucket), AccessControlPolicy: policy})
	return describeErr(err)
}

// withOwnerControl makes sure the owner keeps FULL_CONTROL.
func withOwnerControl(acl Acl) Acl {
	for _, g := range acl.Grants {
		if g.Type == string(types.TypeCanonicalUser) && g.Grantee == acl.OwnerID && g.Permission == "FULL_CONTROL" {
			return acl
		}
	}
	grants := append([]AclGrant{{Type: string(types.TypeCanonicalUser), Grantee: acl.OwnerID, Permission: "FULL_CONTROL"}}, acl.Grants...)
	acl.Grants = grants
	return acl
}
